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

package controllers_test

import (
	"context"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
	"time"

	diecorev1 "dies.dev/apis/core/v1"
	diemetav1 "dies.dev/apis/meta/v1"
	"github.com/vmware-labs/reconciler-runtime/reconcilers"
	rtesting "github.com/vmware-labs/reconciler-runtime/testing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	sourcev1alpha1 "github.com/vmware-tanzu/tanzu-source-controller/apis/source/v1alpha1"
	"github.com/vmware-tanzu/tanzu-source-controller/controllers"
	diesourcev1alpha1 "github.com/vmware-tanzu/tanzu-source-controller/dies/source/v1alpha1"
)

func TestMavenArtifactSecretsSyncReconciler(t *testing.T) {
	namespace := "test-namespace"
	name := "my-maven-artifact"

	authSecret := diecorev1.SecretBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name("auth-secret-ref")
		})
	missingSecretRef := diecorev1.SecretBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name("missing-secret-ref")
		})

	repositoryURL := "https://artifact.example.com/repository/project"

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))

	parent := diesourcev1alpha1.MavenArtifactBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		}).
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(repositoryURL)
			})
		})

	rts := rtesting.SubReconcilerTests[*sourcev1alpha1.MavenArtifact]{
		"auth-secret-ref not provided": {
			Resource: parent.DieReleasePtr(),
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
					})
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: nil,
			},
		},
		"auth-secret-ref provided and secret found": {
			Resource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
						d.SecretRef(corev1.LocalObjectReference{Name: "auth-secret-ref"})
					})
				}).DieReleasePtr(),
			GivenObjects: []client.Object{
				authSecret,
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
						d.SecretRef(corev1.LocalObjectReference{Name: "auth-secret-ref"})
					})
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: authSecret.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.ResourceVersion("999")
					}).
					DieRelease(),
			},
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(authSecret, parent, scheme),
			},
			ShouldErr: false,
		},
		"auth secret provided but not found": {
			Resource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
						d.SecretRef(corev1.LocalObjectReference{Name: "missing-secret-ref"})
					})
				}).DieReleasePtr(),
			GivenObjects: []client.Object{
				authSecret,
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
						d.SecretRef(corev1.LocalObjectReference{Name: "missing-secret-ref"})
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionFalse).Reason("SecretMissing").Messagef("Secret %q not found in namespace %q", "missing-secret-ref", "test-namespace"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("SecretMissing").Messagef("Secret %q not found in namespace %q", "missing-secret-ref", "test-namespace"),
					)
				}).DieReleasePtr(),
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(missingSecretRef, parent, scheme),
			},
		}}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*sourcev1alpha1.MavenArtifact], c reconcilers.Config) reconcilers.SubReconciler[*sourcev1alpha1.MavenArtifact] {
		return controllers.MavenArtifactSecretsSyncReconciler([]controllers.Cert{})
	})
}

func TestMavenArtifactWithCustomCA(t *testing.T) {
	namespace := "test-namespace"
	name := "my-maven-repository"

	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer tlsServer.Close()

	certSecret := diecorev1.SecretBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name("cert-secret-ref")
		}).
		AddData("caFile", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsServer.Certificate().Raw}))

	nilCertInSecretRef := diecorev1.SecretBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name("nil-cert-in-secret-ref")
		}).
		AddData("caFile", nil)

	repositoryURL := "https://artifact.example.com/repository/project"

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))

	parent := diesourcev1alpha1.MavenArtifactBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		}).
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(repositoryURL)
			})
		})

	rts := rtesting.SubReconcilerTests[*sourcev1alpha1.MavenArtifact]{
		"cert not provided in secret": {
			Resource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
						d.SecretRef(corev1.LocalObjectReference{Name: "nil-cert-in-secret-ref"})
					})
				}).DieReleasePtr(),
			GivenObjects: []client.Object{
				nilCertInSecretRef,
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
						d.SecretRef(corev1.LocalObjectReference{Name: "nil-cert-in-secret-ref"})
					})
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: nilCertInSecretRef.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.ResourceVersion("999")
					}).
					DieRelease(),
			},
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(nilCertInSecretRef, parent, scheme),
			},
		},
		"cert secret provided and secret found": {
			Resource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
				}).DieReleasePtr(),
			GivenObjects: []client.Object{
				certSecret,
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
				}).DieReleasePtr(),
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(certSecret, parent, scheme),
			},
			ShouldErr: false,
		},
		"cert secret provided but not found": {
			Resource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
						d.SecretRef(corev1.LocalObjectReference{Name: "nil-cert-in-secret-ref"})
					})
				}).DieReleasePtr(),
			GivenObjects: []client.Object{
				certSecret,
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(repositoryURL)
						d.SecretRef(corev1.LocalObjectReference{Name: "nil-cert-in-secret-ref"})
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionFalse).Reason("SecretMissing").
							Message(`Secret "nil-cert-in-secret-ref" not found in namespace "test-namespace"`),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("SecretMissing").
							Message(`Secret "nil-cert-in-secret-ref" not found in namespace "test-namespace"`),
					)
				}).DieReleasePtr(),
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(nilCertInSecretRef, parent, scheme),
			},
		}}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*sourcev1alpha1.MavenArtifact], c reconcilers.Config) reconcilers.SubReconciler[*sourcev1alpha1.MavenArtifact] {
		return controllers.MavenArtifactSecretsSyncReconciler([]controllers.Cert{})
	})
}

