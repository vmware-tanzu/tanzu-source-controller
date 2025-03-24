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
	"context"
	"fmt"
	"net/url"
	"strings"

	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-source-apps-tanzu-vmware-com-v1alpha1-mavenartifact,mutating=false,failurePolicy=fail,sideEffects=none,admissionReviewVersions=v1beta1,groups=source.apps.tanzu.vmware.com,resources=mavenartifacts,verbs=create;update,versions=v1alpha1,name=mavenartifacts.source.apps.tanzu.vmware.com

type MavenArtifactValidator struct{}

var _ webhook.CustomValidator = &MavenArtifactValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *MavenArtifactValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	mavenArtifact, ok := obj.(*MavenArtifact)
	if !ok {
		return nil, fmt.Errorf("expected a MavenArtifact but got a %T", obj)
	}
	return nil, mavenArtifact.validate().ToAggregate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *MavenArtifactValidator) ValidateUpdate(ctx context.Context, old runtime.Object, new runtime.Object) (admission.Warnings, error) {
	mavenArtifact, ok := new.(*MavenArtifact)
	if !ok {
		return nil, fmt.Errorf("expected a MavenArtifact but got a %T", new)
	}
	// TODO check for immutable fields
	return nil, mavenArtifact.validate().ToAggregate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *MavenArtifactValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (c *MavenArtifact) validate() field.ErrorList {
	errs := field.ErrorList{}

	errs = append(errs, c.Spec.validate(field.NewPath("spec"))...)

	return errs
}

func (s *MavenArtifactSpec) validate(fldPath *field.Path) field.ErrorList {
	errs := field.ErrorList{}

	errs = append(errs, s.Artifact.validate(fldPath.Child("artifact"))...)
	errs = append(errs, s.Repository.validate(fldPath.Child("repository"))...)

	if s.Interval.Duration <= 0 {
		errs = append(errs, field.Invalid(fldPath.Child("interval"), s.Interval, ""))
	}
	if s.Timeout != nil && s.Timeout.Duration <= 0 {
		errs = append(errs, field.Invalid(fldPath.Child("timeout"), s.Timeout, ""))
	}

	return errs
}

func (s *MavenArtifactType) validate(fldPath *field.Path) field.ErrorList {
	errs := field.ErrorList{}

	if s.GroupId == "" {
		errs = append(errs, field.Required(fldPath.Child("groupId"), ""))
	}
	if s.ArtifactId == "" {
		errs = append(errs, field.Required(fldPath.Child("artifactId"), ""))
	}

	if s.Version == "" {
		errs = append(errs, field.Required(fldPath.Child("version"), ""))
	} else if strings.HasPrefix(s.Version, "[") ||
		strings.HasPrefix(s.Version, "(") {
		// TODO remove this validation rule when version range is resolvable
		errs = append(errs, field.Invalid(fldPath.Child("version"), s.Version, ""))
	}

	return errs
}

func (s *Repository) validate(fldPath *field.Path) field.ErrorList {
	errs := field.ErrorList{}

	if s.URL == "" {
		errs = append(errs, field.Required(fldPath.Child("url"), ""))
	} else if u, err := url.Parse(s.URL); err != nil {
		errs = append(errs, field.Invalid(fldPath.Child("url"), s.URL, ""))
	} else {
		if u.Scheme != "https" {
			errs = append(errs, field.Invalid(fldPath.Child("url"), s.URL, fmt.Sprintf(`Scheme "https" is required; scheme %q is not allowed in repository URL %q`, u.Scheme, s.URL)))
		}
	}

	if n := s.SecretRef.Name; n != "" {
		if out := validation.NameIsDNSLabel(n, false); len(out) != 0 {
			errs = append(errs, field.Invalid(fldPath.Child("secretRef", "name"), s.SecretRef.Name, ""))
		}
	}

	return errs
}
