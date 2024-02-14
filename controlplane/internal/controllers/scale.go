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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/storage/names"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"

	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1beta1"
	rke2 "github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/rke2"
	bsutil "github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/util"
)

func (r *RKE2ControlPlaneReconciler) initializeControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	controlPlane *rke2.ControlPlane,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	// Perform an uncached read of all the owned machines. This check is in place to make sure
	// that the controller cache is not misbehaving and we end up initializing the cluster more than once.
	ownedMachines, err := r.managementClusterUncached.GetMachinesForCluster(ctx, util.ObjectKey(cluster), collections.OwnedMachines(rcp))
	if err != nil {
		logger.Error(err, "failed to perform an uncached read of control plane machines for cluster")

		return ctrl.Result{}, err
	}

	if len(ownedMachines) > 0 {
		return ctrl.Result{}, errors.Errorf(
			"control plane has already been initialized, found %d owned machine for cluster %s/%s: controller cache or management cluster is misbehaving",
			len(ownedMachines), cluster.Namespace, cluster.Name,
		)
	}

	bootstrapSpec := controlPlane.InitialControlPlaneConfig()
	fd := controlPlane.NextFailureDomainForScaleUp()

	if err := r.cloneConfigsAndGenerateMachine(ctx, cluster, rcp, bootstrapSpec, fd); err != nil {
		logger.Error(err, "Failed to create initial control plane Machine")
		r.recorder.Eventf(
			rcp,
			corev1.EventTypeWarning,
			"FailedInitialization",
			"Failed to create initial control plane Machine for cluster %s/%s control plane: %v",
			cluster.Namespace,
			cluster.Name,
			err)

		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are additional operations to perform
	return ctrl.Result{Requeue: true}, nil
}

func (r *RKE2ControlPlaneReconciler) scaleUpControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	controlPlane *rke2.ControlPlane,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	// Run preflight checks to ensure that the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
	if result := r.preflightChecks(ctx, controlPlane); !result.IsZero() {
		return result, nil
	}

	// Create the bootstrap configuration
	bootstrapSpec := controlPlane.JoinControlPlaneConfig()
	fd := controlPlane.NextFailureDomainForScaleUp()

	if err := r.cloneConfigsAndGenerateMachine(ctx, cluster, rcp, bootstrapSpec, fd); err != nil {
		logger.Error(err, "Failed to create additional control plane Machine")
		r.recorder.Eventf(
			rcp,
			corev1.EventTypeWarning,
			"FailedScaleUp",
			"Failed to create additional control plane Machine for cluster %s/%s control plane: %v",
			cluster.Namespace,
			cluster.Name,
			err,
		)

		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are other operations to perform
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *RKE2ControlPlaneReconciler) scaleDownControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	controlPlane *rke2.ControlPlane,
	outdatedMachines collections.Machines,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	// Pick the Machine that we should scale down.
	machineToDelete, err := selectMachineForScaleDown(controlPlane, outdatedMachines)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to select machine for scale down")
	}

	// Run preflight checks ensuring the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
	// Given that we're scaling down, we can exclude the machineToDelete from the preflight checks.
	if result := r.preflightChecks(ctx, controlPlane, machineToDelete); !result.IsZero() {
		return result, nil
	}

	if machineToDelete == nil {
		logger.Info("Failed to pick control plane Machine to delete")

		return ctrl.Result{}, errors.New("failed to pick control plane Machine to delete")
	}

	// If etcd leadership is on machine that is about to be deleted, move it to the newest member available.
	etcdLeaderCandidate := controlPlane.Machines.Newest()
	if err := r.workloadCluster.ForwardEtcdLeadership(ctx, machineToDelete, etcdLeaderCandidate); err != nil {
		logger.Error(err, "Failed to move leadership to candidate machine", "candidate", etcdLeaderCandidate.Name)
		return ctrl.Result{}, err
	}
	if err := r.workloadCluster.RemoveEtcdMemberForMachine(ctx, machineToDelete); err != nil {
		logger.Error(err, "Failed to remove etcd member for machine")
		return ctrl.Result{}, err
	}

	logger = logger.WithValues("machine", machineToDelete)
	if err := r.Client.Delete(ctx, machineToDelete); err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to delete control plane machine")
		r.recorder.Eventf(rcp, corev1.EventTypeWarning, "FailedScaleDown",
			"Failed to delete control plane Machine %s for cluster %s/%s control plane: %v", machineToDelete.Name, cluster.Namespace, cluster.Name, err)

		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are additional operations to perform
	return ctrl.Result{Requeue: true}, nil
}

