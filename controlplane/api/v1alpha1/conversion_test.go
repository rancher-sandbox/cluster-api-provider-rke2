/*
Copyright 2023 SUSE.

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
	"testing"

	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(AddToScheme(scheme)).To(Succeed())
	g.Expect(controlplanev1.AddToScheme(scheme)).To(Succeed())

	t.Run("for RKE2ControlPlane", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme:      scheme,
		Hub:         &controlplanev1.RKE2ControlPlane{},
		Spoke:       &RKE2ControlPlane{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{},
	}))

	t.Run("for RKE2ControlPlaneTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme:      scheme,
		Hub:         &controlplanev1.RKE2ControlPlaneTemplate{},
		Spoke:       &RKE2ControlPlaneTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{},
	}))
}
