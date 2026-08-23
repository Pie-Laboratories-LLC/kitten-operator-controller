/*
Copyright 2026.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// KittenOperatorSpec defines the desired state of KittenOperator
type KittenOperatorSpec struct {
	// replicas is the desired number of kitten-serving pods
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// image is the container image for the kitten-operator Flask app
	// +required
	Image string `json:"image"`

	// refreshIntervalSeconds controls how often status.lastObservedKitten is refreshed
	// +optional
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=5
	RefreshIntervalSeconds *int32 `json:"refreshIntervalSeconds,omitempty"`
}

// KittenOperatorStatus defines the observed state of KittenOperator.
type KittenOperatorStatus struct {
	// conditions represent the current state of the KittenOperator resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// availableReplicas is the number of pods currently ready to serve kittens
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// lastObservedKitten is the URL of the most recently confirmed-reachable kitten image
	// +optional
	LastObservedKitten string `json:"lastObservedKitten,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// KittenOperator is the Schema for the kittenoperators API
type KittenOperator struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of KittenOperator
	// +required
	Spec KittenOperatorSpec `json:"spec"`

	// status defines the observed state of KittenOperator
	// +optional
	Status KittenOperatorStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// KittenOperatorList contains a list of KittenOperator
type KittenOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []KittenOperator `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &KittenOperator{}, &KittenOperatorList{})
		return nil
	})
}
