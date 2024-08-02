//go:build e2e
// +build e2e

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
package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"

	controlplanev1alpha1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1alpha1"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	bsutil "github.com/rancher/cluster-api-provider-rke2/pkg/util"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: the code in this file is largely copied from the cluster-api test framework with
// modifications so that Kubeadm Control Plane isn't used.
// Source: sigs.k8s.io/cluster-api/test/framework/*

const (
	retryableOperationInterval = 3 * time.Second
	retryableOperationTimeout  = 3 * time.Minute
)

// ApplyClusterTemplateAndWaitInput is the input type for ApplyClusterTemplateAndWait.
type ApplyClusterTemplateAndWaitInput struct {
	ClusterProxy                   framework.ClusterProxy
	ConfigCluster                  clusterctl.ConfigClusterInput
	WaitForClusterIntervals        []interface{}
	WaitForControlPlaneIntervals   []interface{}
	WaitForMachineDeployments      []interface{}
	Args                           []string // extra args to be used during `kubectl apply`
	Legacy                         bool     // using v1alpha1 crds version or not
	PreWaitForCluster              func()
	PostMachinesProvisioned        func()
	WaitForControlPlaneInitialized Waiter
}

// Waiter is a function that runs and waits for a long-running operation to finish and updates the result.
type Waiter func(ctx context.Context, input ApplyClusterTemplateAndWaitInput, result *ApplyClusterTemplateAndWaitResult)

// ApplyClusterTemplateAndWaitResult is the output type for ApplyClusterTemplateAndWait.
type ApplyClusterTemplateAndWaitResult struct {
	ClusterClass       *clusterv1.ClusterClass
	Cluster            *clusterv1.Cluster
	ControlPlane       *controlplanev1.RKE2ControlPlane
	LegacyControlPlane *controlplanev1alpha1.RKE2ControlPlane
	MachineDeployments []*clusterv1.MachineDeployment
}

// ApplyClusterTemplateAndWait gets a managed cluster template using clusterctl, and waits for the cluster to be ready.
// Important! this method assumes the cluster uses a RKE2ControlPlane and MachineDeployment.
func ApplyClusterTemplateAndWait(ctx context.Context, input ApplyClusterTemplateAndWaitInput, result *ApplyClusterTemplateAndWaitResult) {
	setDefaults(&input)
	Expect(ctx).NotTo(BeNil(), "ctx is required for ApplyClusterTemplateAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ApplyManagedClusterTemplateAndWait")
	Expect(result).ToNot(BeNil(), "Invalid argument. result can't be nil when calling ApplyClusterTemplateAndWait")
	Expect(input.ConfigCluster.Flavor).ToNot(BeEmpty(), "Invalid argument. input.ConfigCluster.Flavor can't be empty")
	Expect(input.ConfigCluster.ControlPlaneMachineCount).ToNot(BeNil())
	Expect(input.ConfigCluster.WorkerMachineCount).ToNot(BeNil())

	Byf("Creating the RKE2 based workload cluster with name %q using the %q template (Kubernetes %s)",
		input.ConfigCluster.ClusterName, input.ConfigCluster.Flavor, input.ConfigCluster.KubernetesVersion)

	By("Getting the cluster template yaml")
	workloadClusterTemplate := clusterctl.ConfigCluster(ctx, clusterctl.ConfigClusterInput{
		// pass reference to the management cluster hosting this test
		KubeconfigPath: input.ConfigCluster.KubeconfigPath,
		// pass the clusterctl config file that points to the local provider repository created for this test,
		ClusterctlConfigPath: input.ConfigCluster.ClusterctlConfigPath,
		// select template
		Flavor: input.ConfigCluster.Flavor,
		// define template variables
		Namespace:                input.ConfigCluster.Namespace,
		ClusterName:              input.ConfigCluster.ClusterName,
		KubernetesVersion:        input.ConfigCluster.KubernetesVersion,
		ControlPlaneMachineCount: input.ConfigCluster.ControlPlaneMachineCount,
		WorkerMachineCount:       input.ConfigCluster.WorkerMachineCount,
		InfrastructureProvider:   input.ConfigCluster.InfrastructureProvider,
		// setup clusterctl logs folder
		LogFolder:           input.ConfigCluster.LogFolder,
		ClusterctlVariables: input.ConfigCluster.ClusterctlVariables,
	})
	Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

	By("Applying the cluster template yaml to the cluster")
	Eventually(func() error {
		return input.ClusterProxy.Apply(ctx, workloadClusterTemplate, input.Args...)
	}, 10*time.Second).Should(Succeed(), "Failed to apply the cluster template")

	// Once we applied the cluster template we can run PreWaitForCluster.
	// Note: This can e.g. be used to verify the BeforeClusterCreate lifecycle hook is executed
	// and blocking correctly.
	if input.PreWaitForCluster != nil {
		By("Calling PreWaitForCluster")
		input.PreWaitForCluster()
	}

	By("Waiting for the cluster infrastructure to be provisioned")
	result.Cluster = framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
		Getter:    input.ClusterProxy.GetClient(),
		Namespace: input.ConfigCluster.Namespace,
		Name:      input.ConfigCluster.ClusterName,
	}, input.WaitForClusterIntervals...)

	By("Waiting for RKE2 control plane to be initialized")
	input.WaitForControlPlaneInitialized(ctx, input, result)

	Byf("Waiting for the machine deployments to be provisioned")
	result.MachineDeployments = DiscoveryAndWaitForMachineDeployments(ctx, framework.DiscoveryAndWaitForMachineDeploymentsInput{
		Lister:  input.ClusterProxy.GetClient(),
		Cluster: result.Cluster,
	}, input.WaitForMachineDeployments...)

	if input.PostMachinesProvisioned != nil {
		By("Calling PostMachinesProvisioned")
		input.PostMachinesProvisioned()
	}
}

