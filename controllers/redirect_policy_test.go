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

package controllers

import (
	"net/http"
	"net/url"
	"testing"
)

// TestSameHostRedirectPolicy covers TNZGOV-13098: the shared http.Client used
// for Maven repository requests has no CheckRedirect, so it will transparently
// follow a redirect to any host/scheme (e.g. cloud IMDS). sameHostRedirectPolicy
// must pin every redirect to the same scheme+host as the original request.
func TestSameHostRedirectPolicy(t *testing.T) {
	mustParse := func(t *testing.T, raw string) *url.URL {
		t.Helper()
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("failed to parse url %q: %v", raw, err)
		}
		return u
	}

	req := func(t *testing.T, raw string) *http.Request {
		t.Helper()
		return &http.Request{URL: mustParse(t, raw)}
	}

	tests := []struct {
		name    string
		via     []string
		next    string
		wantErr bool
	}{
		{
			name:    "same host and scheme",
			via:     []string{"https://repo.example.com/maven-metadata.xml"},
			next:    "https://repo.example.com/redirected/maven-metadata.xml",
			wantErr: false,
		},
		{
			name:    "same host, different path and query",
			via:     []string{"https://repo.example.com/a/b?x=1"},
			next:    "https://repo.example.com/c/d?y=2",
			wantErr: false,
		},
		{
			name:    "cross-host redirect rejected",
			via:     []string{"https://repo.example.com/maven-metadata.xml"},
			next:    "https://169.254.169.254/latest/meta-data/iam/security-credentials/",
			wantErr: true,
		},
		{
			name:    "scheme downgrade rejected",
			via:     []string{"https://repo.example.com/maven-metadata.xml"},
			next:    "http://repo.example.com/maven-metadata.xml",
			wantErr: true,
		},
		{
			name: "too many redirects rejected",
			via: func() []string {
				via := make([]string, 10)
				for i := range via {
					via[i] = "https://repo.example.com/hop"
				}
				return via
			}(),
			next:    "https://repo.example.com/hop",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			via := make([]*http.Request, len(tc.via))
			for i, raw := range tc.via {
				via[i] = req(t, raw)
			}

			err := sameHostRedirectPolicy(req(t, tc.next), via)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error redirecting to %q, got nil", tc.next)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error redirecting to %q, got: %v", tc.next, err)
			}
		})
	}
}
