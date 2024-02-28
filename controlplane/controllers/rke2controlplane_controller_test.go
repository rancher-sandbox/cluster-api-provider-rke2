package controllers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1beta1"

	// "github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/kubeconfig"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/rke2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Reconclie control plane conditions", func() {
	var (
		err            error
		cp             *rke2.ControlPlane
		rcp            *controlplanev1.RKE2ControlPlane
		ns             *corev1.Namespace
		nodeName       = "node1"
		node           *corev1.Node
		nodeByRef      *corev1.Node
		orphanedNode   *corev1.Node
		machine        *clusterv1.Machine
		machineWithRef *clusterv1.Machine
		config         *bootstrapv1.RKE2Config
	)

	BeforeEach(func() {
		ns, err = testEnv.CreateNamespace(ctx, "ns")
		Expect(err).ToNot(HaveOccurred())

		annotations := map[string]string{
			"test": "true",
		}

		config = &bootstrapv1.RKE2Config{ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: ns.Name,
		}, Spec: bootstrapv1.RKE2ConfigSpec{
			AgentConfig: bootstrapv1.RKE2AgentConfig{
				NodeAnnotations: annotations,
			},
		}}
		Expect(testEnv.Create(ctx, config)).To(Succeed())

		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Labels: map[string]string{
					"node-role.kubernetes.io/master": "true",
				},
				Annotations: map[string]string{
					clusterv1.MachineAnnotation: nodeName,
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				}},
			},
		}
		Expect(testEnv.Create(ctx, node.DeepCopy())).To(Succeed())
		Eventually(testEnv.Get(ctx, client.ObjectKeyFromObject(node), node)).Should(Succeed())
		Expect(testEnv.Status().Update(ctx, node.DeepCopy())).To(Succeed())

		nodeRefName := "ref-node"
		machineWithRef = &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "machine-with-ref",
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
			},
			Status: clusterv1.MachineStatus{
				NodeRef: &corev1.ObjectReference{
					Kind:       "Node",
					APIVersion: "v1",
					Name:       nodeRefName,
				},
				Conditions: clusterv1.Conditions{
					clusterv1.Condition{
						Type:               clusterv1.ReadyCondition,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				},
			},
		}

		ml := clusterv1.MachineList{Items: []clusterv1.Machine{*machineWithRef.DeepCopy()}}
		updatedMachine := machineWithRef.DeepCopy()
		Expect(testEnv.Create(ctx, updatedMachine)).To(Succeed())
		updatedMachine.Status = *machineWithRef.Status.DeepCopy()
		machineWithRef = updatedMachine.DeepCopy()
		Expect(testEnv.Status().Update(ctx, machineWithRef)).To(Succeed())

		nodeByRef = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeRefName,
				Labels: map[string]string{
					"node-role.kubernetes.io/master": "true",
				},
				Annotations: map[string]string{
					clusterv1.MachineAnnotation: machineWithRef.Name,
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				}},
			},
		}
		Expect(testEnv.Create(ctx, nodeByRef.DeepCopy())).To(Succeed())
		Eventually(testEnv.Get(ctx, client.ObjectKeyFromObject(nodeByRef), nodeByRef)).Should(Succeed())
		Expect(testEnv.Status().Update(ctx, nodeByRef.DeepCopy())).To(Succeed())

		orphanedNode = &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: "missing-machine",
			Labels: map[string]string{
				"node-role.kubernetes.io/master": "true",
			},
		}}
		Expect(testEnv.Create(ctx, orphanedNode)).To(Succeed())

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
			},
			Status: clusterv1.MachineStatus{
				NodeRef: &corev1.ObjectReference{
					Kind:      "Node",
					Name:      nodeName,
					UID:       node.GetUID(),
					Namespace: "",
				},
				Conditions: clusterv1.Conditions{
					clusterv1.Condition{
						Type:               clusterv1.ReadyCondition,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.Now(),
					},
				},
			},
		}

		ml.Items = append(ml.Items, *machine.DeepCopy())
		updatedMachine = machine.DeepCopy()
		Expect(testEnv.Create(ctx, updatedMachine)).To(Succeed())
		updatedMachine.Status = *machine.Status.DeepCopy()
		machine = updatedMachine.DeepCopy()
		Expect(testEnv.Status().Update(ctx, machine)).To(Succeed())

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: ns.Name,
			},
		}
		Expect(testEnv.Client.Create(ctx, cluster)).To(Succeed())

		rcp = &controlplanev1.RKE2ControlPlane{
			Status: controlplanev1.RKE2ControlPlaneStatus{
				Initialized: true,
			},
		}

		cp, err = rke2.NewControlPlane(ctx, testEnv.GetClient(), cluster, rcp, collections.FromMachineList(&ml))
		Expect(err).ToNot(HaveOccurred())

		ref := metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       clusterv1.ClusterKind,
			UID:        cp.Cluster.GetUID(),
			Name:       cp.Cluster.GetName(),
		}
		Expect(testEnv.Client.Create(ctx, kubeconfig.GenerateSecretWithOwner(
			client.ObjectKeyFromObject(cp.Cluster),
			kubeconfig.FromEnvTestConfig(testEnv.Config, cp.Cluster),
			ref))).To(Succeed())
	})

	AfterEach(func() {
		Expect(testEnv.DeleteAllOf(ctx, node)).To(Succeed())
		testEnv.Cleanup(ctx, node, ns)
	})

	It("should reconcile cp and machine conditions successfully", func() {
		r := &RKE2ControlPlaneReconciler{
			Client:                    testEnv.GetClient(),
			Scheme:                    testEnv.GetScheme(),
			managementCluster:         &rke2.Management{Client: testEnv.GetClient()},
			managementClusterUncached: &rke2.Management{Client: testEnv.GetClient()},
		}
		_, err := r.reconcileControlPlaneConditions(ctx, cp)
		Expect(err).ToNot(HaveOccurred())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(machine), machine)).To(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(machineWithRef), machineWithRef)).To(Succeed())
		Expect(conditions.IsTrue(machine, controlplanev1.NodeMetadataUpToDate)).To(BeTrue())
		Expect(conditions.IsTrue(machineWithRef, controlplanev1.NodeMetadataUpToDate)).To(BeTrue())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(node), node)).To(Succeed())
		Expect(testEnv.Get(ctx, client.ObjectKeyFromObject(nodeByRef), nodeByRef)).To(Succeed())
		Expect(node.GetAnnotations()).To(HaveKeyWithValue("test", "true"))
		Expect(nodeByRef.GetAnnotations()).To(HaveKeyWithValue("test", "true"))
		Expect(conditions.IsFalse(rcp, controlplanev1.ControlPlaneComponentsHealthyCondition)).To(BeTrue())
		Expect(conditions.GetMessage(rcp, controlplanev1.ControlPlaneComponentsHealthyCondition)).To(Equal(
			"Control plane node missing-machine does not have a corresponding machine"))
	})
})
