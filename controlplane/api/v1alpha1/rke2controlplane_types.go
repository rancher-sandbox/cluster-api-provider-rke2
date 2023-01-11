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

package v1alpha1

import (
	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	RKE2ControlPlaneFinalizer = "rke2.controleplane.cluster.x-k8s.io"

	// RKE2ServerConfigurationAnnotation is a machine annotation that stores the json-marshalled string of RKE2Config
	// This annotation is used to detect any changes in RKE2Config and trigger machine rollout.
	RKE2ServerConfigurationAnnotation = "controlplane.cluster.x-k8s.io/rke2-server-configuration"
)

// RKE2ControlPlaneSpec defines the desired state of RKE2ControlPlane
type RKE2ControlPlaneSpec struct {
	// Replicas is the number of replicas for the Control Plane
	Replicas *int32 `json:"replicas,omitempty"`

	// RKE2AgentConfig references fields from the Agent Configuration in the Bootstrap Provider because an RKE2 Server node also has an agent
	bootstrapv1.RKE2AgentConfig `json:",inline"`

	// ServerConfig specifies configuration for the agent nodes.
	//+optional
	ServerConfig RKE2ServerConfig `json:"serverConfig,omitempty"`

	// ManifestsConfigMapReference references a ConfigMap which contains Kubernetes manifests to be deployed automatically on the cluster
	// Each data entry in the ConfigMap will be will be copied to a folder on the control plane nodes that RKE2 scans and uses to deploy manifests.
	//+optional
	ManifestsConfigMapReference corev1.ObjectReference `json:"manifestsConfigMapReference,omitempty"`

	// InfrastructureRef is a required reference to a custom resource
	// offered by an infrastructure provider.
	InfrastructureRef corev1.ObjectReference `json:"infrastructureRef"`

	// NodeDrainTimeout is the total amount of time that the controller will spend on draining a controlplane node
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`
}

type RKE2ServerConfig struct {
	// AuditPolicySecret path to the file that defines the audit policy configuration.
	//+optional
	AuditPolicySecret *corev1.ObjectReference `json:"auditPolicySecret,omitempty"`

	// BindAddress describes the rke2 bind address (default: 0.0.0.0).
	//+optional
	BindAddress string `json:"bindAddress,omitempty"`

	// AdvertiseAddress IP address that apiserver uses to advertise to members of the cluster (default: node-external-ip/node-ip).
	//+optional
	AdvertiseAddress string `json:"advertiseAddress,omitempty"`

	// TLSSan Add additional hostname or IP as a Subject Alternative Name in the TLS cert.
	//+optional
	TLSSan []string `json:"tlsSan,omitempty"`

	// ServiceNodePortRange is the port range to reserve for services with NodePort visibility (default: "30000-32767").
	//+optional
	ServiceNodePortRange string `json:"serviceNodePortRange,omitempty"`

	// ClusterDNS is the cluster IP for CoreDNS service. Should be in your service-cidr range (default: 10.43.0.10).
	//+optional
	ClusterDNS string `json:"clusterDNS,omitempty"`

	// ClusterDomain is the cluster domain name (default: "cluster.local").
	//+optional
	ClusterDomain string `json:"clusterDomain,omitempty"`

	// DisableComponents lists Kubernetes components and RKE2 plugin components that will be disabled.
	//+optional
	DisableComponents DisableComponents `json:"disableComponents,omitempty"`

	// CNI describes the CNI Plugins to deploy, one of none, calico, canal, cilium; optionally with multus as the first value to enable the multus meta-plugin (default: canal).
	// +kubebuilder:validation:Enum=none;calico;canal;cilium
	//+optional
	CNI CNI `json:"cni,omitempty"`

	// PauseImage Override image to use for pause.
	//+optional
	PauseImage string `json:"pauseImage,omitempty"`

	// Etcd defines optional custom configuration of ETCD.
	//+optional
	Etcd EtcdConfig `json:"etcd,omitempty"`

	// KubeAPIServer defines optional custom configuration of the Kube API Server.
	//+optional
	KubeAPIServer *bootstrapv1.ComponentConfig `json:"kubeAPIServer,omitempty"`

	// KubeControllerManager defines optional custom configuration of the Kube Controller Manager.
	//+optional
	KubeControllerManager *bootstrapv1.ComponentConfig `json:"kubeControllerManager,omitempty"`

	// KubeScheduler defines optional custom configuration of the Kube Scheduler.
	//+optional
	KubeScheduler *bootstrapv1.ComponentConfig `json:"kubeScheduler,omitempty"`

	// CloudControllerManager defines optional custom configuration of the Cloud Controller Manager.
	//+optional
	CloudControllerManager *bootstrapv1.ComponentConfig `json:"cloudControllerManager,omitempty"`

	// CloudProviderName cloud provider name.
	//+optional
	CloudProviderName string `json:"cloudProviderName,omitempty"`
	// CloudProviderConfigMap is a reference to a ConfigMap containing Cloud provider configuration.
	// The config map must contain a key named cloud-config.
	//+optional
	CloudProviderConfigMap *corev1.ObjectReference `json:"cloudProviderConfigMap,omitempty"`
}

// RKE2ControlPlaneStatus defines the observed state of RKE2ControlPlane
type RKE2ControlPlaneStatus struct {
	// Ready indicates the BootstrapData field is ready to be consumed.
	Ready bool `json:"ready,omitempty"`

	// Initialized indicates the target cluster has completed initialization
	Initialized bool `json:"initialized,omitempty"`

	// DataSecretName is the name of the secret that stores the bootstrap data script.
	// +optional
	DataSecretName *string `json:"dataSecretName,omitempty"`

	// FailureReason will be set on non-retryable errors.
	// +optional
	FailureReason string `json:"failureReason,omitempty"`

	// FailureMessage will be set on non-retryable errors.
	// +optional
	FailureMessage string `json:"failureMessage,omitempty"`

	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions defines current service state of the RKE2Config.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// Replicas is the number of replicas current attached to this ControlPlane Resource
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of replicas current attached to this ControlPlane Resource and that have Ready Status
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// UpdatedReplicas is the number of replicas current attached to this ControlPlane Resource and that are up-to-date with Control Plane config
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// UnavailableReplicas is the number of replicas current attached to this ControlPlane Resource and that are up-to-date with Control Plane config
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RKE2ControlPlane is the Schema for the rke2controlplanes API
type RKE2ControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RKE2ControlPlaneSpec   `json:"spec,omitempty"`
	Status RKE2ControlPlaneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RKE2ControlPlaneList contains a list of RKE2ControlPlane
type RKE2ControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RKE2ControlPlane `json:"items"`
}

// EtcdConfig regroups the ETCD-specific configuration of the control plane
type EtcdConfig struct {
	// ExposeEtcdMetrics defines the policy for ETCD Metrics exposure.
	// if value is true, ETCD metrics will be exposed
	// if value is false, ETCD metrics will NOT be exposed
	// +optional
	ExposeMetrics bool `json:"exposeMetrics,omitempty"`

	// BackupConfig defines how RKE2 will snapshot ETCD: target storage, schedule, etc.
	//+optional
	BackupConfig EtcdBackupConfig `json:"backupConfig,omitempty"`

	// CustomConfig defines the custom settings for ETCD.
	CustomConfig *bootstrapv1.ComponentConfig `json:"customConfig,omitempty"`
}

// EtcdBackupConfig describes the backup configuration for ETCD.
type EtcdBackupConfig struct {
	// DisableAutomaticSnapshots defines the policy for ETCD snapshots. true means automatic snapshots will be scheduled, false means automatic snapshots will not be scheduled.
	//+optional
	DisableAutomaticSnapshots *bool `json:"disableAutomaticSnapshots,omitempty"`

	// SnapshotName Set the base name of etcd snapshots. Default: etcd-snapshot-<unix-timestamp> (default: "etcd-snapshot").
	//+optional
	SnapshotName string `json:"snapshotName,omitempty"`

	// ScheduleCron Snapshot interval time in cron spec. eg. every 5 hours '* */5 * * *' (default: "0 */12 * * *").
	//+optional
	ScheduleCron string `json:"scheduleCron,omitempty"`

	// Retention Number of snapshots to retain Default: 5 (default: 5).
	//+optional
	Retention string `json:"retention,omitempty"`

	// Directory Directory to save db snapshots.
	//+optional
	Directory string `json:"directory,omitempty"`

	// S3 Enable backup to an S3-compatible Object Store.
	//+optional
	S3 *EtcdS3 `json:"s3,omitempty"`
}

type EtcdS3 struct {
	// Endpoint S3 endpoint url (default: "s3.amazonaws.com").
	Endpoint string `json:"endpoint"`

	// EndpointCA references the Secret that contains a custom CA that should be trusted to connect to S3 endpoint.
	// The secret must contain a key named "ca.pem" that contains the CA certificate.
	//+optional
	EndpointCASecret *corev1.ObjectReference `json:"endpointCAsecret,omitempty"`

	// EnforceSSLVerify may be set to false to skip verifying the registry's certificate, default is true.
	//+optional
	EnforceSSLVerify bool `json:"enforceSslVerify,omitempty"`

	// S3CredentialSecret is a reference to a Secret containing the Access Key and Secret Key necessary to access the target S3 Bucket.
	// The Secret must contain the following keys: "aws_access_key_id" and "aws_secret_access_key".
	S3CredentialSecret corev1.ObjectReference `json:"s3CredentialSecret"`

	// Bucket S3 bucket name.
	//+optional
	Bucket string `json:"bucket,omitempty"`

	// Region S3 region / bucket location (optional) (default: "us-east-1").
	//+optional
	Region string `json:"region,omitempty"`

	// Folder S3 folder.
	//+optional
	Folder string `json:"folder,omitempty"`
}

// CNI defines the Cni options for deploying RKE2.
type CNI string

const (
	// Cilium references the RKE2 CNI Plugin "cilium"
	Cilium CNI = "cilium"
	// Calico references the RKE2 CNI Plugin "calico"
	Calico CNI = "calico"
	// Canal references the RKE2 CNI Plugin "canal"
	Canal CNI = "canal"
	// None means that no CNI Plugin will be installed with RKE2, letting the operator install his own CNI afterwards.
	None CNI = "none"
)

// DisableComponents describes components of RKE2 (Kubernetes components and plugin components) that should be disabled
type DisableComponents struct {
	// KubernetesComponents is a list of Kubernetes components to disable.
	// +kubebuilder:validation:Enum=scheduler;kubeProxy;cloudController
	KubernetesComponents []DisabledKubernetesComponent `json:"kubernetesComponents,omitempty"`

	// PluginComponents is a list of PluginComponents to disable.
	// +kubebuilder:validation:Enum=rke2-coredns;rke2-ingress-nginx;rke2-metrics-server
	PluginComponents []DisabledPluginComponent `json:"pluginComponents,omitempty"`
}

// DisabledKubernetesComponent is an enum field that can take one of the following values: scheduler, kubeProxy or cloudController.
type DisabledKubernetesComponent string

const (
	// Scheduler references the Kube Scheduler Kubernetes components of the control plane/server nodes
	Scheduler DisabledKubernetesComponent = "scheduler"

	// KubeProxy references the Kube Proxy Kubernetes components on the agents
	KubeProxy DisabledKubernetesComponent = "kubeProxy"

	// CloudController references the Cloud Controller Manager Kubernetes Components on the control plane / server nodes
	CloudController DisabledKubernetesComponent = "cloudController"
)

// DisabledItem selects a plugin Components to be disabled.
type DisabledPluginComponent string

const (
	// CoreDNS references the RKE2 Plugin "rke2-coredns"
	CoreDNS DisabledPluginComponent = "rke2-coredns"
	// IngressNginx references the RKE2 Plugin "rke2-ingress-nginx"
	IngressNginx DisabledPluginComponent = "rke2-ingress-nginx"
	// MetricsServer references the RKE2 Plugin "rke2-metrics-server"
	MetricsServer DisabledPluginComponent = "rke2-metrics-server"
)

func init() {
	SchemeBuilder.Register(&RKE2ControlPlane{}, &RKE2ControlPlaneList{})
}

func (c *RKE2ControlPlane) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

func (c *RKE2ControlPlane) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}
