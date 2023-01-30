/*
Copyright 2022 VMware, Inc.

This product is licensed to you under the Apache License, V2.0 (the "License"). You may not use this product except in compliance with the License.

This product may include a number of subcomponents with separate copyright notices and license terms. Your use of these subcomponents is subject to the terms and conditions of the subcomponent's license, as noted in the LICENSE file.
*/

package v1alpha1

import (
	diemetav1 "dies.dev/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