func TestMavenArtifactVersionSyncReconciler(t *testing.T) {

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))

	var (
		namespace                         = "test-namespace"
		name                              = "my-maven-artifact"
		badHostname                       = "\t"
		groupId                           = "org.my-group"
		artifactType                      = "jar"
		missingArtifactId                 = "missing-artifact"
		pinnedVersion                     = "2.6.0"
		latestVersion                     = "2.6.7"
		releaseVersion                    = "2.6.7"
		snapshotVersion                   = "2.7.0-SNAPSHOT"
		noVersionSnapshot                 = "0.0.1-SNAPSHOT"
		badSnapshotVersion                = "1.0-SNAPSHOT"
		resolvedSnapshotFileVersion       = "2.7.0-20220708.171442-1"
		artifactWithMissingReleaseVersion = "missing-release"
		artifactId                        = "my-artifact"
		badArtifactId                     = "bad-artifact"
		snapshot                          = "2.7.0-SNAPSHOT"
		latestArtifactId                  = "spring-boot"
		latestSnapshotVersion             = "0.0.5-SNAPSHOT"
		simpleTestData                    = `
			<?xml version="1.0" encoding="UTF-8"?>
			<metadata modelVersion="1.1.0">
				<groupId>org.springframework.boot</groupId>
				<artifactId>spring-boot</artifactId>
				<version>2.6.7</version>
				<versioning>
					<latest>2.6.7</latest>
					<release>2.6.7</release>
					<versions>
						<version>2.6.0</version>
						<version>2.6.7</version>
					</versions>
					<lastUpdated>20220421111446</lastUpdated>
				</versioning>
			</metadata>`

		latestWithNoReleaseData = `
			<?xml version="1.0" encoding="UTF-8"?>
			<metadata modelVersion="1.1.0">
				<groupId>org.springframework.boot</groupId>
				<artifactId>spring-boot</artifactId>
				<version>2.6.7</version>
				<versioning>
					<latest>2.6.7</latest>
					<versions>
						<version>2.6.0</version>
						<version>2.6.7</version>
					</versions>
					<lastUpdated>20220421111446</lastUpdated>
				</versioning>
			</metadata>`

		snapshotTestData = `
			<?xml version="1.0" encoding="UTF-8"?>
			<metadata modelVersion="1.1.0">
			    <groupId>org.springframework.boot</groupId>
			    <artifactId>spring-boot</artifactId>
			    <versioning>
			        <lastUpdated>20220708171442</lastUpdated>
			        <snapshot>
			            <timestamp>20220708.171442</timestamp>
			            <buildNumber>1</buildNumber>
			        </snapshot>
			        <snapshotVersions>
			            <snapshotVersion>
			            	<extension>jar</extension>
			            	<value>2.7.0-20220708.171442-1</value>
			                <updated>20220708171442</updated>
			            </snapshotVersion>
			            <snapshotVersion>
			            	<extension>pom</extension>
			            	<value>2.7.0-20220708.171442-1</value>
			                <updated>20220708171442</updated>
			            </snapshotVersion>
			        </snapshotVersions>
			    </versioning>
			    <version>2.7.0-SNAPSHOT</version>
			</metadata>`

		snapshotNoVersioningTestData = `
			<?xml version="1.0" encoding="UTF-8"?>
			<metadata modelVersion="1.1.0">
			    <groupId>org.springframework.boot</groupId>
			    <artifactId>spring-boot</artifactId>
			    <versioning>
			        <lastUpdated>20220708171442</lastUpdated>
			        <snapshot>
			            <buildNumber>1</buildNumber>
			        </snapshot>
			    </versioning>
			    <version>0.0.1-SNAPSHOT</version>
			</metadata>`

		latestArtifactWithSnapshotData = `
		<?xml version="1.0" encoding="UTF-8"?>
		<metadata>
		<groupId>org.springframework.boot</groupId>
		<artifactId>spring-boot</artifactId>
		<versioning>
			<latest>0.0.5-SNAPSHOT</latest>
			<release>0.0.4</release>
			<versions>
			<version>0.0.1-SNAPSHOT</version>
			<version>0.0.1</version>
			<version>0.0.2-SNAPSHOT</version>
			<version>0.0.2</version>
			<version>0.0.3</version>
			<version>0.0.4-SNAPSHOT</version>
			<version>0.0.4</version>
			<version>0.0.5-SNAPSHOT</version>
			</versions>
			<lastUpdated>20220801151016</lastUpdated>
		</versioning>
		</metadata>
		`

		latestArtifactWithSnapshotVersionData = `
		<?xml version="1.0" encoding="UTF-8"?>
		<metadata modelVersion="1.1.0">
		<groupId>org.springframework.boot</groupId>
		<artifactId>spring-boot</artifactId>
		<version>0.0.5-SNAPSHOT</version>
		<versioning>
			<snapshot>
			<buildNumber>1</buildNumber>
			</snapshot>
			<lastUpdated>20220801151015</lastUpdated>
		</versioning>
		</metadata>

		`

		badTestData = `
			<?xml version="1.0" encoding="UTF-8"?>
			<metadata modelVersion="1.1.0">
				<groupId>org.springframework.boot</groupId>
				<artifactId>spring-boot</artifactId>
				<version>2.6.7</version>
				<versioning>
					<latest>2.6.7</latest>
					<release>2.6.7</rel`

		validAuthorisedSecret = corev1.Secret{
			Type: corev1.BasicAuthUsernameKey,
			ObjectMeta: metav1.ObjectMeta{
				Name:            "authorized-stashed-secret-ref",
				Namespace:       "test-namespace",
				ResourceVersion: "999",
			},
			Data: map[string][]byte{"username": []byte("authorised_user"), "password": []byte("password")},
		}

		validUnAuthorisedSecret = corev1.Secret{
			Type: corev1.BasicAuthUsernameKey,
			ObjectMeta: metav1.ObjectMeta{
				Name:            "unauthorized-stashed-secret-ref",
				Namespace:       "test-namespace",
				ResourceVersion: "999",
			},
			Data: map[string][]byte{"username": []byte("unauthorised_user"), "password": []byte("password")},
		}

		invalidSecret = corev1.Secret{
			Type: corev1.BasicAuthUsernameKey,
			ObjectMeta: metav1.ObjectMeta{
				Name:            "invalid-stashed-secret-ref",
				Namespace:       "test-namespace",
				ResourceVersion: "999",
			},
			Data: map[string][]byte{"username": []byte(""), "password": []byte("invalidpass")},
		}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(user), []byte("authorised_user")) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte("password")) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(401)
			w.Write([]byte("Unauthorized\n"))
			return
		}
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == fmt.Sprintf("/releases/org/my-group/%v/maven-metadata.xml", artifactId) {
				w.Write([]byte(simpleTestData))
			} else if r.URL.Path == fmt.Sprintf("/releases/org/my-group/%v/%v/maven-metadata.xml", artifactId, snapshot) {
				w.Write([]byte(snapshotTestData))
			} else if r.URL.Path == fmt.Sprintf("/releases/org/my-group/%v/maven-metadata.xml", badArtifactId) {
				w.Write([]byte(badTestData))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}(w, r)
	}))
	defer server.Close()

	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == fmt.Sprintf("/ca-releases/org/my-group/%v/maven-metadata.xml", artifactId) {
				w.Write([]byte(simpleTestData))
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/org/my-group/%v/%v/maven-metadata.xml", artifactId, snapshot) {
				w.Write([]byte(snapshotTestData))
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/org/my-group/%v/maven-metadata.xml", badArtifactId) {
				w.Write([]byte(badTestData))
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/org/my-group/%v/maven-metadata.xml", artifactWithMissingReleaseVersion) {
				w.Write([]byte(latestWithNoReleaseData))
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/org/my-group/%v/%v/maven-metadata.xml", artifactId, noVersionSnapshot) {
				w.Write([]byte(snapshotNoVersioningTestData))
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/org/my-group/%v/maven-metadata.xml", latestArtifactId) {
				w.Write([]byte(latestArtifactWithSnapshotData))
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/org/my-group/%v/%v/maven-metadata.xml", latestArtifactId, latestSnapshotVersion) {
				w.Write([]byte(latestArtifactWithSnapshotVersionData))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}(w, r)
	}))
	defer tlsServer.Close()

	tlsServerWithCred := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(user), []byte("authorised_user")) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte("password")) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(401)
			w.Write([]byte("Unauthorized\n"))
			return
		}
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == fmt.Sprintf("/ca-cred-releases/org/my-group/%v/maven-metadata.xml", artifactId) {
				w.Write([]byte(simpleTestData))
			} else if r.URL.Path == fmt.Sprintf("/ca-cred-releases/org/my-group/%v/maven-metadata.xml", badArtifactId) {
				w.Write([]byte(badTestData))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}(w, r)
	}))
	defer tlsServerWithCred.Close()

	parent := diesourcev1alpha1.MavenArtifactBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		}).
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(server.URL + "/releases")
			})
			d.Interval(metav1.Duration{Duration: 5 * time.Minute})
			d.Timeout(&metav1.Duration{Duration: 5 * time.Minute})
		}).
		StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
			d.ObservedGeneration(1)
			d.ConditionsDie(
				diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("AuthenticationResolved").Message("authentication secrets set"),
			)
		})

	parentWithReleaseVersion := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(artifactId)
				d.Type(artifactType)
				d.Version("RELEASE")
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithLatestVersionPinned := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(artifactId)
				d.Type(artifactType)
				d.Version("LATEST")
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithLatestArtifactSnapshotVersion := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(latestArtifactId)
				d.Type(artifactType)
				d.Version("LATEST")
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithPinnedVersion := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(artifactId)
				d.Type(artifactType)
				d.Version(pinnedVersion)
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithSnapshotVersion := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(artifactId)
				d.Type(artifactType)
				d.Version(snapshotVersion)
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithBadSnapshotVersion := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(artifactId)
				d.Type(artifactType)
				d.Version(badSnapshotVersion)
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithSnapshotNoVersion := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(artifactId)
				d.Type(artifactType)
				d.Version(noVersionSnapshot)
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithBadHostname := diesourcev1alpha1.MavenArtifactBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		}).
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(artifactId)
				d.Type(artifactType)
				d.Version(latestVersion)
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(badHostname)
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
			d.Interval(metav1.Duration{Duration: 5 * time.Minute})
			d.Timeout(&metav1.Duration{Duration: 5 * time.Minute})
		}).
		StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
			d.ObservedGeneration(1)
			d.ConditionsDie(
				diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("AuthenticationResolved").Message("authentication secrets set"),
			)
		})

	parentWithMissingArtifact := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(missingArtifactId)
				d.Type(artifactType)
				d.Version("RELEASE")
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithMissingReleaseArtifact := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(artifactWithMissingReleaseVersion)
				d.Type(artifactType)
				d.Version("RELEASE")
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithCaCertificate := parentWithReleaseVersion.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithBadArtifact := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.GroupId(groupId)
				d.ArtifactId(badArtifactId)
				d.Type(artifactType)
				d.Version("RELEASE")
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	rts := rtesting.SubReconcilerTests[*sourcev1alpha1.MavenArtifact]{
		"release version": {
			Resource: parentWithReleaseVersion.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parentWithReleaseVersion.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ObservedGeneration(1)
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.
							Status(metav1.ConditionTrue).
							Reason("Resolved").
							Messagef(`Resolved version %q for artifact "%s/ca-releases/org/my-group/%s/%s/%s-%s.%s"`, releaseVersion, tlsServer.URL, artifactId, releaseVersion, artifactId, releaseVersion, artifactType),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     latestVersion,
					ResolvedFileName:    fmt.Sprintf("%s-%s.%s", artifactId, releaseVersion, artifactType),
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/org/my-group/%s/%s/%s-%s.%s", tlsServer.URL, artifactId, latestVersion, artifactId, releaseVersion, artifactType),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
			},
		},
		"latest version": {
			Resource: parentWithLatestVersionPinned.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parentWithLatestVersionPinned.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ObservedGeneration(1)
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.
							Status(metav1.ConditionTrue).
							Reason("Resolved").
							Messagef(`Resolved version %q for artifact "%s/ca-releases/org/my-group/%s/%s/%s-%s.%s"`, latestVersion, tlsServer.URL, artifactId, latestVersion, artifactId, latestVersion, artifactType),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     latestVersion,
					ResolvedFileName:    fmt.Sprintf("%s-%s.%s", artifactId, latestVersion, artifactType),
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/org/my-group/%s/%s/%s-%s.%s", tlsServer.URL, artifactId, latestVersion, artifactId, latestVersion, artifactType),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
			},
		},
		"latest version with snapshot": {
			Resource: parentWithLatestArtifactSnapshotVersion.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parentWithLatestArtifactSnapshotVersion.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ObservedGeneration(1)
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.
							Status(metav1.ConditionTrue).
							Reason("Resolved").
							Messagef(`Resolved version %q for artifact "%s/ca-releases/org/my-group/%s/%s/%s-%s.%s"`, latestSnapshotVersion, tlsServer.URL, latestArtifactId, latestSnapshotVersion, latestArtifactId, latestSnapshotVersion, artifactType),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     latestSnapshotVersion,
					ResolvedFileName:    fmt.Sprintf("%s-%s.%s", latestArtifactId, latestSnapshotVersion, artifactType),
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/org/my-group/%s/%s/%s-%s.%s", tlsServer.URL, latestArtifactId, latestSnapshotVersion, latestArtifactId, latestSnapshotVersion, artifactType),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
			},
		},
		"pinned version": {
			Resource: parentWithPinnedVersion.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parentWithPinnedVersion.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ObservedGeneration(1)
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.
							Status(metav1.ConditionTrue).
							Reason("Resolved").
							Messagef(`Resolved version %q for artifact "%s/ca-releases/org/my-group/%s/%s/%s-%s.%s"`, pinnedVersion, tlsServer.URL, artifactId, pinnedVersion, artifactId, pinnedVersion, artifactType),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     pinnedVersion,
					ResolvedFileName:    fmt.Sprintf("%s-%s.%s", artifactId, pinnedVersion, artifactType),
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/org/my-group/%s/%s/%s-%s.%s", tlsServer.URL, artifactId, pinnedVersion, artifactId, pinnedVersion, artifactType),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
			},
		},
		"snapshot version": {
			Resource: parentWithSnapshotVersion.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parentWithSnapshotVersion.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ObservedGeneration(1)
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.
							Status(metav1.ConditionTrue).
							Reason("Resolved").
							Messagef(`Resolved version %q for artifact "%s/ca-releases/org/my-group/%s/%s/%s-%s.%s"`, snapshotVersion, tlsServer.URL, artifactId, snapshotVersion, artifactId, resolvedSnapshotFileVersion, artifactType),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     snapshotVersion,
					ResolvedFileName:    fmt.Sprintf("%s-%s.%s", artifactId, resolvedSnapshotFileVersion, artifactType),
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/org/my-group/%s/%s/%s-%s.%s", tlsServer.URL, artifactId, snapshotVersion, artifactId, resolvedSnapshotFileVersion, artifactType),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
			},
		},
		"snapshot no-version": {
			Resource: parentWithSnapshotNoVersion.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parentWithSnapshotNoVersion.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ObservedGeneration(1)
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.
							Status(metav1.ConditionTrue).
							Reason("Resolved").
							Messagef(`Resolved version %q for artifact "%s/ca-releases/org/my-group/%s/%s/%s-%s.%s"`, noVersionSnapshot, tlsServer.URL, artifactId, noVersionSnapshot, artifactId, noVersionSnapshot, artifactType),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     noVersionSnapshot,
					ResolvedFileName:    fmt.Sprintf("%s-%s.%s", artifactId, noVersionSnapshot, artifactType),
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/org/my-group/%s/%s/%s-%s.%s", tlsServer.URL, artifactId, noVersionSnapshot, artifactId, noVersionSnapshot, artifactType),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
			},
		},
		"missing snapshot metadata": {
			Resource: parentWithBadSnapshotVersion.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parentWithBadSnapshotVersion.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Maven metadata file not found (HTTP 404) for artifact "%v:%v" from repository URL "%s/ca-releases".`, groupId, artifactId, tlsServer.URL),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Maven metadata file not found (HTTP 404) for artifact "%v:%v" from repository URL "%s/ca-releases".`, groupId, artifactId, tlsServer.URL),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
			},
		},
		"missing release version": {
			Resource: parentWithMissingReleaseArtifact.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parentWithMissingReleaseArtifact.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionFalse).Reason("VersionError").Message("artifact metadata does not have a RELEASE version"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("VersionError").Message("artifact metadata does not have a RELEASE version"),
					)
				}).DieReleasePtr(),
		},
		"missing artifact": {
			Resource: parentWithMissingArtifact.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parentWithMissingArtifact.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Maven metadata file not found (HTTP 404) for artifact "%v:%v" from repository URL "%s/ca-releases".`, groupId, missingArtifactId, tlsServer.URL),

						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Maven metadata file not found (HTTP 404) for artifact "%v:%v" from repository URL "%s/ca-releases".`, groupId, missingArtifactId, tlsServer.URL),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey:    nil,
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
			},
		},
		"bad artifact data": {
			Resource: parentWithBadArtifact.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parentWithBadArtifact.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionFalse).Reason("VersionError").
							Messagef(`Error "XML syntax error on line 9: unexpected EOF" while parsing XML data at "%s/ca-releases/org/my-group/%s/maven-metadata.xml"`, tlsServer.URL, badArtifactId),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("VersionError").
							Messagef(`Error "XML syntax error on line 9: unexpected EOF" while parsing XML data at "%s/ca-releases/org/my-group/%s/maven-metadata.xml"`, tlsServer.URL, badArtifactId),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
			},
		},
		"metadata protected by TLS": {
			Resource: parentWithCaCertificate.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactHttpClientKey: tlsServer.Client(),
			},
			ExpectResource: parentWithCaCertificate.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ObservedGeneration(1)
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.
							Status(metav1.ConditionTrue).
							Reason("Resolved").
							Messagef(`Resolved version %q for artifact "%s/ca-releases/org/my-group/%s/%s/%s-%s.%s"`, latestVersion, tlsServer.URL, artifactId, latestVersion, artifactId, latestVersion, artifactType),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     latestVersion,
					ResolvedFileName:    fmt.Sprintf("%s-%s.%s", artifactId, latestVersion, artifactType),
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/org/my-group/%s/%s/%s-%s.%s", tlsServer.URL, artifactId, latestVersion, artifactId, latestVersion, artifactType),
				},
			},
		},
		"bad hostname": {
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactHttpClientKey: server.Client(),
			},
			Resource: parentWithBadHostname.DieReleasePtr(),
			ExpectResource: parentWithBadHostname.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionFalse).Reason("ConfigurationError").
							Messagef(`Error parsing repository URL %q: parse "\t": net/url: invalid control character in URL`, badHostname),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("ConfigurationError").
							Messagef(`Error parsing repository URL %q: parse "\t": net/url: invalid control character in URL`, badHostname),
					)
				}).DieReleasePtr(),
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: nil,
			},
		},
		"invalid credentials in download maven artifact metadata request": {
			Resource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.GroupId(groupId)
						d.ArtifactId(artifactId)
						d.Type(artifactType)
						d.Version("RELEASE")
					})
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServerWithCred.URL + "/ca-cred-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "invalid-stashed-secret-ref"})
					})
				}).DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: invalidSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServerWithCred.Client(),
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: invalidSecret,
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.GroupId(groupId)
						d.ArtifactId(artifactId)
						d.Type(artifactType)
						d.Version("RELEASE")
					})
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServerWithCred.URL + "/ca-cred-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "invalid-stashed-secret-ref"})
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ObservedGeneration(1)
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unauthorized credentials (HTTP 401) error downloading artifact metadata "%v:%v" from repository URL "%v/ca-cred-releases". Check the credentials provided in the Secret.`, groupId, artifactId, tlsServerWithCred.URL),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unauthorized credentials (HTTP 401) error downloading artifact metadata "%v:%v" from repository URL "%v/ca-cred-releases". Check the credentials provided in the Secret.`, groupId, artifactId, tlsServerWithCred.URL),
					)
				}).DieReleasePtr(),
		},
		"unauthorized credentials in download maven artifact metadata request": {
			Resource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.GroupId(groupId)
						d.ArtifactId(artifactId)
						d.Type(artifactType)
						d.Version("RELEASE")
					})
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServerWithCred.URL + "/ca-cred-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "unauthorised-stashed-secret-ref"})
					})
				}).DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validUnAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServerWithCred.Client(),
			},
			ExpectStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactAuthSecretStashKey: validUnAuthorisedSecret,
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.GroupId(groupId)
						d.ArtifactId(artifactId)
						d.Type(artifactType)
						d.Version("RELEASE")
					})
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServerWithCred.URL + "/ca-cred-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "unauthorised-stashed-secret-ref"})
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ObservedGeneration(1)
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unauthorized credentials (HTTP 401) error downloading artifact metadata "%v:%v" from repository URL "%v/ca-cred-releases". Check the credentials provided in the Secret.`, groupId, artifactId, tlsServerWithCred.URL),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unauthorized credentials (HTTP 401) error downloading artifact metadata "%v:%v" from repository URL "%v/ca-cred-releases". Check the credentials provided in the Secret.`, groupId, artifactId, tlsServerWithCred.URL),
					)
				}).DieReleasePtr(),
		}}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*sourcev1alpha1.MavenArtifact], c reconcilers.Config) reconcilers.SubReconciler[*sourcev1alpha1.MavenArtifact] {
		return controllers.MavenArtifactVersionSyncReconciler()
	})
}

