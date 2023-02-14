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
	"fmt"
	"io/ioutil"
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
	btesting "github.com/vmware-tanzu/tanzu-source-controller/testing"
)

func TestImageRepositoryReconciler(t *testing.T) {
	namespace := "test-namespace"
	name := "my-image"
	key := types.NamespacedName{Namespace: namespace, Name: name}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))

	registry, registryHost, err := btesting.NewRegistry()
	utilruntime.Must(err)
	defer registry.Close()

	helloImage := fmt.Sprintf("%s/%s", registryHost, "hello")
	helloDigest := "66201d7a2285b74eef3221c5f548ebcaba03f9891eef305be94f4d51c661d933"
	helloChecksum := "00a04fda65d6d2c7924a2729b8369efbe3f4e978"
	utilruntime.Must(btesting.LoadImage(registry, "fixtures/hello.tar", helloImage))

	artifactRootDir, err := ioutil.TempDir(os.TempDir(), "artifacts.*")
	utilruntime.Must(err)
	defer os.RemoveAll(artifactRootDir)

	now := func() metav1.Time {
		return metav1.Time{
			Time: time.Unix(1, 0),
		}
	}

	parent := diesourcev1alpha1.ImageRepositoryBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		})

	defaultServiceAccount := diecorev1.ServiceAccountBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name("default")
		})

	rts := rtesting.ReconcilerTestSuite{{
		Name: "in sync",
		Key:  key,
		Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) (context.Context, error) {
			dir := path.Join(artifactRootDir, "imagerepository", namespace, name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return ctx, err
			}
			if _, err := os.Create(path.Join(dir, helloDigest+".tar.gz")); err != nil {
				return ctx, err
			}

			return ctx, nil
		},
		GivenObjects: []client.Object{
			parent.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.Finalizers("source.apps.tanzu.vmware.com/finalizer")
				}).
				SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
					d.Image(helloImage)
				}).
				StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
					d.ObservedGeneration(1)
					d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
						d.Revision(fmt.Sprintf("%s:latest@sha256:%s", helloImage, helloDigest))
						d.Path("imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
						d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
						d.Checksum(helloChecksum)
						// use an old timestamp as an indication the resource wasn't updated
						d.LastUpdateTime(metav1.Time{Time: time.Unix(100, 0)})
					})
					d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.ConditionsDie(
						diesourcev1alpha1.ImageRepositoryConditionArtifactAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
						diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
					)
				}),
			defaultServiceAccount,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(defaultServiceAccount, parent, scheme),
		},
	}, {
		Name: "resolve image and pull",
		Key:  key,
		GivenObjects: []client.Object{
			parent.
				SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
					d.Image(helloImage)
				}),
			defaultServiceAccount,
		},
		ExpectStatusUpdates: []client.Object{
			parent.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.Finalizers("source.apps.tanzu.vmware.com/finalizer")
					d.ResourceVersion("1000")
				}).
				SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
					d.Image(helloImage)
				}).
				StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
					d.ObservedGeneration(1)
					d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
						d.Revision(fmt.Sprintf("%s:latest@sha256:%s", helloImage, helloDigest))
						d.Path("imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
						d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
						d.Checksum(helloChecksum)
						d.LastUpdateTime(now())
					})
					d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.ConditionsDie(
						diesourcev1alpha1.ImageRepositoryConditionArtifactAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
						diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
					)
				}),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(parent, scheme, corev1.EventTypeNormal, "FinalizerPatched", "Patched finalizer %q", "source.apps.tanzu.vmware.com/finalizer"),
			rtesting.NewEvent(parent, scheme, corev1.EventTypeNormal, "StatusUpdated", "Updated status"),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(defaultServiceAccount, parent, scheme),
		},
		ExpectPatches: []rtesting.PatchRef{
			{
				Group:     "source.apps.tanzu.vmware.com",
				Kind:      "ImageRepository",
				Namespace: parent.GetNamespace(),
				Name:      parent.GetName(),
				PatchType: types.MergePatchType,
				Patch:     []byte(`{"metadata":{"finalizers":["source.apps.tanzu.vmware.com/finalizer"],"resourceVersion":"999"}}`),
			},
		},
	}, {
		Name: "cleanup",
		Key:  key,
		Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) (context.Context, error) {
			dir := path.Join(artifactRootDir, "imagerepository", namespace, name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return ctx, err
			}
			if _, err := os.Create(path.Join(dir, helloDigest+".tar.gz")); err != nil {
				return ctx, err
			}

			return ctx, nil
		},
		CleanUp: func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) error {
			dir := path.Join(artifactRootDir, "imagerepository", namespace, name)
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
			defaultServiceAccount,
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(parent, scheme, corev1.EventTypeNormal, "FinalizerPatched", "Patched finalizer %q", "source.apps.tanzu.vmware.com/finalizer"),
		},
		ExpectPatches: []rtesting.PatchRef{
			{
				Group:     "source.apps.tanzu.vmware.com",
				Kind:      "ImageRepository",
				Namespace: parent.GetNamespace(),
				Name:      parent.GetName(),
				PatchType: types.MergePatchType,
				Patch:     []byte(`{"metadata":{"finalizers":null,"resourceVersion":"999"}}`),
			},
		},
	}, {
		Name: "requeue interval",
		Key:  key,
		Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.ReconcilerTestCase) (context.Context, error) {
			dir := path.Join(artifactRootDir, "imagerepository", namespace, name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return ctx, err
			}
			if _, err := os.Create(path.Join(dir, helloDigest+".tar.gz")); err != nil {
				return ctx, err
			}

			return ctx, nil
		},
		GivenObjects: []client.Object{
			parent.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.Finalizers("source.apps.tanzu.vmware.com/finalizer")
				}).
				SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
					d.Image(helloImage)
					d.Interval(metav1.Duration{Duration: 5 * time.Minute})
				}).
				StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
					d.ObservedGeneration(1)
					d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
						d.Revision(fmt.Sprintf("%s:latest@sha256:%s", helloImage, helloDigest))
						d.Path("imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
						d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
						d.Checksum(helloChecksum)
						// use an old timestamp as an indication the resource wasn't updated
						d.LastUpdateTime(metav1.Time{Time: time.Unix(100, 0)})
					})
					d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.ConditionsDie(
						diesourcev1alpha1.ImageRepositoryConditionArtifactAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
						diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
						diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
					)
				}),
			defaultServiceAccount,
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(defaultServiceAccount, parent, scheme),
		},
		ExpectedResult: reconcile.Result{
			RequeueAfter: 5 * time.Minute,
		},
	}, {
		Name: "image pull error",
		Key:  key,
		GivenObjects: []client.Object{
			parent.
				MetadataDie(func(d *diemetav1.ObjectMetaDie) {
					d.Finalizers("source.apps.tanzu.vmware.com/finalizer")
				}).
				SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
					d.Image(fmt.Sprintf("%s/this/does/not/exist:latest", registryHost))
				}),
			defaultServiceAccount,
		},
		ExpectStatusUpdates: []client.Object{
			parent.
				SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
					d.Image(fmt.Sprintf("%s/this/does/not/exist:latest", registryHost))
				}).
				StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
					d.ObservedGeneration(1)
					d.ConditionsDie(
						diesourcev1alpha1.ImageRepositoryConditionArtifactAvailableBlank.Status(metav1.ConditionUnknown).Reason("Initializing"),
						diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unable to resolve image with tag "%s/this/does/not/exist:latest" to a digest: HEAD https://%s/v2/this/does/not/exist/manifests/latest: unexpected status code 404 Not Found (HEAD responses have no body, use GET for details)`, registryHost, registryHost),
						diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").
							Messagef(`Unable to resolve image with tag "%s/this/does/not/exist:latest" to a digest: HEAD https://%s/v2/this/does/not/exist/manifests/latest: unexpected status code 404 Not Found (HEAD responses have no body, use GET for details)`, registryHost, registryHost),
					)
				}),
		},
		ExpectEvents: []rtesting.Event{
			rtesting.NewEvent(parent, scheme, corev1.EventTypeNormal, "StatusUpdated", `Updated status`),
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(defaultServiceAccount, parent, scheme),
		},
	}}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.ReconcilerTestCase, c reconcilers.Config) reconcile.Reconciler {
		// drop the artifactRootDir between test cases
		err := os.RemoveAll(artifactRootDir)
		utilruntime.Must(err)

		certs := []controllers.Cert{
			{Certificate: registry.Certificate()},
		}
		return controllers.ImageRepositoryReconciler(c, artifactRootDir, "artifact.example", now, certs)
	})
}