// DiscoveryAndWaitForMachineDeployments discovers the MachineDeployments existing in a cluster and waits for them to be ready (all the machine provisioned).
func DiscoveryAndWaitForMachineDeployments(ctx context.Context, input framework.DiscoveryAndWaitForMachineDeploymentsInput, intervals ...interface{}) []*clusterv1.MachineDeployment {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoveryAndWaitForMachineDeployments")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling DiscoveryAndWaitForMachineDeployments")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DiscoveryAndWaitForMachineDeployments")

	machineDeployments := framework.GetMachineDeploymentsByCluster(ctx, framework.GetMachineDeploymentsByClusterInput{
		Lister:      input.Lister,
		ClusterName: input.Cluster.Name,
		Namespace:   input.Cluster.Namespace,
	})

	for _, deployment := range machineDeployments {
		framework.AssertMachineDeploymentFailureDomains(ctx, framework.AssertMachineDeploymentFailureDomainsInput{
			Lister:            input.Lister,
			Cluster:           input.Cluster,
			MachineDeployment: deployment,
		})
	}

	Eventually(func(g Gomega) {
		machineDeployments := framework.GetMachineDeploymentsByCluster(ctx, framework.GetMachineDeploymentsByClusterInput{
			Lister:      input.Lister,
			ClusterName: input.Cluster.Name,
			Namespace:   input.Cluster.Namespace,
		})
		for _, deployment := range machineDeployments {
			g.Expect(*deployment.Spec.Replicas).To(BeEquivalentTo(deployment.Status.ReadyReplicas))
		}
	}, intervals...).Should(Succeed())

	return machineDeployments
}

func SetControllerVersionAndWait(ctx context.Context, proxy framework.ClusterProxy, version string) {
	cp := &v1.Deployment{}
	Expect(proxy.GetClient().Get(ctx, types.NamespacedName{
		Name:      "rke2-control-plane-controller-manager",
		Namespace: "rke2-control-plane-system",
	}, cp)).ToNot(HaveOccurred())
	cpImage := strings.Split(cp.Spec.Template.Spec.Containers[0].Image, ":")
	cp.Spec.Template.Spec.Containers[0].Image = cpImage[0] + ":" + version
	Expect(proxy.GetClient().Update(ctx, cp)).ToNot(HaveOccurred())
	Eventually(func(g Gomega) {
		g.Expect(proxy.GetClient().Get(ctx, client.ObjectKeyFromObject(cp), cp)).To(Succeed())
		g.Expect(cp.Status.ReadyReplicas).To(Equal(*cp.Spec.Replicas))
		g.Expect(cp.Status.ReadyReplicas).To(Equal(cp.Status.UpdatedReplicas))
		g.Expect(cp.Status.ReadyReplicas).To(Equal(cp.Status.AvailableReplicas))
	}).Should(Succeed())

	bs := &v1.Deployment{}
	Expect(proxy.GetClient().Get(ctx, types.NamespacedName{
		Name:      "rke2-bootstrap-controller-manager",
		Namespace: "rke2-bootstrap-system",
	}, bs)).ToNot(HaveOccurred())
	bsImage := strings.Split(bs.Spec.Template.Spec.Containers[0].Image, ":")
	bs.Spec.Template.Spec.Containers[0].Image = bsImage[0] + ":" + version
	Expect(proxy.GetClient().Update(ctx, bs)).ToNot(HaveOccurred())
	Eventually(func(g Gomega) {
		g.Expect(proxy.GetClient().Get(ctx, client.ObjectKeyFromObject(bs), bs)).To(Succeed())
		g.Expect(bs.Status.ReadyReplicas).To(Equal(*bs.Spec.Replicas))
		g.Expect(bs.Status.ReadyReplicas).To(Equal(bs.Status.UpdatedReplicas))
		g.Expect(bs.Status.ReadyReplicas).To(Equal(bs.Status.AvailableReplicas))
	}).Should(Succeed())
}

