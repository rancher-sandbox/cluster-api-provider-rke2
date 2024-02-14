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
	"fmt"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"

	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/kubeconfig"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/registration"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/rke2"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/secret"
)

const (
	// dependentCertRequeueAfter is how long to wait before checking again to see if
	// dependent certificates have been created.
	dependentCertRequeueAfter = 30 * time.Second

	// DefaultRequeueTime is the default requeue time for the controller.
	DefaultRequeueTime = 20 * time.Second
)

// RKE2ControlPlaneReconciler reconciles a RKE2ControlPlane object.
type RKE2ControlPlaneReconciler struct {
	Log logr.Logger
	client.Client
	Scheme *runtime.Scheme

	SecretCachingClient client.Client
	Tracker             *remote.ClusterCacheTracker

	managementClusterUncached rke2.ManagementCluster
	managementCluster         rke2.ManagementCluster
	recorder                  record.EventRecorder
	controller                controller.Controller
	workloadCluster           rke2.WorkloadCluster
}

//nolint:lll
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machinesets;machines;machines/status;machinepools;machinepools/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="bootstrap.cluster.x-k8s.io",resources=rke2configs,verbs=get;list;watch;create;patch;delete
// +kubebuilder:rbac:groups="infrastructure.cluster.x-k8s.io",resources=*,verbs=get;list;watch;create;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RKE2ControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)
	r.Log = logger
	rcp := &controlplanev1.RKE2ControlPlane{}

	if err := r.Get(ctx, req.NamespacedName, rcp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, rcp.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to retrieve owner Cluster from the API Server")

		return ctrl.Result{}, err
	}

	if cluster == nil {
		logger.Info("Cluster Controller has not yet set OwnerRef")

		return ctrl.Result{Requeue: true}, nil
	}

	logger = logger.WithValues("cluster", cluster.Name)

	if annotations.IsPaused(cluster, rcp) {
		logger.Info("Reconciliation is paused for this object")

		return ctrl.Result{}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(rcp, r.Client)
	if err != nil {
		logger.Error(err, "Failed to configure the patch helper")

		return ctrl.Result{Requeue: true}, nil
	}

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(rcp, controlplanev1.RKE2ControlPlaneFinalizer) {
		controllerutil.AddFinalizer(rcp, controlplanev1.RKE2ControlPlaneFinalizer)

		// patch and return right away instead of reusing the main defer,
		// because the main defer may take too much time to get cluster status
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{patch.WithStatusObservedGeneration{}}
		if err := patchHelper.Patch(ctx, rcp, patchOpts...); err != nil {
			logger.Error(err, "Failed to patch RKE2ControlPlane to add finalizer")

			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	defer func() {
		// Always attempt to update status.
		if err := r.updateStatus(ctx, rcp, cluster); err != nil {
			var connFailure *rke2.RemoteClusterConnectionError
			if errors.As(err, &connFailure) {
				logger.Info("Could not connect to workload cluster to fetch status", "err", err.Error())
			} else {
				logger.Error(err, "Failed to update RKE2ControlPlane Status")
				reterr = kerrors.NewAggregate([]error{reterr, err})
			}
		}

		// Always attempt to Patch the RKE2ControlPlane object and status after each reconciliation.
		if err := patchRKE2ControlPlane(ctx, patchHelper, rcp); err != nil {
			logger.Error(err, "Failed to patch RKE2ControlPlane")
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// Make rcp to requeue in case status is not ready, so we can check for node
		// status without waiting for a full resync (by default 10 minutes).
		// Only requeue if we are not going in exponential backoff due to error,
		// or if we are not already re-queueing, or if the object has a deletion timestamp.
		if reterr == nil && !res.Requeue && res.RequeueAfter <= 0 && rcp.ObjectMeta.DeletionTimestamp.IsZero() {
			if !rcp.Status.Ready {
				res = ctrl.Result{RequeueAfter: DefaultRequeueTime}
			}
		}
	}()

	if !rcp.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion reconciliation loop.
		res, err = r.reconcileDelete(ctx, cluster, rcp)

		return res, err
	}

	// Handle normal reconciliation loop.
	res, err = r.reconcileNormal(ctx, cluster, rcp)

	return res, err
}

func patchRKE2ControlPlane(ctx context.Context, patchHelper *patch.Helper, rcp *controlplanev1.RKE2ControlPlane) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(rcp,
		conditions.WithConditions(
			controlplanev1.MachinesReadyCondition,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.ResizedCondition,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.AvailableCondition,
			// controlplanev1.CertificatesAvailableCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		rcp,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.ResizedCondition,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.AvailableCondition,
		}},
		patch.WithStatusObservedGeneration{},
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RKE2ControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.RKE2ControlPlane{}).
		Owns(&clusterv1.Machine{}).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	err = c.Watch(
		source.Kind(mgr.GetCache(), &clusterv1.Cluster{}),
		handler.EnqueueRequestsFromMapFunc(r.ClusterToRKE2ControlPlane(ctx)),
	)
	if err != nil {
		return errors.Wrap(err, "failed adding Watch for Clusters to controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("rke2-control-plane-controller")

	if r.managementCluster == nil {
		if r.Tracker == nil {
			return errors.New("cluster cache tracker is nil, cannot create the internal management cluster resource")
		}
		r.managementCluster = &rke2.Management{
			Client:              r.Client,
			Tracker:             r.Tracker,
			SecretCachingClient: r.SecretCachingClient,
		}
	}

	if r.managementClusterUncached == nil {
		r.managementClusterUncached = &rke2.Management{Client: mgr.GetAPIReader()}
	}

	return nil
}

func (r *RKE2ControlPlaneReconciler) updateStatus(ctx context.Context, rcp *controlplanev1.RKE2ControlPlane, cluster *clusterv1.Cluster) error {
	logger := log.FromContext(ctx)

	ownedMachines, err := r.managementCluster.GetMachinesForCluster(
		ctx,
		util.ObjectKey(cluster),
		collections.OwnedMachines(rcp))
	if err != nil {
		return errors.Wrap(err, "failed to get list of owned machines")
	}

	readyMachines := ownedMachines.Filter(collections.IsReady())
	for _, readyMachine := range readyMachines {
		logger.V(3).Info("Ready Machine : " + readyMachine.Name)
	}

	controlPlane, err := rke2.NewControlPlane(ctx, r.Client, cluster, rcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")

		return err
	}

	rcp.Status.UpdatedReplicas = int32(len(controlPlane.UpToDateMachines()))
	replicas := int32(len(ownedMachines))
	desiredReplicas := *rcp.Spec.Replicas

	// set basic data that does not require interacting with the workload cluster
	// ReadyReplicas and UnavailableReplicas are set in case the function returns before updating them
	rcp.Status.Replicas = replicas
	rcp.Status.ReadyReplicas = 0
	rcp.Status.UnavailableReplicas = replicas

	// Return early if the deletion timestamp is set, because we don't want to try to connect to the workload cluster
	// and we don't want to report resize condition (because it is set to deleting into reconcile delete).
	if !rcp.DeletionTimestamp.IsZero() {
		return nil
	}

	switch {
	// We are scaling up
	case replicas < desiredReplicas:
		conditions.MarkFalse(
			rcp,
			controlplanev1.ResizedCondition,
			controlplanev1.ScalingUpReason,
			clusterv1.ConditionSeverityWarning,
			"Scaling up control plane to %d replicas (actual %d)",
			desiredReplicas,
			replicas)

	// We are scaling down
	case replicas > desiredReplicas:
		conditions.MarkFalse(
			rcp,
			controlplanev1.ResizedCondition,
			controlplanev1.ScalingDownReason,
			clusterv1.ConditionSeverityWarning,
			"Scaling down control plane to %d replicas (actual %d)",
			desiredReplicas,
			replicas)

	default:
		// make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created).
		if int32(len(readyMachines)) == replicas {
			conditions.MarkTrue(rcp, controlplanev1.ResizedCondition)
		}
	}

	kubeconfigSecret := corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      secret.Name(cluster.Name, secret.Kubeconfig),
	}, &kubeconfigSecret)

	if err != nil {
		r.Log.Info("Kubeconfig secret does not yet exist")

		return err
	}

	kubeConfig := kubeconfigSecret.Data[secret.KubeconfigDataName]
	if kubeConfig == nil {
		return fmt.Errorf("unable to find a value entry in the kubeconfig secret")
	}

	rcp.Status.ReadyReplicas = int32(len(readyMachines))
	rcp.Status.UnavailableReplicas = replicas - rcp.Status.ReadyReplicas

	if rcp.Status.ReadyReplicas > 0 {
		rcp.Status.Initialized = true
	}

	if len(ownedMachines) == 0 {
		logger.Info(fmt.Sprintf("no Control Plane Machines exist for RKE2ControlPlane %s/%s", rcp.Namespace, rcp.Name))

		return nil
	}

	if len(readyMachines) == 0 {
		logger.Info(fmt.Sprintf("no Control Plane Machines are ready for RKE2ControlPlane %s/%s", rcp.Namespace, rcp.Name))

		return nil
	}

	availableCPMachines := readyMachines

	registrationmethod, err := registration.NewRegistrationMethod(string(rcp.Spec.RegistrationMethod))
	if err != nil {
		return fmt.Errorf("getting node registration method: %w", err)
	}

	validIPAddresses, err := registrationmethod(rcp, availableCPMachines)
	if err != nil {
		return fmt.Errorf("getting registration addresses: %w", err)
	}

	rcp.Status.AvailableServerIPs = validIPAddresses
	if len(rcp.Status.AvailableServerIPs) == 0 {
		return fmt.Errorf("some Control Plane machines exist and are ready but they have no IP Address available")
	}

	if len(readyMachines) == len(ownedMachines) {
		rcp.Status.Ready = true
	}

	conditions.MarkTrue(rcp, controlplanev1.AvailableCondition)

	return nil
}

