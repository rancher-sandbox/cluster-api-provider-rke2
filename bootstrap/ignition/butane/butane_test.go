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

// Package butane_test tests butane package.
package butane

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ignition "github.com/coreos/ignition/v2/config/v3_3"

	"k8s.io/utils/pointer"

	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/cloudinit"
)

func TestButane(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Butane Suite")
}

var additionalIgnition = `
variant: fcos
version: 1.4.0
systemd:
  units:
    - name: test.service
      enabled: true
      contents: |
        [Unit]
        Description=test
        [Service]
        ExecStart=/etc/test.sh
        [Install]
        WantedBy=test.target
`

var _ = Describe("Render", func() {
	var (
		input            *cloudinit.BaseUserData
		additionalConfig *bootstrapv1.AdditionalUserData
	)

	BeforeEach(func() {
		input = &cloudinit.BaseUserData{
			PreRKE2Commands: []string{
				"echo 'test'",
			},
			PostRKE2Commands: []string{
				"echo 'test'",
			},
			NTPServers: []string{
				"test",
			},
			RKE2Version: "v1.21.3+rke2r1",
			WriteFiles: []bootstrapv1.File{
				{
					Path:        "/test/file",
					Content:     "test",
					Permissions: "0644",
				},
				{
					Path:        "/test/base64",
					Encoding:    bootstrapv1.Base64,
					Content:     "Zm9vCg==",
					Permissions: "0644",
				},
			},
			ConfigFile: bootstrapv1.File{
				Path:        "/test/config",
				Content:     "test",
				Permissions: "0644",
			},
		}
		additionalConfig = &bootstrapv1.AdditionalUserData{
			Config: additionalIgnition,
			Strict: true,
		}
	})

	It("should render a valid ignition config", func() {
		ignitionJson, err := Render(input, additionalConfig)
		Expect(err).ToNot(HaveOccurred())

		ign, reports, err := ignition.Parse(ignitionJson)
		Expect(err).ToNot(HaveOccurred())
		Expect(reports.IsFatal()).To(BeFalse())

		Expect(ign.Ignition.Version).To(Equal("3.3.0"))

		Expect(ign.Storage.Files).To(HaveLen(5))

		Expect(ign.Storage.Files[0].Path).To(Equal("/etc/ssh/sshd_config"))

		Expect(ign.Storage.Files[1].Path).To(Equal("/test/file"))

		Expect(ign.Storage.Files[2].Path).To(Equal("/test/base64"))

		Expect(ign.Storage.Files[3].Path).To(Equal("/etc/rke2-install.sh"))

		Expect(ign.Storage.Files[4].Path).To(Equal("/etc/ntp.conf"))

		Expect(ign.Systemd.Units).To(HaveLen(3))
		Expect(ign.Systemd.Units[0].Name).To(Equal("rke2-install.service"))
		Expect(ign.Systemd.Units[0].Contents).To(Equal(pointer.String("[Unit]\nDescription=rke2-install\nWants=network-online.target\nAfter=network-online.target network.target\nConditionPathExists=!/run/cluster-api/bootstrap-success.complete\n[Service]\nUser=root\n# To not restart the unit when it exits, as it is expected.\nType=oneshot\nExecStart=/etc/rke2-install.sh\n[Install]\nWantedBy=multi-user.target\n")))
		Expect(ign.Systemd.Units[0].Enabled).To(Equal(pointer.Bool(true)))

		Expect(ign.Systemd.Units[1].Name).To(Equal("ntpd.service"))
		Expect(ign.Systemd.Units[1].Enabled).To(Equal(pointer.Bool(true)))

		Expect(ign.Systemd.Units[2].Name).To(Equal("test.service"))
		Expect(ign.Systemd.Units[2].Contents).To(Equal(pointer.String("[Unit]\nDescription=test\n[Service]\nExecStart=/etc/test.sh\n[Install]\nWantedBy=test.target\n")))
		Expect(ign.Systemd.Units[2].Enabled).To(Equal(pointer.Bool(true)))
	})

	It("accepts empty additional config", func() {
		additionalConfig = nil
		_, err := Render(input, additionalConfig)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return error if input is nil", func() {
		_, err := Render(nil, additionalConfig)
		Expect(err).To(HaveOccurred())
	})

	It("treats warnings as errors in strict mode", func() {
		// Should generate an Ignition warning about the colon in the partition label.
		configWithIgnitionWarning := `
storage:
    - path: /var/lib/static_key_example
      device: /dev/disk/by-id/dm-name-static-key-example
      format: ext4
      label: STATIC-EXAMPLE
      with_mount_unit: true
`
		additionalConfig = &bootstrapv1.AdditionalUserData{
			Config: configWithIgnitionWarning,
			Strict: true,
		}

		_, err := Render(input, additionalConfig)
		Expect(err).To(HaveOccurred())
	})
})
