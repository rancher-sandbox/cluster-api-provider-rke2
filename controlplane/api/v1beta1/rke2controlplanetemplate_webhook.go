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
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
)

// SetupWebhookWithManager sets up the Controller Manager for the Webhook for the RKE2ControlPlaneTemplate resource.
func (r *RKE2ControlPlaneTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-controlplane-cluster-x-k8s-io-v1beta1-rke2controlplanetemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanetemplates,verbs=create;update,versions=v1beta1,name=mrke2controlplanetemplate.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &RKE2ControlPlaneTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplate) Default() {
	bootstrapv1.DefaultRKE2ConfigSpec(&r.Spec.Template.Spec.RKE2ConfigSpec)
}

//+kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1beta1-rke2controlplanetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanetemplates,verbs=create;update,versions=v1beta1,name=vrke2controlplanetemplate.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &RKE2ControlPlaneTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplate) ValidateCreate() (admission.Warnings, error) {
	rke2controlplanelog.Info("RKE2ControlPlane validate create", "control-plane", klog.KObj(r))

	var allErrs field.ErrorList

	allErrs = append(allErrs, bootstrapv1.ValidateRKE2ConfigSpec(r.Name, &r.Spec.Template.Spec.RKE2ConfigSpec)...)
	allErrs = append(allErrs, r.validateCNI()...)
	allErrs = append(allErrs, r.validateRegistrationMethod()...)

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplate) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	oldControlplane, ok := old.(*RKE2ControlPlaneTemplate)
	if !ok {
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), r.Name, field.ErrorList{
			field.InternalError(nil, errors.New("failed to convert old RKE2ControlPlane to object")),
		})
	}

	var allErrs field.ErrorList

	allErrs = append(allErrs, bootstrapv1.ValidateRKE2ConfigSpec(r.Name, &r.Spec.Template.Spec.RKE2ConfigSpec)...)
	allErrs = append(allErrs, r.validateCNI()...)

	if r.Spec.Template.Spec.RegistrationMethod != oldControlplane.Spec.Template.Spec.RegistrationMethod {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "registrationMethod"), r.Spec.Template.Spec.RegistrationMethod, "field is immutable"),
		)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(GroupVersion.WithKind("RKE2ControlPlane").GroupKind(), r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *RKE2ControlPlaneTemplate) ValidateDelete() (admission.Warnings, error) {
	rke2controlplanelog.Info("validate delete", "name", r.Name)

	return nil, nil
}

func (r *RKE2ControlPlaneTemplate) validateCNI() field.ErrorList {
	var allErrs field.ErrorList

	spec := r.Spec.Template.Spec

	if spec.ServerConfig.CNIMultusEnable && r.Spec.Template.Spec.ServerConfig.CNI == "" {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "serverConfig", "cni"),
				r.Spec.Template.Spec.ServerConfig.CNI, "must be specified when cniMultusEnable is true"))
	}

	return allErrs
}

func (r *RKE2ControlPlaneTemplate) validateRegistrationMethod() field.ErrorList {
	var allErrs field.ErrorList

	spec := r.Spec.Template.Spec

	if spec.RegistrationMethod == RegistrationMethodAddress {
		if spec.RegistrationAddress == "" {
			allErrs = append(allErrs,
				field.Invalid(field.NewPath("spec.registrationAddress"),
					spec.RegistrationAddress, "registrationAddress must be supplied when using registration method 'address'"))
		}
	}

	return allErrs
}
