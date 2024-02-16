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

package rke2

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"

	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/etcd"
)

const (
	labelNodeRoleControlPlane = "node-role.kubernetes.io/master"
)

// ErrControlPlaneMinNodes is returned when the control plane has fewer than 2 nodes.
var ErrControlPlaneMinNodes = errors.New("cluster has fewer than 2 control plane nodes; removing an etcd member is not supported")

// WorkloadCluster defines all behaviors necessary to upgrade kubernetes on a workload cluster.
type WorkloadCluster interface {
	// Basic health and status checks.
	InitWorkload(ctx context.Context, controlPlane *ControlPlane) error
	UpdateNodeMetadata(ctx context.Context, controlPlane *ControlPlane) error

	ClusterStatus() ClusterStatus
	UpdateAgentConditions(controlPlane *ControlPlane)
	UpdateEtcdConditions(controlPlane *ControlPlane)
	// Upgrade related tasks.

	//	AllowBootstrapTokensToGetNodes(ctx context.Context) error

	// State recovery tasks.
	RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error
	ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error
	ReconcileEtcdMembers(ctx context.Context, nodeNames []string, version semver.Version) ([]string, error)
	EtcdMembers(ctx context.Context) ([]string, error)
}

// Workload defines operations on workload clusters.
type Workload struct {
	client.Client

	Nodes               map[string]*corev1.Node
	nodePatchHelpers    map[string]*patch.Helper
	etcdClientGenerator etcd.EtcdClientFor
}

// NewWorkload is creating a new ClusterWorkload instance.
func (m *Management) NewWorkload(ctx context.Context, cl client.Client, clusterKey ctrlclient.ObjectKey) (*Workload, error) {
	restConfig, err := m.Tracker.GetRESTConfig(ctx, clusterKey)
	if err != nil {
		return nil, &RemoteClusterConnectionError{Name: clusterKey.String(), Err: err}
	}
	restConfig = rest.CopyConfig(restConfig)
	restConfig.Timeout = 30 * time.Second

	// Retrieves the etcd CA key Pair
	crtData, keyData, err := m.getEtcdCAKeyPair(ctx, clusterKey)
	if err != nil {
		return nil, err
	}

	clientCert, err := tls.X509KeyPair(crtData, keyData)
	if err != nil {
		return nil, err
	}

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(crtData)
	tlsConfig := &tls.Config{
		RootCAs:            caPool,
		Certificates:       []tls.Certificate{clientCert},
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}

	return &Workload{
		Client:              cl,
		Nodes:               map[string]*corev1.Node{},
		nodePatchHelpers:    map[string]*patch.Helper{},
		etcdClientGenerator: etcd.NewEtcdClientGenerator(restConfig, tlsConfig, 10*time.Second, 15*time.Second),
	}, nil
}

// InitWorkload prepares workload for evaluating status conditions.
func (w *Workload) InitWorkload(ctx context.Context, cp *ControlPlane) error {
	nodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		conditions.MarkUnknown(
			cp.RCP,
			controlplanev1.ControlPlaneComponentsHealthyCondition,
			controlplanev1.ControlPlaneComponentsInspectionFailedReason, "Failed to list nodes which are hosting control plane components")

		return err
	}

	for _, node := range nodes.Items {
		nodeCopy := node
		w.Nodes[node.Name] = &nodeCopy
	}

	for _, node := range w.Nodes {
		patchHelper, err := patch.NewHelper(node, w.Client)
		if err != nil {
			conditions.MarkUnknown(
				cp.RCP,
				controlplanev1.ControlPlaneComponentsHealthyCondition,
				controlplanev1.ControlPlaneComponentsInspectionFailedReason, "Failed to create patch helpers for control plane nodes")

			return errors.Wrapf(err, "failed to create patch helper for node %s", node.Name)
		}

		w.nodePatchHelpers[node.Name] = patchHelper
	}

	return nil
}

// ClusterStatus holds stats information about the cluster.
type ClusterStatus struct {
	// Nodes are a total count of nodes
	Nodes int32
	// ReadyNodes are the count of nodes that are reporting ready
	ReadyNodes int32
}