func TestMavenArtifactDownloadSyncReconciler(t *testing.T) {
	namespace := "test-namespace"
	name := "my-maven-artifact"
	groupId := "my-group"
	artifactId := "helloworld"
	badArtifactId := "goodbyeworld"
	failDownloadArtifact := "fail-download"
	artifactVersion := "1.1"
	classifier := "sources"
	failDownloadZip := fmt.Sprintf("%s-%s.zip", failDownloadArtifact, artifactVersion)
	fileName := fmt.Sprintf("%s-%s.jar", artifactId, artifactVersion)
	badFilename := fmt.Sprintf("%s-%s.jar", badArtifactId, artifactVersion)
	fileNameWithZip := fmt.Sprintf("%s-%s.zip", artifactId, artifactVersion)
	fileNameAndClassifier := fmt.Sprintf("%s-%s-%s.jar", artifactId, artifactVersion, classifier)
	artifactJarToTgzFilename := "8fdea0bf0e6441c8717853230a270e4ed51cd77a"
	artifactZipToTgzFilename := "a3794eec54f0ab3a2d62c31cf5a3b947c1ecc2b1"
	checksum := "6271d8d39c1936f8e0b25c8b2d43fe671f7de1f8"
	zipChecksum := "d1f7d7c82fdb54a360e7f3c29024d3af2f10600c"

	now := func() metav1.Time {
		return metav1.Time{
			Time: time.Unix(1, 0),
		}
	}

	olderTime := func() metav1.Time {
		return metav1.Time{
			Time: time.Unix(100, 0),
		}
	}

	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(user), []byte("authorised_user")) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte("password")) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
				w.WriteHeader(401)
				w.Write([]byte("Unauthorized\n"))
				return
			}
			if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v", groupId, artifactId, artifactVersion, fileName) {
				fileBytes, err := os.ReadFile("fixtures/maven-artifact/helloworld-1.1.jar")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write(fileBytes)
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v", groupId, artifactId, artifactVersion, fileName) {
				checksum, err := sha1Checksum("fixtures/maven-artifact/helloworld-1.1.jar")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(checksum))
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v", groupId, artifactId, artifactVersion, fileName) {
				fileBytes, err := os.ReadFile("fixtures/maven-artifact/helloworld-1.1.jar")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write(fileBytes)
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v.sha1", groupId, artifactId, artifactVersion, fileName) {
				checksum, err := sha1Checksum("fixtures/maven-artifact/helloworld-1.1.jar")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(checksum))
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v", groupId, artifactId, artifactVersion, fileNameAndClassifier) {
				fileBytes, err := os.ReadFile("fixtures/maven-artifact/helloworld-1.1.jar")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write(fileBytes)
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v.sha1", groupId, artifactId, artifactVersion, fileNameAndClassifier) {
				checksum, err := sha1Checksum("fixtures/maven-artifact/helloworld-1.1.jar")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(checksum))
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v", groupId, artifactId, artifactVersion, fileNameWithZip) {
				fileBytes, err := os.ReadFile("fixtures/maven-artifact/helloworld-1.1.zip")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write(fileBytes)
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v.sha1", groupId, artifactId, artifactVersion, fileNameWithZip) {
				checksum, err := sha1Checksum("fixtures/maven-artifact/helloworld-1.1.zip")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(checksum))
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v.sha1", groupId, failDownloadArtifact, artifactVersion, failDownloadZip) {
				checksum, err := sha1Checksum("fixtures/maven-artifact/helloworld-1.1.zip")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(checksum))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}(w, r)
	}))
	defer tlsServer.Close()

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))

	artifactRootDir, err := os.MkdirTemp(os.TempDir(), "maven-artifacts.*")
	utilruntime.Must(err)
	defer os.RemoveAll(artifactRootDir)

	parent := diesourcev1alpha1.MavenArtifactBlank.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.Type("jar")
				d.ArtifactId(artifactId)
				d.GroupId(groupId)
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
			d.Interval(metav1.Duration{Duration: 5 * time.Minute})
			d.Timeout(&metav1.Duration{Duration: 5 * time.Minute})
		}).
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		}).
		StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
			d.ObservedGeneration(1)
			d.ConditionsDie(
				diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
			)
		})

	parentWithZip := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.Type("zip")
				d.ArtifactId(artifactId)
				d.GroupId(groupId)
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithBadZip := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.Type("zip")
				d.ArtifactId(failDownloadArtifact)
				d.GroupId(groupId)
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithClassifier := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.Type("jar")
				d.ArtifactId(artifactId)
				d.GroupId(groupId)
				d.Classifier(classifier)
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithoutClassifier := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
				d.Type("jar")
				d.ArtifactId(artifactId)
				d.GroupId(groupId)
			})
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		})

	parentWithCaCertificate := parent.
		SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
			d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
				d.URL(tlsServer.URL + "/ca-releases")
				d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
			})
		}).
		StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
			d.ObservedGeneration(1)
			d.ConditionsDie(
				diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
			)
		})

	var (
		validAuthorisedSecret = corev1.Secret{
			Type: corev1.BasicAuthUsernameKey,
			ObjectMeta: metav1.ObjectMeta{
				Name:            "authorized-stashed-secret-ref",
				Namespace:       "test-namespace",
				ResourceVersion: "999",
			},
			Data: map[string][]byte{"username": []byte("authorised_user"), "password": []byte("password")},
		}

		validUnAuthorisedSecret = corev1.Secret{
			Type: corev1.BasicAuthUsernameKey,
			ObjectMeta: metav1.ObjectMeta{
				Name:            "unauthorized-stashed-secret-ref",
				Namespace:       "test-namespace",
				ResourceVersion: "999",
			},
			Data: map[string][]byte{"username": []byte("unauthorised_user"), "password": []byte("password")},
		}

		invalidSecret = corev1.Secret{
			Type: corev1.BasicAuthUsernameKey,
			ObjectMeta: metav1.ObjectMeta{
				Name:            "invalid-stashed-secret-ref",
				Namespace:       "test-namespace",
				ResourceVersion: "999",
			},
			Data: map[string][]byte{"username": []byte(""), "password": []byte("invalidpass")},
		}
	)
	successRTS := rtesting.SubReconcilerTests[*sourcev1alpha1.MavenArtifact]{
		"download a zip": {
			Resource: parentWithZip.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    fileNameWithZip,
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s", tlsServer.URL, artifactId, artifactVersion, fileNameWithZip),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("zip")
						d.ArtifactId(artifactId)
						d.GroupId(groupId)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
						d.Revision(fileNameWithZip)
						d.Path("mavenartifact/test-namespace/my-maven-artifact/" + artifactZipToTgzFilename + ".tar.gz")
						d.URL("http://artifact.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactZipToTgzFilename + ".tar.gz")
						d.LastUpdateTime(now())
						d.Checksum(zipChecksum)
					})
					d.URL("http://artifact.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactZipToTgzFilename + ".tar.gz")
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
					)
				}).DieReleasePtr(),
		},
		"download fail missing zip": {
			Resource: parentWithBadZip.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    failDownloadZip,
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s", tlsServer.URL, failDownloadArtifact, artifactVersion, failDownloadZip),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},

			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("zip")
						d.ArtifactId(failDownloadArtifact)
						d.GroupId(groupId)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionFalse).Reason("DownloadError").
							Messagef(`Maven artifact file not found (HTTP 404) at URL "%s/ca-releases/my-group/%s/1.1/%s-1.1.zip".`, tlsServer.URL, failDownloadArtifact, failDownloadArtifact),
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("DownloadError").
							Messagef(`Maven artifact file not found (HTTP 404) at URL "%s/ca-releases/my-group/%s/1.1/%s-1.1.zip".`, tlsServer.URL, failDownloadArtifact, failDownloadArtifact),
					)
				}).DieReleasePtr(),
		},
		"download artifact without classifier": {
			Resource: parentWithoutClassifier.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    fmt.Sprintf("%s-%s.%s", artifactId, artifactVersion, "jar"),
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s-%s.%s", tlsServer.URL, artifactId, artifactVersion, artifactId, artifactVersion, "jar"),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("jar")
						d.ArtifactId(artifactId)
						d.GroupId(groupId)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
						d.Revision(fileName)
						d.Path("mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
						d.URL("http://artifact.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
						d.LastUpdateTime(now())
						d.Checksum(checksum)
					})
					d.URL("http://artifact.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
					)
				}).DieReleasePtr(),
		},
		"download artifact with classifier": {
			Resource: parentWithClassifier.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    fileNameAndClassifier,
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s", tlsServer.URL, artifactId, artifactVersion, fileNameAndClassifier),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("jar")
						d.ArtifactId(artifactId)
						d.GroupId(groupId)
						d.Classifier(classifier)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
						d.Revision(fileNameAndClassifier)
						d.Path("mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
						d.URL("http://artifact.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
						d.LastUpdateTime(now())
						d.Checksum(checksum)
					})
					d.URL("http://artifact.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
					)
				}).DieReleasePtr(),
		},
		"Update http host URL": {
			Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.SubReconcilerTestCase[*sourcev1alpha1.MavenArtifact]) (context.Context, error) {
				dir := path.Join(artifactRootDir, "mavenartifact", namespace, name)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return ctx, err
				}
				if _, err := os.Create(path.Join(dir, artifactJarToTgzFilename+".tar.gz")); err != nil {
					return ctx, err
				}

				return ctx, setCache(fmt.Sprintf("%s/helloworld-1.1.jar.sha1", dir), fmt.Sprintf("%s/my-group/helloworld/1.1/helloworld-1.1.jar|%s", tlsServer.URL+"/ca-releases", "8fdea0bf0e6441c8717853230a270e4ed51cd77a"))
			},
			Resource: parent.
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
						d.Revision(fileName)
						d.Path("mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
						d.URL("http://localhost.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
						d.LastUpdateTime(olderTime())
						d.Checksum(checksum)
					})
					d.URL("http://localhost.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
				}).DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    fileName,
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s", tlsServer.URL, artifactId, artifactVersion, fileName),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("jar")
						d.ArtifactId(artifactId)
						d.GroupId(groupId)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
						d.Revision(fileName)
						d.Path("mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
						d.URL("http://localhost.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
						d.LastUpdateTime(olderTime())
						d.Checksum(checksum)
					})
					d.URL("http://localhost.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
					)
				}).DieReleasePtr(),
			CleanUp: func(t *testing.T, ctx context.Context, tc *rtesting.SubReconcilerTestCase[*sourcev1alpha1.MavenArtifact]) error {
				artifact := path.Join(artifactRootDir, "mavenartifact")
				os.RemoveAll(artifact)
				return nil
			},
		},
		"artifact was not found": {
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    badFilename,
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s", tlsServer.URL, badArtifactId, artifactVersion, badFilename),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			Resource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("jar")
						d.ArtifactId(badArtifactId)
						d.GroupId(groupId)
					})
				}).DieReleasePtr(),
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("jar")
						d.ArtifactId(badArtifactId)
						d.GroupId(groupId)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Maven artifact checksum file not found (HTTP 404) at URL "%s/ca-releases/my-group/%s/1.1/%s-1.1.jar.sha1".`, tlsServer.URL, badArtifactId, badArtifactId),
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Maven artifact checksum file not found (HTTP 404) at URL "%s/ca-releases/my-group/%s/1.1/%s-1.1.jar.sha1".`, tlsServer.URL, badArtifactId, badArtifactId),
					)
				}).DieReleasePtr(),
		},
		"authentication was not stashed": {
			Resource: parent.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    fileName,
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s", tlsServer.URL, artifactId, artifactVersion, fileName),
				},
				controllers.MavenArtifactHttpClientKey: tlsServer.Client(),
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("jar")
						d.ArtifactId(artifactId)
						d.GroupId(groupId)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unauthorized credentials (HTTP 401) error downloading Maven artifact checksum from URL "%v/ca-releases/my-group/helloworld/1.1/helloworld-1.1.jar.sha1". Check the credentials provided in the Secret.`, tlsServer.URL),
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unauthorized credentials (HTTP 401) error downloading Maven artifact checksum from URL "%v/ca-releases/my-group/helloworld/1.1/helloworld-1.1.jar.sha1". Check the credentials provided in the Secret.`, tlsServer.URL),
					)
				}).DieReleasePtr(),
		},
		"download artifact protected by TLS": {
			Resource: parentWithCaCertificate.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    fileName,
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s", tlsServer.URL, artifactId, artifactVersion, fileName),
				},
				controllers.MavenArtifactAuthSecretStashKey: validAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("jar")
						d.ArtifactId(artifactId)
						d.GroupId(groupId)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
						d.Revision(fileName)
						d.Path("mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
						d.URL("http://artifact.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
						d.LastUpdateTime(now())
						d.Checksum(checksum)
					})
					d.URL("http://artifact.example/mavenartifact/test-namespace/my-maven-artifact/" + artifactJarToTgzFilename + ".tar.gz")
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
					)
				}).DieReleasePtr(),
		},
		"download artifact protected by TLS but certificate empty": {
			Resource: parentWithCaCertificate.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    fileName,
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s", tlsServer.URL, artifactId, artifactVersion, fileName),
				},
				controllers.MavenArtifactHttpClientKey: tlsServer.Client(),
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("jar")
						d.ArtifactId(artifactId)
						d.GroupId(groupId)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unauthorized credentials (HTTP 401) error downloading Maven artifact checksum from URL "%v/ca-releases/my-group/helloworld/1.1/helloworld-1.1.jar.sha1". Check the credentials provided in the Secret.`, tlsServer.URL),
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unauthorized credentials (HTTP 401) error downloading Maven artifact checksum from URL "%v/ca-releases/my-group/helloworld/1.1/helloworld-1.1.jar.sha1". Check the credentials provided in the Secret.`, tlsServer.URL),
					)
				}).DieReleasePtr(),
		}}

	successRTS.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*sourcev1alpha1.MavenArtifact], c reconcilers.Config) reconcilers.SubReconciler[*sourcev1alpha1.MavenArtifact] {
		return controllers.MavenArtifactDownloadSyncReconciler(artifactRootDir, "artifact.example", now)
	})

	failRTS := rtesting.SubReconcilerTests[*sourcev1alpha1.MavenArtifact]{
		"invalid credentials in download artifact request": {
			Resource: parent.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    fileName,
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s", tlsServer.URL, artifactId, artifactVersion, fileName),
				},
				controllers.MavenArtifactAuthSecretStashKey: invalidSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("jar")
						d.ArtifactId(artifactId)
						d.GroupId(groupId)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unauthorized credentials (HTTP 401) error downloading Maven artifact checksum from URL "%v/ca-releases/my-group/helloworld/1.1/helloworld-1.1.jar.sha1". Check the credentials provided in the Secret.`, tlsServer.URL),
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unauthorized credentials (HTTP 401) error downloading Maven artifact checksum from URL "%v/ca-releases/my-group/helloworld/1.1/helloworld-1.1.jar.sha1". Check the credentials provided in the Secret.`, tlsServer.URL),
					)
				}).DieReleasePtr(),
		},
		"valid but incorrect credentials in download artifact request": {
			Resource: parent.DieReleasePtr(),
			GivenStashedValues: map[reconcilers.StashKey]interface{}{
				controllers.MavenArtifactVersionStashKey: controllers.ArtifactDetails{
					ArtifactVersion:     artifactVersion,
					ResolvedFileName:    fileName,
					ArtifactDownloadURL: fmt.Sprintf("%s/ca-releases/my-group/%s/%s/%s", tlsServer.URL, artifactId, artifactVersion, fileName),
				},
				controllers.MavenArtifactAuthSecretStashKey: validUnAuthorisedSecret,
				controllers.MavenArtifactHttpClientKey:      tlsServer.Client(),
			},
			ExpectResource: parent.
				SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
					d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
						d.URL(tlsServer.URL + "/ca-releases")
						d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
					})
					d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
						d.Type("jar")
						d.ArtifactId(artifactId)
						d.GroupId(groupId)
					})
				}).
				StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
					d.ConditionsDie(
						diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionFalse).Reason("RemoteError").Messagef(`Unauthorized credentials (HTTP 401) error downloading Maven artifact checksum from URL "%s/ca-releases/my-group/helloworld/1.1/helloworld-1.1.jar.sha1". Check the credentials provided in the Secret.`, tlsServer.URL),
						diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").Messagef(`Unauthorized credentials (HTTP 401) error downloading Maven artifact checksum from URL "%s/ca-releases/my-group/helloworld/1.1/helloworld-1.1.jar.sha1". Check the credentials provided in the Secret.`, tlsServer.URL),
					)
				}).DieReleasePtr(),
		}}
	failRTS.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase[*sourcev1alpha1.MavenArtifact], c reconcilers.Config) reconcilers.SubReconciler[*sourcev1alpha1.MavenArtifact] {
		return controllers.MavenArtifactDownloadSyncReconciler(artifactRootDir, "artifact.example", now)
	})
}

