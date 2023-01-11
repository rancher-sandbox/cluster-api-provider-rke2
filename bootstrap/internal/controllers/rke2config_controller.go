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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha1"
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha1"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/cloudinit"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/locking"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/rke2"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/secret"
	bsutil "github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	kubeyaml "sigs.k8s.io/yaml"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RKE2ConfigReconciler reconciles a Rke2Config object
type RKE2ConfigReconciler struct {
	RKE2InitLock RKE2InitLock
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=rke2configs;rke2configs/status;rke2configs/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes;rke2controlplanes/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machinesets;machines;machines/status;machinepools;machinepools/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Rke2Config object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *RKE2ConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rerr error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconcile RKE2Config")

	config := &bootstrapv1.RKE2Config{}

	if err := r.Get(ctx, req.NamespacedName, config, &client.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Error(err, "rke2Config not found", "rke2-config-name", req.NamespacedName)
			return ctrl.Result{}, err
		}
		logger.Error(err, "", "rke2-config-namespaced-name", req.NamespacedName)
		return ctrl.Result{Requeue: true}, err
	}

	scope := &Scope{}

	machine, err := util.GetOwnerMachine(ctx, r.Client, config.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to retrieve owner Machine from the API Server", config.Namespace+"/"+config.Name, "machine", machine.Name)
		return ctrl.Result{}, err
	}
	if machine == nil {
		logger.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{Requeue: true}, nil
	}
	scope.Machine = machine
	logger = logger.WithValues(machine.Kind, machine.GetNamespace()+"/"+machine.GetName(), "resourceVersion", machine.GetResourceVersion())

	// Getting the ControlPlane owner
	cp, err := bsutil.GetOwnerControlPlane(ctx, r.Client, scope.Machine.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to retrieve owner ControlPlane from the API Server", config.Namespace+"/"+config.Name, "cluster", cp.Name)
		return ctrl.Result{}, err
	}
	if cp == nil {
		logger.Info("This config is for a worker node")
		scope.HasControlPlaneOwner = false
	} else {
		logger.Info("This config is for a ControlPlane node")
		scope.HasControlPlaneOwner = true
		scope.ControlPlane = cp
		logger = logger.WithValues(cp.Kind, cp.GetNamespace()+"/"+cp.GetName(), "resourceVersion", cp.GetResourceVersion())
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, machine.GetNamespace(), machine.Spec.ClusterName)
	if err != nil {
		if errors.Cause(err) == util.ErrNoCluster {
			logger.Info(fmt.Sprintf("%s does not belong to a cluster yet, waiting until it's part of a cluster", machine.Kind))
			return ctrl.Result{}, nil
		}

		if apierrors.IsNotFound(err) {
			logger.Info("Cluster does not exist yet, waiting until it is created")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Could not get cluster with metadata")
		return ctrl.Result{}, err
	}

	if annotations.IsPaused(cluster, config) {
		logger.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	scope.Cluster = cluster
	scope.Config = config
	scope.Logger = logger
	ctx = ctrl.LoggerInto(ctx, logger)

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(config, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Attempt to Patch the RKE2Config object and status after each reconciliation if no error occurs.
	defer func() {
		// always update the readyCondition; the summary is represented using the "1 of x completed" notation.
		conditions.SetSummary(config,
			conditions.WithConditions(
				bootstrapv1.DataSecretAvailableCondition,
			),
		)
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{}
		if rerr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, config, patchOpts...); err != nil {
			logger.Error(rerr, "Failed to patch config")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	if !cluster.Status.InfrastructureReady {
		logger.Info("Infrastructure machine not yet ready")
		conditions.MarkFalse(config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{Requeue: true}, nil
	}
	// Migrate plaintext data to secret.
	// if config.Status.BootstrapData != nil && config.Status.DataSecretName == nil {
	// 	return ctrl.Result{}, r.storeBootstrapData(ctx, scope, config.Status.BootstrapData)
	// }
	// Reconcile status for machines that already have a secret reference, but our status isn't up to date.
	// This case solves the pivoting scenario (or a backup restore) which doesn't preserve the status subresource on objects.
	if machine.Spec.Bootstrap.DataSecretName != nil && (!config.Status.Ready || config.Status.DataSecretName == nil) {
		config.Status.Ready = true
		config.Status.DataSecretName = machine.Spec.Bootstrap.DataSecretName
		conditions.MarkTrue(config, bootstrapv1.DataSecretAvailableCondition)
		return ctrl.Result{}, nil
	}
	// Status is ready means a config has been generated.
	if config.Status.Ready {
		// In any other case just return as the config is already generated and need not be generated again.
		conditions.MarkTrue(config, bootstrapv1.DataSecretAvailableCondition)
		return ctrl.Result{}, nil
	}

	// Note: can't use IsFalse here because we need to handle the absence of the condition as well as false.
	if !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		return r.handleClusterNotInitialized(ctx, scope)
	}

	// Every other case it's a join scenario

	// Unlock any locks that might have been set during init process
	r.RKE2InitLock.Unlock(ctx, cluster)

	// it's a control plane join
	if scope.HasControlPlaneOwner {
		return r.joinControlplane(ctx, scope)
	}

	// It's a worker join
	return r.joinWorker(ctx, scope)
	// if machine.Status.Phase != string(clusterv1.MachinePhasePending) {
	// 	logger.Info("Machine is not in pending state")
	// 	return ctrl.Result{Requeue: true}, nil
	// }

	// return ctrl.Result{}, nil
}

// Scope is a scoped struct used during reconciliation.
type Scope struct {
	Logger               logr.Logger
	Config               *bootstrapv1.RKE2Config
	Machine              *clusterv1.Machine
	Cluster              *clusterv1.Cluster
	HasControlPlaneOwner bool
	ControlPlane         *controlplanev1.RKE2ControlPlane
}

// SetupWithManager sets up the controller with the Manager.
func (r *RKE2ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {

	if r.RKE2InitLock == nil {
		r.RKE2InitLock = locking.NewControlPlaneInitMutex(mgr.GetClient())
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&bootstrapv1.RKE2Config{}).
		Complete(r)
}

// TODO: Implement these functions

// handleClusterNotInitialized handles the first control plane node
func (r *RKE2ConfigReconciler) handleClusterNotInitialized(ctx context.Context, scope *Scope) (res ctrl.Result, reterr error) {

	// initialize the DataSecretAvailableCondition if missing.
	// this is required in order to avoid the condition's LastTransitionTime to flicker in case of errors surfacing
	// using the DataSecretGeneratedFailedReason
	// if conditions.GetReason(scope.Config, bootstrapv1.DataSecretAvailableCondition) != bootstrapv1.DataSecretGenerationFailedReason {
	// 	conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, clusterv1.WaitingForControlPlaneAvailableReason, clusterv1.ConditionSeverityInfo, "")
	// }
	if !scope.HasControlPlaneOwner {
		scope.Logger.Info("Requeuing because this machine is not a Control Plane machine")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	if !r.RKE2InitLock.Lock(ctx, scope.Cluster, scope.Machine) {
		scope.Logger.Info("A control plane is already being initialized, requeuing until control plane is ready")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	defer func() {
		if reterr != nil {
			if !r.RKE2InitLock.Unlock(ctx, scope.Cluster) {
				reterr = kerrors.NewAggregate([]error{reterr, errors.New("failed to unlock the rke2 init lock")})
			}
		}
	}()

	certificates := secret.NewCertificatesForInitialControlPlane()
	err := certificates.LookupOrGenerate(
		ctx,
		r.Client,
		util.ObjectKey(scope.Cluster),
		*metav1.NewControllerRef(scope.Config, bootstrapv1.GroupVersion.WithKind("RKE2Config")),
	)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.CertificatesAvailableCondition, bootstrapv1.CertificatesGenerationFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, err
	}
	conditions.MarkTrue(scope.Config, bootstrapv1.CertificatesAvailableCondition)

	token, err := r.generateAndStoreToken(ctx, scope)
	if err != nil {
		scope.Logger.Error(err, "unable to generate and store an RKE2 server token")
		return ctrl.Result{}, err
	}
	scope.Logger.Info("RKE2 server token generated and stored in Secret!")

	configStruct, files, err := rke2.GenerateInitControlPlaneConfig(
		rke2.RKE2ServerConfigOpts{
			ControlPlaneEndpoint: scope.Cluster.Spec.ControlPlaneEndpoint.Host,
			Token:                token,
			ServerURL:            "https://" + scope.Cluster.Spec.ControlPlaneEndpoint.Host + ":9345",
			ServerConfig:         scope.ControlPlane.Spec.ServerConfig,
			AgentConfig:          scope.Config.Spec.AgentConfig,
			Ctx:                  ctx,
			Client:               r.Client,
		})

	if err != nil {
		return ctrl.Result{}, err
	}

	b, err := kubeyaml.Marshal(configStruct)
	if err != nil {

		return ctrl.Result{}, err
	}
	scope.Logger.Info("Server config marshalled successfully")

	initConfigFile := bootstrapv1.File{
		Path:        rke2.DefaultRKE2ConfigLocation,
		Content:     string(b),
		Owner:       "root:root",
		Permissions: "0640",
	}

	// TODO: Implement adding additional files through API

	cpinput := &cloudinit.ControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			AirGapped:        scope.Config.Spec.AgentConfig.AirGapped,
			PreRKE2Commands:  scope.Config.Spec.PreRKE2Commands,
			PostRKE2Commands: scope.Config.Spec.PostRKE2Commands,
			ConfigFile:       initConfigFile,
			RKE2Version:      scope.Config.Spec.AgentConfig.Version,
			WriteFiles:       files,
		},
		Certificates: certificates,
	}

	cloudInitData, err := cloudinit.NewInitControlPlane(cpinput)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.storeBootstrapData(ctx, scope, cloudInitData); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

type RKE2InitLock interface {
	Unlock(ctx context.Context, cluster *clusterv1.Cluster) bool
	Lock(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) bool
}

// TODO: Implement these functions
func (r *RKE2ConfigReconciler) joinControlplane(ctx context.Context, scope *Scope) (res ctrl.Result, rerr error) {

	tokenSecret := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: scope.Cluster.Namespace, Name: scope.Cluster.Name + "-token"}, tokenSecret); err != nil {
		scope.Logger.Info("Token for already initialized RKE2 Cluster not found", "token-namespace", scope.Cluster.Namespace, "token-name", scope.Cluster.Name+"-token")
		return ctrl.Result{}, err
	}
	token := string(tokenSecret.Data["value"])

	scope.Logger.Info("RKE2 server token found in Secret!")

	configStruct, files, err := rke2.GenerateJoinControlPlaneConfig(
		rke2.RKE2ServerConfigOpts{
			Cluster:              *scope.Cluster,
			Token:                token,
			ControlPlaneEndpoint: scope.Cluster.Spec.ControlPlaneEndpoint.Host,
			ServerURL:            "https://" + scope.Cluster.Spec.ControlPlaneEndpoint.Host + ":9345",
			ServerConfig:         scope.ControlPlane.Spec.ServerConfig,
			AgentConfig:          scope.Config.Spec.AgentConfig,
			Ctx:                  ctx,
			Client:               r.Client,
		},
	)

	if err != nil {
		scope.Logger.Error(err, "unable to generate config.yaml for a Secondary Control Plane node")
		return ctrl.Result{}, err
	}

	b, err := kubeyaml.Marshal(configStruct)

	scope.Logger.Info("Showing marshalled config.yaml", "config.yaml", string(b))
	if err != nil {

		return ctrl.Result{}, err
	}
	scope.Logger.Info("Joining Server config marshalled successfully")

	initConfigFile := bootstrapv1.File{
		Path:        rke2.DefaultRKE2ConfigLocation,
		Content:     string(b),
		Owner:       "root:root",
		Permissions: "0640",
	}

	// TODO: Implement adding additional files through API

	cpinput := &cloudinit.ControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			AirGapped:        scope.Config.Spec.AgentConfig.AirGapped,
			PreRKE2Commands:  scope.Config.Spec.PreRKE2Commands,
			PostRKE2Commands: scope.Config.Spec.PostRKE2Commands,
			ConfigFile:       initConfigFile,
			RKE2Version:      scope.Config.Spec.AgentConfig.Version,
			WriteFiles:       files,
		},
	}

	cloudInitData, err := cloudinit.NewJoinControlPlane(cpinput)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.storeBootstrapData(ctx, scope, cloudInitData); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// TODO: Implement these functions
func (r *RKE2ConfigReconciler) joinWorker(ctx context.Context, scope *Scope) (res ctrl.Result, rerr error) {
	tokenSecret := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: scope.Cluster.Namespace, Name: scope.Cluster.Name + "-token"}, tokenSecret); err != nil {
		scope.Logger.Info("Token for already initialized RKE2 Cluster not found", "token-namespace", scope.Cluster.Namespace, "token-name", scope.Cluster.Name+"-token")
		return ctrl.Result{}, err
	}
	token := string(tokenSecret.Data["value"])
	scope.Logger.Info("RKE2 server token found in Secret!")

	configStruct, files, err := rke2.GenerateWorkerConfig(
		rke2.RKE2AgentConfigOpts{
			ServerURL:   "https://" + scope.Cluster.Spec.ControlPlaneEndpoint.Host + ":9345",
			Token:       token,
			AgentConfig: scope.Config.Spec.AgentConfig,
			Ctx:         ctx,
			Client:      r.Client,
		})

	if err != nil {
		return ctrl.Result{}, err
	}
	b, err := kubeyaml.Marshal(configStruct)

	scope.Logger.V(5).Info("Showing marshalled config.yaml", "config.yaml", string(b))
	if err != nil {
		return ctrl.Result{}, err
	}
	scope.Logger.Info("Joining Worker config marshalled successfully")

	wkJoinConfigFile := bootstrapv1.File{
		Path:        rke2.DefaultRKE2ConfigLocation,
		Content:     string(b),
		Owner:       "root:root",
		Permissions: "0640",
	}

	// TODO: Implement adding additional files through API

	wkInput :=
		&cloudinit.BaseUserData{
			PreRKE2Commands:  scope.Config.Spec.PreRKE2Commands,
			AirGapped:        scope.Config.Spec.AgentConfig.AirGapped,
			PostRKE2Commands: scope.Config.Spec.PostRKE2Commands,
			ConfigFile:       wkJoinConfigFile,
			RKE2Version:      scope.Config.Spec.AgentConfig.Version,
			WriteFiles:       files,
		}

	cloudInitData, err := cloudinit.NewJoinWorker(wkInput)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.storeBootstrapData(ctx, scope, cloudInitData); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RKE2ConfigReconciler) generateAndStoreToken(ctx context.Context, scope *Scope) (string, error) {
	tokn, err := bsutil.Random(16)
	if err != nil {
		return "", err
	}

	scope.Logger = scope.Logger.WithValues("cluster-name", scope.Cluster.Name)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bsutil.TokenName(scope.Cluster.Name),
			Namespace: scope.Config.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName: scope.Cluster.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: scope.Cluster.APIVersion,
					Kind:       scope.Cluster.Kind,
					Name:       scope.Cluster.Name,
					UID:        scope.Cluster.UID,
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Data: map[string][]byte{
			"value": []byte(tokn),
		},
		Type: clusterv1.ClusterSecretType,
	}

	// as secret creation and scope.Config status patch are not atomic operations
	// it is possible that secret creation happens but the config.Status patches are not applied
	if err := r.Client.Create(ctx, secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return "", errors.Wrapf(err, "failed to create token for RKE2Config %s/%s", scope.Config.Namespace, scope.Config.Name)
		}
		// r.Log.Info("bootstrap data secret for RKE2Config already exists, updating", "secret", secret.Name, "RKE2Config", scope.Config.Name)
		if err := r.Client.Update(ctx, secret); err != nil {
			return "", errors.Wrapf(err, "failed to update bootstrap token secret for RKE2Config %s/%s", scope.Config.Namespace, scope.Config.Name)
		}
	}

	return tokn, nil
}