func TestImageRepositoryImagePullSecretsSyncReconciler(t *testing.T) {
	namespace := "test-namespace"
	name := "my-image"

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))

	parent := diesourcev1alpha1.ImageRepositoryBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		}).
		StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
			d.ObservedGeneration(1)
		})

	defaultServiceAccount := diecorev1.ServiceAccountBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name("default")
		})
	customServiceAccount := diecorev1.ServiceAccountBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name("custom-sa")
		})
	imagePullSecret := diecorev1.SecretBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name("pull-secret")
		})

	rts := rtesting.SubReconcilerTestSuite{{
		Name:     "default service account",
		Resource: parent,
		GivenObjects: []client.Object{
			defaultServiceAccount.
				ImagePullSecretsDie(
					diecorev1.LocalObjectReferenceBlank.Name("pull-secret"),
				),
			imagePullSecret,
		},
		ExpectResource: parent,
		ExpectStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImagePullSecretsStashKey: []corev1.Secret{
				imagePullSecret.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.ResourceVersion("999")
					}).
					DieRelease(),
			},
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(defaultServiceAccount, parent, scheme),
			rtesting.NewTrackRequest(imagePullSecret, parent, scheme),
		},
	}, {
		Name: "custom service account",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.ServiceAccountName("custom-sa")
			}),
		GivenObjects: []client.Object{
			customServiceAccount.
				ImagePullSecretsDie(
					diecorev1.LocalObjectReferenceBlank.Name("pull-secret"),
				),
			imagePullSecret,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.ServiceAccountName("custom-sa")
			}),
		ExpectStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImagePullSecretsStashKey: []corev1.Secret{
				imagePullSecret.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.ResourceVersion("999")
					}).
					DieRelease(),
			},
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(customServiceAccount, parent, scheme),
			rtesting.NewTrackRequest(imagePullSecret, parent, scheme),
		},
	}, {
		Name: "custom image pull secrets",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.ImagePullSecrets(
					corev1.LocalObjectReference{Name: "pull-secret"},
				)
			}),
		GivenObjects: []client.Object{
			defaultServiceAccount,
			imagePullSecret,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.ImagePullSecrets(
					corev1.LocalObjectReference{Name: "pull-secret"},
				)
			}),
		ExpectStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImagePullSecretsStashKey: []corev1.Secret{
				imagePullSecret.
					MetadataDie(func(d *diemetav1.ObjectMetaDie) {
						d.ResourceVersion("999")
					}).
					DieRelease(),
			},
		},
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(defaultServiceAccount, parent, scheme),
			rtesting.NewTrackRequest(imagePullSecret, parent, scheme),
		},
	}, {
		Name:         "service account not found",
		Resource:     parent,
		GivenObjects: []client.Object{},
		ExpectResource: parent.
			StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
				d.ConditionsDie(
					diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionFalse).Reason("ServiceAccountMissing").Message(`ServiceAccount "default" not found in namespace "test-namespace"`),
					diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionFalse).Reason("ServiceAccountMissing").Message(`ServiceAccount "default" not found in namespace "test-namespace"`),
				)
			}),
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(defaultServiceAccount, parent, scheme),
		},
	}, {
		Name:     "error fetching service account",
		Resource: parent,
		GivenObjects: []client.Object{
			defaultServiceAccount.
				ImagePullSecrets(
					corev1.LocalObjectReference{Name: "pull-secret"},
				),
			imagePullSecret,
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "ServiceAccount"),
		},
		ShouldErr:      true,
		ExpectResource: parent,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(defaultServiceAccount, parent, scheme),
		},
	}, {
		Name:     "secret not found",
		Resource: parent,
		GivenObjects: []client.Object{
			defaultServiceAccount.
				ImagePullSecrets(
					corev1.LocalObjectReference{Name: "pull-secret"},
				),
		},
		ExpectResource: parent.
			StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
				d.ConditionsDie(
					diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionFalse).Reason("SecretMissing").Message(`Secret "pull-secret" not found in namespace "test-namespace"`),
					diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionFalse).Reason("SecretMissing").Message(`Secret "pull-secret" not found in namespace "test-namespace"`),
				)
			}),
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(defaultServiceAccount, parent, scheme),
			rtesting.NewTrackRequest(imagePullSecret, parent, scheme),
		},
	}, {
		Name:     "error fetching secret",
		Resource: parent,
		GivenObjects: []client.Object{
			defaultServiceAccount.
				ImagePullSecrets(
					corev1.LocalObjectReference{Name: "pull-secret"},
				),
			imagePullSecret,
		},
		WithReactors: []rtesting.ReactionFunc{
			rtesting.InduceFailure("get", "Secret"),
		},
		ShouldErr:      true,
		ExpectResource: parent,
		ExpectTracks: []rtesting.TrackRequest{
			rtesting.NewTrackRequest(defaultServiceAccount, parent, scheme),
			rtesting.NewTrackRequest(imagePullSecret, parent, scheme),
		},
	}}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase, c reconcilers.Config) reconcilers.SubReconciler {
		return controllers.ImageRepositoryImagePullSecretsSyncReconciler()
	})
}

