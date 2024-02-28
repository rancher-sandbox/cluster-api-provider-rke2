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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	kubeyaml "sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"

	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/cloudinit"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/ignition"
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/consts"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/locking"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/rke2"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/secret"
	bsutil "github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/util"
)

const (
	filePermissions  string = "0640"
	registrationPort int    = 9345
	serverURLFormat  string = "https://%v:%v"
	tokenPrefix      string = "-token"
)

// RKE2ConfigReconciler reconciles a Rke2Config object.
type RKE2ConfigReconciler struct {
	RKE2InitLock RKE2InitLock
	client.Client
	Scheme *runtime.Scheme
}

const (
	// DefaultManifestDirectory is the default directory to store kubernetes manifests that RKE2 will deploy automatically.
	DefaultManifestDirectory string = "/var/lib/rancher/rke2/server/manifests"

	// DefaultRequeueAfter is the default requeue time.
	DefaultRequeueAfter time.Duration = 20 * time.Second
	defaultTokenLength                = 16
)

//+kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=rke2configs;rke2configs/status;rke2configs/finalizers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=rke2controlplanes;rke2controlplanes/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machinesets;machines;machines/status;machinepools;machinepools/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RKE2ConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rerr error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconcile RKE2Config")

	scope, res, err := r.prepareScope(ctx, logger, req)
	if err != nil {
		if errors.Is(errors.Cause(err), util.ErrNoCluster) {
			logger.Info(fmt.Sprintf("%s does not belong to a cluster yet, waiting until it's part of a cluster", scope.Machine.Kind))

			return res, nil
		}

		if apierrors.IsNotFound(err) {
			logger.Info("Cluster does not exist yet, waiting until it is created")

			return res, nil
		}

		logger.Error(err, "Could not get cluster with metadata")

		return res, nil
	}

	if res != (ctrl.Result{}) {
		return res, nil
	}

	ctx = ctrl.LoggerInto(ctx, logger)

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(scope.Config, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Attempt to Patch the RKE2Config object and status after each reconciliation if no error occurs.
	defer func() {
		conditions.SetSummary(scope.Config,
			conditions.WithConditions(
				bootstrapv1.DataSecretAvailableCondition,
			),
		)

		patchOpts := []patch.Option{}
		if rerr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}

		if err := patchHelper.Patch(ctx, scope.Config, patchOpts...); err != nil {
			logger.Error(rerr, "Failed to patch config")

			if rerr == nil {
				rerr = err
			}
		}
	}()

	if !scope.Cluster.Status.InfrastructureReady {
		logger.Info("Infrastructure machine not yet ready")
		conditions.MarkFalse(
			scope.Config,
			bootstrapv1.DataSecretAvailableCondition,
			bootstrapv1.WaitingForClusterInfrastructureReason,
			clusterv1.ConditionSeverityInfo,
			"")

		return ctrl.Result{Requeue: true}, nil
	}

	if scope.Machine.Spec.Bootstrap.DataSecretName != nil && (!scope.Config.Status.Ready || scope.Config.Status.DataSecretName == nil) {
		scope.Config.Status.Ready = true
		scope.Config.Status.DataSecretName = scope.Machine.Spec.Bootstrap.DataSecretName
		conditions.MarkTrue(scope.Config, bootstrapv1.DataSecretAvailableCondition)

		return ctrl.Result{}, nil
	}
	// Status is ready means a config has been generated.
	if scope.Config.Status.Ready {
		// In any other case just return as the config is already generated and need not be generated again.
		conditions.MarkTrue(scope.Config, bootstrapv1.DataSecretAvailableCondition)

		return ctrl.Result{}, nil
	}

	// Note: can't use IsFalse here because we need to handle the absence of the condition as well as false.
	if !conditions.IsTrue(scope.Cluster, clusterv1.ControlPlaneInitializedCondition) {
		return r.handleClusterNotInitialized(ctx, scope)
	}

	// Unlock any locks that might have been set during init process
	r.RKE2InitLock.Unlock(ctx, scope.Cluster)

	// it's a control plane join
	if scope.HasControlPlaneOwner {
		return r.joinControlplane(ctx, scope)
	}

	// It's a worker join
	// GetTheControlPlane for the worker
	wkControlPlane := controlplanev1.RKE2ControlPlane{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Namespace: scope.Cluster.Spec.ControlPlaneRef.Namespace,
		Name:      scope.Cluster.Spec.ControlPlaneRef.Name,
	}, &wkControlPlane)

	if err != nil {
		scope.Logger.Info("Unable to find control plane object for owning Cluster", "error", err)

		return ctrl.Result{Requeue: true}, nil
	}

	scope.ControlPlane = &wkControlPlane

	return r.joinWorker(ctx, scope)
}