func (w *Workload) getControlPlaneNodes(ctx context.Context) (*corev1.NodeList, error) {
	nodes := &corev1.NodeList{}
	labels := map[string]string{
		labelNodeRoleControlPlane: "true",
	}

	if err := w.Client.List(ctx, nodes, client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return nodes, nil
}

// PatchNodes patches the nodes in the workload cluster.
func (w *Workload) PatchNodes(ctx context.Context, cp *ControlPlane) error {
	errList := []error{}

	for i := range w.Nodes {
		node := w.Nodes[i]
		machine, found := cp.Machines[node.Name]

		if !found {
			for _, m := range cp.Machines {
				if m.Status.NodeRef != nil && m.Status.NodeRef.Name == node.Name {
					machine = m

					break
				}
			}

			if machine == nil {
				continue
			}
		}

		if helper, ok := w.nodePatchHelpers[node.Name]; ok {
			if err := helper.Patch(ctx, node); err != nil {
				conditions.MarkUnknown(
					machine,
					controlplanev1.NodeMetadataUpToDate,
					controlplanev1.NodePatchFailedReason, errors.Wrapf(err, "failed to patch node %s", node.Name).Error())

				errList = append(errList, errors.Wrapf(err, "failed to patch node %s", node.Name))
			}

			continue
		}

		errList = append(errList, errors.Errorf("failed to get patch helper for node %s", node.Name))
	}

	return kerrors.NewAggregate(errList)
}

// ClusterStatus returns the status of the cluster.
func (w *Workload) ClusterStatus() ClusterStatus {
	status := ClusterStatus{}

	// count the control plane nodes
	for _, node := range w.Nodes {
		nodeCopy := node
		status.Nodes++

		if util.IsNodeReady(nodeCopy) {
			status.ReadyNodes++
		}
	}

	return status
}

func hasProvisioningMachine(machines collections.Machines) bool {
	for _, machine := range machines {
		if machine.Status.NodeRef == nil {
			return true
		}
	}

	return false
}

// nodeHasUnreachableTaint returns true if the node has is unreachable from the node controller.
func nodeHasUnreachableTaint(node corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == corev1.TaintNodeUnreachable && taint.Effect == corev1.TaintEffectNoExecute {
			return true
		}
	}

	return false
}

// UpdateAgentConditions is responsible for updating machine conditions reflecting the status of all the control plane
// components running in a static pod generated by RKE2. This operation is best effort, in the sense that in case
// of problems in retrieving the pod status, it sets the condition to Unknown state without returning any error.
func (w *Workload) UpdateAgentConditions(controlPlane *ControlPlane) {
	allMachinePodConditions := []clusterv1.ConditionType{
		controlplanev1.MachineAgentHealthyCondition,
	}

	// Update conditions for control plane components hosted as static pods on the nodes.
	var rcpErrors []string

	for k := range w.Nodes {
		node := w.Nodes[k]

		// Search for the machine corresponding to the node.
		machine, found := controlPlane.Machines[node.Name]
		// If there is no machine corresponding to a node, determine if this is an error or not.
		if !found {
			// If there are machines still provisioning there is the chance that a chance that a node might be linked to a machine soon,
			// otherwise report the error at RCP level given that there is no machine to report on.
			if hasProvisioningMachine(controlPlane.Machines) {
				continue
			}

			for _, m := range controlPlane.Machines {
				if m.Status.NodeRef != nil && m.Status.NodeRef.Name == node.Name {
					machine = m

					break
				}
			}

			if machine == nil {
				rcpErrors = append(rcpErrors, fmt.Sprintf("Control plane node %s does not have a corresponding machine", node.Name))

				continue
			}
		}

		// If the machine is deleting, report all the conditions as deleting
		if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
			for _, condition := range allMachinePodConditions {
				conditions.MarkFalse(machine, condition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
			}

			continue
		}

		// If the node is Unreachable, information about static pods could be stale so set all conditions to unknown.
		if nodeHasUnreachableTaint(*node) {
			// NOTE: We are assuming unreachable as a temporary condition, leaving to MHC
			// the responsibility to determine if the node is unhealthy or not.
			for _, condition := range allMachinePodConditions {
				conditions.MarkUnknown(machine, condition, controlplanev1.PodInspectionFailedReason, "Node is unreachable")
			}

			continue
		}
	}

	// If there are provisioned machines without corresponding nodes, report this as a failing conditions with SeverityError.
	for i := range controlPlane.Machines {
		machine := controlPlane.Machines[i]
		if machine.Status.NodeRef == nil {
			continue
		}

		node, found := w.Nodes[machine.Status.NodeRef.Name]
		if !found {
			for _, condition := range allMachinePodConditions {
				conditions.MarkFalse(machine, condition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing node")
			}

			continue
		}

		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				conditions.MarkTrue(machine, controlplanev1.MachineAgentHealthyCondition)
			}
		}
	}

	// Aggregate components error from machines at RCP level.
	aggregateFromMachinesToRCP(aggregateFromMachinesToRCPInput{
		controlPlane:      controlPlane,
		machineConditions: allMachinePodConditions,
		rcpErrors:         rcpErrors,
		condition:         controlplanev1.ControlPlaneComponentsHealthyCondition,
		unhealthyReason:   controlplanev1.ControlPlaneComponentsUnhealthyReason,
		unknownReason:     controlplanev1.ControlPlaneComponentsUnknownReason,
		note:              "control plane",
	})
}

