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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reconciler.io/runtime/apis"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Repository type defines the parameters for accessing a maven repository
type Repository struct {
	// URL is the HTTPS address of the repository. HTTP is not supported.
	// +required
	URL string `json:"url"`

	// SecretRef can be given the name of a secret containing
	// Authentication data.
	//
	// For Basic Authentication use
	// - username: <BASE64>
	//   password: <BASE64>
	//
	// For mTLS authenticationa use
	//  - certFile: <BASE64> a PEM-encoded client certificate
	//  - keyFile: <BASE64> private key
	//
	// For a Certificate Authority to trust while connecting use
	//  - caFile: <BASE64> a PEM-encoded CA certificate
	// +optional
	SecretRef corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// MavenArtifactType describes properties for a maven created artifact
type MavenArtifactType struct {

	// Artifact Group ID
	// +required
	GroupId string `json:"groupId"`

	// Artifact Version
	// The version element identifies the current version of the artifact.
	// Supported values: "0.1.2" (version) and "RELEASE"
	// Unsupported values: "LATEST", "SNAPSHOT" and Maven Version Ranges
	// https://maven.apache.org/enforcer/enforcer-rules/versionRanges.html
	// +required
	Version string `json:"version"`

	// Artifact identifier
	// +required
	ArtifactId string `json:"artifactId"`

	// Package type (jar, war, pom), defaults to jar
	// +optional
	Type string `json:"type,omitempty"`

	// Classifier distinguishes artifacts that were built from the same POM but differed in content
	// +optional
	Classifier string `json:"classifier,omitempty"`
}

// MavenArtifactSpec defines the required configuration to provide a MavenArtifact from MavenRepository
type MavenArtifactSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Maven Artifact defines meta Type
	// +required
	Artifact MavenArtifactType `json:"artifact"`

	// Repository defines the parameters for accessing a repository
	// +required
	Repository Repository `json:"repository"`

	// Interval at which to check the repository for updates.
	// +required
	Interval metav1.Duration `json:"interval"`

	// Timeout for artifact download operation.
	// Defaults to 'Interval' duration.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// MavenArtifactStatus defines the observed state of MavenArtifact
type MavenArtifactStatus struct {
	apis.Status `json:",inline"`

	// URL is the download link for the artifact output of the last repository
	// sync.
	// +optional
	URL string `json:"url,omitempty"`

	// Artifact represents the output of the last successful repository sync.
	// +optional
	Artifact *Artifact `json:"artifact,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Artifact",type=string,JSONPath=`.spec.artifact.artifactId`
//+kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.artifact.url`
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
//+kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MavenArtifact is the Schema for the mavenartifacts API
type MavenArtifact struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MavenArtifactSpec   `json:"spec,omitempty"`
	Status MavenArtifactStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MavenArtifactList contains a list of MavenArtifact
type MavenArtifactList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MavenArtifact `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MavenArtifact{}, &MavenArtifactList{})
}
