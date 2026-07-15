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

package controllers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reconciler.io/runtime/reconcilers"

	sourcev1alpha1 "github.com/vmware-tanzu/tanzu-source-controller/apis/source/v1alpha1"
	"github.com/vmware-tanzu/tanzu-source-controller/controllers"
)

// TestMavenResolverRejectsPathTraversal covers TNZGOV-5941: ArtifactId,
// Classifier, Type are attacker-controlled CR fields, and the resolved
// version for RELEASE/LATEST/SNAPSHOT is attacker-controlled via a malicious
// maven-metadata.xml served by spec.repository.url. Any of these containing
// "/" or ".." must cause Resolve to return an error rather than yielding a
// ResolvedFilename usable to escape the artifact directory.
func TestMavenResolverRejectsPathTraversal(t *testing.T) {
	tests := []struct {
		name         string
		artifact     sourcev1alpha1.MavenArtifactType
		metadataBody string
	}{
		{
			name: "artifactId path traversal, pinned version",
			artifact: sourcev1alpha1.MavenArtifactType{
				GroupId:    "com.example",
				ArtifactId: "../../../tmp/pwn",
				Version:    "1.0.0",
				Type:       "jar",
			},
		},
		{
			name: "classifier path traversal, pinned version",
			artifact: sourcev1alpha1.MavenArtifactType{
				GroupId:    "com.example",
				ArtifactId: "my-artifact",
				Version:    "1.0.0",
				Type:       "jar",
				Classifier: "../../../tmp/pwn",
			},
		},
		{
			name: "type path traversal, pinned version",
			artifact: sourcev1alpha1.MavenArtifactType{
				GroupId:    "com.example",
				ArtifactId: "my-artifact",
				Version:    "1.0.0",
				Type:       "../../../tmp/pwn",
			},
		},
		{
			name: "RELEASE resolves to a path traversal value from remote metadata",
			artifact: sourcev1alpha1.MavenArtifactType{
				GroupId:    "com.example",
				ArtifactId: "my-artifact",
				Version:    "RELEASE",
				Type:       "jar",
			},
			metadataBody: `<metadata><versioning><release>../../../tmp/pwn</release></versioning></metadata>`,
		},
		{
			name: "LATEST resolves to a path traversal value from remote metadata",
			artifact: sourcev1alpha1.MavenArtifactType{
				GroupId:    "com.example",
				ArtifactId: "my-artifact",
				Version:    "LATEST",
				Type:       "jar",
			},
			metadataBody: `<metadata><versioning><latest>../../../tmp/pwn</latest></versioning></metadata>`,
		},
		{
			name: "SNAPSHOT resolves to a path traversal value from remote metadata",
			artifact: sourcev1alpha1.MavenArtifactType{
				GroupId:    "com.example",
				ArtifactId: "my-artifact",
				Version:    "1.0.0-SNAPSHOT",
				Type:       "jar",
			},
			metadataBody: `<metadata>
				<version>1.0.0-SNAPSHOT</version>
				<versioning>
					<snapshot><timestamp>../../../tmp</timestamp><buildNumber>pwn</buildNumber></snapshot>
					<snapshotVersions>
						<snapshotVersion><extension>jar</extension><value>1.0.0-../../../tmp-pwn</value></snapshotVersion>
					</snapshotVersions>
				</versioning>
			</metadata>`,
		},
	}

	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(c.metadataBody))
			}))
			defer server.Close()

			resolver := controllers.MavenResolver{
				Artifact:      c.artifact,
				RepositoryURL: server.URL,
				RequestPath:   "com/example/my-artifact",
			}

			ctx := reconcilers.WithStash(context.Background())
			err := resolver.Resolve(ctx, server.Client())
			if err == nil {
				t.Fatalf("expected Resolve to return an error, but got resolved filename %q", resolver.ResolvedFilename)
			}
			if !strings.Contains(err.Error(), "not a valid filename") {
				t.Fatalf("expected a resolved-filename validation error, got: %v", err)
			}
		})
	}
}
