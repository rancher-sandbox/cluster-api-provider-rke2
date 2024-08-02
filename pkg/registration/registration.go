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

package registration

import (
	"errors"
	"fmt"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
)

// GetRegistrationAddresses is a function type that is used to provide different implementations of
// getting the addresses just when registering a new node into a cluster.
type GetRegistrationAddresses func(cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	cpMachines collections.Machines) ([]string, error)

// NewRegistrationMethod returns the function for the registration addresses based on the passed method name.
func NewRegistrationMethod(method string) (GetRegistrationAddresses, error) {
	switch method {
	case "internal-first":
		return registrationMethodWithFilter(filterInternalFirst), nil
	case "internal-only-ips":
		return registrationMethodWithFilter(filterInternalOnly), nil
	case "external-only-ips":
		return registrationMethodWithFilter(filterExternalOnly), nil
	case "address":
		return registrationMethodAddress, nil
	case "control-plane-endpoint":
		return registrationMethodControlPlaneEndpoint, nil
	default:
		return nil, fmt.Errorf("unsupported registration method: %s", method)
	}
}

func registrationMethodWithFilter(filter addressFilter) GetRegistrationAddresses {
	return func(_ *clusterv1.Cluster, rcp *controlplanev1.RKE2ControlPlane, availableMachines collections.Machines) ([]string, error) {
		validIPAddresses := []string{}

		for _, availableMachine := range availableMachines {
			ip := filter(availableMachine)
			if ip != "" {
				validIPAddresses = append(validIPAddresses, ip)
			}
		}

		return validIPAddresses, nil
	}
}

func registrationMethodAddress(_ *clusterv1.Cluster, rcp *controlplanev1.RKE2ControlPlane, _ collections.Machines) ([]string, error) {
	validIPAddresses := []string{}

	validIPAddresses = append(validIPAddresses, rcp.Spec.RegistrationAddress)

	if len(validIPAddresses) == 0 {
		return nil, errors.New("no registration address supplied")
	}

	return validIPAddresses, nil
}

func registrationMethodControlPlaneEndpoint(cluster *clusterv1.Cluster,
	_ *controlplanev1.RKE2ControlPlane,
	_ collections.Machines,
) ([]string, error) {
	validAddresses := []string{}

	if cluster.Spec.ControlPlaneEndpoint.IsZero() {
		return nil, errors.New("no control plane endpoint set")
	}

	if cluster.Spec.ControlPlaneEndpoint.Host == "" {
		return nil, errors.New("no control plane host set")
	}

	validAddresses = append(validAddresses, cluster.Spec.ControlPlaneEndpoint.Host)

	return validAddresses, nil
}

type addressFilter func(machine *clusterv1.Machine) string

func filterInternalFirst(machine *clusterv1.Machine) string {
	for _, address := range machine.Status.Addresses {
		switch address.Type {
		case clusterv1.MachineInternalIP:
			if address.Address != "" {
				return address.Address
			}
		case clusterv1.MachineExternalIP:
			if address.Address != "" {
				return address.Address
			}
		}
	}

	return ""
}

func filterInternalOnly(machine *clusterv1.Machine) string {
	for _, address := range machine.Status.Addresses {
		if address.Type == clusterv1.MachineInternalIP {
			if address.Address != "" {
				return address.Address
			}
		}
	}

	return ""
}

func filterExternalOnly(machine *clusterv1.Machine) string {
	for _, address := range machine.Status.Addresses {
		if address.Type == clusterv1.MachineExternalIP {
			if address.Address != "" {
				return address.Address
			}
		}
	}

	return ""
}
