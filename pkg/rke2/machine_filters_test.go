package rke2

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
)

var (
	k8sMachineVersion  = "v1.24.6"
	rke2MachineVersion = "v1.24.6+rke2r1"
	regionEuCentral1   = "eu-central-1"
)

var rcp = controlplanev1.RKE2ControlPlane{
	ObjectMeta: v1.ObjectMeta{
		Name:      "rke2-cluster-control-plane",
		Namespace: "example",
	},
	Spec: controlplanev1.RKE2ControlPlaneSpec{
		Version: rke2MachineVersion,
		ServerConfig: controlplanev1.RKE2ServerConfig{
			CNI:               "calico",
			CloudProviderName: "aws",
			ClusterDomain:     "example.com",
		},
		RKE2ConfigSpec: bootstrapv1.RKE2ConfigSpec{
			AgentConfig: bootstrapv1.RKE2AgentConfig{
				NodeLabels: []string{"hello=world"},
			},
		},
	},
}

var machine = clusterv1.Machine{
	ObjectMeta: v1.ObjectMeta{
		Name:      "machine-test",
		Namespace: "example",
		Annotations: map[string]string{
			controlplanev1.RKE2ServerConfigurationAnnotation: "{\"cni\":\"calico\",\"cloudProviderName\":\"aws\",\"clusterDomain\":\"example.com\"}",
		},
	},
	Spec: clusterv1.MachineSpec{
		ClusterName:   "rke2-cluster",
		Version:       &k8sMachineVersion,
		FailureDomain: &regionEuCentral1,
		Bootstrap: clusterv1.Bootstrap{
			ConfigRef: &corev1.ObjectReference{
				Kind:       "RKE2ConfigTemplate",
				Namespace:  "example",
				Name:       "rke2-cluster-config-template",
				APIVersion: bootstrapv1.GroupVersion.Version,
			},
		},
	},
}

var _ = Describe("ServerConfigMatching", func() {
	It("should match the machine annotation", func() {
		res := matchServerConfig(&rcp, &machine)
		Expect(res).To(BeTrue())
	})
})

var _ = Describe("matchAgentConfig", func() {
	It("should match Agent Config", func() {
		machineConfigs := map[string]*bootstrapv1.RKE2Config{
			"someMachine": {},
			"machine-test": {
				ObjectMeta: v1.ObjectMeta{
					Name:      "rke2-config-example",
					Namespace: "example",
				},
				Spec: bootstrapv1.RKE2ConfigSpec{
					AgentConfig: bootstrapv1.RKE2AgentConfig{
						NodeLabels: []string{"hello=world"},
					},
				},
			},
		}
		machineCollection := collections.FromMachines(&machine)
		Expect(len(machineCollection)).To(Equal(1))
		matches := machineCollection.AnyFilter(matchesRKE2BootstrapConfig(machineConfigs, &rcp))

		Expect(len(matches)).To(Equal(1))
		Expect(matches.Oldest().Name).To(Equal("machine-test"))
	},
	)

	It("shouldn't match Agent Config and different preBootstrapCommands", func() {
		machineConfigs := map[string]*bootstrapv1.RKE2Config{
			"someMachine": {},
			"machine-test": {
				ObjectMeta: v1.ObjectMeta{
					Name:      "rke2-config-example",
					Namespace: "example",
				},
				Spec: bootstrapv1.RKE2ConfigSpec{
					AgentConfig: bootstrapv1.RKE2AgentConfig{
						NodeLabels: []string{"hello=world"},
					},
					PreRKE2Commands: []string{"test"},
				},
			},
		}
		machineCollection := collections.FromMachines(&machine)
		Expect(len(machineCollection)).To(Equal(1))
		matches := machineCollection.AnyFilter(matchesRKE2BootstrapConfig(machineConfigs, &rcp))

		Expect(len(matches)).To(Equal(0))
	},
	)

	It("shouldn't match Agent Config and different postBootstrapCommands", func() {
		machineConfigs := map[string]*bootstrapv1.RKE2Config{
			"someMachine": {},
			"machine-test": {
				ObjectMeta: v1.ObjectMeta{
					Name:      "rke2-config-example",
					Namespace: "example",
				},
				Spec: bootstrapv1.RKE2ConfigSpec{
					AgentConfig: bootstrapv1.RKE2AgentConfig{
						NodeLabels: []string{"hello=world"},
					},
					PostRKE2Commands: []string{"test"},
				},
			},
		}
		machineCollection := collections.FromMachines(&machine)
		Expect(len(machineCollection)).To(Equal(1))
		matches := machineCollection.AnyFilter(matchesRKE2BootstrapConfig(machineConfigs, &rcp))

		Expect(len(matches)).To(Equal(0))
	},
	)
})

var _ = Describe("matching Kubernetes Version", func() {
	It("should match version", func() {
		machineCollection := collections.FromMachines(&machine)
		matches := machineCollection.AnyFilter(matchesKubernetesOrRKE2Version(rcp.GetDesiredVersion()))
		Expect(len(matches)).To(Equal(1))
	})

	It("should match when RKE2 version is set on the machine", func() {
		machine.Spec.Version = &rke2MachineVersion
		machineCollection := collections.FromMachines(&machine)
		matches := machineCollection.AnyFilter(matchesKubernetesOrRKE2Version(rcp.GetDesiredVersion()))
		Expect(len(matches)).To(Equal(1))
		machine.Spec.Version = &k8sMachineVersion
	})
})