func (r *RKE2ConfigReconciler) prepareScope(
	ctx context.Context,
	logger logr.Logger,
	req ctrl.Request,
) (*Scope, ctrl.Result, error) {
	config := &bootstrapv1.RKE2Config{}

	if err := r.Get(ctx, req.NamespacedName, config, &client.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Error(err, "rke2Config not found", "rke2-config-name", req.NamespacedName)

			return nil, ctrl.Result{}, err
		}

		logger.Error(err, "", "rke2-config-namespaced-name", req.NamespacedName)

		return nil, ctrl.Result{Requeue: true}, err
	}

	machine, err := util.GetOwnerMachine(ctx, r.Client, config.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to retrieve owner Machine from the API Server", "RKE2Config", config.Namespace+"/"+config.Name)

		return nil, ctrl.Result{}, err
	}

	if machine == nil {
		logger.Info("Machine Controller has not yet set OwnerRef")

		return nil, ctrl.Result{Requeue: true}, nil
	}

	logger = logger.WithValues(machine.Kind, machine.GetNamespace()+"/"+machine.GetName(), "resourceVersion", machine.GetResourceVersion())
	scope := &Scope{
		Config:  config,
		Machine: machine,
		Logger:  logger,
	}

	// Getting the ControlPlane owner
	cp, err := bsutil.GetOwnerControlPlane(ctx, r.Client, scope.Machine.ObjectMeta)
	if err != nil && cp != nil {
		logger.Error(err, "Failed to retrieve owner ControlPlane from the API Server", "RKE2Config", config.Namespace+"/"+config.Name)

		return nil, ctrl.Result{RequeueAfter: DefaultRequeueAfter}, err
	}

	if cp == nil {
		logger.V(5).Info("This config is for a worker node")

		scope.HasControlPlaneOwner = false
	} else {
		logger.Info("This config is for a ControlPlane node")
		scope.HasControlPlaneOwner = true
		scope.ControlPlane = cp
		logger = logger.WithValues(cp.Kind, cp.GetNamespace()+"/"+cp.GetName(), "resourceVersion", cp.GetResourceVersion())
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, machine.GetNamespace(), machine.Spec.ClusterName)
	if err != nil {
		return nil, ctrl.Result{RequeueAfter: DefaultRequeueAfter}, err
	}

	if annotations.IsPaused(cluster, config) {
		logger.Info("Reconciliation is paused for this object")

		return nil, ctrl.Result{RequeueAfter: DefaultRequeueAfter}, fmt.Errorf("reconciliation is paused for this object")
	}

	scope.Cluster = cluster

	return scope, ctrl.Result{}, nil
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

