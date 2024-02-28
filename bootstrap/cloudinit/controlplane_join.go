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

// NewJoinControlPlane returns the user data string to be used on a controlplane instance.
//
// nolint:gofumpt
func NewJoinControlPlane(input *ControlPlaneInput) ([]byte, error) {
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

	controlPlaneCloudJoinWithVersion := fmt.Sprintf(controlPlaneCloudInit, input.RKE2Version)
	userData, err := generate("JoinControlplane", controlPlaneCloudJoinWithVersion, input)

	if err != nil {
		return nil, err
	}

	return userData, nil
}
