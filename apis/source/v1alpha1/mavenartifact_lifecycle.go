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
	MavenArtifactLabelKey = GroupVersion.Group + "/maven-artifact"
)

const (
	MavenArtifactConditionReady             = apis.ConditionReady
	MavenArtifactConditionArtifactResolved  = "ArtifactVersionResolved"
	MavenArtifactConditionArtifactAvailable = "ArtifactAvailable"
)

var MavenArtifactCondSet = apis.NewLivingConditionSet(
	MavenArtifactConditionArtifactResolved,
	MavenArtifactConditionArtifactAvailable,
)

func (s *MavenArtifact) ManageConditions() apis.ConditionManager {
	return s.GetConditionSet().Manage(s.GetConditionsAccessor())
}

func (s *MavenArtifact) GetConditionsAccessor() apis.ConditionsAccessor {
	return &s.Status
}

func (s *MavenArtifact) GetConditionSet() apis.ConditionSet {
	return MavenArtifactCondSet
}

func (s *MavenArtifactStatus) InitializeConditions() {
	MavenArtifactCondSet.Manage(s).InitializeConditions()
}