// handleClusterNotInitialized handles the first control plane node.
func (r *RKE2ConfigReconciler) handleClusterNotInitialized(ctx context.Context, scope *Scope) (res ctrl.Result, reterr error) { //nolint:funlen
	if !scope.HasControlPlaneOwner {
		scope.Logger.Info("Requeuing because this machine is not a Control Plane machine")

		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, nil
	}

	if !r.RKE2InitLock.Lock(ctx, scope.Cluster, scope.Machine) {
		scope.Logger.Info("A control plane is already being initialized, requeuing until control plane is ready")

		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, nil
	}

	defer func() {
		if reterr != nil {
			if !r.RKE2InitLock.Unlock(ctx, scope.Cluster) {
				reterr = kerrors.NewAggregate([]error{reterr, errors.New("failed to unlock the rke2 init lock")})
			}
		}
	}()

	certificates := secret.NewCertificatesForInitialControlPlane()
	if err := certificates.LookupOrGenerate(
		ctx,
		r.Client,
		util.ObjectKey(scope.Cluster),
		*metav1.NewControllerRef(scope.Config, bootstrapv1.GroupVersion.WithKind("RKE2Config")),
	); err != nil {
		conditions.MarkFalse(
			scope.Config,
			bootstrapv1.CertificatesAvailableCondition,
			bootstrapv1.CertificatesGenerationFailedReason,
			clusterv1.ConditionSeverityWarning,
			err.Error())

		return ctrl.Result{}, err
	}

	conditions.MarkTrue(scope.Config, bootstrapv1.CertificatesAvailableCondition)

	// RKE2 server token must only be generated once, so all nodes join the cluster with the same registration token.
	var token string

	tokenName := bsutil.TokenName(scope.Cluster.Name)
	token, err := r.generateAndStoreToken(ctx, scope, tokenName)

	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			scope.Logger.Error(err, "unable to generate and store an RKE2 server token")

			return ctrl.Result{}, err
		}

		token, err = r.getRegistrationTokenFromSecretValue(ctx, tokenName, scope.Cluster.Namespace)
		if err != nil {
			scope.Logger.Error(err, "unable to retrieve an RKE2 server token from existing secret")

			return ctrl.Result{}, err
		}
	} else {
		scope.Logger.Info("RKE2 server token generated and stored in Secret!")
	}

	configStruct, configFiles, err := rke2.GenerateInitControlPlaneConfig(
		rke2.ServerConfigOpts{
			Cluster:              *scope.Cluster,
			ControlPlaneEndpoint: scope.Cluster.Spec.ControlPlaneEndpoint.Host,
			Token:                token,
			ServerURL:            fmt.Sprintf(serverURLFormat, scope.Cluster.Spec.ControlPlaneEndpoint.Host, registrationPort),
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
		Owner:       consts.DefaultFileOwner,
		Permissions: filePermissions,
	}

	files, err := r.generateFileListIncludingRegistries(ctx, scope, configFiles)
	if err != nil {
		return ctrl.Result{}, err
	}

	manifestFiles, err := generateFilesFromManifestConfig(ctx, r.Client, scope.ControlPlane.Spec.ManifestsConfigMapReference)
	if err != nil {
		manifestCm := scope.ControlPlane.Spec.ManifestsConfigMapReference.Name
		ns := scope.ControlPlane.Spec.ManifestsConfigMapReference.Namespace

		if apierrors.IsNotFound(err) {
			scope.Logger.Error(err, "Manifest ConfigMap referenced!", "namespace", ns, "name", manifestCm)

			return ctrl.Result{}, err
		}

		scope.Logger.Error(err, "Problem when getting Manifest ConfigMap!", "namespace", ns, "name", manifestCm)

		return ctrl.Result{}, err
	}

	files = append(files, manifestFiles...)

	var ntpServers []string
	if scope.Config.Spec.AgentConfig.NTP != nil {
		ntpServers = scope.Config.Spec.AgentConfig.NTP.Servers
	}

	cpinput := &cloudinit.ControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			AirGapped:               scope.Config.Spec.AgentConfig.AirGapped,
			CISEnabled:              scope.Config.Spec.AgentConfig.CISProfile != "",
			PreRKE2Commands:         scope.Config.Spec.PreRKE2Commands,
			PostRKE2Commands:        scope.Config.Spec.PostRKE2Commands,
			ConfigFile:              initConfigFile,
			RKE2Version:             scope.Config.Spec.AgentConfig.Version,
			WriteFiles:              files,
			NTPServers:              ntpServers,
			AdditionalCloudInit:     scope.Config.Spec.AgentConfig.AdditionalUserData.Config,
			AdditionalArbitraryData: scope.Config.Spec.AgentConfig.AdditionalUserData.Data,
		},
		Certificates: certificates,
	}

	var userData []byte

	switch scope.Config.Spec.AgentConfig.Format {
	case bootstrapv1.Ignition:
		userData, err = ignition.NewInitControlPlane(&ignition.ControlPlaneInput{
			ControlPlaneInput:  cpinput,
			AdditionalIgnition: &scope.Config.Spec.AgentConfig.AdditionalUserData,
		})
	default:
		userData, err = cloudinit.NewInitControlPlane(cpinput)
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.storeBootstrapData(ctx, scope, userData); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// generateFileListIncludingRegistries generates a list of files to be written to disk on the node
// This list includes a registries.yaml file if the user has provided a PrivateRegistriesConfig
// and the files fields provided in the RKE2Config.
func (r *RKE2ConfigReconciler) generateFileListIncludingRegistries(
	ctx context.Context,
	scope *Scope,
	configFiles []bootstrapv1.File,
) ([]bootstrapv1.File, error) {
	registries, registryFiles, err := rke2.GenerateRegistries(rke2.RegistryScope{
		Registry: scope.Config.Spec.PrivateRegistriesConfig,
		Client:   r.Client,
		Ctx:      ctx,
		Logger:   scope.Logger,
	})
	if err != nil {
		scope.Logger.Error(err, "unable to generate registries.yaml for Init Control Plane node")

		return nil, err
	}

	registriesYAML, err := kubeyaml.Marshal(registries)
	if err != nil {
		scope.Logger.Error(err, "unable to marshall registries.yaml")

		return nil, err
	}

	scope.Logger.V(4).Info("Registries.yaml marshalled successfully")

	initRegistriesFile := bootstrapv1.File{
		Path:        rke2.DefaultRKE2RegistriesLocation,
		Content:     string(registriesYAML),
		Owner:       consts.DefaultFileOwner,
		Permissions: filePermissions,
	}

	additionalFiles := []bootstrapv1.File{}

	for _, file := range scope.Config.Spec.Files {
		if file.ContentFrom != nil {
			scope.Logger.V(5).Info("File content is coming from a Secret, getting the content...")

			fileContentSecret := &corev1.Secret{}

			if err := r.Client.Get(ctx, types.NamespacedName{
				Name:      file.ContentFrom.Secret.Name,
				Namespace: scope.Config.Namespace,
			}, fileContentSecret); err != nil {
				return nil, fmt.Errorf("unable to get secret %s/%s: %w", scope.Config.Namespace, file.ContentFrom.Secret.Name, err)
			}

			fileContent := fileContentSecret.Data[file.ContentFrom.Secret.Key]
			if fileContent == nil {
				return nil, fmt.Errorf("file content is empty for secret %s/%s, secret key %s",
					scope.Config.Namespace, file.ContentFrom.Secret.Name, file.ContentFrom.Secret.Key)
			}

			file.Content = string(fileContent)
			file.ContentFrom = nil
		}

		additionalFiles = append(additionalFiles, file)
	}

	files := configFiles
	files = append(files, registryFiles...)
	files = append(files, initRegistriesFile)
	files = append(files, additionalFiles...)

	return files, nil
}

// RKE2InitLock is an interface for locking/unlocking Machine Creation as soon as an Init Process for the Control Plane
// has been started.
type RKE2InitLock interface {
	Unlock(ctx context.Context, cluster *clusterv1.Cluster) bool
	Lock(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) bool
}

// joinControlPlane implements the part of the Reconciler which bootstraps a secondary
// Control Plane machine joining a cluster that is already initialized.
func (r *RKE2ConfigReconciler) joinControlplane(ctx context.Context, scope *Scope) (res ctrl.Result, rerr error) {
	tokenSecret := &corev1.Secret{}

	secretKey := types.NamespacedName{
		Namespace: scope.Cluster.Namespace,
		Name:      scope.Cluster.Name + tokenPrefix,
	}
	if err := r.Client.Get(ctx, secretKey, tokenSecret); err != nil {
		scope.Logger.Error(
			err,
			"Token for already initialized RKE2 Cluster not found", "token-namespace",
			scope.Cluster.Namespace, "token-name",
			scope.Cluster.Name+tokenPrefix)

		return ctrl.Result{}, err
	}

	token := string(tokenSecret.Data["value"])

	scope.Logger.Info("RKE2 server token found in Secret!")

	if len(scope.ControlPlane.Status.AvailableServerIPs) == 0 {
		scope.Logger.Info("No ControlPlane IP Address found for node registration")

		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, nil
	}

	configStruct, configFiles, err := rke2.GenerateJoinControlPlaneConfig(
		rke2.ServerConfigOpts{
			Cluster:              *scope.Cluster,
			Token:                token,
			ControlPlaneEndpoint: scope.Cluster.Spec.ControlPlaneEndpoint.Host,
			ServerURL:            fmt.Sprintf(serverURLFormat, scope.ControlPlane.Status.AvailableServerIPs[0], registrationPort),
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
		Owner:       consts.DefaultFileOwner,
		Permissions: filePermissions,
	}

	files, err := r.generateFileListIncludingRegistries(ctx, scope, configFiles)
	if err != nil {
		return ctrl.Result{}, err
	}

	manifestFiles, err := generateFilesFromManifestConfig(ctx, r.Client, scope.ControlPlane.Spec.ManifestsConfigMapReference)
	if err != nil {
		manifestCm := scope.ControlPlane.Spec.ManifestsConfigMapReference.Name
		ns := scope.ControlPlane.Spec.ManifestsConfigMapReference.Namespace

		if apierrors.IsNotFound(err) {
			scope.Logger.Error(err, "Manifest ConfigMap not found!", "namespace", ns, "name", manifestCm)

			return ctrl.Result{}, err
		}

		scope.Logger.Error(err, "Problem when getting ConfigMap referenced by manifestsConfigMapReference", "namespace", ns, "name", manifestCm)

		return ctrl.Result{}, err
	}

	files = append(files, manifestFiles...)

	var ntpServers []string
	if scope.Config.Spec.AgentConfig.NTP != nil {
		ntpServers = scope.Config.Spec.AgentConfig.NTP.Servers
	}

	cpinput := &cloudinit.ControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			AirGapped:           scope.Config.Spec.AgentConfig.AirGapped,
			CISEnabled:          scope.Config.Spec.AgentConfig.CISProfile != "",
			PreRKE2Commands:     scope.Config.Spec.PreRKE2Commands,
			PostRKE2Commands:    scope.Config.Spec.PostRKE2Commands,
			ConfigFile:          initConfigFile,
			RKE2Version:         scope.Config.Spec.AgentConfig.Version,
			WriteFiles:          files,
			NTPServers:          ntpServers,
			AdditionalCloudInit: scope.Config.Spec.AgentConfig.AdditionalUserData.Config,
		},
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	var userData []byte

	switch scope.Config.Spec.AgentConfig.Format {
	case bootstrapv1.Ignition:
		userData, err = ignition.NewJoinControlPlane(&ignition.ControlPlaneInput{
			ControlPlaneInput:  cpinput,
			AdditionalIgnition: &scope.Config.Spec.AgentConfig.AdditionalUserData,
		})
	default:
		userData, err = cloudinit.NewJoinControlPlane(cpinput)
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.storeBootstrapData(ctx, scope, userData); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// joinWorker implements the part of the Reconciler which bootstraps a worker node
// after the cluster has been initialized.
func (r *RKE2ConfigReconciler) joinWorker(ctx context.Context, scope *Scope) (res ctrl.Result, rerr error) {
	tokenSecret := &corev1.Secret{}

	secretKey := types.NamespacedName{
		Namespace: scope.Cluster.Namespace,
		Name:      scope.Cluster.Name + tokenPrefix,
	}
	if err := r.Client.Get(ctx, secretKey, tokenSecret); err != nil {
		scope.Logger.Info(
			"Token for already initialized RKE2 Cluster not found",
			"token-namespace",
			scope.Cluster.Namespace,
			"token-name",
			scope.Cluster.Name+tokenPrefix)

		return ctrl.Result{}, err
	}

	token := string(tokenSecret.Data["value"])

	scope.Logger.Info("RKE2 server token found in Secret!")

	if len(scope.ControlPlane.Status.AvailableServerIPs) == 0 {
		scope.Logger.V(1).Info("No ControlPlane IP Address found for node registration")

		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, nil
	}

	configStruct, configFiles, err := rke2.GenerateWorkerConfig(
		rke2.AgentConfigOpts{
			ServerURL:              fmt.Sprintf(serverURLFormat, scope.ControlPlane.Status.AvailableServerIPs[0], registrationPort),
			Token:                  token,
			AgentConfig:            scope.Config.Spec.AgentConfig,
			Ctx:                    ctx,
			Client:                 r.Client,
			CloudProviderName:      scope.ControlPlane.Spec.ServerConfig.CloudProviderName,
			CloudProviderConfigMap: scope.ControlPlane.Spec.ServerConfig.CloudProviderConfigMap,
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
		Owner:       consts.DefaultFileOwner,
		Permissions: filePermissions,
	}

	files, err := r.generateFileListIncludingRegistries(ctx, scope, configFiles)
	if err != nil {
		return ctrl.Result{}, err
	}

	var ntpServers []string
	if scope.Config.Spec.AgentConfig.NTP != nil {
		ntpServers = scope.Config.Spec.AgentConfig.NTP.Servers
	}

	wkInput := &cloudinit.BaseUserData{
		PreRKE2Commands:         scope.Config.Spec.PreRKE2Commands,
		AirGapped:               scope.Config.Spec.AgentConfig.AirGapped,
		CISEnabled:              scope.Config.Spec.AgentConfig.CISProfile != "",
		PostRKE2Commands:        scope.Config.Spec.PostRKE2Commands,
		ConfigFile:              wkJoinConfigFile,
		RKE2Version:             scope.Config.Spec.AgentConfig.Version,
		WriteFiles:              files,
		NTPServers:              ntpServers,
		AdditionalCloudInit:     scope.Config.Spec.AgentConfig.AdditionalUserData.Config,
		AdditionalArbitraryData: scope.Config.Spec.AgentConfig.AdditionalUserData.Data,
	}

	var userData []byte

	switch scope.Config.Spec.AgentConfig.Format {
	case bootstrapv1.Ignition:
		userData, err = ignition.NewJoinWorker(&ignition.JoinWorkerInput{
			BaseUserData:       wkInput,
			AdditionalIgnition: &scope.Config.Spec.AgentConfig.AdditionalUserData,
		})
	default:
		userData, err = cloudinit.NewJoinWorker(wkInput)
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.storeBootstrapData(ctx, scope, userData); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// getRegistrationTokenFromSecretValue retrieves the registration token from an existing secret's value.
func (r *RKE2ConfigReconciler) getRegistrationTokenFromSecretValue(ctx context.Context, name, namespace string) (string, error) {
	tokenSecret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}

	err := r.Client.Get(ctx, secretKey, tokenSecret)
	if err != nil {
		return "", errors.Wrapf(err, "could not retrieve secret %s/%s", namespace, name)
	}

	return string(tokenSecret.Data["value"]), nil
}

// generateAndStoreToken generates a random token with 16 characters then stores it in a Secret in the API.
func (r *RKE2ConfigReconciler) generateAndStoreToken(ctx context.Context, scope *Scope, name string) (string, error) {
	token, err := bsutil.Random(defaultTokenLength)
	if err != nil {
		return "", err
	}

	scope.Logger = scope.Logger.WithValues("cluster-name", scope.Cluster.Name)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: scope.Config.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: scope.Cluster.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: scope.Cluster.APIVersion,
					Kind:       scope.Cluster.Kind,
					Name:       scope.Cluster.Name,
					UID:        scope.Cluster.UID,
					Controller: pointer.Bool(true),
				},
			},
		},
		Data: map[string][]byte{
			"value": []byte(token),
		},
		Type: clusterv1.ClusterSecretType,
	}

	if err := r.createSecretFromObject(ctx, *secret, scope.Logger, "token", *scope.Config); err != nil {
		return "", err
	}

	return token, nil
}

// storeBootstrapData creates a new secret with the data passed in as input,
// sets the reference in the configuration status and ready to true.
func (r *RKE2ConfigReconciler) storeBootstrapData(ctx context.Context, scope *Scope, data []byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scope.Config.Name,
			Namespace: scope.Config.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: scope.Cluster.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: scope.Config.APIVersion,
					Kind:       scope.Config.Kind,
					Name:       scope.Config.Name,
					UID:        scope.Config.UID,
					Controller: pointer.Bool(true),
				},
			},
		},
		Data: map[string][]byte{
			"value":  data,
			"format": []byte(scope.Config.Spec.AgentConfig.Format),
		},
		Type: clusterv1.ClusterSecretType,
	}

	if err := r.createOrUpdateSecretFromObject(ctx, *secret, scope.Logger, "bootstrap data", *scope.Config); err != nil {
		return err
	}

	scope.Config.Status.DataSecretName = pointer.String(secret.Name)
	scope.Config.Status.Ready = true
	//	conditions.MarkTrue(scope.Config, bootstrapv1.DataSecretAvailableCondition)
	return nil
}

