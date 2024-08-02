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
	. "github.com/onsi/ginkgo/v2"
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
  - 'mkdir -p /run/cluster-api'
  - 'echo success > /run/cluster-api/bootstrap-success.complete'
`))
	})
})

var _ = Describe("WorkerAirGappedWithChecksumCloudInitTest", func() {
	var input *BaseUserData

	BeforeEach(func() {
		input = &BaseUserData{
			AirGapped:         true,
			AirGappedChecksum: "abcd",
		}
	})
	It("Should use the image embedded install.sh method and check the checksum first", func() {
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
  - [[ $(sha256sum /opt/rke2-artifacts/sha256sum*.txt | awk '{print $1}') == abcd ]] || exit 1
  - 'INSTALL_RKE2_ARTIFACT_PATH=/opt/rke2-artifacts INSTALL_RKE2_TYPE="agent" sh /opt/install.sh'
  - 'systemctl enable rke2-agent.service'
  - 'systemctl start rke2-agent.service'
  - 'mkdir -p /run/cluster-api'
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
  - 'mkdir -p /run/cluster-api'
  - 'echo success > /run/cluster-api/bootstrap-success.complete'
`))
	})
})

var _ = Describe("NTPWorkerTest", func() {
	var input *BaseUserData

	BeforeEach(func() {
		input = &BaseUserData{
			NTPServers: []string{"test.ntp.org"},
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
      
ntp:
  enabled: true
  servers:
  - "test.ntp.org"

runcmd:
  - 'curl -sfL https://get.rke2.io | INSTALL_RKE2_VERSION= INSTALL_RKE2_TYPE="agent" sh -s -'
  - 'systemctl enable rke2-agent.service'
  - 'systemctl start rke2-agent.service'
  - 'mkdir -p /run/cluster-api'
  - 'echo success > /run/cluster-api/bootstrap-success.complete'
`))
	})
})

var _ = Describe("WorkerCISTest", func() {
	var input *BaseUserData

	BeforeEach(func() {
		input = &BaseUserData{
			AirGapped:   false,
			CISEnabled:  true,
			RKE2Version: "v1.25.6+rke2r1",
		}
	})
	It("Should run the CIS script", func() {
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
  - '/opt/rke2-cis-script.sh'
  - 'systemctl enable rke2-agent.service'
  - 'systemctl start rke2-agent.service'
  - 'mkdir -p /run/cluster-api'
  - 'echo success > /run/cluster-api/bootstrap-success.complete'
`))
	})
})

var _ = Describe("CleanupCloudInit test", func() {
	cloudInitData := `## template: jinja
#cloud-config
hello: world
users:
- name: rke2
write_files:
-   path: 
    content: |


runcmd:
  - 'curl -sfL https://get.rke2.io | INSTALL_RKE2_VERSION= INSTALL_RKE2_TYPE=\"agent\" sh -s -'
  - 'systemctl enable rke2-agent.service'
  - 'systemctl start rke2-agent.service'
  - 'mkdir -p /run/cluster-api'
  - 'echo success > /run/cluster-api/bootstrap-success.complete'
`

	It("Should remove the runcmd, write_files and ntp lines", func() {
		cleanCloudInitData, err := cleanupAdditionalCloudInit(cloudInitData)
		Expect(cleanCloudInitData).To(Equal(`hello: world
users:
  - name: rke2
`))
		Expect(err).ToNot(HaveOccurred())
	})
})

var _ = Describe("CloudInit with custom entries", func() {
	var input *BaseUserData
	var cloudInitData string
	var arbitraryData map[string]string

	BeforeEach(func() {
		cloudInitData = `## template: jinja
#cloud-config
device_aliases: {'ephemeral0': '/dev/vdb'}
disk_setup:
  ephemeral0:
    table_type: mbr
    layout: False
    overwrite: False

fs_setup:
  - label: ephemeral0
    filesystem: ext4
    device: ephemeral0.0

write_files:
-   path: /etc/hosts
    content: |
      192.168.0.1 test


runcmd:
  - 'print hello world' 
`
		arbitraryData = map[string]string{
			"disk_setup": `
  ephemeral0:
    table_type: mbr
    layout: False
    overwrite: False`,
			"device_aliases": "{'ephemeral0': '/dev/vdb'}",
			"runcmd": `
  - 'print hello world'`,
		}
	})

	It("Should apply the arbitrary data and cleanup the input values", func() {
		input = &BaseUserData{
			AirGapped:               false,
			CISEnabled:              true,
			RKE2Version:             "v1.25.6+rke2r1",
			AdditionalArbitraryData: arbitraryData,
		}
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
      


device_aliases: {'ephemeral0': '/dev/vdb'}
disk_setup: 
  ephemeral0:
    table_type: mbr
    layout: False
    overwrite: False
runcmd:
  - 'curl -sfL https://get.rke2.io | INSTALL_RKE2_VERSION=v1.25.6+rke2r1 INSTALL_RKE2_TYPE="agent" sh -s -'
  - '/opt/rke2-cis-script.sh'
  - 'systemctl enable rke2-agent.service'
  - 'systemctl start rke2-agent.service'
  - 'mkdir -p /run/cluster-api'
  - 'echo success > /run/cluster-api/bootstrap-success.complete'
`))
	})

	It("Should remove the runcmd, write_files and ntp lines", func() {
		input = &BaseUserData{
			AirGapped:           false,
			CISEnabled:          true,
			RKE2Version:         "v1.25.6+rke2r1",
			AdditionalCloudInit: cloudInitData,
		}
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
  - '/opt/rke2-cis-script.sh'
  - 'systemctl enable rke2-agent.service'
  - 'systemctl start rke2-agent.service'
  - 'mkdir -p /run/cluster-api'
  - 'echo success > /run/cluster-api/bootstrap-success.complete'
device_aliases:
  ephemeral0: /dev/vdb
disk_setup:
  ephemeral0:
    layout: false
    overwrite: false
    table_type: mbr
fs_setup:
  - device: ephemeral0.0
    filesystem: ext4
    label: ephemeral0
`))
	})
})
