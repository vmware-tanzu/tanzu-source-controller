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

package mavenmetadata_test

import (
	"testing"

	"github.com/vmware-tanzu/tanzu-source-controller/pkg/mavenmetadata"
)

var simpleTestData = `
<?xml version="1.0" encoding="UTF-8"?>
<metadata modelVersion="1.1.0">
  <groupId>org.springframework.boot</groupId>
  <artifactId>spring-boot</artifactId>
  <version>2.6.7</version>
  <versioning>
    <latest>2.6.7</latest>
    <release>2.6.7</release>
    <versions>
      <version>2.6.0</version>
      <version>2.6.7</version>
    </versions>
    <lastUpdated>20220421111446</lastUpdated>
  </versioning>
</metadata>
`

var missingLatestMetdata = `
<?xml version="1.0" encoding="UTF-8"?>
<metadata modelVersion="1.1.0">
  <groupId>org.springframework.boot</groupId>
  <artifactId>spring-boot</artifactId>
  <version>2.6.7</version>
  <versioning>
    <release>2.6.7</release>
    <versions>
      <version>2.6.0</version>
      <version>2.6.7</version>
    </versions>
    <lastUpdated>20220421111446</lastUpdated>
  </versioning>
</metadata>
`

var missingReleaseMetdata = `
<?xml version="1.0" encoding="UTF-8"?>
<metadata modelVersion="1.1.0">
  <groupId>org.springframework.boot</groupId>
  <artifactId>spring-boot</artifactId>
  <version>2.6.7</version>
  <versioning>
    <latest>2.6.7</latest>
    <versions>
      <version>2.6.0</version>
      <version>2.6.7</version>
    </versions>
    <lastUpdated>20220421111446</lastUpdated>
  </versioning>
</metadata>
`

var snapshotTestData = `
<?xml version="1.0" encoding="UTF-8"?>
<metadata modelVersion="1.1.0">
  <groupId>org.springframework.boot</groupId>
  <artifactId>spring-boot</artifactId>
  <versioning>
    <lastUpdated>20220708171442</lastUpdated>
    <snapshot>
      <timestamp>20220708.171442</timestamp>
      <buildNumber>1</buildNumber>
    </snapshot>
    <snapshotVersions>
      <snapshotVersion>
        <extension>jar</extension>
        <value>2.7.0-20220708.171442-1</value>
        <updated>20220708171442</updated>
      </snapshotVersion>
      <snapshotVersion>
        <extension>pom</extension>
        <value>2.7.0-20220708.171442-1</value>
        <updated>20220708171442</updated>
      </snapshotVersion>
    </snapshotVersions>
  </versioning>
  <version>2.7.0-SNAPSHOT</version>
</metadata>
`

var brokenTestData = `
<?xml version="1.0" encoding="UTF-8"?>
<metadata modelVersion="1.1.0">
  <groupId>org.springframework.boot</groupId>
  <artifactId>spring-boot</artifactId>
  <version>2.6.7</version>
  <versio
    <latest>2.6.7</latest>
    <release>2.6.7</release>
    <versions>
      <version>2.6.0</version>
      <version>2.6.7</version>
    </versions>
    <lastUpdated>20220421111446</lastUpdated>
  </versioning>
</metadata>
`

func TestParseNil(t *testing.T) {
	_, err := mavenmetadata.Parse(nil)
	if err == nil {
		t.Errorf("Parse returned no error")
	}
}

func TestParseSimple(t *testing.T) {
	metadata, err := mavenmetadata.Parse([]byte(simpleTestData))
	if err != nil {
		t.Errorf("Parse returned error %v", err)
	}
	if metadata == nil {
		t.Error("Parse returned nil metadata")
	}

	expectString(t, "GroupId", metadata.GroupID, "org.springframework.boot")
	expectString(t, "ArtifactID", metadata.ArtifactID, "spring-boot")
	expectString(t, "Version", metadata.Version, "2.6.7")
	expectString(t, "Versioning.Latest", metadata.Versioning.Latest, "2.6.7")
	expectString(t, "Versioning.Release", metadata.Versioning.Release, "2.6.7")
	expectString(t, "Versioning.LastUpdated", metadata.Versioning.LastUpdated, "20220421111446")
	expectString(t, "Versioning.Versions.Version[0]", metadata.Versioning.Versions.Version[0], "2.6.0")
	expectString(t, "Versioning.Versions.Version[1]", metadata.Versioning.Versions.Version[1], "2.6.7")
	expectInt64(t, "len(Versioning.Versions.Version)", int64(len(metadata.Versioning.Versions.Version)), 2)
}