// createSecretFromObject tries to create the given secret in the API, if that secret exists it will return an error.
func (r *RKE2ConfigReconciler) createSecretFromObject(
	ctx context.Context,
	secret corev1.Secret,
	logger logr.Logger,
	secretType string,
	config bootstrapv1.RKE2Config,
) (reterr error) {
	if err := r.Client.Create(ctx, &secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create %s secret for %s: %s/%s", secretType, config.Kind, config.Name, config.Namespace)
		}

		logger.Info("Secret already exists, won't update it",
			"secret-type", secretType,
			"secret-ref", secret.Namespace+"/"+secret.Name,
			"RKE2Config", config.Name)

		return err
	}

	return
}

// createOrUpdateSecret tries to create the given secret in the API, if that secret exists it will update it.
func (r *RKE2ConfigReconciler) createOrUpdateSecretFromObject(
	ctx context.Context,
	secret corev1.Secret,
	logger logr.Logger,
	secretType string,
	config bootstrapv1.RKE2Config,
) (reterr error) {
	if err := r.Client.Create(ctx, &secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create %s secret for %s: %s/%s", secretType, config.Kind, config.Name, config.Namespace)
		}

		logger.Info("Secret for already exists, updating",
			"secret-type", secretType,
			"secret-ref", secret.Namespace+"/"+secret.Name,
			"RKE2Config", config.Name)

		if err := r.Client.Update(ctx, &secret); err != nil {
			return errors.Wrapf(err, "failed to update %s secret for %s: %s/%s", secretType, config.Kind, config.Namespace, config.Name)
		}
	}

	return
}

func generateFilesFromManifestConfig(
	ctx context.Context,
	cl client.Client,
	manifestConfigMap corev1.ObjectReference,
) (files []bootstrapv1.File, err error) {
	if (manifestConfigMap == corev1.ObjectReference{}) {
		return []bootstrapv1.File{}, nil
	}

	manifestSec := &corev1.ConfigMap{}

	err = cl.Get(ctx, types.NamespacedName{
		Namespace: manifestConfigMap.Namespace,
		Name:      manifestConfigMap.Name,
	}, manifestSec)

	if err != nil {
		return
	}

	for filename, content := range manifestSec.Data {
		files = append(files, bootstrapv1.File{
			Path:    DefaultManifestDirectory + "/" + filename,
			Content: content,
		})
	}

	return
}