func TestMavenArtifactReconciler(t *testing.T) {
	namespace := "test-namespace"
	name := "my-maven-artifact"
	request := reconcilers.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}

	groupId := "my-group"
	artifactId := "helloworld"
	latestVersion := "1.1"
	fileName := fmt.Sprintf("%s-%s.jar", artifactId, latestVersion)
	fileNameWithoutType := "8fdea0bf0e6441c8717853230a270e4ed51cd77a"
	checksum := "6271d8d39c1936f8e0b25c8b2d43fe671f7de1f8"

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))

	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(user), []byte("authorised_user")) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte("password")) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
				w.WriteHeader(401)
				w.Write([]byte("Unauthorized\n"))
				return
			}
			if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v", groupId, artifactId, latestVersion, fileName) {
				fileBytes, err := os.ReadFile("fixtures/maven-artifact/helloworld-1.1.jar")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write(fileBytes)
			} else if r.URL.Path == fmt.Sprintf("/ca-releases/%v/%v/%v/%v.sha1", groupId, artifactId, latestVersion, fileName) {
				checksum, err := sha1Checksum("fixtures/maven-artifact/helloworld-1.1.jar")
				if err != nil {
					panic(err)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(checksum))

			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}(w, r)
	}))
	defer tlsServer.Close()

	artifactRootDir, err := os.MkdirTemp(os.TempDir(), "artifacts.*")
	utilruntime.Must(err)
	defer os.RemoveAll(artifactRootDir)

	now := func() metav1.Time {
		return metav1.Time{
			Time: time.Unix(1, 0),
		}
	}

	certSecret := diecorev1.SecretBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name("cert-secret-ref")
		}).
		AddData("caFile", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsServer.Certificate().Raw})).
		AddData("username", []byte("authorised_user")).
		AddData("password", []byte("password"))

	parent := diesourcev1alpha1.MavenArtifactBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		})

	rts := rtesting.ReconcilerTests{
		"requeue interval": {
			Request: request,
			StatusSubResourceTypes: []client.Object{
				&sourcev1alpha1.MavenArtifact{},
			},
			GivenObjects: []client.Object{
				parent.
					SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
						d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
							d.Type("jar")
							d.ArtifactId(artifactId)
							d.GroupId(groupId)
							d.Version(latestVersion)
						})
						d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
							d.URL(tlsServer.URL + "/ca-releases")
							d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
						})
						d.Interval(metav1.Duration{Duration: 5 * time.Minute})
						d.Timeout(&metav1.Duration{Duration: 5 * time.Minute})
					}),
				certSecret,
			},

			ExpectStatusUpdates: []client.Object{
				parent.
					StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
						d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
							d.Revision(fileName)
							d.Path(fmt.Sprintf("mavenartifact/%s/%s/%s.tar.gz", namespace, name, fileNameWithoutType))
							d.URL(fmt.Sprintf("http://artifact.example/mavenartifact/%s/%s/%s.tar.gz", namespace, name, fileNameWithoutType))
							d.LastUpdateTime(now())
							d.Checksum(checksum)
						})
						d.URL(fmt.Sprintf("http://artifact.example/mavenartifact/%s/%s/%s.tar.gz", namespace, name, fileNameWithoutType))
						d.ObservedGeneration(1)
						d.ConditionsDie(
							diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
							diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved").Messagef(`Resolved version %q for artifact "%s/%s/%s/%s/%s-%s.jar"`, latestVersion, tlsServer.URL+"/ca-releases", groupId, artifactId, latestVersion, artifactId, latestVersion),
							diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
						)
					}),
			},

			ExpectPatches: []rtesting.PatchRef{
				{
					Group:     "source.apps.tanzu.vmware.com",
					Kind:      "MavenArtifact",
					Namespace: parent.GetNamespace(),
					Name:      parent.GetName(),
					PatchType: types.MergePatchType,
					Patch:     []byte(`{"metadata":{"finalizers":["source.apps.tanzu.vmware.com/finalizer"],"resourceVersion":"999"}}`),
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(parent, scheme, corev1.EventTypeNormal, "FinalizerPatched", "Patched finalizer %q", "source.apps.tanzu.vmware.com/finalizer"),
				rtesting.NewEvent(parent, scheme, corev1.EventTypeNormal, "StatusUpdated", `Updated status`),
			},
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(certSecret, parent, scheme),
			},
			ExpectedResult: reconcile.Result{
				RequeueAfter: 5 * time.Minute,
			},
		},
		"skip download": {
			Request: request,
			Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) (context.Context, error) {
				dir := path.Join(artifactRootDir, "mavenartifact", namespace, name)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return ctx, err
				}

				return ctx, setCache(fmt.Sprintf("%s/helloworld-1.1.jar.sha1", dir), fmt.Sprintf("%s/my-group/helloworld/1.1/helloworld-1.1.jar|%s", tlsServer.URL+"/ca-releases", "8fdea0bf0e6441c8717853230a270e4ed51cd77a"))
			},
			GivenObjects: []client.Object{
				parent.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.Namespace(namespace)
						d.Name(name)
						d.Generation(1)
					}).
					SpecDie(func(d *diesourcev1alpha1.MavenArtifactSpecDie) {
						d.MavenArtifactDie(func(d *diesourcev1alpha1.MavenArtifactTypeDie) {
							d.Type("jar")
							d.ArtifactId(artifactId)
							d.GroupId(groupId)
							d.Version(latestVersion)
						})
						d.RepositoryDie(func(d *diesourcev1alpha1.RepositoryDie) {
							d.URL(tlsServer.URL + "/ca-releases")
							d.SecretRef(corev1.LocalObjectReference{Name: "cert-secret-ref"})
						})
						d.Interval(metav1.Duration{Duration: 5 * time.Minute})
						d.Timeout(&metav1.Duration{Duration: 5 * time.Minute})
					}).
					StatusDie(func(d *diesourcev1alpha1.MavenArtifactStatusDie) {
						d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
							d.Revision(fileName)
							d.Path(fmt.Sprintf("mavenartifact/%s/%s/%s.tar.gz", namespace, name, fileNameWithoutType))
							d.URL(fmt.Sprintf("http://artifact.example/mavenartifact/%s/%s/%s.tar.gz", namespace, name, fileNameWithoutType))
							d.LastUpdateTime(now())
							d.Checksum(checksum)
						})
						d.URL(fmt.Sprintf("http://artifact.example/mavenartifact/%s/%s/%s.tar.gz", namespace, name, fileNameWithoutType))
						d.ObservedGeneration(1)
						d.ConditionsDie(
							diesourcev1alpha1.MavenArtifactConditionAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
							diesourcev1alpha1.MavenArtifactConditionVersionResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved").Messagef(`Resolved version %q for artifact "%s/%s/%s/%s/%s-%s.jar"`, latestVersion, tlsServer.URL+"/ca-releases", groupId, artifactId, latestVersion, artifactId, latestVersion),
							diesourcev1alpha1.MavenArtifactConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
						)
					}),
				certSecret,
			},

			ExpectPatches: []rtesting.PatchRef{
				{
					Group:     "source.apps.tanzu.vmware.com",
					Kind:      "MavenArtifact",
					Namespace: parent.GetNamespace(),
					Name:      parent.GetName(),
					PatchType: types.MergePatchType,
					Patch:     []byte(`{"metadata":{"finalizers":["source.apps.tanzu.vmware.com/finalizer"],"resourceVersion":"999"}}`),
				},
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(parent, scheme, corev1.EventTypeNormal, "FinalizerPatched", "Patched finalizer %q", "source.apps.tanzu.vmware.com/finalizer"),
			},
			ExpectTracks: []rtesting.TrackRequest{
				rtesting.NewTrackRequest(certSecret, parent, scheme),
			},
			ExpectedResult: reconcile.Result{
				RequeueAfter: 5 * time.Minute,
			},
		},
		"cleanup": {
			Request: request,
			Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) (context.Context, error) {
				dir := path.Join(artifactRootDir, "mavenartifact", namespace, name)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return ctx, err
				}
				if _, err := os.Create(path.Join(dir, artifactId+".tar.gz")); err != nil {
					return ctx, err
				}

				return ctx, nil
			},
			CleanUp: func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) error {
				dir := path.Join(artifactRootDir, "mavenartifact", namespace, name)
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					return fmt.Errorf("artifact directory should no longer exist")
				}
				return nil
			},
			GivenObjects: []client.Object{
				parent.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						t := now()
						d.DeletionTimestamp(&t)
						d.Finalizers("source.apps.tanzu.vmware.com/finalizer")
					}),
			},
			ExpectEvents: []rtesting.Event{
				rtesting.NewEvent(parent, scheme, corev1.EventTypeNormal, "FinalizerPatched", "Patched finalizer %q", "source.apps.tanzu.vmware.com/finalizer"),
			},
			ExpectPatches: []rtesting.PatchRef{
				{
					Group:     "source.apps.tanzu.vmware.com",
					Kind:      "MavenArtifact",
					Namespace: parent.GetNamespace(),
					Name:      parent.GetName(),
					PatchType: types.MergePatchType,
					Patch:     []byte(`{"metadata":{"finalizers":null,"resourceVersion":"999"}}`),
				},
			},
		}}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		// drop the artifactRootDir between test cases
		err := os.RemoveAll(artifactRootDir)
		utilruntime.Must(err)

		return controllers.MavenArtifactReconciler(c, artifactRootDir, "artifact.example", now, []controllers.Cert{})
	})
}

func sha1Checksum(name string) (string, error) {
	artifactFile, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer artifactFile.Close()
	checksum := sha1.New()
	if _, err := io.Copy(checksum, artifactFile); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", checksum.Sum(nil)), nil
}

func setCache(file string, content string) error {
	return os.WriteFile(file, []byte(content), os.ModePerm)
}