type aggregateFromMachinesToRCPInput struct {
	controlPlane      *ControlPlane
	machineConditions []clusterv1.ConditionType
	rcpErrors         []string
	condition         clusterv1.ConditionType
	unhealthyReason   string
	unknownReason     string
	note              string
}

// aggregateFromMachinesToRCP aggregates a group of conditions from machines to RCP.
// NOTE: this func follows the same aggregation rules used by conditions.Merge thus giving priority to
// errors, then warning, info down to unknown.
func aggregateFromMachinesToRCP(input aggregateFromMachinesToRCPInput) {
	// Aggregates machines for condition status.
	// NB. A machine could be assigned to many groups, but only the group with the highest severity will be reported.
	rcpMachinesWithErrors := sets.NewString()
	rcpMachinesWithWarnings := sets.NewString()
	rcpMachinesWithInfo := sets.NewString()
	rcpMachinesWithTrue := sets.NewString()
	rcpMachinesWithUnknown := sets.NewString()

	for i := range input.controlPlane.Machines {
		machine := input.controlPlane.Machines[i]
		for _, condition := range input.machineConditions {
			if machineCondition := conditions.Get(machine, condition); machineCondition != nil {
				switch machineCondition.Status {
				case corev1.ConditionTrue:
					rcpMachinesWithTrue.Insert(machine.Name)
				case corev1.ConditionFalse:
					switch machineCondition.Severity {
					case clusterv1.ConditionSeverityInfo:
						rcpMachinesWithInfo.Insert(machine.Name)
					case clusterv1.ConditionSeverityWarning:
						rcpMachinesWithWarnings.Insert(machine.Name)
					case clusterv1.ConditionSeverityError:
						rcpMachinesWithErrors.Insert(machine.Name)
					}
				case corev1.ConditionUnknown:
					rcpMachinesWithUnknown.Insert(machine.Name)
				}
			}
		}
	}

	// In case of at least one machine with errors or RCP level errors (nodes without machines), report false, error.
	if len(rcpMachinesWithErrors) > 0 {
		input.rcpErrors = append(
			input.rcpErrors,
			fmt.Sprintf("Following machines are reporting %s errors: %s",
				input.note,
				strings.Join(rcpMachinesWithErrors.List(), ", ")))
	}

	if len(input.rcpErrors) > 0 {
		conditions.MarkFalse(
			input.controlPlane.RCP,
			input.condition,
			input.unhealthyReason,
			clusterv1.ConditionSeverityError,
			strings.Join(input.rcpErrors, "; "))

		return
	}

	// In case of no errors and at least one machine with warnings, report false, warnings.
	if len(rcpMachinesWithWarnings) > 0 {
		conditions.MarkFalse(
			input.controlPlane.RCP,
			input.condition,
			input.unhealthyReason,
			clusterv1.ConditionSeverityWarning,
			"Following machines are reporting %s warnings: %s",
			input.note,
			strings.Join(rcpMachinesWithWarnings.List(), ", "))

		return
	}

	// In case of no errors, no warning, and at least one machine with info, report false, info.
	if len(rcpMachinesWithWarnings) > 0 {
		conditions.MarkFalse(
			input.controlPlane.RCP,
			input.condition,
			input.unhealthyReason,
			clusterv1.ConditionSeverityWarning,
			"Following machines are reporting %s info: %s",
			input.note, strings.Join(rcpMachinesWithInfo.List(), ", "))

		return
	}

	// In case of no errors, no warning, no Info, and at least one machine with true conditions, report true.
	if len(rcpMachinesWithTrue) > 0 {
		conditions.MarkTrue(input.controlPlane.RCP, input.condition)

		return
	}

	// Otherwise, if there is at least one machine with unknown, report unknown.
	if len(rcpMachinesWithUnknown) > 0 {
		conditions.MarkUnknown(
			input.controlPlane.RCP,
			input.condition,
			input.unknownReason,
			"Following machines are reporting unknown %s status: %s", input.note, strings.Join(rcpMachinesWithUnknown.List(), ", "))

		return
	}
}

