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
