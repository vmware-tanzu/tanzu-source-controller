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
