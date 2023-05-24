/*
Copyright 2022 VMware, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/vmware-labs/reconciler-runtime/reconcilers"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/source"

	sourcev1alpha1 "github.com/vmware-tanzu/tanzu-source-controller/apis/source/v1alpha1"
	"github.com/vmware-tanzu/tanzu-source-controller/pkg/mavenmetadata"
)

const (
	MavenArtifactVersionStashKey    reconcilers.StashKey = sourcev1alpha1.Group + "/artifact-version"
	MavenArtifactAuthSecretStashKey reconcilers.StashKey = sourcev1alpha1.Group + "/auth-secret"
	MavenArtifactHttpClientKey      reconcilers.StashKey = sourcev1alpha1.Group + "/http-client"
)

type MavenArtifactAuthOptionsFromSecret struct {
	Username string
	Password string
}

// ArtifactDetails contains artifact's information within the remote repository
type ArtifactDetails struct {
	// ResolvedFileName: artifact filename in a remote repository
	ResolvedFileName string

	// Artifact resolved version, it could be different from spec.artifact.version. Ex: spec version LATEST can artifactversion 1.2.3
	ArtifactVersion string

	// Artifact Download URL (remote)
	ArtifactDownloadURL string
}

// artifactCache contains artifact source and checksum iformation saved on disc
type artifactCache struct {
	// source where the artifact came from
	source string
	// checksum of the artifact
	checksum string
}

func (ac *artifactCache) toString() string {
	return fmt.Sprintf("%s|%s", ac.source, ac.checksum)
}

//+kubebuilder:rbac:groups=source.apps.tanzu.vmware.com,resources=mavenartifacts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=source.apps.tanzu.vmware.com,resources=mavenartifacts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=source.apps.tanzu.vmware.com,resources=mavenartifacts/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func MavenArtifactReconciler(c reconcilers.Config, httpRootDir, httpHost string, now func() metav1.Time, certs []Cert) *reconcilers.ParentReconciler {
	return &reconcilers.ParentReconciler{
		Type: &sourcev1alpha1.MavenArtifact{},
		Reconciler: &reconcilers.WithFinalizer{
			Finalizer: sourcev1alpha1.Group + "/finalizer",
			Reconciler: reconcilers.Sequence{
				MavenArtifactSecretsSyncReconciler(certs),
				MavenArtifactVersionSyncReconciler(),
				MavenArtifactDownloadSyncReconciler(httpRootDir, httpHost, now),
				MavenArtifactIntervalReconciler(),
			},
		},

		Config: c,
	}
}

//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

func MavenArtifactSecretsSyncReconciler(certs []Cert) reconcilers.SubReconciler {
	return &reconcilers.SyncReconciler{
		Name: "MavenArtifactSecretsSyncReconciler",
		Sync: func(ctx context.Context, parent *sourcev1alpha1.MavenArtifact) error {
			c := reconcilers.RetrieveConfigOrDie(ctx)
			authSecretRefName := parent.Spec.Repository.SecretRef.Name
			authSecret := corev1.Secret{}
			if authSecretRefName != "" {

				err := c.TrackAndGet(ctx, types.NamespacedName{Namespace: parent.Namespace, Name: authSecretRefName}, &authSecret)
				if err != nil {
					if apierrs.IsNotFound(err) {
						parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactResolved, "SecretMissing", "Secret %q not found in namespace %q", authSecretRefName, parent.Namespace)
						return nil
					}
					return err
				}
				stashAuthSecret(ctx, authSecret)
			}

			if authSecret.Data != nil {
				certBytes := authSecret.Data["caFile"]
				certs = append(certs, Cert{Raw: certBytes})
			}

			// http client to be reused during reconcile
			t, err := newTransport(ctx, certs)
			if err != nil {
				return err
			}
			client := &http.Client{Transport: t}
			stashHttpClient(ctx, client)
			return nil
		},
		Setup: func(ctx context.Context, mgr controllerruntime.Manager, bldr *builder.Builder) error {
			// register an informer to watch Secret's metadata only. This reduces the cache size in memory.
			bldr.Watches(&source.Kind{Type: &corev1.Secret{}}, reconcilers.EnqueueTracked(ctx, &corev1.Secret{}), builder.OnlyMetadata)
			return nil
		},
	}
}

// MavenArtifactVersionSyncReconciler will download the Maven Metadata XML
// file from the artifact store and resolve the appropriate version, as per the
// configuration in the parent MavenArtifact object
func MavenArtifactVersionSyncReconciler() reconcilers.SubReconciler {
	return &reconcilers.SyncReconciler{
		Name: "MavenArtifactVersionSyncReconciler",
		Sync: func(ctx context.Context, parent *sourcev1alpha1.MavenArtifact) error {
			log := logr.FromContextOrDiscard(ctx)
			client := retrieveHttpClient(ctx)
			if client == nil {
				return nil
			}

			timeout := parent.Spec.Timeout.Duration
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			groupId := parent.Spec.Artifact.GroupId
			artifactId := parent.Spec.Artifact.ArtifactId
			repoSpecURL := parent.Spec.Repository.URL
			requestPath := strings.ReplaceAll(groupId, ".", "/") + "/" + artifactId

			// valid repository URL
			repoURL, err := url.Parse(repoSpecURL)
			if err != nil {
				parent.ManageConditions().
					MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactResolved, "ConfigurationError", "Error parsing repository URL %q: %v", parent.Spec.Repository.URL, err)
				return nil
			}

			// validate url scheme
			if repoURL.Scheme != "https" {
				parent.ManageConditions().
					MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactResolved, "ConfigurationError", `The only supported scheme is "https"; scheme %q is not supported in repository URL %q`, repoURL.Scheme, repoSpecURL)
				return nil
			}

			// MavenResolver
			mr := MavenResolver{
				Artifact:      parent.Spec.Artifact,
				RepositoryURL: repoURL.String(),
				RequestPath:   requestPath,
			}
			if err := mr.Resolve(ctx, client); err != nil {
				// handle timeout error
				if errors.Is(err, context.DeadlineExceeded) {
					parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactResolved, "Timeout",
						`Request timeout error downloading Maven artifact metadata "%v:%v" from repository URL %q: %v`, groupId, artifactId, repoSpecURL, err)
					return nil
				}
				// handle http error
				dlerr, isDownloadError := err.(*downloadError)
				if isDownloadError {
					log.Error(err, "error downloading artifact metadata", "statuscode", dlerr.httpStatuscode)
					// retry for statuscode 429
					if dlerr.httpStatuscode == http.StatusTooManyRequests {
						return dlerr.err
					}
					// don't update condition for statuscodes in 500 range
					if dlerr.httpStatuscode >= 500 {
						return dlerr.err
					}

					if dlerr.httpStatuscode == 401 {
						parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactResolved, "RemoteError",
							`Unauthorized credentials (HTTP 401) error downloading artifact metadata "%v:%v" from repository URL %q. Check the credentials provided in the Secret.`, groupId, artifactId, repoSpecURL)
					} else if dlerr.httpStatuscode == 404 {
						parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactResolved, "RemoteError",
							`Maven metadata file not found (HTTP 404) for artifact "%v:%v" from repository URL %q.`, groupId, artifactId, repoSpecURL)
					} else {
						// for all other download errors, including 404 will update the status condition
						parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactResolved, "RemoteError",
							`Error downloading Maven artifact metadata "%v:%v" from repository URL %q: %v`, groupId, artifactId, repoSpecURL, err)
					}
					return nil
				}

				log.Error(err, "error with maven-metadata", "maven-metadata.xml", mr.MetaXML)
				parent.ManageConditions().
					MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactResolved, "VersionError", err.Error())
				return nil

			} else {
				artifactDetails := ArtifactDetails{
					ArtifactVersion:     mr.Artifact.Version,
					ResolvedFileName:    mr.ResolvedFilename,
					ArtifactDownloadURL: mr.DownloadURL,
				}
				log.Info("artifact version resolved", "artifact", mr.Artifact.ArtifactId, "resolved version", mr.ResolvedVersion)
				parent.ManageConditions().
					MarkTrue(sourcev1alpha1.MavenArtifactConditionArtifactResolved, "Resolved", fmt.Sprintf(`Resolved version %q for artifact %q`, artifactDetails.ArtifactVersion, mr.DownloadURL))
				stashArtifactVersion(ctx, artifactDetails)
				return nil
			}
		},
	}
}

func MavenArtifactDownloadSyncReconciler(httpRootDir, httpHost string, now func() metav1.Time) reconcilers.SubReconciler {
	return &reconcilers.SyncReconciler{
		Name: "MavenArtifactDownloadSyncReconciler",
		Finalize: func(ctx context.Context, parent *sourcev1alpha1.MavenArtifact) error {
			log := logr.FromContextOrDiscard(ctx)
			dir := path.Join(httpRootDir, "mavenartifact", parent.Namespace, parent.Name)
			log.Info("removing artifacts", "dir", dir)
			return os.RemoveAll(dir)
		},
		Sync: func(ctx context.Context, parent *sourcev1alpha1.MavenArtifact) error {
			log := logr.FromContextOrDiscard(ctx)

			artifactInfo := retrieveArtifactVersion(ctx)
			if artifactInfo == nil {
				log.Info("error retrieving artifact version from stash")
				return nil
			}

			client := retrieveHttpClient(ctx)
			if client == nil {
				log.Info("error retrieving http client from stash")
				return nil
			}

			timeout := parent.Spec.Timeout.Duration
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// GET artifact checksum (SHA1)
			remoteChecksum, err := downloadChecksum(ctx, client, artifactInfo.ArtifactDownloadURL)
			if err != nil {
				// handle timeout error
				if errors.Is(err, context.DeadlineExceeded) {
					parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "Timeout",
						"Request timeout error downloading Maven artifact checksum file %q: %s", fmt.Sprintf("%s.%s", artifactInfo.ArtifactDownloadURL, "sha1"), err.Error())
					return nil
				}

				// handle http error
				dlerr, isDownloadError := err.(*downloadError)
				if isDownloadError {
					log.Error(err, "error downloading artifact checksum", "statuscode", dlerr.httpStatuscode)
					// retry for statuscode 429
					if dlerr.httpStatuscode == http.StatusTooManyRequests {
						return dlerr.err
					}
					// don't update condition for statuscodes in 500 range
					if dlerr.httpStatuscode >= 500 {
						return dlerr.err
					}
					if dlerr.httpStatuscode == 401 {
						parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "RemoteError",
							`Unauthorized credentials (HTTP 401) error downloading Maven artifact checksum from URL "%v.sha1". Check the credentials provided in the Secret.`, artifactInfo.ArtifactDownloadURL)
					} else if dlerr.httpStatuscode == 404 {
						parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "RemoteError",
							`Maven artifact checksum file not found (HTTP 404) at URL "%v.sha1".`, artifactInfo.ArtifactDownloadURL)
					} else {
						// for all other download errors, including 404 will update the status condition
						parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "RemoteError",
							`Error downloading Maven artifact checksum from URL "%v.sha1": %v`, artifactInfo.ArtifactDownloadURL, err)
					}
					return nil
				}

				parent.ManageConditions().
					MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "RemoteError", err.Error())
				return nil
			}

			// Retrieve cache data if exist
			cacheFile := fmt.Sprintf("%s/%s.%s", path.Join(httpRootDir, "mavenartifact", parent.Namespace, parent.Name), artifactInfo.ResolvedFileName, "sha1")
			cache, err := retrieveChecksumFromFile(cacheFile)
			if err != nil {
				log.Error(err, "Error reading Maven artifact checksum file", "filename", path.Join(httpRootDir, "mavenartifact", parent.Namespace, parent.Name))
				return err
			}

			// Compare checksum with cache if the resource status.artifact is set
			if cache != nil && parent.Status.Artifact != nil {
				if cache.checksum == remoteChecksum && cache.source == artifactInfo.ArtifactDownloadURL {
					log.Info("download skipped", "checksum matched on disc", cache.checksum, "checksum from remote repository", remoteChecksum)
					return nil
				} else {
					log.Info("download continue", "checksum matched on disc", cache.checksum, "checksum from remote repository", remoteChecksum)
				}
			}

			// Set temp dir
			if !path.IsAbs(httpRootDir) {
				httpRootDir = path.Join(httpRootDir)
			}

			dir, err := ioutil.TempDir(os.TempDir(), "maven-artifact.*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)

			// Download the artifact
			artifactDir, err := downloadArtifact(ctx, artifactInfo.ArtifactDownloadURL, dir, artifactInfo.ResolvedFileName, remoteChecksum, client)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "Timeout",
						"Request timeout error downloading Maven artifact file %q: %s", parent.Spec.Artifact.ArtifactId, err.Error())
					return nil
				}
				// Handle http error
				dlerr, isDownloadError := err.(*downloadError)
				if isDownloadError {
					log.Error(err, "error downloading Maven artifact file", "statuscode", dlerr.httpStatuscode)
					// Retry for statuscode 429
					if dlerr.httpStatuscode == http.StatusTooManyRequests {
						return dlerr.err
					}
					// No need to update condition for statuscodes in 500 range
					if dlerr.httpStatuscode >= 500 {
						return dlerr.err
					}
					if dlerr.httpStatuscode == 401 {
						parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "DownloadError",
							`Unauthorized credentials (HTTP 401) error downloading Maven artifact file from URL %q. Check the credentials provided in the Secret.`, artifactInfo.ArtifactDownloadURL)
					} else if dlerr.httpStatuscode == 404 {
						parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "DownloadError",
							`Maven artifact file not found (HTTP 404) at URL %q.`, artifactInfo.ArtifactDownloadURL)
					} else {
						// for all other download errors, including 404 will update the status condition
						parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "DownloadError",
							`Error downloading Maven artifact file from URL %q: %v`, artifactInfo.ArtifactDownloadURL, dlerr.err.Error())
					}
					return nil
				}

				parent.ManageConditions().
					MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "DownloadError",
						"Error downloading Maven artifact file %q: %s", parent.Spec.Artifact.ArtifactId, dlerr.err.Error())
				return nil
			}

			// Establish unique file name by creating a sha1 of downloaded file
			artifactTgzFilename, err := sha1Checksum(path.Join(artifactDir, artifactInfo.ResolvedFileName))
			if err != nil {
				return err
			}
			artifactTgzFilename = fmt.Sprintf("%s.tar.gz", artifactTgzFilename)

			// Unpack if artifact is an archive
			artifactFilePath := path.Join(artifactDir, artifactInfo.ResolvedFileName)
			if isArchive(artifactFilePath) {
				artifactDir, err = extractArchive(dir, artifactFilePath)
				if err != nil {
					parent.ManageConditions().MarkFalse(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "FileError",
						"Failed to extract Maven artifact file %q", artifactInfo.ResolvedFileName)
					log.Error(err, "failed to extract", "file", artifactInfo.ResolvedFileName)
					return nil
				}
			}

			artifactTgzDir := path.Join(dir, "artifactTgz")
			err = os.Mkdir(artifactTgzDir, os.ModePerm)
			if err != nil {
				return err
			}

			artifactTgz := path.Join(artifactTgzDir, artifactTgzFilename)

			// package directory as tgz
			if err := createTarGz(artifactDir, artifactTgz); err != nil {
				log.Error(err, "error creating tar", "dir", artifactDir, "file", artifactTgz)
				return fmt.Errorf("Error creating tar file for Maven artifact file %q: %w", artifactTgzFilename, err)
			}

			// create sha1 checksum for artifact.tgz
			checksum, err := sha1Checksum(artifactTgz)
			if err != nil {
				return err
			}

			httpPath := path.Join("mavenartifact", parent.Namespace, parent.Name, artifactTgzFilename)
			httpUrl := fmt.Sprintf("http://%s", path.Join(httpHost, httpPath))

			// copy artifact.tgz into httpRoot with a placeholder name
			if err := copyCompressedFile(artifactTgz, path.Join(httpRootDir, httpPath)); err != nil {
				return err
			}

			// add artifact cached data
			cacheData := artifactCache{
				source:   artifactInfo.ArtifactDownloadURL,
				checksum: remoteChecksum,
			}
			if err := os.WriteFile(cacheFile, []byte(cacheData.toString()), os.ModePerm); err != nil {
				return err
			}

			parent.Status.Artifact = preserveArtifactLastUpdateTime(parent.Status.Artifact, &sourcev1alpha1.Artifact{
				Checksum:       checksum,
				Revision:       artifactInfo.ResolvedFileName,
				Path:           httpPath,
				URL:            httpUrl,
				LastUpdateTime: now().Rfc3339Copy(),
			})
			parent.Status.URL = httpUrl

			parent.ManageConditions().MarkTrue(sourcev1alpha1.MavenArtifactConditionArtifactAvailable, "Available", "")
			return nil
		},
	}
}

func MavenArtifactIntervalReconciler() reconcilers.SubReconciler {
	return &reconcilers.SyncReconciler{
		Name: "MavenArtifactIntervalReconciler",
		Sync: func(ctx context.Context, parent *sourcev1alpha1.MavenArtifact) (controllerruntime.Result, error) {
			return controllerruntime.Result{RequeueAfter: parent.Spec.Interval.Duration}, nil
		},
	}
}

func basicAuthCredentialsFromSecret(ctx context.Context) *MavenArtifactAuthOptionsFromSecret {
	var authCredentials MavenArtifactAuthOptionsFromSecret

	authSecret := retrieveAuthSecret(ctx)
	if authSecret != nil {
		authCredentials.Username = string(authSecret.Data["username"])
		authCredentials.Password = string(authSecret.Data["password"])
	}
	return &authCredentials
}

func buildRequestObject(ctx context.Context, requestType, url string, authOpts *MavenArtifactAuthOptionsFromSecret) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, requestType, url, nil)
	if err != nil {
		return nil, err
	}

	if authOpts != nil {
		// TODO support "SSL" auth
		request.SetBasicAuth(authOpts.Username, authOpts.Password)
	}
	return request, nil
}

func downloadMetadata(ctx context.Context, client *http.Client, url string) (*mavenmetadata.MavenMetadata, error) {
	// download metadata
	meta, err := download(ctx, url, client)
	if err != nil {
		return nil, err
	}

	// parse metadata
	parsedData, err := mavenmetadata.Parse(meta)
	if err != nil {
		return nil, fmt.Errorf("Error %q while parsing XML data at %q", err, url)
	}

	return parsedData, nil
}

func downloadChecksum(ctx context.Context, client *http.Client, url string) (string, error) {
	// download checksum
	checksum, err := download(ctx, fmt.Sprintf("%s.%s", url, "sha1"), client)
	if err != nil {
		return "", err
	}
	return string(checksum), nil
}

func downloadArtifact(ctx context.Context, url string, dir string, fileName string, checksum string, client *http.Client) (string, error) {
	artifactDir := path.Join(dir, "artifact")
	err := os.Mkdir(artifactDir, os.ModePerm)
	if err != nil {
		return "", err
	}
	out, err := os.Create(path.Join(artifactDir, fileName))
	if err != nil {
		return "", fmt.Errorf("Error creating local Maven artifact file %s %s", fileName, err)
	}
	defer out.Close()

	// build httpRequest object
	request, err := buildRequestObject(ctx, "GET", url, basicAuthCredentialsFromSecret(ctx))
	if err != nil {
		return "", fmt.Errorf("Error %q while request parsing URL %q", err, url)
	}

	// process request
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("%s download error %s", url, err)
	}
	defer response.Body.Close()

	// if no error from the client, inspect statuscode and return download error from non 200s
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", &downloadError{err: fmt.Errorf("Error received HTTP status %v getting %q", response.StatusCode, url), httpStatuscode: response.StatusCode}
	}

	// copy response body to file
	_, err = io.Copy(out, response.Body)
	if err != nil {
		return "", fmt.Errorf("Error downloading Maven artifact file data %q: %q", out.Name(), err)
	}

	os.Chtimes(out.Name(), time.UnixMilli(0), time.UnixMilli(0))

	// verify checksum
	fileChecksum, err := sha1Checksum(out.Name())
	if err != nil {
		return "", fmt.Errorf("Error creating checksum value for Maven artifact file %q: %q", out.Name(), err)
	}

	if fileChecksum != checksum {
		return "", fmt.Errorf("Checksum (%v) of downloaded Maven artifact file %q does not match expected remote checksum (%v). This file may have been tampered with in transit!", fileChecksum, checksum, out.Name())
	}
	return artifactDir, nil
}

func download(ctx context.Context, url string, client *http.Client) ([]byte, error) {
	// build httpRequest object
	request, err := buildRequestObject(ctx, "GET", url, basicAuthCredentialsFromSecret(ctx))
	if err != nil {
		return nil, fmt.Errorf("Error %q while parsing request URL %q", err, url)
	}

	// process request
	response, err := client.Do(request)
	// we can bail here, and return the error with some deatils when available
	if err != nil {
		return nil, fmt.Errorf("%s download error %s", url, err)
	}
	defer response.Body.Close()

	// if no error from the client, inspect statuscode and return download error from non 200s
	if response != nil && (response.StatusCode < 200 || response.StatusCode >= 300) {
		return nil, &downloadError{err: fmt.Errorf("Error received HTTP status %v getting %q", response.StatusCode, url), httpStatuscode: response.StatusCode}
	}

	// read response body
	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, &downloadError{err: fmt.Errorf("Error downloading file data from URL %q: %q", url, err), httpStatuscode: response.StatusCode}
	}

	return responseBody, nil
}

func stashHttpClient(ctx context.Context, httpclient *http.Client) {
	reconcilers.StashValue(ctx, MavenArtifactHttpClientKey, httpclient)
}

func retrieveHttpClient(ctx context.Context) *http.Client {
	client, ok := reconcilers.RetrieveValue(ctx, MavenArtifactHttpClientKey).(*http.Client)

	if !ok {
		return nil
	}
	return client
}

func stashAuthSecret(ctx context.Context, authSecret corev1.Secret) {
	reconcilers.StashValue(ctx, MavenArtifactAuthSecretStashKey, authSecret)
}

func retrieveAuthSecret(ctx context.Context) *corev1.Secret {
	authSecret, ok := reconcilers.RetrieveValue(ctx, MavenArtifactAuthSecretStashKey).(corev1.Secret)
	if !ok {
		return nil
	}
	return &authSecret
}

func stashArtifactVersion(ctx context.Context, artifactDetails ArtifactDetails) {
	reconcilers.StashValue(ctx, MavenArtifactVersionStashKey, artifactDetails)
}

func retrieveArtifactVersion(ctx context.Context) *ArtifactDetails {
	artifactVersion, ok := reconcilers.RetrieveValue(ctx, MavenArtifactVersionStashKey).(ArtifactDetails)
	if !ok {
		return nil
	}
	return &artifactVersion
}

func isArchive(artifactFilePath string) bool {
	artifact, err := os.Open(artifactFilePath)
	if err != nil {
		return false
	}
	defer artifact.Close()

	buffer := make([]byte, 512)
	_, err = artifact.Read(buffer)
	if err != nil {
		return false
	}
	filetype := http.DetectContentType(buffer)

	return filetype == "application/zip"
}

func copyCompressedFile(from, to string) error {
	intermediateDestination := fmt.Sprintf("%s.new", to)
	if err := os.MkdirAll(path.Dir(intermediateDestination), 0755); err != nil {
		return err
	}
	fromFile, err := os.Open(from)
	if err != nil {
		return err
	}
	defer fromFile.Close()
	toFile, err := os.Create(intermediateDestination)
	if err != nil {
		return nil
	}
	defer toFile.Close()
	if _, err := io.Copy(toFile, fromFile); err != nil {
		return err
	}

	if err := os.Rename(intermediateDestination, to); err != nil {
		return err
	}
	return nil
}

func extractArchive(parentDir string, pathToJarFile string) (string, error) {
	openedFile, err := zip.OpenReader(pathToJarFile)
	if err != nil {
		return "", err
	}

	fileDestinationFolder := path.Join(parentDir, "extracted-artifact")
	if err := os.Mkdir(fileDestinationFolder, os.ModePerm); err != nil {
		return "", err
	}

	defer openedFile.Close()

	for _, file := range openedFile.File {
		filePath := filepath.Join(fileDestinationFolder, file.Name)
		if err = extractFile(file, filePath); err != nil {
			return "", err
		}
	}
	return fileDestinationFolder, err
}

func extractFile(file *zip.File, filePath string) error {
	if file.FileInfo().IsDir() {
		if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
			return err
		}
	} else {
		fileInArchive, err := file.Open()
		if err != nil {
			return err
		}
		defer fileInArchive.Close()

		destinationFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}
		defer destinationFile.Close()
		if _, err := io.Copy(destinationFile, fileInArchive); err != nil {
			return err
		}
	}
	os.Chtimes(filePath, time.UnixMilli(0), time.UnixMilli(0))
	return nil
}

func retrieveChecksumFromFile(filename string) (*artifactCache, error) {
	cache, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		} else {
			return nil, err
		}
	}

	return &artifactCache{
		source:   strings.Split(string(cache), "|")[0],
		checksum: strings.Split(string(cache), "|")[1],
	}, nil
}