func TestImageRepositoryImageDigestSyncReconciler(t *testing.T) {
	namespace := "test-namespace"
	name := "my-image"
	digestedImage := "registry.example/image@sha256:0000000000000000000000000000000000000000000000000000000000000000"

	registry, registryHost, err := btesting.NewRegistry()
	utilruntime.Must(err)
	defer registry.Close()

	helloImage := fmt.Sprintf("%s/%s", registryHost, "hello")
	helloDigest := "66201d7a2285b74eef3221c5f548ebcaba03f9891eef305be94f4d51c661d933"
	// helloChecksum := "d0ab7a6a9af1c8d60aa7d0fc1392853c9b7ccade"
	utilruntime.Must(btesting.LoadImage(registry, "fixtures/hello.tar", helloImage))

	taggedImage := fmt.Sprintf("%s:latest", helloImage)
	taggedImageDigest := fmt.Sprintf("%s@sha256:%s", taggedImage, helloDigest)

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))

	parent := diesourcev1alpha1.ImageRepositoryBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		}).
		StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
			d.ObservedGeneration(1)
		})

	rts := rtesting.SubReconcilerTestSuite{{
		Name: "propagate digested images",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(digestedImage)
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.HttpRoundTripperStashKey: registry.Client().Transport,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(digestedImage)
			}),
		ExpectStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey: digestedImage,
		},
	}, {
		Name: "resolve tagged image",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(taggedImage)
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImagePullSecretsStashKey: []corev1.Secret{},
			controllers.HttpRoundTripperStashKey: registry.Client().Transport,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(taggedImage)
			}),
		ExpectStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey: taggedImageDigest,
		},
	}, {
		Name: "skip when pull secrets are missing",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(taggedImage)
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImagePullSecretsStashKey: nil,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(taggedImage)
			}),
		ExpectStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey: nil,
		},
	}, {
		Name: "malformed repository",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(registryHost + "/a")
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImagePullSecretsStashKey: []corev1.Secret{},
			controllers.HttpRoundTripperStashKey: registry.Client().Transport,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(registryHost + "/a")
			}).
			StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
				d.ConditionsDie(
					diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionFalse).Reason("MalformedRepository").Message(`Image name "`+registryHost+`/a" failed validation: repository must be between 2 and 255 characters in length: a`),
					diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionFalse).Reason("MalformedRepository").Message(`Image name "`+registryHost+`/a" failed validation: repository must be between 2 and 255 characters in length: a`),
				)
			}),
		ExpectStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey: nil,
		},
	}, {
		Name: "remote error",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(registryHost + "/missing")
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImagePullSecretsStashKey: []corev1.Secret{},
			controllers.HttpRoundTripperStashKey: registry.Client().Transport,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(registryHost + "/missing")
			}).
			StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
				d.ConditionsDie(
					// TODO(scothis) can we better handle a forbidden or not found error?
					diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionFalse).Reason("RemoteError").Message(`Unable to resolve image with tag "`+registryHost+`/missing" to a digest: HEAD https://`+registryHost+`/v2/missing/manifests/latest: unexpected status code 404 Not Found (HEAD responses have no body, use GET for details)`),
					diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionFalse).Reason("RemoteError").Message(`Unable to resolve image with tag "`+registryHost+`/missing" to a digest: HEAD https://`+registryHost+`/v2/missing/manifests/latest: unexpected status code 404 Not Found (HEAD responses have no body, use GET for details)`),
				)
			}),
		ExpectStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey: nil,
		},
	}}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase, c reconcilers.Config) reconcilers.SubReconciler {
		return controllers.ImageRepositoryImageDigestSyncReconciler()
	})
}