func (r *RKE2ControlPlaneReconciler) reconcileNormal(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconcile RKE2 Control Plane")

	// Wait for the cluster infrastructure to be ready before creating machines
	if !cluster.Status.InfrastructureReady {
		logger.Info("Cluster infrastructure is not ready yet")

		return ctrl.Result{}, nil
	}

	certificates := secret.NewCertificatesForInitialControlPlane()
	controllerRef := metav1.NewControllerRef(rcp, controlplanev1.GroupVersion.WithKind("RKE2ControlPlane"))

	if err := certificates.LookupOrGenerate(ctx, r.Client, util.ObjectKey(cluster), *controllerRef); err != nil {
		logger.Error(err, "unable to lookup or create cluster certificates")
		conditions.MarkFalse(
			rcp, controlplanev1.CertificatesAvailableCondition,
			controlplanev1.CertificatesGenerationFailedReason,
			clusterv1.ConditionSeverityWarning, err.Error())

		return ctrl.Result{}, err
	}

	conditions.MarkTrue(rcp, controlplanev1.CertificatesAvailableCondition)

	// If ControlPlaneEndpoint is not set, return early
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		logger.Info("Cluster does not yet have a ControlPlaneEndpoint defined")

		return ctrl.Result{}, nil
	}

	// Generate Cluster Kubeconfig if needed
	if result, err := r.reconcileKubeconfig(
		ctx,
		util.ObjectKey(cluster),
		cluster.Spec.ControlPlaneEndpoint,
		rcp); err != nil {
		logger.Error(err, "failed to reconcile Kubeconfig")

		return result, err
	}

	controlPlaneMachines, err := r.managementClusterUncached.GetMachinesForCluster(
		ctx,
		util.ObjectKey(cluster),
		collections.ControlPlaneMachines(cluster.Name))
	if err != nil {
		logger.Error(err, "failed to retrieve control plane machines for cluster")

		return ctrl.Result{}, err
	}

	ownedMachines := controlPlaneMachines.Filter(collections.OwnedMachines(rcp))
	if len(ownedMachines) != len(controlPlaneMachines) {
		logger.Info("Not all control plane machines are owned by this RKE2ControlPlane, refusing to operate in mixed management mode") //nolint:lll

		return ctrl.Result{}, nil
	}

	controlPlane, err := rke2.NewControlPlane(ctx, r.Client, cluster, rcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")

		return ctrl.Result{}, err
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	conditions.SetAggregate(controlPlane.RCP, controlplanev1.MachinesReadyCondition,
		ownedMachines.ConditionGetters(),
		conditions.AddSourceRef(),
		conditions.WithStepCounterIf(false))

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Conditions reporting RCP operation progress like e.g. Resized or SpecUpToDate are inlined with the rest of the execution.
	if result, err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil || !result.IsZero() {
		logger.Error(err, "failed to reconcile Control Plane conditions")

		return result, err
	}

	// Ensures the number of etcd members is in sync with the number of machines/nodes.
	// NOTE: This is usually required after a machine deletion.
	if err := r.reconcileEtcdMembers(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	// Control plane machines rollout due to configuration changes (e.g. upgrades) takes precedence over other operations.
	needRollout := controlPlane.MachinesNeedingRollout()

	switch {
	case len(needRollout) > 0:
		logger.Info("Rolling out Control Plane machines", "needRollout", needRollout.Names())
		conditions.MarkFalse(controlPlane.RCP,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.RollingUpdateInProgressReason,
			clusterv1.ConditionSeverityWarning,
			"Rolling %d replicas with outdated spec (%d replicas up to date)",
			len(needRollout),
			len(controlPlane.Machines)-len(needRollout))

		return r.upgradeControlPlane(ctx, cluster, rcp, controlPlane, needRollout)
	default:
		// make sure last upgrade operation is marked as completed.
		// NOTE: we are checking the condition already exists in order to avoid to set this condition at the first
		// reconciliation/before a rolling upgrade actually starts.
		if conditions.Has(controlPlane.RCP, controlplanev1.MachinesSpecUpToDateCondition) {
			conditions.MarkTrue(controlPlane.RCP, controlplanev1.MachinesSpecUpToDateCondition)
		}
	}

	// If we've made it this far, we can assume that all ownedMachines are up to date
	numMachines := len(ownedMachines)
	desiredReplicas := int(*rcp.Spec.Replicas)

	switch {
	// We are creating the first replica
	case numMachines < desiredReplicas && numMachines == 0:
		// Create new Machine w/ init
		logger.Info("Initializing control plane", "Desired", desiredReplicas, "Existing", numMachines)
		conditions.MarkFalse(controlPlane.RCP,
			controlplanev1.AvailableCondition,
			controlplanev1.WaitingForRKE2ServerReason,
			clusterv1.ConditionSeverityInfo, "")

		return r.initializeControlPlane(ctx, cluster, rcp, controlPlane)
	// We are scaling up
	case numMachines < desiredReplicas && numMachines > 0:
		// Create a new Machine w/ join
		logger.Info("Scaling up control plane", "Desired", desiredReplicas, "Existing", numMachines)

		return r.scaleUpControlPlane(ctx, cluster, rcp, controlPlane)

	// We are scaling down
	case numMachines > desiredReplicas:
		logger.Info("Scaling down control plane", "Desired", desiredReplicas, "Existing", numMachines)
		// The last parameter (i.e. machines needing to be rolled out) should always be empty here.
		return r.scaleDownControlPlane(ctx, cluster, rcp, controlPlane, collections.Machines{})
	}

	return ctrl.Result{}, nil
}

// GetWorkloadCluster builds a cluster object.
// The cluster comes with an etcd client generator to connect to any etcd pod living on a managed machine.
func (c *RKE2ControlPlaneReconciler) GetWorkloadCluster(ctx context.Context, controlPlane *rke2.ControlPlane) (rke2.WorkloadCluster, error) {
	if c.workloadCluster != nil {
		return c.workloadCluster, nil
	}

	workloadCluster, err := c.managementCluster.GetWorkloadCluster(ctx, client.ObjectKeyFromObject(controlPlane.Cluster))
	if err != nil {
		return nil, err
	}
	c.workloadCluster = workloadCluster
	return c.workloadCluster, nil
}

// reconcileEtcdMembers ensures the number of etcd members is in sync with the number of machines/nodes.
// This is usually required after a machine deletion.
//
// NOTE: this func uses KCP conditions, it is required to call reconcileControlPlaneConditions before this.
func (r *RKE2ControlPlaneReconciler) reconcileEtcdMembers(ctx context.Context, controlPlane *rke2.ControlPlane) error {
	log := ctrl.LoggerFrom(ctx)

	// If there is no KCP-owned control-plane machines, then control-plane has not been initialized yet.
	if controlPlane.Machines.Len() == 0 {
		return nil
	}

	// Collect all the node names.
	nodeNames := []string{}
	for _, machine := range controlPlane.Machines {
		if machine.Status.NodeRef == nil {
			// If there are provisioning machines (machines without a node yet), return.
			return nil
		}
		nodeNames = append(nodeNames, machine.Status.NodeRef.Name)
	}

	// Potential inconsistencies between the list of members and the list of machines/nodes are
	// surfaced using the EtcdClusterHealthyCondition; if this condition is true, meaning no inconsistencies exists, return early.
	if conditions.IsTrue(controlPlane.RCP, controlplanev1.EtcdClusterHealthyCondition) {
		return nil
	}

	workloadCluster, err := r.GetWorkloadCluster(ctx, controlPlane)
	if err != nil {
		// Failing at connecting to the workload cluster can mean workload cluster is unhealthy for a variety of reasons such as etcd quorum loss.
		return errors.Wrap(err, "cannot get remote client to workload cluster")
	}

	parsedVersion, err := semver.ParseTolerant(controlPlane.RCP.Spec.AgentConfig.Version)
	if err != nil {
		return errors.Wrapf(err, "failed to parse kubernetes version %q", controlPlane.RCP.Spec.AgentConfig.Version)
	}

	removedMembers, err := workloadCluster.ReconcileEtcdMembers(ctx, nodeNames, parsedVersion)
	if err != nil {
		return errors.Wrap(err, "failed attempt to reconcile etcd members")
	}

	if len(removedMembers) > 0 {
		log.Info("Etcd members without nodes removed from the cluster", "members", removedMembers)
	}

	return nil
}

func (r *RKE2ControlPlaneReconciler) reconcileDelete(ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
) (res ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	// Gets all machines, not just control plane machines.
	allMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}

	ownedMachines := allMachines.Filter(collections.OwnedMachines(rcp))

	// If no control plane machines remain, remove the finalizer
	if len(ownedMachines) == 0 {
		controllerutil.RemoveFinalizer(rcp, controlplanev1.RKE2ControlPlaneFinalizer)

		return ctrl.Result{}, nil
	}

	controlPlane, err := rke2.NewControlPlane(ctx, r.Client, cluster, rcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")

		return ctrl.Result{}, err
	}

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Ignoring failures given that we are deleting
	if _, err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil {
		logger.Info("failed to reconcile conditions", "error", err.Error())
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	// However, during delete we are hiding the counter (1 of x) because it does not make sense given that
	// all the machines are deleted in parallel.
	conditions.SetAggregate(rcp,
		controlplanev1.MachinesReadyCondition,
		ownedMachines.ConditionGetters(),
		conditions.AddSourceRef(),
		conditions.WithStepCounterIf(false))

	// Verify that only control plane machines remain
	if len(allMachines) != len(ownedMachines) {
		logger.Info("Waiting for worker nodes to be deleted first")
		conditions.MarkFalse(rcp,
			controlplanev1.ResizedCondition,
			clusterv1.DeletingReason,
			clusterv1.ConditionSeverityInfo,
			"Waiting for worker nodes to be deleted first")

		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Delete control plane machines in parallel
	machinesToDelete := ownedMachines.Filter(collections.Not(collections.HasDeletionTimestamp))

	var errs []error

	for i := range machinesToDelete {
		m := machinesToDelete[i]
		logger := logger.WithValues("machine", m)

		if err := r.Client.Delete(ctx, machinesToDelete[i]); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to cleanup owned machine")
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		err := kerrors.NewAggregate(errs)
		r.recorder.Eventf(rcp, corev1.EventTypeWarning, "FailedDelete",
			"Failed to delete control plane Machines for cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)

		return ctrl.Result{}, err
	}

	conditions.MarkFalse(rcp, controlplanev1.ResizedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")

	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

func (r *RKE2ControlPlaneReconciler) reconcileKubeconfig(
	ctx context.Context,
	clusterName client.ObjectKey,
	endpoint clusterv1.APIEndpoint,
	rcp *controlplanev1.RKE2ControlPlane,
) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)
	if endpoint.IsZero() {
		logger.V(5).Info("API Endpoint not yet known")

		return ctrl.Result{RequeueAfter: DefaultRequeueTime}, nil
	}

	controllerOwnerRef := *metav1.NewControllerRef(rcp, controlplanev1.GroupVersion.WithKind("RKE2ControlPlane"))
	configSecret, err := secret.GetFromNamespacedName(ctx, r.Client, clusterName, secret.Kubeconfig)

	switch {
	case apierrors.IsNotFound(errors.Cause(err)):
		createErr := kubeconfig.CreateSecretWithOwner(
			ctx,
			r.Client,
			clusterName,
			endpoint.String(),
			controllerOwnerRef,
		)
		if errors.Is(createErr, kubeconfig.ErrDependentCertificateNotFound) {
			return ctrl.Result{RequeueAfter: dependentCertRequeueAfter}, nil
		}
		// always return if we have just created in order to skip rotation checks
		return ctrl.Result{}, createErr

	case err != nil:
		return ctrl.Result{}, errors.Wrap(err, "failed to retrieve kubeconfig Secret")
	}

	// only do rotation on owned secrets
	if !util.IsControlledBy(configSecret, rcp) {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// reconcileControlPlaneConditions is responsible of reconciling conditions reporting the status of static pods and
// the status of the etcd cluster.
func (r *RKE2ControlPlaneReconciler) reconcileControlPlaneConditions(
	ctx context.Context, controlPlane *rke2.ControlPlane,
) (res ctrl.Result, retErr error) {
	logger := log.FromContext(ctx)

	readyCPMachines := controlPlane.Machines.Filter(collections.IsReady())

	if readyCPMachines.Len() == 0 {
		controlPlane.RCP.Status.Initialized = false
		controlPlane.RCP.Status.Ready = false
		controlPlane.RCP.Status.ReadyReplicas = 0
		controlPlane.RCP.Status.AvailableServerIPs = nil
		conditions.MarkFalse(
			controlPlane.RCP,
			controlplanev1.AvailableCondition,
			controlplanev1.WaitingForRKE2ServerReason,
			clusterv1.ConditionSeverityInfo, "")
		conditions.MarkFalse(
			controlPlane.RCP,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.WaitingForRKE2ServerReason,
			clusterv1.ConditionSeverityInfo, "")
	}

	// If the cluster is not yet initialized, there is no way to connect to the workload cluster and fetch information
	// for updating conditions. Return early.
	if !controlPlane.RCP.Status.Initialized {
		return ctrl.Result{}, nil
	}

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(controlPlane.Cluster))
	if err != nil {
		logger.Error(err, "Unable to get Workload cluster")

		return ctrl.Result{}, errors.Wrap(err, "cannot get remote client to workload cluster")
	}

	defer func() {
		// Always attempt to Patch the Machine conditions after each reconcile.
		if err := controlPlane.PatchMachines(ctx); err != nil {
			retErr = kerrors.NewAggregate([]error{retErr, err})
		}
	}()

	if err := workloadCluster.InitWorkload(ctx, controlPlane); err != nil {
		logger.Error(err, "Unable to initialize workload cluster")

		return ctrl.Result{}, err
	}

	// Update conditions status
	workloadCluster.UpdateAgentConditions(controlPlane)
	workloadCluster.UpdateEtcdConditions(controlPlane)

	// Patch nodes metadata
	if err := workloadCluster.UpdateNodeMetadata(ctx, controlPlane); err != nil {
		logger.Error(err, "Unable to update node metadata")

		return ctrl.Result{}, err
	}

	// RCP will be patched at the end of Reconcile to reflect updated conditions, so we can return now.
	return ctrl.Result{}, nil
}

func (r *RKE2ControlPlaneReconciler) upgradeControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	rcp *controlplanev1.RKE2ControlPlane,
	controlPlane *rke2.ControlPlane,
	machinesRequireUpgrade collections.Machines,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	// If the cluster is not yet initialized, there is no way to connect to the workload cluster and fetch information
	// for updating conditions. Return early.
	if !rcp.Status.Initialized {
		logger.Info("ControlPlane not yet initialized")

		return ctrl.Result{}, nil
	}

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		logger.Error(err, "failed to get remote client for workload cluster", "cluster key", util.ObjectKey(cluster))

		return ctrl.Result{}, err
	}

	if err := workloadCluster.InitWorkload(ctx, controlPlane); err != nil {
		return ctrl.Result{}, err
	}

	switch rcp.Spec.RolloutStrategy.Type {
	case controlplanev1.RollingUpdateStrategyType:
		// RolloutStrategy is currently defaulted and validated to be RollingUpdate.
		maxNodes := *rcp.Spec.Replicas + int32(rcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntValue())
		if int32(controlPlane.Machines.Len()) < maxNodes {
			// scaleUpControlPlane ensures that we don't continue scaling up while waiting for Machines to have NodeRefs
			return r.scaleUpControlPlane(ctx, cluster, rcp, controlPlane)
		}

		return r.scaleDownControlPlane(ctx, cluster, rcp, controlPlane, machinesRequireUpgrade)
	default:
		err := fmt.Errorf("unknown rollout strategy type %q", rcp.Spec.RolloutStrategy.Type)
		logger.Error(err, "RolloutStrategy type is not set to RollingUpdateStrategyType, unable to determine the strategy for rolling out machines")

		return ctrl.Result{}, nil
	}
}

// ClusterToRKE2ControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for RKE2ControlPlane based on updates to a Cluster.
func (r *RKE2ControlPlaneReconciler) ClusterToRKE2ControlPlane(ctx context.Context) handler.MapFunc {
	log := log.FromContext(ctx)

	return func(_ context.Context, o client.Object) []ctrl.Request {
		c, ok := o.(*clusterv1.Cluster)
		if !ok {
			log.Error(nil, fmt.Sprintf("Expected a Cluster but got a %T", o))

			return nil
		}

		controlPlaneRef := c.Spec.ControlPlaneRef
		if controlPlaneRef != nil && controlPlaneRef.Kind == "RKE2ControlPlane" {
			return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
		}

		return nil
	}
}
