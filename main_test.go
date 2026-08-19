/*
Copyright 2026 VMware, Inc.

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

package main

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestValidateMaxArtifactSize(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		wantErr     bool
		errContains string
	}{
		{name: "500Mi is valid", value: "500Mi", wantErr: false},
		{name: "0 is valid", value: "0", wantErr: false},
		{name: "-1 is rejected", value: "-1", wantErr: true, errContains: "negative"},
		{name: "-500Mi is rejected", value: "-500Mi", wantErr: true, errContains: "negative"},
		{name: "10E overflows int64 and is rejected", value: "10E", wantErr: true, errContains: "overflow"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMaxArtifactSize(resource.MustParse(tc.value))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateMaxArtifactSize(%q) = nil, want error", tc.value)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("validateMaxArtifactSize(%q) error = %q, want it to contain %q", tc.value, err.Error(), tc.errContains)
				}
			} else if err != nil {
				t.Errorf("validateMaxArtifactSize(%q) returned error: %v", tc.value, err)
			}
		})
	}
}
