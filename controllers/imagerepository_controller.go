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
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cppforlife/go-cli-ui/ui"
	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/vmware-labs/reconciler-runtime/apis"
	"github.com/vmware-labs/reconciler-runtime/reconcilers"
	"github.com/vmware-tanzu/carvel-imgpkg/pkg/imgpkg/plainimage"
	"github.com/vmware-tanzu/carvel-imgpkg/pkg/imgpkg/registry"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/source"

	sourcev1alpha1 "github.com/vmware-tanzu/tanzu-source-controller/apis/source/v1alpha1"
)

//+kubebuilder:rbac:groups=source.apps.tanzu.vmware.com,resources=imagerepositories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=source.apps.tanzu.vmware.com,resources=imagerepositories/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=source.apps.tanzu.vmware.com,resources=imagerepositories/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

// ImageRepositoryReconciler reconciles a ImageRepository object
func ImageRepositoryReconciler(c reconcilers.Config, httpRootDir, httpHost string, now func() metav1.Time, certs []Cert) *reconcilers.ParentReconciler {
	return &reconcilers.ParentReconciler{
		Type: &sourcev1alpha1.ImageRepository{},
		Reconciler: &reconcilers.WithFinalizer{
			Finalizer: sourcev1alpha1.Group + "/finalizer",
			Reconciler: reconcilers.Sequence{
				ImageRepositoryTransportSyncReconciler(certs),
				ImageRepositoryImagePullSecretsSyncReconciler(),
				ImageRepositoryImageDigestSyncReconciler(),
				ImageRepositoryPullImageSyncReconciler(httpRootDir, httpHost, now),
				ImageRepositoryIntervalReconciler(),
			},
		},

		Config: c,
	}
}

func ImageRepositoryTransportSyncReconciler(certs []Cert) reconcilers.SubReconciler {
	return &reconcilers.SyncReconciler{
		Name: "ImageRepositoryTransportSyncReconciler",
		Sync: func(ctx context.Context, _ client.Object) error {
			transport, err := newTransport(ctx, certs)
			if err != nil {
				return err
			}
			StashHttpRoundTripper(ctx, transport)
			return nil
		},
	}
}

//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch

func ImageRepositoryImagePullSecretsSyncReconciler() reconcilers.SubReconciler {
	return &reconcilers.SyncReconciler{
		Name: "ImageRepositoryImagePullSecretsSyncReconciler",
		Sync: func(ctx context.Context, parent *sourcev1alpha1.ImageRepository) error {
			c := reconcilers.RetrieveConfigOrDie(ctx)

			pullSecretNames := sets.NewString()
			for _, ps := range parent.Spec.ImagePullSecrets {
				pullSecretNames.Insert(ps.Name)
			}

			// lookup service account
			serviceAccountName := parent.Spec.ServiceAccountName
			if serviceAccountName == "" {
				serviceAccountName = "default"
			}
			serviceAccount := corev1.ServiceAccount{}
			err := c.TrackAndGet(ctx, types.NamespacedName{Namespace: parent.Namespace, Name: serviceAccountName}, &serviceAccount)
			if err != nil {
				if apierrs.IsNotFound(err) {
					parent.ManageConditions().MarkFalse(sourcev1alpha1.ImageRepositoryConditionImageResolved, "ServiceAccountMissing", "ServiceAccount %q not found in namespace %q", serviceAccountName, parent.Namespace)
					return nil
				}
				return err
			}
			for _, ips := range serviceAccount.ImagePullSecrets {
				pullSecretNames.Insert(ips.Name)
			}

			// lookup image pull secrets
			imagePullSecrets := make([]corev1.Secret, len(pullSecretNames))
			for i, imagePullSecretName := range pullSecretNames.List() {
				imagePullSecret := corev1.Secret{}
				err := c.TrackAndGet(ctx, types.NamespacedName{Namespace: parent.Namespace, Name: imagePullSecretName}, &imagePullSecret)
				if err != nil {
					if apierrs.IsNotFound(err) {
						parent.ManageConditions().MarkFalse(sourcev1alpha1.ImageRepositoryConditionImageResolved, "SecretMissing", "Secret %q not found in namespace %q", imagePullSecretName, parent.Namespace)
						return nil
					}
					return err
				}
				imagePullSecrets[i] = imagePullSecret
			}

			StashImagePullSecrets(ctx, imagePullSecrets)

			return nil
		},

		Setup: func(ctx context.Context, mgr controllerruntime.Manager, bldr *builder.Builder) error {
			// register an informer to watch Secret's metadata only. This reduces the cache size in memory.
			bldr.Watches(&source.Kind{Type: &corev1.Secret{}}, reconcilers.EnqueueTracked(ctx, &corev1.Secret{}), builder.OnlyMetadata)
			// register an informer to watch ServiceAccounts
			bldr.Watches(&source.Kind{Type: &corev1.ServiceAccount{}}, reconcilers.EnqueueTracked(ctx, &corev1.ServiceAccount{}))

			return nil
		},
	}
}