func TestImageRepositoryPullImageSyncReconciler(t *testing.T) {
	namespace := "test-namespace"
	name := "my-image"

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))

	registry, registryHost, err := btesting.NewRegistry()
	utilruntime.Must(err)
	defer registry.Close()

	helloImage := fmt.Sprintf("%s/%s", registryHost, "hello")
	utilruntime.Must(btesting.LoadImage(registry, "fixtures/hello.tar", helloImage))
	helloDigest := "66201d7a2285b74eef3221c5f548ebcaba03f9891eef305be94f4d51c661d933"
	helloChecksum := "00a04fda65d6d2c7924a2729b8369efbe3f4e978"
	image := fmt.Sprintf("%s@sha256:%s", helloImage, helloDigest)

	artifactRootDir, err := ioutil.TempDir(os.TempDir(), "artifacts.*")
	utilruntime.Must(err)
	defer os.RemoveAll(artifactRootDir)

	now := func() metav1.Time {
		return metav1.Time{
			Time: time.Unix(1, 0),
		}
	}

	parent := diesourcev1alpha1.ImageRepositoryBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		}).
		StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
			d.ObservedGeneration(1)
		})

	rts := rtesting.SubReconcilerTestSuite{{
		Name: "pull image",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey:         image,
			controllers.ImagePullSecretsStashKey: []corev1.Secret{},
			controllers.HttpRoundTripperStashKey: registry.Client().Transport,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
			}).
			StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
				d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
					d.Revision(image)
					d.Path("imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.Checksum(helloChecksum)
					d.LastUpdateTime(now())
				})
				d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
				d.ConditionsDie(
					diesourcev1alpha1.ImageRepositoryConditionArtifactAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
					diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
					diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
				)
			}),
		CleanUp: func(t *testing.T, ctx context.Context, tc *rtesting.SubReconcilerTestCase) error {
			// check that the file exists
			artifact := path.Join(artifactRootDir, "imagerepository", namespace, name, helloDigest+".tar.gz")
			if _, err := os.Stat(artifact); err != nil {
				t.Errorf("artifact expected to exist %q", artifact)
			}
			return nil
		},
	}, {
		Name: "skip existing image",
		Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.SubReconcilerTestCase) (context.Context, error) {
			dir := path.Join(artifactRootDir, "imagerepository", namespace, name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return ctx, err
			}
			if _, err := os.Create(path.Join(dir, helloDigest+".tar.gz")); err != nil {
				return ctx, err
			}

			return ctx, nil
		},
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
			}).
			StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
				d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
					d.Revision(image)
					d.Path("imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.Checksum(helloChecksum)
					// use an old timestamp as an indication the resource wasn't updated
					d.LastUpdateTime(metav1.Time{Time: time.Unix(100, 0)})
				})
				d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey:         image,
			controllers.ImagePullSecretsStashKey: []corev1.Secret{},
			controllers.HttpRoundTripperStashKey: registry.Client().Transport,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
			}).
			StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
				d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
					d.Revision(image)
					d.Path("imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.Checksum(helloChecksum)
					// use an old timestamp as an indication the resource wasn't updated
					d.LastUpdateTime(metav1.Time{Time: time.Unix(100, 0)})
				})
				d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
				d.ConditionsDie(
					diesourcev1alpha1.ImageRepositoryConditionArtifactAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
					diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
					diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
				)
			}),
	}, {
		Name: "update if host changes",
		Prepare: func(t *testing.T, ctx context.Context, tc *rtesting.SubReconcilerTestCase) (context.Context, error) {
			dir := path.Join(artifactRootDir, "imagerepository", namespace, name)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return ctx, err
			}
			if _, err := os.Create(path.Join(dir, helloDigest+".tar.gz")); err != nil {
				return ctx, err
			}

			return ctx, nil
		},
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
			}).
			StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
				d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
					d.Revision(image)
					d.Path("imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.URL("http://localhost/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.Checksum(helloChecksum)
					// use an old timestamp as an indication the resource wasn't updated
					d.LastUpdateTime(metav1.Time{Time: time.Unix(100, 0)})
				})
				d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey:         image,
			controllers.ImagePullSecretsStashKey: []corev1.Secret{},
			controllers.HttpRoundTripperStashKey: registry.Client().Transport,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
			}).
			StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
				d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
					d.Revision(image)
					d.Path("imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.Checksum(helloChecksum)
					d.LastUpdateTime(now())
				})
				d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
				d.ConditionsDie(
					diesourcev1alpha1.ImageRepositoryConditionArtifactAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
					diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
					diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
				)
			}),
	}, {
		Name: "missing image",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey:         nil,
			controllers.ImagePullSecretsStashKey: []corev1.Secret{},
			controllers.HttpRoundTripperStashKey: registry.Client().Transport,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
			}),
	}, {
		Name: "missing pull secrets",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey:         image,
			controllers.ImagePullSecretsStashKey: nil,
			controllers.HttpRoundTripperStashKey: registry.Client().Transport,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
			}),
	}}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase, c reconcilers.Config) reconcilers.SubReconciler {
		// drop the artifactRootDir between test cases
		err := os.RemoveAll(artifactRootDir)
		utilruntime.Must(err)

		return controllers.ImageRepositoryPullImageSyncReconciler(artifactRootDir, "artifact.example", now)
	})
}