// UpdateEtcdConditions is responsible for updating machine conditions reflecting the status of all the etcd members.
// This operation is best effort, in the sense that in case of problems in retrieving member status, it sets
// the condition to Unknown state without returning any error.
func (w *Workload) UpdateEtcdConditions(controlPlane *ControlPlane) {
	w.updateManagedEtcdConditions(controlPlane)
}

func (w *Workload) updateManagedEtcdConditions(controlPlane *ControlPlane) {
	// NOTE: This methods uses control plane nodes only to get in contact with etcd but then it relies on etcd
	// as ultimate source of truth for the list of members and for their health.
	for k := range w.Nodes {
		node := w.Nodes[k]

		machine, found := controlPlane.Machines[node.Name]
		if !found {
			// If there are machines still provisioning there is the chance that a chance that a node might be linked to a machine soon,
			// otherwise report the error at RCP level given that there is no machine to report on.
			if hasProvisioningMachine(controlPlane.Machines) {
				continue
			}

			for _, m := range controlPlane.Machines {
				if m.Status.NodeRef != nil && m.Status.NodeRef.Name == node.Name {
					machine = m
				}
			}

			if machine == nil {
				continue
			}
		}

		// If the machine is deleting, report all the conditions as deleting
		if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")

			continue
		}

		conditions.MarkTrue(machine, controlplanev1.MachineEtcdMemberHealthyCondition)
	}
}

// UpdateNodeMetadata is responsible for populating node metadata after
// it is referenced from machine object.
func (w *Workload) UpdateNodeMetadata(ctx context.Context, controlPlane *ControlPlane) error {
	for nodeName, machine := range controlPlane.Machines {
		if machine.Spec.Bootstrap.ConfigRef == nil {
			continue
		}

		if machine.Status.NodeRef != nil {
			nodeName = machine.Status.NodeRef.Name
		}

		conditions.MarkTrue(machine, controlplanev1.NodeMetadataUpToDate)

		node, nodeFound := w.Nodes[nodeName]
		if !nodeFound {
			conditions.MarkUnknown(
				machine,
				controlplanev1.NodeMetadataUpToDate,
				controlplanev1.NodePatchFailedReason, "associated node not found")

			continue
		} else if name, ok := node.Annotations[clusterv1.MachineAnnotation]; !ok || name != machine.Name {
			conditions.MarkUnknown(
				machine,
				controlplanev1.NodeMetadataUpToDate,
				controlplanev1.NodePatchFailedReason, fmt.Sprintf("node object is missing %s annotation", clusterv1.MachineAnnotation))

			continue
		}

		rkeConfig, found := controlPlane.rke2Configs[machine.Name]
		if !found {
			conditions.MarkUnknown(
				machine,
				controlplanev1.NodeMetadataUpToDate,
				controlplanev1.NodePatchFailedReason, "associated RKE2 config not found")

			continue
		}

		annotations.AddAnnotations(node, rkeConfig.Spec.AgentConfig.NodeAnnotations)
	}

	return w.PatchNodes(ctx, controlPlane)
}
