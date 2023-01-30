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
	"fmt"
	"net/url"
	"strings"

	"github.com/vmware-labs/reconciler-runtime/validation"
	kvalidation "k8s.io/apimachinery/pkg/api/validation"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/validate-source-apps-tanzu-vmware-com-v1alpha1-mavenartifact,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1beta1,groups=source.apps.tanzu.vmware.com,resources=mavenartifacts,verbs=create;update,versions=v1alpha1,name=mavenartifacts.source.apps.tanzu.vmware.com

var (
	_ webhook.Validator         = &MavenArtifact{}
	_ validation.FieldValidator = &MavenArtifact{}
)

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *MavenArtifact) ValidateCreate() error {
	return r.Validate().ToAggregate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (c *MavenArtifact) ValidateUpdate(old runtime.Object) error {
	// TODO check for immutable fields
	return c.Validate().ToAggregate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (c *MavenArtifact) ValidateDelete() error {
	return nil
}

func (r *MavenArtifact) Validate() validation.FieldErrors {
	errs := validation.FieldErrors{}
	return errs.Also(r.Spec.Validate().ViaField("spec"))
}

func (s *MavenArtifactSpec) Validate() validation.FieldErrors {
	errs := validation.FieldErrors{}

	errs = errs.Also(s.Artifact.Validate().ViaField("artifact"))
	errs = errs.Also(s.Repository.Validate().ViaField("repository"))

	if s.Interval.Duration <= 0 {
		errs = errs.Also(validation.ErrInvalidValue(s.Interval, "interval"))
	}
	if s.Timeout != nil && s.Timeout.Duration <= 0 {
		errs = errs.Also(validation.ErrInvalidValue(s.Timeout, "timeout"))
	}

	return errs
}

func (s *MavenArtifactType) Validate() validation.FieldErrors {
	errs := validation.FieldErrors{}

	if s.GroupId == "" {
		errs = errs.Also(validation.ErrMissingField("groupId"))
	}
	if s.ArtifactId == "" {
		errs = errs.Also(validation.ErrMissingField("artifactId"))
	}

	if s.Version == "" {
		errs = errs.Also(validation.ErrMissingField("version"))
	} else if strings.HasPrefix(s.Version, "[") ||
		strings.HasPrefix(s.Version, "(") {
		// TODO remove this validation rule when version range is resolvable
		errs = errs.Also(validation.ErrInvalidValue(s.Version, "version"))
	}

	return errs
}

func (s *Repository) Validate() validation.FieldErrors {
	errs := validation.FieldErrors{}

	if s.URL == "" {
		errs = errs.Also(validation.ErrMissingField("url"))
	} else if u, err := url.Parse(s.URL); err != nil {
		errs = errs.Also(validation.ErrInvalidValue(s.URL, "url"))
	} else {
		if u.Scheme != "https" {
			errs = append(errs, field.Invalid(field.NewPath("url"), s.URL, fmt.Sprintf(`Scheme "https" is required; scheme %q is not allowed in repository URL %q`, u.Scheme, s.URL)))
		}
	}

	if n := s.SecretRef.Name; n != "" {
		if out := kvalidation.NameIsDNSLabel(n, false); len(out) != 0 {
			errs = errs.Also(validation.ErrInvalidValue(s.SecretRef.Name, "secretRef.name"))
		}
	}

	return errs
}