// storeBootstrapData creates a new secret with the data passed in as input,
// sets the reference in the configuration status and ready to true.
func (r *RKE2ConfigReconciler) storeBootstrapData(ctx context.Context, scope *Scope, data []byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scope.Config.Name,
			Namespace: scope.Config.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName: scope.Cluster.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: bootstrapv1.GroupVersion.String(),
					Kind:       "RKE2Config",
					Name:       scope.Config.Name,
					UID:        scope.Config.UID,
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Data: map[string][]byte{
			"value": data,
		},
		Type: clusterv1.ClusterSecretType,
	}

	// as secret creation and scope.Config status patch are not atomic operations
	// it is possible that secret creation happens but the config.Status patches are not applied
	if err := r.Client.Create(ctx, secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create bootstrap data secret for RKE2Config %s/%s", scope.Config.Namespace, scope.Config.Name)
		}
		scope.Logger.Info("bootstrap data secret for RKE2Config already exists, updating", "secret", secret.Name, "RKE2Config", scope.Config.Name)
		if err := r.Client.Update(ctx, secret); err != nil {
			return errors.Wrapf(err, "failed to update bootstrap data secret for RKE2Config %s/%s", scope.Config.Namespace, scope.Config.Name)
		}
	}

	scope.Config.Status.DataSecretName = pointer.StringPtr(secret.Name)
	scope.Config.Status.Ready = true
	//	conditions.MarkTrue(scope.Config, bootstrapv1.DataSecretAvailableCondition)
	return nil
}