// DiscoveryAndWaitForRKE2ControlPlaneInitializedInput is the input type for DiscoveryAndWaitForRKE2ControlPlaneInitialized.
type DiscoveryAndWaitForRKE2ControlPlaneInitializedInput struct {
	Lister  framework.Lister
	Cluster *clusterv1.Cluster
}

// DiscoveryAndWaitForRKE2ControlPlaneInitialized discovers the RKE2 object attached to a cluster and waits for it to be initialized.
func DiscoveryAndWaitForRKE2ControlPlaneInitialized(ctx context.Context, input DiscoveryAndWaitForRKE2ControlPlaneInitializedInput, intervals ...interface{}) *controlplanev1.RKE2ControlPlane {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoveryAndWaitForRKE2ControlPlaneInitialized")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling DiscoveryAndWaitForRKE2ControlPlaneInitialized")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DiscoveryAndWaitForRKE2ControlPlaneInitialized")

	By("Getting RKE2ControlPlane control plane")

	var controlPlane *controlplanev1.RKE2ControlPlane
	Eventually(func(g Gomega) {
		controlPlane = GetRKE2ControlPlaneByCluster(ctx, GetRKE2ControlPlaneByClusterInput{
			Lister:      input.Lister,
			ClusterName: input.Cluster.Name,
			Namespace:   input.Cluster.Namespace,
		})
		g.Expect(controlPlane).ToNot(BeNil())
	}, "10s", "1s").Should(Succeed(), "Couldn't get the control plane for the cluster %s", klog.KObj(input.Cluster))

	return controlPlane
}

// DiscoveryAndWaitForLegacyRKE2ControlPlaneInitialized discovers the RKE2 object attached to a cluster and waits for it to be initialized.
func DiscoveryAndWaitForLegacyRKE2ControlPlaneInitialized(ctx context.Context, input DiscoveryAndWaitForRKE2ControlPlaneInitializedInput, intervals ...interface{}) *controlplanev1alpha1.RKE2ControlPlane {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DiscoveryAndWaitForRKE2ControlPlaneInitialized")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling DiscoveryAndWaitForRKE2ControlPlaneInitialized")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling DiscoveryAndWaitForRKE2ControlPlaneInitialized")

	By("Getting legacy RKE2ControlPlane control plane")

	var controlPlane *controlplanev1alpha1.RKE2ControlPlane
	Eventually(func(g Gomega) {
		controlPlane = GetLegacyRKE2ControlPlaneByCluster(ctx, GetRKE2ControlPlaneByClusterInput{
			Lister:      input.Lister,
			ClusterName: input.Cluster.Name,
			Namespace:   input.Cluster.Namespace,
		})
		g.Expect(controlPlane).ToNot(BeNil())
	}, "10s", "1s").Should(Succeed(), "Couldn't get the control plane for the cluster %s", klog.KObj(input.Cluster))

	return controlPlane
}

// GetRKE2ControlPlaneByClusterInput is the input for GetRKE2ControlPlaneByCluster.
type GetRKE2ControlPlaneByClusterInput struct {
	Lister      framework.Lister
	ClusterName string
	Namespace   string
}

