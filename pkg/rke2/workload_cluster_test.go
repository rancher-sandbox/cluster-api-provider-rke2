package rke2

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Node metadata propagation", func() {
	var (
		err                  error
		ns                   *corev1.Namespace
		nodeName             = "node1"
		node                 *corev1.Node
		machine              *clusterv1.Machine
		machineDifferentNode *clusterv1.Machine
		machineNodeRefStatus clusterv1.MachineStatus
		config               *bootstrapv1.RKE2Config
	)

	BeforeEach(func() {
		ns, err = testEnv.CreateNamespace(ctx, "ns")
		Expect(err).ToNot(HaveOccurred())

		annotations := map[string]string{
			"test": "true",
		}
		node = &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Labels: map[string]string{
				"node-role.kubernetes.io/master": "true",
			},
			Annotations: map[string]string{
				clusterv1.MachineAnnotation: nodeName,
			},
		}}

		config = &bootstrapv1.RKE2Config{ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: ns.Name,
		}, Spec: bootstrapv1.RKE2ConfigSpec{
			AgentConfig: bootstrapv1.RKE2AgentConfig{
				NodeAnnotations: annotations,
			},
		}}

		machine = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nodeName,
				Namespace: ns.Name,
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "cluster",
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "RKE2Config",
						APIVersion: bootstrapv1.GroupVersion.String(),
						Name:       config.Name,
						Namespace:  config.Namespace,
					},
				},
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "Pod",
					APIVersion: "v1",
					Name:       "stub",
					Namespace:  ns.Name,
				},
			}}

		machineDifferentNode = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-machine",
				Namespace: ns.Name,
			},
			Spec: clusterv1.MachineSpec{
				ClusterName: "cluster",
				Bootstrap: clusterv1.Bootstrap{
					ConfigRef: &corev1.ObjectReference{
						Kind:       "RKE2Config",
						APIVersion: bootstrapv1.GroupVersion.String(),
						Name:       config.Name,
						Namespace:  config.Namespace,
					},
				},
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "Pod",
					APIVersion: "v1",
					Name:       "stub",
					Namespace:  ns.Name,
				},
			}}

		machineNodeRefStatus = clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Kind:       "Node",
				APIVersion: "v1",
				Name:       nodeName,
			},
		}
	})

	AfterEach(func() {
		testEnv.Cleanup(ctx, node, ns)
	})

	It("should report the missing node", func() {
		Expect(testEnv.Create(ctx, config)).To(Succeed())
		Expect(testEnv.Create(ctx, machine)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machine,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), types.NamespacedName{})
		Expect(err).ToNot(HaveOccurred())

		cp, err := NewControlPlane(ctx, testEnv.GetClient(), nil, nil, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(0))
		Expect(cp.rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Status", Equal(corev1.ConditionUnknown),
		))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Message", Equal("associated node not found"),
		))
	})

	It("should report the missing config", func() {
		Expect(testEnv.Create(ctx, node)).To(Succeed())
		Expect(testEnv.Create(ctx, machine)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machine,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), types.NamespacedName{})
		Expect(err).ToNot(HaveOccurred())
		cp, err := NewControlPlane(ctx, testEnv.GetClient(), nil, nil, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(1))
		Expect(cp.rke2Configs).To(HaveLen(0))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Status", Equal(corev1.ConditionUnknown),
		))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Message", Equal("associated RKE2 config not found"),
		))
	})

	It("should recover from error conditions on successfull node patch", func() {
		Expect(testEnv.Create(ctx, config)).To(Succeed())
		Expect(testEnv.Create(ctx, machine)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machine,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), types.NamespacedName{})
		Expect(err).ToNot(HaveOccurred())
		cp, err := NewControlPlane(ctx, testEnv.GetClient(), nil, nil, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(0))
		Expect(cp.rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Status", Equal(corev1.ConditionUnknown),
		))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Message", Equal("associated node not found"),
		))

		Expect(testEnv.Create(ctx, node)).To(Succeed())
		Eventually(ctx, func() map[string]*corev1.Node {
			Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
			return w.Nodes
		}).Should(HaveLen(1))
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(cp.rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Status", Equal(corev1.ConditionTrue),
		))
		Expect(w.Nodes[nodeName].GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: nodeName,
		}))
	})

	It("should set the node annotations", func() {
		Expect(testEnv.Create(ctx, node)).To(Succeed())
		Expect(testEnv.Create(ctx, config)).To(Succeed())
		Expect(testEnv.Create(ctx, machine)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machine,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), types.NamespacedName{})
		Expect(err).ToNot(HaveOccurred())
		cp, err := NewControlPlane(ctx, testEnv.GetClient(), nil, nil, machines)
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(err).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(1))
		Expect(cp.rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machine.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Status", Equal(corev1.ConditionTrue),
		))
		Expect(w.Nodes[nodeName].GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: nodeName,
		}))

		result := &corev1.Node{}
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), result)).To(Succeed())
		Expect(result.GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: nodeName,
		}))
	})

	It("should set the node annotations for an arbitrary node reference", func() {
		node.SetAnnotations(map[string]string{
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		})
		Expect(testEnv.Create(ctx, node)).To(Succeed())
		Expect(testEnv.Create(ctx, config)).To(Succeed())
		Expect(testEnv.Create(ctx, machineDifferentNode)).To(Succeed())
		machineDifferentNode.Status = machineNodeRefStatus
		Expect(testEnv.Status().Update(ctx, machineDifferentNode)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machineDifferentNode,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), types.NamespacedName{})
		Expect(err).ToNot(HaveOccurred())
		cp, err := NewControlPlane(ctx, testEnv.GetClient(), nil, nil, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(1))
		Expect(cp.rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machineDifferentNode.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Status", Equal(corev1.ConditionTrue),
		))
		Expect(w.Nodes[nodeName].GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		}))

		result := &corev1.Node{}
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), result)).To(Succeed())
		Expect(result.GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		}))
	})

	It("should recover from error condition on successfull node patch for arbitrary node name", func() {
		node.SetAnnotations(map[string]string{
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		})
		Expect(testEnv.Create(ctx, node)).To(Succeed())
		Expect(testEnv.Create(ctx, config)).To(Succeed())
		Expect(testEnv.Create(ctx, machineDifferentNode)).To(Succeed())

		machines := collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machineDifferentNode,
		}})

		m := &Management{
			Client:              testEnv,
			SecretCachingClient: testEnv,
		}

		w, err := m.NewWorkload(ctx, testEnv.GetClient(), testEnv.GetConfig(), types.NamespacedName{})
		Expect(err).ToNot(HaveOccurred())
		cp, err := NewControlPlane(ctx, testEnv.GetClient(), nil, nil, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.Nodes).To(HaveLen(1))
		Expect(cp.rke2Configs).To(HaveLen(1))
		Expect(cp.Machines).To(HaveLen(1))
		Expect(conditions.Get(cp.Machines[machineDifferentNode.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Status", Equal(corev1.ConditionUnknown),
		))
		Expect(conditions.Get(cp.Machines[machineDifferentNode.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Message", Equal("associated node not found"),
		))

		machineDifferentNode.Status = machineNodeRefStatus
		Expect(testEnv.Status().Update(ctx, machineDifferentNode)).To(Succeed())

		machines = collections.FromMachineList(&clusterv1.MachineList{Items: []clusterv1.Machine{
			*machineDifferentNode,
		}})
		cp, err = NewControlPlane(ctx, testEnv.GetClient(), nil, nil, machines)
		Expect(err).ToNot(HaveOccurred())
		Expect(w.InitWorkload(ctx, cp)).ToNot(HaveOccurred())
		Expect(w.UpdateNodeMetadata(ctx, cp)).ToNot(HaveOccurred())

		Expect(conditions.Get(cp.Machines[machineDifferentNode.Name], controlplanev1.NodeMetadataUpToDate)).To(HaveField(
			"Status", Equal(corev1.ConditionTrue),
		))
		Expect(w.Nodes[nodeName].GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		}))

		result := &corev1.Node{}
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), result)).To(Succeed())
		Expect(result.GetAnnotations()).To(Equal(map[string]string{
			"test":                      "true",
			clusterv1.MachineAnnotation: machineDifferentNode.Name,
		}))
	})

})

var _ = Describe("Cloud-init fields validation", func() {
	var (
		err error
		ns  *corev1.Namespace
	)

	BeforeEach(func() {
		ns, err = testEnv.CreateNamespace(ctx, "ns")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		testEnv.Cleanup(ctx, ns)
	})

	It("should prevent populating config and data fields", func() {
		Expect(testEnv.Create(ctx, &bootstrapv1.RKE2Config{ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: ns.Name,
		}, Spec: bootstrapv1.RKE2ConfigSpec{
			AgentConfig: bootstrapv1.RKE2AgentConfig{
				AdditionalUserData: bootstrapv1.AdditionalUserData{
					Config: "some",
					Data: map[string]string{
						"no": "way",
					},
				},
			},
		}})).ToNot(Succeed())
	})
})