func ImageRepositoryImageDigestSyncReconciler() reconcilers.SubReconciler {
	return &reconcilers.SyncReconciler{
		Name: "ImageRepositoryImageDigestSyncReconciler",
		Sync: func(ctx context.Context, parent *sourcev1alpha1.ImageRepository) error {
			log := logr.FromContextOrDiscard(ctx)

			_, err := name.NewDigest(parent.Spec.Image, name.WeakValidation)
			if err == nil {
				// image already resolved to digest
				StashImageRef(ctx, parent.Spec.Image)
				return nil
			}

			// resolve tagged image to digest
			pullSecrets := RetrieveImagePullSecrets(ctx)
			if pullSecrets == nil {
				return nil
			}
			keychain, err := k8schain.NewFromPullSecrets(ctx, pullSecrets)
			if err != nil {
				return err
			}
			tag, err := name.NewTag(parent.Spec.Image, name.WeakValidation)
			if err != nil {
				parent.ManageConditions().MarkFalse(sourcev1alpha1.ImageRepositoryConditionImageResolved, "MalformedRepository", "Image name %q failed validation: %s", parent.Spec.Image, err)
				return nil
			}
			image, err := remote.Head(tag, remote.WithTransport(RetrieveHttpRoundTripper(ctx)), remote.WithContext(ctx), remote.WithAuthFromKeychain(keychain))
			if err != nil {
				// TODO(scothis) handle 403s and 404s as special errors
				log.Error(err, "unable to resolve image tag to a digest", "image", parent.Spec.Image)
				parent.ManageConditions().MarkFalse(sourcev1alpha1.ImageRepositoryConditionImageResolved, "RemoteError", "Unable to resolve image with tag %q to a digest: %s", parent.Spec.Image, err)
				return nil
			}

			StashImageRef(ctx, fmt.Sprintf("%s@%s", tag.Name(), image.Digest))

			return nil
		},
	}
}

