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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/vmware-labs/reconciler-runtime/validation"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestMavenArtifactDefault(t *testing.T) {
	tests := []struct {
		name     string
		seed     *MavenArtifact
		expected *MavenArtifact
	}{
		{
			name: "empty",
			seed: &MavenArtifact{},
			expected: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						Type: "jar",
					},
					Timeout: &metav1.Duration{
						Duration: 0,
					},
				},
			},
		},
		{
			name: "war artifact type",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						Type: "war",
					},
				},
			},
			expected: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						Type: "war",
					},
					Timeout: &metav1.Duration{
						Duration: 0,
					},
				},
			},
		},
		{
			name: "1 minute interval",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Interval: metav1.Duration{
						Duration: time.Minute,
					},
				},
			},
			expected: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						Type: "jar",
					},
					Interval: metav1.Duration{
						Duration: time.Minute,
					},
					Timeout: &metav1.Duration{
						Duration: time.Minute,
					},
				},
			},
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			actual := c.seed.DeepCopy()
			actual.Default()
			if diff := cmp.Diff(c.expected, actual); diff != "" {
				t.Errorf("(-expected, +actual): %s", diff)
			}
		})
	}
}

func TestMavenArtifactValidate(t *testing.T) {
	tests := []struct {
		name     string
		seed     *MavenArtifact
		expected validation.FieldErrors
	}{
		{
			name: "empty is not valid",
			seed: &MavenArtifact{},
			expected: validation.FieldErrors{
				field.Required(field.NewPath("spec", "artifact", "groupId"), ""),
				field.Required(field.NewPath("spec", "artifact", "artifactId"), ""),
				field.Required(field.NewPath("spec", "artifact", "version"), ""),
				field.Required(field.NewPath("spec", "repository", "url"), ""),
				field.Invalid(field.NewPath("spec", "interval"), metav1.Duration{}, ""),
			},
		},
		{
			name: "minimal valid",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						GroupId:    "com.example",
						ArtifactId: "my-artifact",
						Version:    "1.0.0",
					},
					Repository: Repository{
						URL: "https://repo1.maven.org/maven2",
					},
					Interval: metav1.Duration{Duration: time.Minute},
				},
			},
			expected: validation.FieldErrors{},
		},
		{
			name: "fully valid",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						GroupId:    "com.example",
						ArtifactId: "my-artifact",
						Version:    "1.0.0",
						Type:       "jar",
						Classifier: "source",
					},
					Repository: Repository{
						URL: "https://repo1.maven.org/maven2",
						SecretRef: v1.LocalObjectReference{
							Name: "my-creds",
						},
					},
					Interval: metav1.Duration{Duration: time.Minute},
					Timeout:  &metav1.Duration{Duration: time.Minute},
				},
			},
			expected: validation.FieldErrors{},
		},
		{
			name: "invalid version LATEST",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						GroupId:    "com.example",
						ArtifactId: "my-artifact",
						Version:    "LATEST",
					},
					Repository: Repository{
						URL: "https://repo1.maven.org/maven2",
					},
					Interval: metav1.Duration{Duration: time.Minute},
				},
			},
			expected: validation.FieldErrors{},
		},
		{
			name: "valid version SNAPSHOT",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						GroupId:    "com.example",
						ArtifactId: "my-artifact",
						Version:    "1.0.0-BUILD-SNAPSHOT",
					},
					Repository: Repository{
						URL: "https://repo1.maven.org/maven2",
					},
					Interval: metav1.Duration{Duration: time.Minute},
				},
			},
			expected: validation.FieldErrors{},
		},
		{
			// TODO this is valid, but not presently supported
			name: "invalid version range",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						GroupId:    "com.example",
						ArtifactId: "my-artifact",
						Version:    "[1.0,2.0)",
					},
					Repository: Repository{
						URL: "https://repo1.maven.org/maven2",
					},
					Interval: metav1.Duration{Duration: time.Minute},
				},
			},
			expected: validation.FieldErrors{
				field.Invalid(field.NewPath("spec", "artifact", "version"), "[1.0,2.0)", ""),
			},
		},
		{
			name: "invalid malformed url",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						GroupId:    "com.example",
						ArtifactId: "my-artifact",
						Version:    "1.0.0",
					},
					Repository: Repository{
						URL: "https://repo1.maven.org:badport/maven2",
					},
					Interval: metav1.Duration{Duration: time.Minute},
				},
			},
			expected: validation.FieldErrors{
				field.Invalid(field.NewPath("spec", "repository", "url"), "https://repo1.maven.org:badport/maven2", ""),
			},
		},
		{
			name: "invalid http url",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						GroupId:    "com.example",
						ArtifactId: "my-artifact",
						Version:    "1.0.0",
					},
					Repository: Repository{
						URL: "http://repo1.maven.org/maven2",
					},
					Interval: metav1.Duration{Duration: time.Minute},
				},
			},
			expected: validation.FieldErrors{
				field.Invalid(field.NewPath("spec", "repository", "url"), "http://repo1.maven.org/maven2", `Scheme "https" is required; scheme "http" is not allowed in repository URL "http://repo1.maven.org/maven2"`),
			},
		},
		{
			name: "invalid secret ref",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						GroupId:    "com.example",
						ArtifactId: "my-artifact",
						Version:    "1.0.0",
					},
					Repository: Repository{
						URL: "https://repo1.maven.org/maven2",
						SecretRef: v1.LocalObjectReference{
							Name: "-",
						},
					},
					Interval: metav1.Duration{Duration: time.Minute},
				},
			},
			expected: validation.FieldErrors{
				field.Invalid(field.NewPath("spec", "repository", "secretRef", "name"), "-", ""),
			},
		},
		{
			name: "invalid duration",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						GroupId:    "com.example",
						ArtifactId: "my-artifact",
						Version:    "1.0.0",
					},
					Repository: Repository{
						URL: "https://repo1.maven.org/maven2",
					},
					Interval: metav1.Duration{Duration: 0},
				},
			},
			expected: validation.FieldErrors{
				field.Invalid(field.NewPath("spec", "interval"), metav1.Duration{}, ""),
			},
		},
		{
			name: "invalid timeout",
			seed: &MavenArtifact{
				Spec: MavenArtifactSpec{
					Artifact: MavenArtifactType{
						GroupId:    "com.example",
						ArtifactId: "my-artifact",
						Version:    "1.0.0",
					},
					Repository: Repository{
						URL: "https://repo1.maven.org/maven2",
					},
					Interval: metav1.Duration{Duration: time.Minute},
					Timeout:  &metav1.Duration{Duration: 0},
				},
			},
			expected: validation.FieldErrors{
				field.Invalid(field.NewPath("spec", "timeout"), &metav1.Duration{}, ""),
			},
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			if diff := cmp.Diff(c.expected, c.seed.Validate()); diff != "" {
				t.Errorf("validate (-expected, +actual): %s", diff)
			}

			expectedErr := c.expected.ToAggregate()
			if diff := cmp.Diff(expectedErr, c.seed.ValidateCreate()); diff != "" {
				t.Errorf("ValidateCreate (-expected, +actual): %s", diff)
			}
			if diff := cmp.Diff(expectedErr, c.seed.ValidateUpdate(c.seed.DeepCopy())); diff != "" {
				t.Errorf("ValidateCreate (-expected, +actual): %s", diff)
			}
			if diff := cmp.Diff(nil, c.seed.ValidateDelete()); diff != "" {
				t.Errorf("ValidateDelete (-expected, +actual): %s", diff)
			}
		})
	}
}
