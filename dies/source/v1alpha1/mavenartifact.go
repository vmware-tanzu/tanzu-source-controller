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
type _ = sourcev1alpha1.MavenArtifact

// +die
type _ = sourcev1alpha1.MavenArtifactSpec

// +die
type _ = sourcev1alpha1.MavenArtifactStatus

// +die
type _ = sourcev1alpha1.MavenArtifactType

// +die
type _ = sourcev1alpha1.Repository

func (d *MavenArtifactSpecDie) RepositoryDie(fn func(d *RepositoryDie)) *MavenArtifactSpecDie {
	repository := &sourcev1alpha1.Repository{}
	return d.DieStamp(func(r *sourcev1alpha1.MavenArtifactSpec) {
		d := RepositoryBlank.
			DieImmutable(false).
			DieFeedPtr(repository)
		fn(d)
		r.Repository = *d.DieReleasePtr()
	})
}

func (d *MavenArtifactSpecDie) MavenArtifactDie(fn func(d *MavenArtifactTypeDie)) *MavenArtifactSpecDie {
	mavenArtifact := &sourcev1alpha1.MavenArtifactType{}
	return d.DieStamp(func(r *sourcev1alpha1.MavenArtifactSpec) {
		d := MavenArtifactTypeBlank.
			DieImmutable(false).
			DieFeedPtr(mavenArtifact)
		fn(d)
		r.Artifact = *d.DieReleasePtr()
	})
}

func (d *MavenArtifactStatusDie) ArtifactDie(fn func(d *ArtifactDie)) *MavenArtifactStatusDie {
	return d.DieStamp(func(r *sourcev1alpha1.MavenArtifactStatus) {
		d := ArtifactBlank.
			DieImmutable(false).
			DieFeedPtr(r.Artifact)
		fn(d)
		r.Artifact = d.DieReleasePtr()
	})
}

func (d *MavenArtifactStatusDie) ConditionsDie(conditions ...*diemetav1.ConditionDie) *MavenArtifactStatusDie {
	return d.DieStamp(func(r *sourcev1alpha1.MavenArtifactStatus) {
		r.Conditions = make([]metav1.Condition, len(conditions))
		for i := range conditions {
			r.Conditions[i] = conditions[i].DieRelease()
		}
	})
}

func (d *MavenArtifactStatusDie) ObservedGeneration(v int64) *MavenArtifactStatusDie {
	return d.DieStamp(func(r *sourcev1alpha1.MavenArtifactStatus) {
		r.ObservedGeneration = v
	})
}

var (
	MavenArtifactConditionAvailableBlank       = diemetav1.ConditionBlank.Type(sourcev1alpha1.MavenArtifactConditionArtifactAvailable)
	MavenArtifactConditionVersionResolvedBlank = diemetav1.ConditionBlank.Type(sourcev1alpha1.MavenArtifactConditionArtifactResolved)
	MavenArtifactConditionReadyBlank           = diemetav1.ConditionBlank.Type(sourcev1alpha1.MavenArtifactConditionReady)
)