func ImageRepositoryPullImageSyncReconciler(httpRootDir, httpHost string, now func() metav1.Time) reconcilers.SubReconciler {
	return &reconcilers.SyncReconciler{
		Name: "ImageRepositoryPullImageSyncReconciler",
		Finalize: func(ctx context.Context, parent *sourcev1alpha1.ImageRepository) error {
			log := logr.FromContextOrDiscard(ctx)
			dir := path.Join(httpRootDir, "imagerepository", parent.Namespace, parent.Name)
			log.Info("remove artifacts", "dir", dir)
			return os.RemoveAll(dir)
		},
		Sync: func(ctx context.Context, parent *sourcev1alpha1.ImageRepository) error {
			log := logr.FromContextOrDiscard(ctx)

			imageRef := RetrieveImageRef(ctx)
			if imageRef == "" {
				return nil
			}
			artifactTgzFilename := fmt.Sprintf("%s.tar.gz", strings.Split(imageRef, "@sha256:")[1])
			if !path.IsAbs(httpRootDir) {
				httpRootDir = path.Join(httpRootDir)
			}
			httpPath := fmt.Sprintf("imagerepository/%s/%s/%s", parent.Namespace, parent.Name, artifactTgzFilename)
			httpUrl := fmt.Sprintf("http://%s/%s", httpHost, httpPath)

			if _, err := os.Stat(path.Join(httpRootDir, httpPath)); err == nil && httpUrl == parent.Status.URL && httpUrl == parent.Status.Artifact.URL {
				log.Info("artifact already exists, skipping", "image", imageRef)
				if apis.ConditionIsUnknown(parent.ManageConditions().GetCondition(sourcev1alpha1.ImageRepositoryConditionImageResolved)) {
					// if we made it this far with the ImageResolved condition as Unknown, it's actually True
					parent.ManageConditions().MarkTrue(sourcev1alpha1.ImageRepositoryConditionImageResolved, "Resolved", "")
				}
				parent.ManageConditions().MarkTrue(sourcev1alpha1.ImageRepositoryConditionArtifactAvailable, "Available", "")
				return nil
			}

			// pull image
			dir, err := ioutil.TempDir(os.TempDir(), "image.*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)

			log.Info("pulling image", "image", imageRef, "directory", dir)

			artifactDir := path.Join(dir, "artifact")
			artifactTgz := path.Join(dir, artifactTgzFilename)

			pullSecrets := RetrieveImagePullSecrets(ctx)
			if pullSecrets == nil {
				return nil
			}
			keychain, err := k8schain.NewFromPullSecrets(ctx, pullSecrets)
			if err != nil {
				return err
			}

			reg, err := registry.NewSimpleRegistry(
				// TODO support more registry options
				registry.Opts{VerifyCerts: true},
				remote.WithContext(ctx),
				remote.WithAuthFromKeychain(keychain),
				remote.WithTransport(RetrieveHttpRoundTripper(ctx)),
			)
			if err != nil {
				return err
			}
			if err := plainimage.NewPlainImage(imageRef, reg).Pull(artifactDir, ui.NewNoopUI()); err != nil {
				// TODO distinguish forbidden and not found errors
				log.Error(err, "unable to pull imgpkg image", "image", imageRef)
				parent.ManageConditions().MarkFalse(sourcev1alpha1.ImageRepositoryConditionImageResolved, "RemoteError", "unable to pull image %q: %s", parent.Spec.Image, err)
				return nil
			}
			parent.ManageConditions().MarkTrue(sourcev1alpha1.ImageRepositoryConditionImageResolved, "Resolved", "")

			// package directory as tgz
			if err := createTarGz(artifactDir, artifactTgz); err != nil {
				log.Error(err, "error creating tarball", "dir", artifactDir, "file", artifactTgz)
				return fmt.Errorf("error creating tarball: %w", err)
			}

			// create sha1 checksum for artifact.tgz
			checksum, err := sha1Checksum(artifactTgz)
			if err != nil {
				return err
			}

			// copy artifact.tgz into httpRoot with a placeholder name
			if err := copyFile(artifactTgz, path.Join(httpRootDir, fmt.Sprintf("%s.new", httpPath))); err != nil {
				return err
			}
			// rename placeholder file to match the final desired name
			if err := os.Rename(path.Join(httpRootDir, fmt.Sprintf("%s.new", httpPath)), path.Join(httpRootDir, httpPath)); err != nil {
				return err
			}

			parent.Status.Artifact = preserveArtifactLastUpdateTime(parent.Status.Artifact, &sourcev1alpha1.Artifact{
				Checksum:       checksum,
				Revision:       imageRef,
				Path:           httpPath,
				URL:            httpUrl,
				LastUpdateTime: now().Rfc3339Copy(),
			})
			parent.Status.URL = httpUrl

			parent.ManageConditions().MarkTrue(sourcev1alpha1.ImageRepositoryConditionArtifactAvailable, "Available", "")

			return nil
		},
	}
}

func ImageRepositoryIntervalReconciler() reconcilers.SubReconciler {
	return &reconcilers.SyncReconciler{
		Name: "ImageRepositoryIntervalReconciler",
		Sync: func(ctx context.Context, parent *sourcev1alpha1.ImageRepository) (controllerruntime.Result, error) {
			return controllerruntime.Result{RequeueAfter: parent.Spec.Interval.Duration}, nil
		},
	}
}

func preserveArtifactLastUpdateTime(current, desired *sourcev1alpha1.Artifact) *sourcev1alpha1.Artifact {
	if current == nil {
		return desired
	}

	cc := current.DeepCopy()
	cc.LastUpdateTime = metav1.Time{}
	dc := desired.DeepCopy()
	dc.LastUpdateTime = metav1.Time{}

	if *cc == *dc {
		return current
	}
	return desired
}

func createTarGz(dir, name string) error {
	file, err := os.Create(name)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	return filepath.WalkDir(dir, func(fp string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		file, err := os.Open(fp)
		if err != nil {
			return err
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			return nil
		}
		name, err := filepath.Rel(dir, fp)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, name)
		header.Name = name
		header.Uid = 0
		header.Gid = 0
		header.Uname = ""
		header.Gname = ""

		if err != nil {
			return err
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if _, err := io.Copy(tarWriter, file); err != nil {
			return err
		}

		return nil
	})
}

func sha1Checksum(name string) (string, error) {
	artifactTgzFile, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer artifactTgzFile.Close()
	checksum := sha1.New()
	if _, err := io.Copy(checksum, artifactTgzFile); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", checksum.Sum(nil)), nil
}

func copyFile(from, to string) error {
	if err := os.MkdirAll(path.Dir(to), 0755); err != nil {
		return err
	}
	fromFile, err := os.Open(from)
	if err != nil {
		return err
	}
	defer fromFile.Close()
	toFile, err := os.Create(to)
	if err != nil {
		return nil
	}
	defer toFile.Close()
	if _, err := io.Copy(toFile, fromFile); err != nil {
		return err
	}
	return nil
}

const HttpRoundTripperStashKey reconcilers.StashKey = sourcev1alpha1.Group + "/http-round-tripper"

func StashHttpRoundTripper(ctx context.Context, transport http.RoundTripper) {
	reconcilers.StashValue(ctx, HttpRoundTripperStashKey, transport)
}

func RetrieveHttpRoundTripper(ctx context.Context) http.RoundTripper {
	transport, ok := reconcilers.RetrieveValue(ctx, HttpRoundTripperStashKey).(http.RoundTripper)
	if !ok {
		return nil
	}
	return transport
}

const ImagePullSecretsStashKey reconcilers.StashKey = sourcev1alpha1.Group + "/image-pull-secrets"

func StashImagePullSecrets(ctx context.Context, pullSecrets []corev1.Secret) {
	reconcilers.StashValue(ctx, ImagePullSecretsStashKey, pullSecrets)
}

func RetrieveImagePullSecrets(ctx context.Context) []corev1.Secret {
	pullSecrets, ok := reconcilers.RetrieveValue(ctx, ImagePullSecretsStashKey).([]corev1.Secret)
	if !ok {
		return nil
	}
	return pullSecrets
}

const ImageRefStashKey reconcilers.StashKey = sourcev1alpha1.Group + "/image-ref"

func StashImageRef(ctx context.Context, image string) {
	reconcilers.StashValue(ctx, ImageRefStashKey, image)
}

func RetrieveImageRef(ctx context.Context) string {
	image, ok := reconcilers.RetrieveValue(ctx, ImageRefStashKey).(string)
	if !ok {
		return ""
	}
	return image
}
