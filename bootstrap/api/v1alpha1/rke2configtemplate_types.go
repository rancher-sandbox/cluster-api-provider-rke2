/*
Copyright 2022 SUSE.

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
)

// RKE2ConfigTemplateSpec defines the desired state of RKE2ConfigTemplate
type RKE2ConfigTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of RKE2ConfigTemplate. Edit RKE2configtemplate_types.go to remove/update
	Template RKE2ConfigTemplateResource `json:"template"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RKE2ConfigTemplate is the Schema for the RKE2configtemplates API
type RKE2ConfigTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RKE2ConfigTemplateSpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// RKE2ConfigTemplateList contains a list of RKE2ConfigTemplate
type RKE2ConfigTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RKE2ConfigTemplate `json:"items"`
}

type RKE2ConfigTemplateResource struct {
	Spec RKE2ConfigSpec `json:"spec"`
}

func init() {
	SchemeBuilder.Register(&RKE2ConfigTemplate{}, &RKE2ConfigTemplateList{})
}
