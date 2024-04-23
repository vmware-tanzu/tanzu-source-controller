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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	diemetav1 "reconciler.io/dies/apis/meta/v1"

	sourcev1alpha1 "github.com/vmware-tanzu/tanzu-source-controller/apis/source/v1alpha1"
)

// +die:object=true
type _ = sourcev1alpha1.ImageRepository

// +die
type _ = sourcev1alpha1.ImageRepositorySpec

// +die
type _ = sourcev1alpha1.ImageRepositoryStatus

// +die
type _ = sourcev1alpha1.Artifact

func (d *ImageRepositoryStatusDie) ArtifactDie(fn func(d *ArtifactDie)) *ImageRepositoryStatusDie {
	return d.DieStamp(func(r *sourcev1alpha1.ImageRepositoryStatus) {
		d := ArtifactBlank.
			DieImmutable(false).
			DieFeedPtr(r.Artifact)
		fn(d)
		r.Artifact = d.DieReleasePtr()
	})
}

func (d *ImageRepositoryStatusDie) ConditionsDie(conditions ...*diemetav1.ConditionDie) *ImageRepositoryStatusDie {
	return d.DieStamp(func(r *sourcev1alpha1.ImageRepositoryStatus) {
		r.Conditions = make([]metav1.Condition, len(conditions))
		for i := range conditions {
			r.Conditions[i] = conditions[i].DieRelease()
		}
	})
}

func (d *ImageRepositoryStatusDie) ObservedGeneration(v int64) *ImageRepositoryStatusDie {
	return d.DieStamp(func(r *sourcev1alpha1.ImageRepositoryStatus) {
		r.ObservedGeneration = v
	})
}

var (
	ImageRepositoryConditionArtifactAvailableBlank = diemetav1.ConditionBlank.Type(sourcev1alpha1.ImageRepositoryConditionArtifactAvailable)
	ImageRepositoryConditionImageResolvedBlank     = diemetav1.ConditionBlank.Type(sourcev1alpha1.ImageRepositoryConditionImageResolved)
	ImageRepositoryConditionReadyBlank             = diemetav1.ConditionBlank.Type(sourcev1alpha1.ImageRepositoryConditionReady)
)
