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

package cloudinit

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("WorkerAirGappedCloudInitTest", func() {
	var input *BaseUserData

	BeforeEach(func() {
		input = &BaseUserData{
			AirGapped: true,
		}
	})
	It("Should use the image embedded install.sh method", func() {
		workerCloudInitData, err := NewJoinWorker(input)
		Expect(err).ToNot(HaveOccurred())
		workerCloudInitString := string(workerCloudInitData)
		_, err = GinkgoWriter.Write(workerCloudInitData)
		Expect(err).NotTo(HaveOccurred())
		Expect(workerCloudInitString).To(Equal(`## template: jinja
#cloud-config

write_files:
-   path: 
    content: |
      
runcmd:
  - 'INSTALL_RKE2_ARTIFACT_PATH=/opt/rke2-artifacts INSTALL_RKE2_TYPE="agent" sh /opt/install.sh'
  - 'systemctl enable rke2-agent.service'
  - 'systemctl start rke2-agent.service'
  - 'mkdir /run/cluster-api' 
  - 'echo success > /run/cluster-api/bootstrap-success.complete'
`))
	})
})

var _ = Describe("WorkerOnlineCloudInitTest", func() {
	var input *BaseUserData

	BeforeEach(func() {
		input = &BaseUserData{
			AirGapped:   false,
			RKE2Version: "v1.25.6+rke2r1",
		}
	})
	It("Should use the RKE2 Online installation method", func() {
		workerCloudInitData, err := NewJoinWorker(input)
		Expect(err).ToNot(HaveOccurred())
		workerCloudInitString := string(workerCloudInitData)
		_, err = GinkgoWriter.Write(workerCloudInitData)
		Expect(err).NotTo(HaveOccurred())
		Expect(workerCloudInitString).To(Equal(`## template: jinja
#cloud-config

write_files:
-   path: 
    content: |
      
runcmd:
  - 'curl -sfL https://get.rke2.io | INSTALL_RKE2_VERSION=v1.25.6+rke2r1 INSTALL_RKE2_TYPE="agent" sh -s -'
  - 'systemctl enable rke2-agent.service'
  - 'systemctl start rke2-agent.service'
  - 'mkdir /run/cluster-api' 
  - 'echo success > /run/cluster-api/bootstrap-success.complete'
`))
	})
})
