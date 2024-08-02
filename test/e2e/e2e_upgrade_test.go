//go:build e2e
// +build e2e

/*
Copyright 2024 SUSE.

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
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Workload cluster creation", func() {
	var (
		specName            = "create-workload-cluster"
		namespace           *corev1.Namespace
		cancelWatches       context.CancelFunc
		result              *ApplyClusterTemplateAndWaitResult
		clusterName         string
		clusterctlLogFolder string
	)

	BeforeEach(func() {
		Expect(e2eConfig).ToNot(BeNil(), "Invalid argument. e2eConfig can't be nil when calling %s spec", specName)
		Expect(clusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. clusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(bootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. bootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(artifactFolder, 0755)).To(Succeed(), "Invalid argument. artifactFolder can't be created for %s spec", specName)

		Expect(e2eConfig.Variables).To(HaveKey(KubernetesVersion))

		clusterName = fmt.Sprintf("caprke2-e2e-%s-upgrade", util.RandomString(6))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, bootstrapClusterProxy, artifactFolder)

		result = new(ApplyClusterTemplateAndWaitResult)

		// We need to override clusterctl apply log folder to avoid getting our credentials exposed.
		clusterctlLogFolder = filepath.Join(os.TempDir(), "clusters", bootstrapClusterProxy.GetName())
	})

	AfterEach(func() {
		err := CollectArtifacts(ctx, bootstrapClusterProxy.GetKubeconfigPath(), filepath.Join(artifactFolder, bootstrapClusterProxy.GetName(), clusterName+specName))
		Expect(err).ToNot(HaveOccurred())

		cleanInput := cleanupInput{
			SpecName:          specName,
			Cluster:           result.Cluster,
			ClusterProxy:      bootstrapClusterProxy,
			Namespace:         namespace,
			CancelWatches:     cancelWatches,
			IntervalsGetter:   e2eConfig.GetIntervals,
			SkipCleanup:       skipCleanup,
			ArtifactFolder:    artifactFolder,
			AdditionalCleanup: cleanupInstallation(ctx, clusterctlLogFolder, clusterctlConfigPath, bootstrapClusterProxy),
		}

		dumpSpecResourcesAndCleanup(ctx, cleanInput)
	})

	Context("Creating a single control-plane cluster", func() {
		It("Should create a cluster with legacy version and perform upgrade with the current version", func() {
			By("Installing legacy version")
			initLegacyBootstrapCluster(bootstrapClusterProxy, e2eConfig, clusterctlConfigPath, artifactFolder)

			By("Initializing the cluster")
			ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
				Legacy:       true,
				ClusterProxy: bootstrapClusterProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                clusterctlLogFolder,
					ClusterctlConfigPath:     clusterctlConfigPath,
					KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
					InfrastructureProvider:   "docker",
					Flavor:                   "docker-legacy",
					Namespace:                namespace.Name,
					ClusterName:              clusterName,
					KubernetesVersion:        e2eConfig.GetVariable(KubernetesVersion),
					ControlPlaneMachineCount: ptr.To(int64(3)),
					WorkerMachineCount:       ptr.To(int64(1)),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForLegacyControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(result.LegacyControlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			By("Upgrading to v0.4.0 boostrap/controlplane provider version")
			clusterctl.UpgradeManagementClusterAndWait(ctx, clusterctl.UpgradeManagementClusterAndWaitInput{
				ClusterProxy:          bootstrapClusterProxy,
				ClusterctlConfigPath:  clusterctlConfigPath,
				BootstrapProviders:    []string{"rke2-bootstrap:v0.4.0"},
				ControlPlaneProviders: []string{"rke2-control-plane:v0.4.0"},
				LogFolder:             clusterctlLogFolder,
			}, e2eConfig.GetIntervals(specName, "wait-controllers")...)

			By("Upgrading to current boostrap/controlplane provider version")
			clusterctl.UpgradeManagementClusterAndWait(ctx, clusterctl.UpgradeManagementClusterAndWaitInput{
				ClusterProxy:          bootstrapClusterProxy,
				ClusterctlConfigPath:  clusterctlConfigPath,
				BootstrapProviders:    []string{"rke2-bootstrap:v0.5.99"},
				ControlPlaneProviders: []string{"rke2-control-plane:v0.5.99"},
				LogFolder:             clusterctlLogFolder,
			}, e2eConfig.GetIntervals(specName, "wait-controllers")...)

			// At this point provider does not have an etcd secret, as a new node was never rolled out
			// and the cluster was created in the old version. Should still be possible to do.
			By("Scaling down control plane to 2 and workers up to 2 using v1apha1")
			ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
				Legacy:       true,
				ClusterProxy: bootstrapClusterProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                clusterctlLogFolder,
					ClusterctlConfigPath:     clusterctlConfigPath,
					KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
					InfrastructureProvider:   "docker",
					Flavor:                   "docker-legacy",
					Namespace:                namespace.Name,
					ClusterName:              clusterName,
					KubernetesVersion:        e2eConfig.GetVariable(KubernetesVersion),
					ControlPlaneMachineCount: ptr.To(int64(2)),
					WorkerMachineCount:       ptr.To(int64(2)),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForLegacyControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(result.LegacyControlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			// Possible only with valid etcd certificate in the secret
			// Created machine is a scale up, so the secret will be populated for the
			// remaning 2 machines to scale down to 1 later
			By("Upgrading control plane and worker machines using v1beta1")
			ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
				ClusterProxy: bootstrapClusterProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                clusterctlLogFolder,
					ClusterctlConfigPath:     clusterctlConfigPath,
					KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
					InfrastructureProvider:   "docker",
					Flavor:                   "docker",
					Namespace:                namespace.Name,
					ClusterName:              clusterName,
					KubernetesVersion:        e2eConfig.GetVariable(KubernetesVersionUpgradeTo),
					ControlPlaneMachineCount: ptr.To(int64(2)),
					WorkerMachineCount:       ptr.To(int64(1)),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForClusterToUpgrade(ctx, WaitForClusterToUpgradeInput{
				Lister:              bootstrapClusterProxy.GetClient(),
				ControlPlane:        result.ControlPlane,
				MachineDeployments:  result.MachineDeployments,
				VersionAfterUpgrade: e2eConfig.GetVariable(KubernetesVersionUpgradeTo),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(result.ControlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)

			By("Scale down CP and workers to 1")
			ApplyClusterTemplateAndWait(ctx, ApplyClusterTemplateAndWaitInput{
				ClusterProxy: bootstrapClusterProxy,
				ConfigCluster: clusterctl.ConfigClusterInput{
					LogFolder:                clusterctlLogFolder,
					ClusterctlConfigPath:     clusterctlConfigPath,
					KubeconfigPath:           bootstrapClusterProxy.GetKubeconfigPath(),
					InfrastructureProvider:   "docker",
					Flavor:                   "docker",
					Namespace:                namespace.Name,
					ClusterName:              clusterName,
					KubernetesVersion:        e2eConfig.GetVariable(KubernetesVersionUpgradeTo),
					ControlPlaneMachineCount: ptr.To(int64(1)),
					WorkerMachineCount:       ptr.To(int64(1)),
				},
				WaitForClusterIntervals:      e2eConfig.GetIntervals(specName, "wait-cluster"),
				WaitForControlPlaneIntervals: e2eConfig.GetIntervals(specName, "wait-control-plane"),
				WaitForMachineDeployments:    e2eConfig.GetIntervals(specName, "wait-worker-nodes"),
			}, result)

			WaitForControlPlaneToBeReady(ctx, WaitForControlPlaneToBeReadyInput{
				Getter:       bootstrapClusterProxy.GetClient(),
				ControlPlane: client.ObjectKeyFromObject(result.ControlPlane),
			}, e2eConfig.GetIntervals(specName, "wait-control-plane")...)
		})
	})
})
