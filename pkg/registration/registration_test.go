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

package registration_test

import (
	"testing"

	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher/cluster-api-provider-rke2/pkg/registration"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
)

func TestNewRegistrationMethod(t *testing.T) {
	testCases := []struct {
		name        string
		expectError bool
	}{
		{
			name:        "internal-first",
			expectError: false,
		},
		{
			name:        "internal-only-ips",
			expectError: false,
		},
		{
			name:        "external-only-ips",
			expectError: false,
		},
		{
			name:        "address",
			expectError: false,
		},
		{
			name:        "control-plane-endpoint",
			expectError: false,
		},
		{
			name:        "unknownmethod",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			regMethod, err := registration.NewRegistrationMethod(tc.name)
			if !tc.expectError {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(regMethod).ToNot(BeNil())
			} else {
				g.Expect(err).To(HaveOccurred())
			}
		})
	}
}

func TestInternalFirstMethod(t *testing.T) {
	testCases := []struct {
		name              string
		rcp               *controlplanev1.RKE2ControlPlane
		machines          []*clusterv1.Machine
		expectedAddresses []string
	}{
		{
			name: "only internal",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodFavourInternalIPs), ""),
			machines: []*clusterv1.Machine{
				createMachine("machine1", []string{"10.0.0.3"}, nil),
			},
			expectedAddresses: []string{"10.0.0.3"},
		},
		{
			name: "internal and external",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodFavourInternalIPs), ""),
			machines: []*clusterv1.Machine{
				createMachine("machine1", []string{"10.0.0.3"}, []string{"201.55.56.77"}),
			},
			expectedAddresses: []string{"10.0.0.3"},
		},
		{
			name: "multiple machines one with each type",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodFavourInternalIPs), ""),
			machines: []*clusterv1.Machine{
				createMachine("machine1", nil, []string{"201.55.56.77"}),
				createMachine("machine2", []string{"10.0.0.3"}, nil),
			},
			expectedAddresses: []string{"201.55.56.77", "10.0.0.3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			regMethod, err := registration.NewRegistrationMethod(string(controlplanev1.RegistrationMethodFavourInternalIPs))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(regMethod).NotTo(BeNil())

			col := collections.FromMachines(tc.machines...)

			actualAddresses, err := regMethod(nil, tc.rcp, col)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(actualAddresses).To(HaveLen(len(tc.expectedAddresses)))
			g.Expect(actualAddresses).To(ContainElements(tc.expectedAddresses))
		})
	}
}

func TestInternalOnlyMethod(t *testing.T) {
	testCases := []struct {
		name              string
		rcp               *controlplanev1.RKE2ControlPlane
		machines          []*clusterv1.Machine
		expectedAddresses []string
	}{
		{
			name: "only internal",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodInternalIPs), ""),
			machines: []*clusterv1.Machine{
				createMachine("machine1", []string{"10.0.0.3"}, nil),
			},
			expectedAddresses: []string{"10.0.0.3"},
		},
		{
			name: "internal and external",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodInternalIPs), ""),
			machines: []*clusterv1.Machine{
				createMachine("machine1", []string{"10.0.0.3"}, []string{"201.55.56.77"}),
			},
			expectedAddresses: []string{"10.0.0.3"},
		},
		{
			name: "multiple machines one with each type",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodInternalIPs), ""),
			machines: []*clusterv1.Machine{
				createMachine("machine1", nil, []string{"201.55.56.77"}),
				createMachine("machine2", []string{"10.0.0.3"}, nil),
			},
			expectedAddresses: []string{"10.0.0.3"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			regMethod, err := registration.NewRegistrationMethod(string(controlplanev1.RegistrationMethodInternalIPs))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(regMethod).NotTo(BeNil())

			col := collections.FromMachines(tc.machines...)

			actualAddresses, err := regMethod(nil, tc.rcp, col)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(actualAddresses).To(HaveLen(len(tc.expectedAddresses)))
			g.Expect(actualAddresses).To(ContainElements(tc.expectedAddresses))
		})
	}
}

func TestExternalOnlyMethod(t *testing.T) {
	testCases := []struct {
		name              string
		rcp               *controlplanev1.RKE2ControlPlane
		machines          []*clusterv1.Machine
		expectedAddresses []string
	}{
		{
			name: "only internal",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodExternalIPs), ""),
			machines: []*clusterv1.Machine{
				createMachine("machine1", []string{"10.0.0.3"}, nil),
			},
			expectedAddresses: []string{},
		},
		{
			name: "internal and external",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodExternalIPs), ""),
			machines: []*clusterv1.Machine{
				createMachine("machine1", []string{"10.0.0.3"}, []string{"201.55.56.77"}),
			},
			expectedAddresses: []string{"201.55.56.77"},
		},
		{
			name: "multiple machines one with each type",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodExternalIPs), ""),
			machines: []*clusterv1.Machine{
				createMachine("machine1", nil, []string{"201.55.56.77"}),
				createMachine("machine2", []string{"10.0.0.3"}, nil),
			},
			expectedAddresses: []string{"201.55.56.77"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			regMethod, err := registration.NewRegistrationMethod(string(controlplanev1.RegistrationMethodExternalIPs))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(regMethod).NotTo(BeNil())

			col := collections.FromMachines(tc.machines...)

			actualAddresses, err := regMethod(nil, tc.rcp, col)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(len(actualAddresses)).To(Equal(len(tc.expectedAddresses)))
			g.Expect(actualAddresses).To(ContainElements(tc.expectedAddresses))
		})
	}
}

