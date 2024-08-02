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

package rke2

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
)

var _ = Describe("RKE2RegistryConfig", func() {
	var rke2ConfigReg RegistryScope
	BeforeEach(func() {
		rke2RegistryConfig := bootstrapv1.Registry{
			Mirrors: map[string]bootstrapv1.Mirror{
				"docker.io": {
					Endpoint: []string{
						"https://test-registry",
					},
					Rewrite: map[string]string{
						"/path-test": "/new-path-test",
					},
				},
			},
			Configs: map[string]bootstrapv1.RegistryConfig{
				"https://test-registry": {
					AuthSecret: corev1.ObjectReference{
						APIVersion: "v1",
						Kind:       "Secret",
						Namespace:  "test-ns",
						Name:       "test-auth-secret",
					},
					TLS: bootstrapv1.TLSConfig{
						TLSConfigSecret: corev1.ObjectReference{
							APIVersion: "v1",
							Kind:       "Secret",
							Namespace:  "test-ns",
							Name:       "test-tls-secret",
						},
						InsecureSkipVerify: true,
					},
				},
			},
		}

		rke2ConfigReg = RegistryScope{
			Registry: rke2RegistryConfig,
			Client: fake.NewClientBuilder().WithObjects(
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-auth-secret",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"username": []byte("test-username"),
						"password": []byte("test-password"),
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-tls-secret",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"tls.crt": []byte("certificate-test"),
						"tls.key": []byte("cert-key-test"),
						"ca.crt":  []byte("ca-cert-test"),
					},
				},
			).Build(),
			Ctx:    context.Background(),
			Logger: log.FromContext(context.Background()),
		}
		// secret := corev1.Secret{}
		// rke2ConfigReg.Client.Get(rke2ConfigReg.Ctx, types.NamespacedName{
		// 	Namespace: "test-ns",
		// 	Name:      "test-tls-secret",
		// }, &secret)
		// GinkgoWriter.Write(secret.Data["tls.crt"])
	},
	)

	It("should generate a valid registries.yaml file", func() {
		registryResult, files, err := GenerateRegistries(rke2ConfigReg)
		Expect(err).To(Not(HaveOccurred()))
		Expect(len(files)).To(Equal(3))

		tlsMap := map[string]string{
			"tls.crt": "certificate-test",
			"tls.key": "cert-key-test",
			"ca.crt":  "ca-cert-test",
		}
		fileNameArray := []string{"tls.crt", "tls.key", "ca.crt"}
		for _, file := range files {

			found := false
			var position int
			for i, filename := range fileNameArray {
				pathSlice := strings.Split(file.Path, "/")
				origFilename := pathSlice[len(pathSlice)-1]
				if origFilename == filename {
					found = true
					position = i

					break
				}
			}
			Expect(found).To(BeTrue())
			Expect(file.Content).To(Equal(tlsMap[fileNameArray[position]]))
			Expect(file.Path).To(Equal(registryCertsPath + "/" + fileNameArray[position]))
		}
		Expect(len(registryResult.Mirrors["docker.io"].Endpoint)).To(Equal(1))
		Expect(registryResult.Mirrors["docker.io"].Endpoint[0]).To(Equal("https://test-registry"))
		Expect(registryResult.Mirrors["docker.io"].Rewrite["/path-test"]).To(Equal("/new-path-test"))
		Expect(registryResult.Configs["https://test-registry"].Auth.Username).To(Equal("test-username"))
		Expect(registryResult.Configs["https://test-registry"].Auth.Password).To(Equal("test-password"))
		Expect(registryResult.Configs["https://test-registry"].TLS.CAFile).To(Equal(registryCertsPath + "/" + "ca.crt"))
		Expect(registryResult.Configs["https://test-registry"].TLS.CertFile).To(Equal(registryCertsPath + "/" + "tls.crt"))
		Expect(registryResult.Configs["https://test-registry"].TLS.KeyFile).To(Equal(registryCertsPath + "/" + "tls.key"))
		Expect(registryResult.Configs["https://test-registry"].TLS.InsecureSkipVerify).To(BeTrue())
	})
},
)

var _ = Describe("RKE2RegistryConfig is empty", func() {
	var rke2ConfigReg RegistryScope
	BeforeEach(func() {
		rke2RegistryConfig := bootstrapv1.Registry{}
		rke2ConfigReg = RegistryScope{
			Registry: rke2RegistryConfig,
			Client:   fake.NewClientBuilder().Build(),
			Ctx:      context.Background(),
			Logger:   log.FromContext(context.Background()),
		}
	},
	)

	It("should generate a valid registries.yaml file", func() {
		registryResult, files, err := GenerateRegistries(rke2ConfigReg)
		Expect(err).To(Not(HaveOccurred()))
		Expect(len(files)).To(Equal(0))
		Expect(len(registryResult.Mirrors)).To(Equal(0))
		Expect(len(registryResult.Configs)).To(Equal(0))
	})
})

var _ = Describe("RKE2RegistryConfig is nil", func() {
	var rke2ConfigReg RegistryScope
	BeforeEach(func() {
		rke2ConfigReg = RegistryScope{
			Client: fake.NewClientBuilder().Build(),
			Ctx:    context.Background(),
			Logger: log.FromContext(context.Background()),
		}
	},
	)

	It("should generate a valid registries.yaml file", func() {
		registryResult, files, err := GenerateRegistries(rke2ConfigReg)
		Expect(err).To(Not(HaveOccurred()))
		Expect(len(files)).To(Equal(0))
		Expect(len(registryResult.Mirrors)).To(Equal(0))
		Expect(len(registryResult.Configs)).To(Equal(0))
	})
})