// preflightChecks checks if the control plane is stable before proceeding with a scale up/scale down operation,
// where stable means that:
// - There are no machine deletion in progress
// - All the health conditions on RCP are true.
// - All the health conditions on the control plane machines are true.
// If the control plane is not passing preflight checks, it requeue.
//
// NOTE: this func uses RCP conditions, it is required to call reconcileControlPlaneConditions before this.
func (r *RKE2ControlPlaneReconciler) preflightChecks(
	ctx context.Context,
	controlPlane *rke2.ControlPlane,
	excludeFor ...*clusterv1.Machine,
) ctrl.Result {
	logger := log.FromContext(ctx)

	// If there is no RCP-owned control-plane machines, then control-plane has not been initialized yet,
	// so it is considered ok to proceed.
	if controlPlane.Machines.Len() == 0 {
		return ctrl.Result{}
	}

	// If there are deleting machines, wait for the operation to complete.
	if controlPlane.HasDeletingMachine() {
		logger.Info("Waiting for machines to be deleted", "Machines",
			strings.Join(controlPlane.Machines.Filter(collections.HasDeletionTimestamp).Names(),
				", ",
			))

		return ctrl.Result{RequeueAfter: deleteRequeueAfter}
	}

	// Check machine health conditions; if there are conditions with False or Unknown, then wait.
	allMachineHealthConditions := []clusterv1.ConditionType{
		controlplanev1.MachineAgentHealthyCondition,
		controlplanev1.MachineEtcdMemberHealthyCondition,
	}
	machineErrors := []error{}

loopmachines:
	for _, machine := range controlPlane.Machines {
		for _, excluded := range excludeFor {
			// If this machine should be excluded from the individual
			// health check, continue the out loop.
			if machine.Name == excluded.Name {
				continue loopmachines
			}
		}

		for _, condition := range allMachineHealthConditions {
			if err := preflightCheckCondition("machine", machine, condition); err != nil {
				machineErrors = append(machineErrors, err)
			}
		}
	}

	if len(machineErrors) > 0 {
		aggregatedError := kerrors.NewAggregate(machineErrors)
		r.recorder.Eventf(controlPlane.RCP, corev1.EventTypeWarning, "ControlPlaneUnhealthy",
			"Waiting for control plane to pass preflight checks to continue reconciliation: %v", aggregatedError)
		logger.Info("Waiting for control plane to pass preflight checks", "failures", aggregatedError.Error())

		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}
	}

	return ctrl.Result{}
}

func preflightCheckCondition(kind string, obj conditions.Getter, condition clusterv1.ConditionType) error {
	c := conditions.Get(obj, condition)
	if c == nil {
		return errors.Errorf("%s %s does not have %s condition", kind, obj.GetName(), condition)
	}

	if c.Status == corev1.ConditionFalse {
		return errors.Errorf("%s %s reports %s condition is false (%s, %s)", kind, obj.GetName(), condition, c.Severity, c.Message)
	}

	if c.Status == corev1.ConditionUnknown {
		return errors.Errorf("%s %s reports %s condition is unknown (%s)", kind, obj.GetName(), condition, c.Message)
	}

	return nil
}

func selectMachineForScaleDown(controlPlane *rke2.ControlPlane, outdatedMachines collections.Machines) (*clusterv1.Machine, error) {
	machines := controlPlane.Machines

	switch {
	case controlPlane.MachineWithDeleteAnnotation(outdatedMachines).Len() > 0:
		machines = controlPlane.MachineWithDeleteAnnotation(outdatedMachines)
		controlPlane.Logger().V(5).Info("Inside the withDeleteAnnotation-outdated case", "machines", machines.Names())
	case controlPlane.MachineWithDeleteAnnotation(machines).Len() > 0:
		machines = controlPlane.MachineWithDeleteAnnotation(machines)
		controlPlane.Logger().V(5).Info("Inside the withDeleteAnnotation case", "machines", machines.Names())
	case outdatedMachines.Len() > 0:
		machines = outdatedMachines
		controlPlane.Logger().V(5).Info("Inside the Outdated case", "machines", machines.Names())
	case machines.Filter(collections.Not(collections.IsReady())).Len() > 0:
		machines = machines.Filter(collections.Not(collections.IsReady()))
		controlPlane.Logger().V(5).Info("Inside the IsReady case", "machines", machines.Names())
	}

	return controlPlane.MachineInFailureDomainWithMostMachines(machines)
}

