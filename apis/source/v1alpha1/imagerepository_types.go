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
	"github.com/vmware-labs/reconciler-runtime/apis"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ImageRepositorySpec defines the desired state of ImageRepository
type ImageRepositorySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Image is a reference to an image in a remote repository
	Image string `json:"image"`

	// The interval at which to check for repository updates.
	Interval metav1.Duration `json:"interval,omitempty"`

	// ImagePullSecrets contains the names of the Kubernetes Secrets containing registry login
	// information to resolve image metadata.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// ServiceAccountName is the name of the Kubernetes ServiceAccount used to authenticate
	// the image pull if the service account has attached pull secrets. For more information:
	// https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// ImageRepositoryStatus defines the observed state of ImageRepository
type ImageRepositoryStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

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
//+kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
//+kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.artifact.url`
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
//+kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ImageRepository is the Schema for the imagerepositories API
type ImageRepository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageRepositorySpec   `json:"spec,omitempty"`
	Status ImageRepositoryStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ImageRepositoryList contains a list of ImageRepository
type ImageRepositoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageRepository `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageRepository{}, &ImageRepositoryList{})
}