func TestImageRepositoryPullImageSyncReconcilerWithAuth(t *testing.T) {
	namespace := "test-namespace"
	name := "my-image"
	reg_user := "user"
	reg_pwd := "pass"

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1alpha1.AddToScheme(scheme))

	registry, registryHost, err := btesting.NewAuthRegistryServer(reg_user, reg_pwd)
	utilruntime.Must(err)
	defer registry.Close()

	helloImage := fmt.Sprintf("%s/%s", registryHost, "hello")
	utilruntime.Must(btesting.LoadImageWithAuth(registry, "fixtures/hello.tar", helloImage, reg_user, reg_pwd))
	helloDigest := "66201d7a2285b74eef3221c5f548ebcaba03f9891eef305be94f4d51c661d933"
	helloChecksum := "00a04fda65d6d2c7924a2729b8369efbe3f4e978"
	image := fmt.Sprintf("%s@sha256:%s", helloImage, helloDigest)

	var pullsecrets = []corev1.Secret{}
	dcj := fmt.Sprintf(`{"auths": {%q: {"username": %q, "password": %q}}}`, registryHost, reg_user, reg_pwd)
	secret := corev1.Secret{
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: []byte(dcj),
		},
	}
	secret.Namespace = namespace
	secret.Name = "docker"

	var secretRef = []corev1.LocalObjectReference{
		{
			Name: "docker",
		},
	}
	pullsecrets = append(pullsecrets, secret)

	artifactRootDir, err := ioutil.TempDir(os.TempDir(), "artifacts.*")
	utilruntime.Must(err)
	defer os.RemoveAll(artifactRootDir)

	now := func() metav1.Time {
		return metav1.Time{
			Time: time.Unix(1, 0),
		}
	}

	parent := diesourcev1alpha1.ImageRepositoryBlank.
		MetadataDie(func(d *diemetav1.ObjectMetaDie) {
			d.Namespace(namespace)
			d.Name(name)
			d.Generation(1)
		}).
		StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
			d.ObservedGeneration(1)
		})

	rts := rtesting.SubReconcilerTestSuite{{
		Name: "pull image from authenticated repository",
		Resource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
				d.ImagePullSecrets(secretRef...)
			}),
		GivenStashedValues: map[reconcilers.StashKey]interface{}{
			controllers.ImageRefStashKey:         image,
			controllers.ImagePullSecretsStashKey: pullsecrets,
			controllers.HttpRoundTripperStashKey: registry.Client().Transport,
		},
		ExpectResource: parent.
			SpecDie(func(d *diesourcev1alpha1.ImageRepositorySpecDie) {
				d.Image(image)
				d.ImagePullSecrets(secretRef...)
			}).
			StatusDie(func(d *diesourcev1alpha1.ImageRepositoryStatusDie) {
				d.ArtifactDie(func(d *diesourcev1alpha1.ArtifactDie) {
					d.Revision(image)
					d.Path("imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
					d.Checksum(helloChecksum)
					d.LastUpdateTime(now())
				})
				d.URL("http://artifact.example/imagerepository/test-namespace/my-image/" + helloDigest + ".tar.gz")
				d.ConditionsDie(
					diesourcev1alpha1.ImageRepositoryConditionArtifactAvailableBlank.Status(metav1.ConditionTrue).Reason("Available"),
					diesourcev1alpha1.ImageRepositoryConditionImageResolvedBlank.Status(metav1.ConditionTrue).Reason("Resolved"),
					diesourcev1alpha1.ImageRepositoryConditionReadyBlank.Status(metav1.ConditionTrue).Reason("Ready"),
				)
			}),
		CleanUp: func(t *testing.T, ctx context.Context, tc *rtesting.SubReconcilerTestCase) error {
			// check that the file exists
			artifact := path.Join(artifactRootDir, "imagerepository", namespace, name, helloDigest+".tar.gz")
			if _, err := os.Stat(artifact); err != nil {
				t.Errorf("artifact expected to exist %q", artifact)
			}
			return nil
		},
	}}

	rts.Run(t, scheme, func(t *testing.T, rtc *rtesting.SubReconcilerTestCase, c reconcilers.Config) reconcilers.SubReconciler {
		// drop the artifactRootDir between test cases
		err := os.RemoveAll(artifactRootDir)
		utilruntime.Must(err)

		return controllers.ImageRepositoryPullImageSyncReconciler(artifactRootDir, "artifact.example", now)
	})
}
