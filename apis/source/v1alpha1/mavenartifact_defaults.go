/*
Copyright 2022 VMware, Inc.

This product is licensed to you under the Apache License, V2.0 (the "License"). You may not use this product except in compliance with the License.

This product may include a number of subcomponents with separate copyright notices and license terms. Your use of these subcomponents is subject to the terms and conditions of the subcomponent's license, as noted in the LICENSE file.
*/

package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var _ webhook.Defaulter = &MavenArtifact{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *MavenArtifact) Default() {
	r.Spec.Default()
}

func (s *MavenArtifactSpec) Default() {
	s.Artifact.Default()
	s.Repository.Default()
	if s.Timeout == nil {
		s.Timeout = s.Interval.DeepCopy()
	}
}

func (s *MavenArtifactType) Default() {
	if s.Type == "" {
		s.Type = "jar"
	}
}

func (s *Repository) Default() {
	// nothing to default
}
