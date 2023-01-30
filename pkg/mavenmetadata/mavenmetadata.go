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

package mavenmetadata

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

// MavenMetadata is the top-level structure for unmarshaled Maven Metadata XML
type MavenMetadata struct {
	GroupID    string     `xml:"groupId"`
	ArtifactID string     `xml:"artifactId"`
	Version    string     `xml:"version"`
	Versioning Versioning `xml:"versioning"`
}

// Versioning is the structure for the unmarshaled 'versioning' field in Maven
// Metadata XML
type Versioning struct {
	Latest           string           `xml:"latest"`
	Release          string           `xml:"release"`
	Versions         Versions         `xml:"versions"`
	LastUpdated      string           `xml:"lastUpdated"`
	SnapshotVersions SnapshotVersions `xml:"snapshotVersions"`
	Snapshot         SnapshotElement  `xml:"snapshot"`
}

// Versions is the structure for the unmarshaled 'Version' array field in Maven
// Metadata XML
type Versions struct {
	Version []string `xml:"version"`
}

type SnapshotVersions struct {
	SnapshotVersion []SnapshotVersion `xml:"snapshotVersion"`
}

type SnapshotElement struct {
	Timestamp   string `xml:"timestamp"`
	BuildNumber string `xml:"buildNumber"`
}

type SnapshotVersion struct {
	Extension string `xml:"extension"`
	Value     string `xml:"value"`
	Updated   string `xml:"updated"`
}

func (m *MavenMetadata) ReleaseVersion() (string, error) {
	if m.Versioning.Release == "" {
		return "", fmt.Errorf("artifact metadata does not have a RELEASE version")
	}
	return m.Versioning.Release, nil
}

func (m *MavenMetadata) LatestVersion() (string, error) {
	if m.Versioning.Latest == "" {
		return "", fmt.Errorf("artifact metadata does not have a LATEST version")
	}
	return m.Versioning.Latest, nil
}

// SnapshotResolvedFileName returns resolved artifact version
// if SNAPSHOT version is enabled, set the SNAPSHOT artifact name
// based on match conditions Snapshot.Timestamp, Snapshot.BuildNumber and Version.Extension
func (m *MavenMetadata) SnapshotResolvedVersion(filetype string) string {
	response := m.Version

	var sv *SnapshotVersion
	for _, v := range m.Versioning.SnapshotVersions.SnapshotVersion {
		tempvalue := fmt.Sprintf("%s-%s-%s", strings.Replace(m.Version, "-SNAPSHOT", "", 1), m.Versioning.Snapshot.Timestamp, m.Versioning.Snapshot.BuildNumber)
		if v.Extension == filetype && v.Value == tempvalue {
			sv = &v
		}
	}
	if sv != nil {
		response = sv.Value
	}

	return response
}

// Parse parses a byte array containing marshaled Maven Metadata XML data and
// returned an unmarshaled MavenMetadata structure
func Parse(input []byte) (*MavenMetadata, error) {
	if input == nil {
		return nil, errors.New("nil input")
	}

	var metadata MavenMetadata
	if err := xml.Unmarshal(input, &metadata); err != nil {
		return nil, err
	}
	return &metadata, nil
}

type VersionType string

const (
	Latest   VersionType = "LATEST"
	Release  VersionType = "RELEASE"
	Snapshot VersionType = "SNAPSHOT"
	Version  VersionType = "VERSION"
	Range    VersionType = "RANGE"
	Unknown  VersionType = "UNKNOWN"
)
