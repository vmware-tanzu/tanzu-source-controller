/*
Copyright 2022 VMware, Inc.

This product is licensed to you under the Apache License, V2.0 (the "License"). You may not use this product except in compliance with the License.

This product may include a number of subcomponents with separate copyright notices and license terms. Your use of these subcomponents is subject to the terms and conditions of the subcomponent's license, as noted in the LICENSE file.
*/

package v1alpha1

import (
	"github.com/vmware-labs/reconciler-runtime/apis"
)

var (
	ImageRepositoryLabelKey = GroupVersion.Group + "/image-repository"
)

const (
	ImageRepositoryConditionReady             = apis.ConditionReady
	ImageRepositoryConditionImageResolved     = "ImageResolved"
	ImageRepositoryConditionArtifactAvailable = "ArtifactAvailable"
)

var imagerepositoryCondSet = apis.NewLivingConditionSet(
	ImageRepositoryConditionImageResolved,
	ImageRepositoryConditionArtifactAvailable,
)

func (s *ImageRepository) ManageConditions() apis.ConditionManager {
	return s.GetConditionSet().Manage(s.GetConditionsAccessor())
}

func (s *ImageRepository) GetConditionsAccessor() apis.ConditionsAccessor {
	return &s.Status
}

func (s *ImageRepository) GetConditionSet() apis.ConditionSet {
	return imagerepositoryCondSet
}

func (s *ImageRepositoryStatus) InitializeConditions() {
	// reset conditions
	s.Conditions = nil
	imagerepositoryCondSet.Manage(s).InitializeConditions()
}
