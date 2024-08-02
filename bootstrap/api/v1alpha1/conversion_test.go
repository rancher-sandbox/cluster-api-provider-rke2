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

package v1alpha1

import (
	"testing"

	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(AddToScheme(scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme)).To(Succeed())

	t.Run("for RKE2Config", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme:      scheme,
		Hub:         &bootstrapv1.RKE2Config{},
		Spoke:       &RKE2Config{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))

	t.Run("for RKE2ConfigTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme:      scheme,
		Hub:         &bootstrapv1.RKE2ConfigTemplate{},
		Spoke:       &RKE2ConfigTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
}

func fuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		rke2ConfigFuzzer,
		rke2ConfigTemplateFuzzer,
	}
}

func rke2ConfigFuzzer(obj *RKE2Config, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.Spec.AgentConfig.Version = ""
}

func rke2ConfigTemplateFuzzer(obj *RKE2ConfigTemplate, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	obj.Spec.Template.Spec.AgentConfig.Version = ""
}
