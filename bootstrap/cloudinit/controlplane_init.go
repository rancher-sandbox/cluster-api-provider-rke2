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
	"fmt"

	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/secret"
)

//nolint:lll
const (
	controlPlaneCloudInit = `{{.Header}}
{{template "files" .WriteFiles}}
{{template "ntp" .NTPServers}}
{{template "arbitrary" .AdditionalArbitraryData}}
runcmd:
{{- template "commands" .PreRKE2Commands }}
  - {{ if .AirGapped }}INSTALL_RKE2_ARTIFACT_PATH=/opt/rke2-artifacts sh /opt/install.sh{{ else }}'curl -sfL https://get.rke2.io | INSTALL_RKE2_VERSION=%[1]s sh -s - server'{{ end }} 
{{- if .CISEnabled }}
  - '/opt/rke2-cis-script.sh'{{ end }}
  - 'systemctl enable rke2-server.service'
  - 'systemctl start rke2-server.service'
  - 'mkdir /run/cluster-api' 
  - '{{ .SentinelFileCommand }}'
{{- template "commands" .PostRKE2Commands }}
{{ .AdditionalCloudInit -}}
`
)

// ControlPlaneInput defines the context to generate a controlplane instance user data.
type ControlPlaneInput struct {
	BaseUserData
	secret.Certificates
}

// NewInitControlPlane returns the user data string to be used on a controlplane instance.
//
//nolint:gofumpt
func NewInitControlPlane(input *ControlPlaneInput) ([]byte, error) {
	input.Header = cloudConfigHeader
	input.WriteFiles = append(input.WriteFiles, input.Certificates.AsFiles()...)
	input.WriteFiles = append(input.WriteFiles, input.ConfigFile)
	input.SentinelFileCommand = sentinelFileCommand

	var err error

	input.AdditionalCloudInit, err = cleanupAdditionalCloudInit(input.AdditionalCloudInit)
	if err != nil {
		return nil, err
	}

	if err := cleanupArbitraryData(input.AdditionalArbitraryData); err != nil {
		return nil, err
	}

	controlPlaneCloudJoinWithVersion := fmt.Sprintf(controlPlaneCloudInit, input.RKE2Version)
	userData, err := generate("InitControlplane", controlPlaneCloudJoinWithVersion, input)

	if err != nil {
		return nil, err
	}

	return userData, nil
}