func TestAddressMethod(t *testing.T) {
	testCases := []struct {
		name     string
		rcp      *controlplanev1.RKE2ControlPlane
		machines []*clusterv1.Machine
	}{
		{
			name: "only internal",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodAddress), "100.100.100.100"),
			machines: []*clusterv1.Machine{
				createMachine("machine1", []string{"10.0.0.3"}, nil),
			},
		},
		{
			name: "internal and external",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodAddress), "100.100.100.100"),
			machines: []*clusterv1.Machine{
				createMachine("machine1", []string{"10.0.0.3"}, []string{"201.55.56.77"}),
			},
		},
		{
			name: "multiple machines one with each type",
			rcp:  createControlPlane(string(controlplanev1.RegistrationMethodAddress), "100.100.100.100"),
			machines: []*clusterv1.Machine{
				createMachine("machine1", nil, []string{"201.55.56.77"}),
				createMachine("machine2", []string{"10.0.0.3"}, nil),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			regMethod, err := registration.NewRegistrationMethod(string(controlplanev1.RegistrationMethodAddress))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(regMethod).NotTo(BeNil())

			col := collections.FromMachines(tc.machines...)

			actualAddresses, err := regMethod(nil, tc.rcp, col)
			g.Expect(err).NotTo(HaveOccurred())

			expectedAddresses := []string{"100.100.100.100"}

			g.Expect(len(actualAddresses)).To(Equal(len(expectedAddresses)))
			g.Expect(actualAddresses).To(ContainElements(expectedAddresses))
		})
	}
}

func TestControlPlaneMethod(t *testing.T) {
	testCases := []struct {
		name            string
		cluster         *clusterv1.Cluster
		expectErr       bool
		expectedAddress string
	}{
		{
			name:            "with host and port",
			cluster:         createCluster("test1", "server1", 9445),
			expectErr:       false,
			expectedAddress: "server1",
		},
		{
			name:            "with host and no port",
			cluster:         createCluster("test1", "server1", 0),
			expectErr:       false,
			expectedAddress: "server1",
		},
		{
			name:            "with port and no host",
			cluster:         createCluster("test1", "", 9445),
			expectErr:       true,
			expectedAddress: "",
		},
		{
			name:            "with no port and no host",
			cluster:         createCluster("test1", "", 0),
			expectErr:       true,
			expectedAddress: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			regMethod, err := registration.NewRegistrationMethod(string(controlplanev1.RegistrationMethodControlPlaneEndpoint))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(regMethod).NotTo(BeNil())

			machines := []*clusterv1.Machine{}
			rcp := createControlPlane(string(controlplanev1.RegistrationMethodControlPlaneEndpoint), "")
			col := collections.FromMachines(machines...)

			actualAddresses, err := regMethod(tc.cluster, rcp, col)
			if tc.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			expectedAddresses := []string{tc.expectedAddress}
			g.Expect(len(actualAddresses)).To(Equal(len(expectedAddresses)))
			g.Expect(actualAddresses).To(ContainElements(expectedAddresses))
		})
	}
}

func createControlPlane(registrationMethod, registrationAddress string) *controlplanev1.RKE2ControlPlane {
	return &controlplanev1.RKE2ControlPlane{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: controlplanev1.RKE2ControlPlaneSpec{
			RegistrationMethod:  controlplanev1.RegistrationMethod(registrationMethod),
			RegistrationAddress: registrationAddress,
		},
		Status: controlplanev1.RKE2ControlPlaneStatus{},
	}
}

func createCluster(name string, cpHost string, cpPort int) *clusterv1.Cluster {
	cluster := &clusterv1.Cluster{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec:   clusterv1.ClusterSpec{},
		Status: clusterv1.ClusterStatus{},
	}

	if cpHost == "" && cpPort == 0 {
		return cluster
	}

	cluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
		Host: cpHost,
		Port: int32(cpPort),
	}

	return cluster
}

func createMachine(name string, internalIPs []string, externalIPs []string) *clusterv1.Machine {
	machine := &clusterv1.Machine{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: clusterv1.MachineSpec{},
		Status: clusterv1.MachineStatus{
			Addresses: clusterv1.MachineAddresses{},
		},
	}

	for _, internalIP := range internalIPs {
		machine.Status.Addresses = append(machine.Status.Addresses, clusterv1.MachineAddress{
			Type:    clusterv1.MachineInternalIP,
			Address: internalIP,
		})
	}

	for _, externalIP := range externalIPs {
		machine.Status.Addresses = append(machine.Status.Addresses, clusterv1.MachineAddress{
			Type:    clusterv1.MachineExternalIP,
			Address: externalIP,
		})
	}

	return machine
}

func strEquals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
