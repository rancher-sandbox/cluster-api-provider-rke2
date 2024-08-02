/*
Copyright 2024 SUSE LLC.

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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RKE2ControlPlaneTemplateSpec defines the desired state of RKE2ControlPlaneTemplate.
type RKE2ControlPlaneTemplateSpec struct {
	Template RKE2ControlPlaneTemplateResource `json:"template"`
}

// RKE2ControlPlaneTemplateResource contains spec for RKE2ControlPlaneTemplate.
type RKE2ControlPlaneTemplateResource struct {
	// Spec is the specification of the desired behavior of the control plane.
	Spec RKE2ControlPlaneSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=rke2controlplanetemplates,scope=Namespaced,categories=cluster-api,shortName=rke2ct
// +kubebuilder:storageversion

// RKE2ControlPlaneTemplate is the Schema for the rke2controlplanetemplates API.
type RKE2ControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the control plane specification for the template resource.
	Spec RKE2ControlPlaneTemplateSpec `json:"spec,omitempty"`
	// Status is the current state of the control plane.
	Status RKE2ControlPlaneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RKE2ControlPlaneTemplateList contains a list of RKE2ControlPlaneTemplate.
type RKE2ControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RKE2ControlPlaneTemplate `json:"items"`
}

func init() { //nolint:gochecknoinits
	objectTypes = append(objectTypes, &RKE2ControlPlaneTemplate{}, &RKE2ControlPlaneTemplateList{})
}