func TestParseSnapshot(t *testing.T) {
	metadata, err := mavenmetadata.Parse([]byte(snapshotTestData))
	if err != nil {
		t.Errorf("Parse returned error %v", err)
	}
	if metadata == nil {
		t.Error("Parse returned nil metadata")
	}

	expectString(t, "GroupId", metadata.GroupID, "org.springframework.boot")
	expectString(t, "ArtifactID", metadata.ArtifactID, "spring-boot")
	expectString(t, "Version", metadata.Version, "2.7.0-SNAPSHOT")
	expectString(t, "Versioning.SnapshotElement.Timestamp", metadata.Versioning.Snapshot.Timestamp, "20220708.171442")
	expectString(t, "Versioning.SnapshotElement.BuildNumber", metadata.Versioning.Snapshot.BuildNumber, "1")
	expectString(t, "SnapshotResolvedFileVersion", metadata.SnapshotResolvedVersion("jar"), "2.7.0-20220708.171442-1")
	expectString(t, "Versioning.SnapshotVersions.SnapshotVersion[0].Extension", metadata.Versioning.SnapshotVersions.SnapshotVersion[0].Extension, "jar")
	expectString(t, "Versioning.SnapshotVersions.SnapshotVersion[0].Value", metadata.Versioning.SnapshotVersions.SnapshotVersion[0].Value, "2.7.0-20220708.171442-1")
	expectString(t, "Versioning.SnapshotVersions.SnapshotVersion[0].Extension", metadata.Versioning.SnapshotVersions.SnapshotVersion[0].Updated, "20220708171442")
	expectString(t, "Versioning.SnapshotVersions.SnapshotVersion[1].Extension", metadata.Versioning.SnapshotVersions.SnapshotVersion[1].Extension, "pom")
	expectString(t, "Versioning.SnapshotVersions.SnapshotVersion[1].Value", metadata.Versioning.SnapshotVersions.SnapshotVersion[1].Value, "2.7.0-20220708.171442-1")
	expectString(t, "Versioning.SnapshotVersions.SnapshotVersion[1].Extension", metadata.Versioning.SnapshotVersions.SnapshotVersion[1].Updated, "20220708171442")
}

func TestParseBroken(t *testing.T) {
	_, err := mavenmetadata.Parse([]byte(brokenTestData))
	if err == nil {
		t.Errorf("Parse returned no error")
	}
}

func expectString(t *testing.T, name, actual, expected string) {
	if actual != expected {
		t.Errorf("Error on field '%v'. Actual '%v'. Expected '%v'.", name, actual, expected)
	}
}

func expectInt64(t *testing.T, name string, actual, expected int64) {
	if actual != expected {
		t.Errorf("Error on field '%v'. Actual '%v'. Expected '%v'.", name, actual, expected)
	}
}

func TestMavenMetadata_LatestVersion(t *testing.T) {
	meta, err := mavenmetadata.Parse([]byte(simpleTestData))
	if err != nil {
		t.Errorf("Parse returned error %s", err)
	}

	missingLatestMeta, err := mavenmetadata.Parse([]byte(missingLatestMetdata))
	if err != nil {
		t.Errorf("Parse returned error %s", err)
	}

	tests := []struct {
		name     string
		m        mavenmetadata.MavenMetadata
		want     string
		wantErr  bool
		err_masg string
	}{
		{
			name:    "latest version",
			m:       *meta,
			want:    "2.6.7",
			wantErr: false,
		},
		{
			name:     "missing-latest version",
			m:        *missingLatestMeta,
			want:     "",
			wantErr:  true,
			err_masg: "artifact metadata does not have a LATEST version",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.m.LatestVersion()
			if (err != nil) != tt.wantErr {
				t.Errorf("MavenMetadata.LatestVersion() error = %v, wantErr %v", err, tt.wantErr)
				if got != tt.err_masg {
					t.Errorf("MavenMetadata.LatestVersion() = %v, want %v", got, tt.want)
				}
				return
			}

			if got != tt.want {
				t.Errorf("MavenMetadata.LatestVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMavenMetadata_ReleaseVersion(t *testing.T) {
	meta, err := mavenmetadata.Parse([]byte(simpleTestData))
	if err != nil {
		t.Errorf("Parse returned error %s", err)
	}

	missingLatestMeta, err := mavenmetadata.Parse([]byte(missingReleaseMetdata))
	if err != nil {
		t.Errorf("Parse returned error %s", err)
	}

	tests := []struct {
		name     string
		m        mavenmetadata.MavenMetadata
		want     string
		wantErr  bool
		err_masg string
	}{
		{
			name:    "release version",
			m:       *meta,
			want:    "2.6.7",
			wantErr: false,
		},
		{
			name:     "missing-relase version",
			m:        *missingLatestMeta,
			want:     "",
			wantErr:  true,
			err_masg: "artifact metadata does not have a RELEASE version",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.m.ReleaseVersion()
			if (err != nil) != tt.wantErr {
				t.Errorf("MavenMetadata.ReleaseVersion() error = %v, wantErr %v", err, tt.wantErr)
				if got != tt.err_masg {
					t.Errorf("MavenMetadata.ReleaseVersion() = %v, want %v", got, tt.want)
				}
				return
			}

			if got != tt.want {
				t.Errorf("MavenMetadata.ReleaseVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