// GetRKE2ControlPlaneByCluster returns the RKE2ControlPlane objects for a cluster.
func GetRKE2ControlPlaneByCluster(ctx context.Context, input GetRKE2ControlPlaneByClusterInput) *controlplanev1.RKE2ControlPlane {
	opts := []client.ListOption{
		client.InNamespace(input.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel: input.ClusterName,
		},
	}

	controlPlaneList := &controlplanev1.RKE2ControlPlaneList{}
	Eventually(func() error {
		return input.Lister.List(ctx, controlPlaneList, opts...)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list RKE2ControlPlane object for Cluster %s", klog.KRef(input.Namespace, input.ClusterName))
	Expect(len(controlPlaneList.Items)).ToNot(BeNumerically(">", 1), "Cluster %s should not have more than 1 RKE2ControlPlane object", klog.KRef(input.Namespace, input.ClusterName))
	if len(controlPlaneList.Items) == 1 {
		return &controlPlaneList.Items[0]
	}
	return nil
}

// GetLegacyRKE2ControlPlaneByCluster returns the RKE2ControlPlane objects for a cluster.
func GetLegacyRKE2ControlPlaneByCluster(ctx context.Context, input GetRKE2ControlPlaneByClusterInput) *controlplanev1alpha1.RKE2ControlPlane {
	opts := []client.ListOption{
		client.InNamespace(input.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel: input.ClusterName,
		},
	}

	controlPlaneList := &controlplanev1alpha1.RKE2ControlPlaneList{}
	Eventually(func() error {
		return input.Lister.List(ctx, controlPlaneList, opts...)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to list RKE2ControlPlane object for Cluster %s", klog.KRef(input.Namespace, input.ClusterName))
	Expect(len(controlPlaneList.Items)).ToNot(BeNumerically(">", 1), "Cluster %s should not have more than 1 RKE2ControlPlane object", klog.KRef(input.Namespace, input.ClusterName))
	if len(controlPlaneList.Items) == 1 {
		return &controlPlaneList.Items[0]
	}
	return nil
}

// WaitForControlPlaneToBeReadyInput is the input for WaitForControlPlaneToBeReady.
type WaitForControlPlaneToBeReadyInput struct {
	Getter             framework.Getter
	ControlPlane       types.NamespacedName
	LegacyControlPlane *controlplanev1alpha1.RKE2ControlPlane
}

// WaitForControlPlaneToBeReady will wait for a control plane to be ready.
func WaitForControlPlaneToBeReady(ctx context.Context, input WaitForControlPlaneToBeReadyInput, intervals ...interface{}) {
	By("Waiting for the control plane to be ready")
	controlplane := &controlplanev1.RKE2ControlPlane{}
	Eventually(func() (bool, error) {
		key := client.ObjectKey{
			Namespace: input.ControlPlane.Namespace,
			Name:      input.ControlPlane.Name,
		}
		if err := input.Getter.Get(ctx, key, controlplane); err != nil {
			return false, errors.Wrapf(err, "failed to get RKE2 control plane")
		}

		desiredReplicas := controlplane.Spec.Replicas
		statusReplicas := controlplane.Status.Replicas
		updatedReplicas := controlplane.Status.UpdatedReplicas
		readyReplicas := controlplane.Status.ReadyReplicas
		unavailableReplicas := controlplane.Status.UnavailableReplicas

		// Control plane is still rolling out (and thus not ready) if:
		// * .spec.replicas, .status.replicas, .status.updatedReplicas,
		//   .status.readyReplicas are not equal and
		// * unavailableReplicas > 0
		if statusReplicas != *desiredReplicas ||
			updatedReplicas != *desiredReplicas ||
			readyReplicas != *desiredReplicas ||
			unavailableReplicas > 0 {
			return false, nil
		}

		return true, nil
	}, intervals...).Should(BeTrue(), framework.PrettyPrint(controlplane)+"\n")
}

// WaitForLegacyControlPlaneToBeReady will wait for a control plane to be ready.
func WaitForLegacyControlPlaneToBeReady(ctx context.Context, input WaitForControlPlaneToBeReadyInput, intervals ...interface{}) {
	By("Waiting for the control plane to be ready")
	controlplane := &controlplanev1alpha1.RKE2ControlPlane{}
	Eventually(func() (bool, error) {
		key := client.ObjectKey{
			Namespace: input.ControlPlane.Namespace,
			Name:      input.ControlPlane.Name,
		}
		if err := input.Getter.Get(ctx, key, controlplane); err != nil {
			return false, errors.Wrapf(err, "failed to get RKE2 control plane")
		}

		desiredReplicas := controlplane.Spec.Replicas
		statusReplicas := controlplane.Status.Replicas
		updatedReplicas := controlplane.Status.UpdatedReplicas
		readyReplicas := controlplane.Status.ReadyReplicas
		unavailableReplicas := controlplane.Status.UnavailableReplicas

		// Control plane is still rolling out (and thus not ready) if:
		// * .spec.replicas, .status.replicas, .status.updatedReplicas,
		//   .status.readyReplicas are not equal and
		// * unavailableReplicas > 0
		if statusReplicas != *desiredReplicas ||
			updatedReplicas != *desiredReplicas ||
			readyReplicas != *desiredReplicas ||
			unavailableReplicas > 0 {
			return false, nil
		}

		return true, nil
	}, intervals...).Should(BeTrue(), framework.PrettyPrint(controlplane)+"\n")
}

type WaitForMachineConditionsInput struct {
	Getter    framework.Getter
	Machine   *clusterv1.Machine
	Checker   func(_ conditions.Getter, _ clusterv1.ConditionType) bool
	Condition clusterv1.ConditionType
}

func WaitForMachineConditions(ctx context.Context, input WaitForMachineConditionsInput, intervals ...interface{}) {
	Eventually(func() (bool, error) {
		if err := input.Getter.Get(ctx, client.ObjectKeyFromObject(input.Machine), input.Machine); err != nil {
			return false, errors.Wrapf(err, "failed to get machine")
		}

		return input.Checker(input.Machine, input.Condition), nil
	}, intervals...).Should(BeTrue(), framework.PrettyPrint(input.Machine)+"\n")
}

// WaitForClusterToUpgradeInput is the input for WaitForClusterToUpgrade.
type WaitForClusterToUpgradeInput struct {
	Lister              framework.Lister
	ControlPlane        *controlplanev1.RKE2ControlPlane
	LegacyControlPlane  *controlplanev1alpha1.RKE2ControlPlane
	MachineDeployments  []*clusterv1.MachineDeployment
	VersionAfterUpgrade string
}

// WaitForClusterToUpgrade will wait for a cluster to be upgraded.
func WaitForClusterToUpgrade(ctx context.Context, input WaitForClusterToUpgradeInput, intervals ...interface{}) {
	By("Waiting for machines to update")

	var totalMachineCount int32
	if input.ControlPlane != nil {
		totalMachineCount = *input.ControlPlane.Spec.Replicas
	} else {
		totalMachineCount = *input.LegacyControlPlane.Spec.Replicas
	}
	for _, md := range input.MachineDeployments {
		totalMachineCount += *md.Spec.Replicas
	}

	Eventually(func() (bool, error) {
		machineList := &clusterv1.MachineList{}
		if err := input.Lister.List(ctx, machineList); err != nil {
			return false, fmt.Errorf("failed to list machines: %w", err)
		}

		if len(machineList.Items) != int(totalMachineCount) { // not all replicas are created
			return false, nil
		}

		for _, machine := range machineList.Items {
			expectedVersion := input.VersionAfterUpgrade
			if bsutil.IsRKE2Version(*machine.Spec.Version) {
				expectedVersion = input.VersionAfterUpgrade + "+rke2r1"
			}

			if machine.Spec.Version != nil && *machine.Spec.Version != expectedVersion {
				return false, nil
			}
		}

		return true, nil
	}, intervals...).Should(BeTrue(), framework.PrettyPrint(input.ControlPlane))
}

func setDefaults(input *ApplyClusterTemplateAndWaitInput) {
	if input.WaitForControlPlaneInitialized == nil {
		if !input.Legacy {
			input.WaitForControlPlaneInitialized = func(ctx context.Context, input ApplyClusterTemplateAndWaitInput, result *ApplyClusterTemplateAndWaitResult) {
				result.ControlPlane = DiscoveryAndWaitForRKE2ControlPlaneInitialized(ctx, DiscoveryAndWaitForRKE2ControlPlaneInitializedInput{
					Lister:  input.ClusterProxy.GetClient(),
					Cluster: result.Cluster,
				}, input.WaitForControlPlaneIntervals...)
			}
		} else {
			input.WaitForControlPlaneInitialized = func(ctx context.Context, input ApplyClusterTemplateAndWaitInput, result *ApplyClusterTemplateAndWaitResult) {
				result.LegacyControlPlane = DiscoveryAndWaitForLegacyRKE2ControlPlaneInitialized(ctx, DiscoveryAndWaitForRKE2ControlPlaneInitializedInput{
					Lister:  input.ClusterProxy.GetClient(),
					Cluster: result.Cluster,
				}, input.WaitForControlPlaneIntervals...)
			}
		}
	}
}

var secrets = []string{}

func CollectArtifacts(ctx context.Context, kubeconfigPath, name string, args ...string) error {
	if kubeconfigPath == "" {
		return fmt.Errorf("Unable to collect artifacts: kubeconfig path is empty")
	}

	aargs := append([]string{"crust-gather", "collect", "--kubeconfig", kubeconfigPath, "-v", "ERROR", "-f", name}, args...)
	for _, secret := range secrets {
		aargs = append(aargs, "-s", secret)
	}

	cmd := exec.Command("kubectl", aargs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Minute

	fmt.Printf("Running kubectl %s\n", strings.Join(aargs, " "))
	err := cmd.Run()
	fmt.Printf("stderr:\n%s\n", string(stderr.Bytes()))
	fmt.Printf("stdout:\n%s\n", string(stdout.Bytes()))
	return err
}