func (r *RKE2ControlPlaneReconciler) cloneConfigsAndGenerateMachine(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	bootstrapSpec *bootstrapv1.RKE2ConfigSpec,
	failureDomain *string,
) error {
	var errs []error

	// Since the cloned resource should eventually have a controller ref for the Machine, we create an
	// OwnerReference here without the Controller field set
	infraCloneOwner := &metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "RKE2ControlPlane",
		Name:       rcp.Name,
		UID:        rcp.UID,
	}

	// Clone the infrastructure template
	infraRef, err := external.CreateFromTemplate(ctx, &external.CreateFromTemplateInput{
		Client:      r.Client,
		TemplateRef: &rcp.Spec.InfrastructureRef,
		Namespace:   rcp.Namespace,
		OwnerRef:    infraCloneOwner,
		ClusterName: cluster.Name,
		Labels:      rke2.ControlPlaneLabelsForCluster(cluster.Name),
	})
	if err != nil {
		// Safe to return early here since no resources have been created yet.
		return errors.Wrap(err, "failed to clone infrastructure template")
	}

	// Clone the bootstrap configuration
	bootstrapRef, err := r.generateRKE2Config(ctx, rcp, cluster, bootstrapSpec)
	if err != nil {
		errs = append(errs, errors.Wrap(err, "failed to generate bootstrap config"))
	}

	// Only proceed to generating the Machine if we haven't encountered an error
	if len(errs) == 0 {
		if err := r.generateMachine(ctx, rcp, cluster, infraRef, bootstrapRef, failureDomain); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to create Machine"))
		}
	}

	// If we encountered any errors, attempt to clean up any dangling resources
	if len(errs) > 0 {
		if err := r.cleanupFromGeneration(ctx, infraRef, bootstrapRef); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to cleanup generated resources"))
		}

		return kerrors.NewAggregate(errs)
	}

	return nil
}

func (r *RKE2ControlPlaneReconciler) cleanupFromGeneration(ctx context.Context, remoteRefs ...*corev1.ObjectReference) error {
	var errs []error

	for _, ref := range remoteRefs {
		if ref != nil {
			config := &unstructured.Unstructured{}
			config.SetKind(ref.Kind)
			config.SetAPIVersion(ref.APIVersion)
			config.SetNamespace(ref.Namespace)
			config.SetName(ref.Name)

			if err := r.Client.Delete(ctx, config); err != nil && !apierrors.IsNotFound(err) {
				errs = append(errs, errors.Wrap(err, "failed to cleanup generated resources after error"))
			}
		}
	}

	return kerrors.NewAggregate(errs)
}

func (r *RKE2ControlPlaneReconciler) generateRKE2Config(
	ctx context.Context,
	rcp *controlplanev1.RKE2ControlPlane,
	cluster *clusterv1.Cluster,
	spec *bootstrapv1.RKE2ConfigSpec,
) (*corev1.ObjectReference, error) {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	owner := metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "RKE2ControlPlane",
		Name:       rcp.Name,
		UID:        rcp.UID,
	}

	bootstrapConfig := &bootstrapv1.RKE2Config{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.SimpleNameGenerator.GenerateName(rcp.Name + "-"),
			Namespace:       rcp.Namespace,
			Labels:          rke2.ControlPlaneLabelsForCluster(cluster.Name),
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: *spec,
	}

	if err := r.Client.Create(ctx, bootstrapConfig); err != nil {
		return nil, errors.Wrap(err, "Failed to create bootstrap configuration")
	}

	bootstrapRef := &corev1.ObjectReference{
		APIVersion: bootstrapv1.GroupVersion.String(),
		Kind:       "RKE2Config",
		Name:       bootstrapConfig.GetName(),
		Namespace:  bootstrapConfig.GetNamespace(),
		UID:        bootstrapConfig.GetUID(),
	}

	return bootstrapRef, nil
}

func (r *RKE2ControlPlaneReconciler) generateMachine(
	ctx context.Context,
	rcp *controlplanev1.RKE2ControlPlane,
	cluster *clusterv1.Cluster,
	infraRef,
	bootstrapRef *corev1.ObjectReference,
	failureDomain *string,
) error {
	newVersion, err := bsutil.Rke2ToKubeVersion(rcp.Spec.AgentConfig.Version)
	if err != nil {
		return fmt.Errorf("failed to convert rke2 version to kubernetes version: %w", err)
	}

	logger := log.FromContext(ctx)

	logger.Info("Version checking...", "rke2-version", rcp.Spec.AgentConfig.Version, "machine-version: ", newVersion)

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName(rcp.Name + "-"),
			Namespace: rcp.Namespace,
			Labels:    rke2.ControlPlaneLabelsForCluster(cluster.Name),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(rcp, controlplanev1.GroupVersion.WithKind("RKE2ControlPlane")),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       cluster.Name,
			Version:           &newVersion,
			InfrastructureRef: *infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
			FailureDomain:    failureDomain,
			NodeDrainTimeout: rcp.Spec.NodeDrainTimeout,
		},
	}

	logger.Info("generating machine:", "machine-spec-version", machine.Spec.Version)

	// Machine's bootstrap config may be missing RKE2Config if it is not the first machine in the control plane.
	// We store RKE2Config as annotation here to detect any changes in RCP RKE2Config and rollout the machine if any.
	serverConfig, err := json.Marshal(rcp.Spec.ServerConfig)
	if err != nil {
		return errors.Wrap(err, "failed to marshal cluster configuration")
	}

	machine.SetAnnotations(map[string]string{controlplanev1.RKE2ServerConfigurationAnnotation: string(serverConfig)})

	if err := r.Client.Create(ctx, machine); err != nil {
		return errors.Wrap(err, "failed to create machine")
	}

	return nil
}
