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
)

//nolint:lll
const (
	workerCloudInit = `{{.Header}}
{{template "files" .WriteFiles}}
{{template "ntp" .NTPServers}}
{{template "arbitrary" .AdditionalArbitraryData}}
runcmd:
{{- template "commands" .PreRKE2Commands }}
{{- if .AirGappedChecksum }}
  - [[ $(sha256sum /opt/rke2-artifacts/sha256sum*.txt | awk '{print $1}') == {{ .AirGappedChecksum }} ]] || exit 1{{ end }}
  - '{{ if .AirGapped }}INSTALL_RKE2_ARTIFACT_PATH=/opt/rke2-artifacts INSTALL_RKE2_TYPE="agent" sh /opt/install.sh{{ else }}curl -sfL https://get.rke2.io | INSTALL_RKE2_VERSION=%[1]s INSTALL_RKE2_TYPE="agent" sh -s -{{end}}'
{{- if .CISEnabled }}
  - '/opt/rke2-cis-script.sh'{{ end }}
  - 'systemctl enable rke2-agent.service'
  - 'systemctl start rke2-agent.service'
  - 'mkdir -p /run/cluster-api'
  - '{{ .SentinelFileCommand }}'
{{- template "commands" .PostRKE2Commands }}
{{ .AdditionalCloudInit -}}
`
)

// NewJoinWorker returns the user data string to be used on a controlplane instance.
//
// nolint:gofumpt
func NewJoinWorker(input *BaseUserData) ([]byte, error) {
	input.Header = cloudConfigHeader
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

	workerCloudJoinWithVersion := fmt.Sprintf(workerCloudInit, input.RKE2Version)
	userData, err := generate("JoinWorker", workerCloudJoinWithVersion, input)

	if err != nil {
		return nil, err
	}

	return userData, nil
}
