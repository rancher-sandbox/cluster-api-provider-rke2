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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// RKE2ConfigSpec defines the desired state of RKE2Config.
type RKE2ConfigSpec struct {
	// Files specifies extra files to be passed to user_data upon creation.
	//+optional
	Files []File `json:"files,omitempty"`

	// PreRKE2Commands specifies extra commands to run before rke2 setup runs.
	//+optional
	PreRKE2Commands []string `json:"preRKE2Commands,omitempty"`

	// PostRKE2Commands specifies extra commands to run after rke2 setup runs.
	//+optional
	PostRKE2Commands []string `json:"postRKE2Commands,omitempty"`

	// AgentConfig specifies configuration for the agent nodes.
	//+optional
	AgentConfig RKE2AgentConfig `json:"agentConfig,omitempty"`

	// PrivateRegistriesConfig defines the containerd configuration for private registries and local registry mirrors.
	//+optional
	PrivateRegistriesConfig Registry `json:"privateRegistriesConfig,omitempty"`
}

// RKE2CommonNodeConfig describes some attributes that are common to agent and server nodes
type RKE2AgentConfig struct {
	// DataDir Folder to hold state.
	//+optional
	DataDir string `json:"dataDir,omitempty"`

	// NodeLabels  Registering and starting kubelet with set of labels.
	//+optional
	NodeLabels []string `json:"nodeLabels,omitempty"`

	// NodeTaints Registering kubelet with set of taints.
	//+optional
	NodeTaints []string `json:"nodeTaints,omitempty"`

	// NodeNamePrefix Prefix to the Node Name that CAPI will generate.
	//+optional
	NodeNamePrefix string `json:"nodeName,omitempty"`

	// NTP specifies NTP configuration
	// +optional
	NTP *NTP `json:"ntp,omitempty"`

	// ImageCredentialProviderConfigMap is a reference to the ConfigMap that contains credential provider plugin config
	// The config map should contain a key "credential-config.yaml" with YAML file content and
	// a key "credential-provider-binaries" with the a path to the binaries for the credential provider.
	//+optional
	ImageCredentialProviderConfigMap *corev1.ObjectReference `json:"imageCredentialProviderConfigMap,omitempty"`

	// TODO: Remove ContainerRuntimeEndpoint since this feature will probably not be offered by CAPI Bootstrap provider?

	// ContainerRuntimeEndpoint Disable embedded containerd and use alternative CRI implementation.
	//+optional
	ContainerRuntimeEndpoint string `json:"containerRuntimeEndpoint,omitempty"`

	// Snapshotter override default containerd snapshotter (default: "overlayfs").
	//+optional
	Snapshotter string `json:"snapshotter,omitempty"`

	// TODO: Find a way to handle IP addresses that should be advertised but that RKE2 cannot find on the host (Example: Elastic IPs on Cloud Providers).

	// NodeIp IPv4/IPv6 addresses to advertise for node.
	//+optional.
	//NodeIp string `json:"nodeIp,omitempty"`

	// NodeExternalIp IPv4/IPv6 external IP addresses to advertise for node.
	//+optional
	// NodeExternalIp string `json:"nodeExternalIp,omitempty"`

	// CISProfile activates CIS compliance of RKE2 for a certain profile
	// +kubebuilder:validation:Enum=cis-1.23
	//+optional
	CISProfile CISProfile `json:"cisProfile,omitempty"`

	// ResolvConf is a reference to a ConfigMap containing resolv.conf content for the node.
	//+optional
	ResolvConf *corev1.ObjectReference `json:"resolvConf,omitempty"`

	// ProtectKernelDefaults defines Kernel tuning behavior. If true, error if kernel tunables are different than kubelet defaults.
	// if false, kernel tunable can be different from kubelet defaults
	//+optional
	ProtectKernelDefaults bool `json:"protectKernelDefaults,omitempty"`

	// SystemDefaultRegistry Private registry to be used for all system images.
	//+optional
	SystemDefaultRegistry string `json:"systemDefaultRegistry,omitempty"`

	// EnableContainerdSElinux defines the policy for enabling SELinux for Containerd
	// if value is true, Containerd will run with selinux-enabled=true flag
	// if value is false, Containerd will run without the above flag
	//+optional
	EnableContainerdSElinux bool `json:"enableContainerdSElinux,omitempty"`

	// KubeletPath Override kubelet binary path.
	//+optional
	KubeletPath string `json:"kubeletPath,omitempty"`

	// KubeletArgs Customized flag for kubelet process.
	//+optional
	Kubelet *ComponentConfig `json:"kubelet,omitempty"`

	// KubeProxyArgs Customized flag for kube-proxy process.
	//+optional
	KubeProxy *ComponentConfig `json:"kubeProxy,omitempty"`

	// RuntimeImage override image to use for runtime binaries (containerd, kubectl, crictl, etc).
	//+optional
	RuntimeImage string `json:"runtimeImage,omitempty"`

	// LoadBalancerPort local port for supervisor client load-balancer. If the supervisor and apiserver are not colocated an additional port 1 less than this port will also be used for the apiserver client load-balancer (default: 6444).
	//+optional
	LoadBalancerPort int `json:"loadBalancerPort,omitempty"`

	// Version specifies the rke2 version.
	//+optional
	Version string `json:"version,omitempty"`

	// AirGapped is a boolean value to define if the bootstrapping should be air-gapped,
	// basically supposing that online container registries and RKE2 install scripts are not reachable.
	// NOTE: Make sure when using this option that you are using a custom machine image in the InfraMachineTemplates, this image needs to contain the necessary RKE2 files
	// which are:
	// - install.sh
	// - rke2-images.linux-amd64.tar.zst
	// - rke2.linux-amd64.tar.gz
	// - sha256sum-amd64.txt
	// from the RKE2 release page, these files should be the same as the ones from RKE2AgentConfig
	AirGapped bool `json:"airGapped,omitempty"`
}

// NTP defines input for generated ntp in cloud-init.
type NTP struct {
	// Servers specifies which NTP servers to use
	// +optional
	Servers []string `json:"servers,omitempty"`

	// Enabled specifies whether NTP should be enabled
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// RKE2ConfigStatus defines the observed state of RKE2Config.
type RKE2ConfigStatus struct {
	// Ready indicates the BootstrapData field is ready to be consumed.
	Ready bool `json:"ready,omitempty"`

	// DataSecretName is the name of the secret that stores the bootstrap data script.
	//+optional
	DataSecretName *string `json:"dataSecretName,omitempty"`

	// FailureReason will be set on non-retryable errors.
	//+optional
	FailureReason string `json:"failureReason,omitempty"`

	// FailureMessage will be set on non-retryable errors.
	//+optional
	FailureMessage string `json:"failureMessage,omitempty"`

	// ObservedGeneration is the latest generation observed by the controller.
	//+optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions defines current service state of the RKE2Config.
	//+optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RKE2Config is the Schema for the rke2configs API.
type RKE2Config struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RKE2ConfigSpec   `json:"spec,omitempty"`
	Status RKE2ConfigStatus `json:"status,omitempty"`
}

func (c *RKE2Config) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

func (c *RKE2Config) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// RKE2ConfigList contains a list of RKE2Config.
type RKE2ConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RKE2Config `json:"items"`
}

// CISProfile defines the CIS Benchmark profile to be activated in RKE2.
type CISProfile string

const (
	// CIS1_23 references RKE2's CIS Profile "cis-1.23"
	CIS1_23 CISProfile = "cis-1.23"
)

// Encoding specifies the cloud-init file encoding.
type Encoding string

const (
	// Base64 implies the contents of the file are encoded as base64.
	Base64 Encoding = "base64"
	// Gzip implies the contents of the file are encoded with gzip.
	Gzip Encoding = "gzip"
	// GzipBase64 implies the contents of the file are first base64 encoded and then gzip encoded.
	GzipBase64 Encoding = "gzip+base64"
)

// File defines the input for generating write_files in cloud-init.
type File struct {
	// Path specifies the full path on disk where to store the file.
	Path string `json:"path"`

	// Owner specifies the ownership of the file, e.g. "root:root".
	//+optional
	Owner string `json:"owner,omitempty"`

	// Permissions specifies the permissions to assign to the file, e.g. "0640".
	//+optional
	Permissions string `json:"permissions,omitempty"`

	// Encoding specifies the encoding of the file contents.
	// +kubebuilder:validation:Enum=base64;gzip;gzip+base64
	//+optional
	Encoding Encoding `json:"encoding,omitempty"`

	// Content is the actual content of the file.
	//+optional
	Content string `json:"content,omitempty"`

	// ContentFrom is a referenced source of content to populate the file.
	//+optional
	ContentFrom *FileSource `json:"contentFrom,omitempty"`
}

// FileSource is a union of all possible external source types for file data.
// Only one field may be populated in any given instance. Developers adding new
// sources of data for target systems should add them here.
type FileSource struct {
	// Secret represents a secret that should populate this file.
	Secret SecretFileSource `json:"secret"`
}

// Adapts a Secret into a FileSource.
//
// The contents of the target Secret's Data field will be presented
// as files using the keys in the Data field as the file names.
type SecretFileSource struct {
	// Name of the secret in the RKE2BootstrapConfig's namespace to use.
	Name string `json:"name"`

	// Key is the key in the secret's data map for this value.
	Key string `json:"key"`
}

// Registry is registry settings including mirrors, TLS, and credentials.
type Registry struct {
	// Mirrors are namespace to mirror mapping for all namespaces.
	//+optional
	Mirrors map[string]Mirror `json:"mirrors,omitempty"`

	// Configs are configs for each registry.
	// The key is the FDQN or IP of the registry.
	//+optional
	Configs map[string]RegistryConfig `json:"configs,omitempty"`
}

// Mirror contains the config related to the registry mirror.
type Mirror struct {
	// Endpoints are endpoints for a namespace. CRI plugin will try the endpoints
	// one by one until a working one is found. The endpoint must be a valid url
	// with host specified.
	// The scheme, host and path from the endpoint URL will be used.
	//+optional
	Endpoints []string `json:"endpoint,omitempty"`

	// Rewrites are repository rewrite rules for a namespace. When fetching image resources
	// from an endpoint and a key matches the repository via regular expression matching
	// it will be replaced with the corresponding value from the map in the resource request.
	//+optional
	Rewrites map[string]string `json:"rewrite,omitempty"`
}

// RegistryConfig contains configuration used to communicate with the registry.
type RegistryConfig struct {
	// Auth si a reference to a Secret containing information to authenticate to the registry.
	// The Secret must provite a username and a password data entry.
	//+optional
	AuthSecret corev1.ObjectReference `json:"authSecret,omitempty"`
	// TLS is a pair of CA/Cert/Key which then are used when creating the transport
	// that communicates with the registry.
	//+optional
	TLS TLSConfig `json:"tls,omitempty"`
}

// TLSConfig contains the CA/Cert/Key used for a registry.
type TLSConfig struct {
	// TLSConfigSecret is a reference to a secret of type `kubernetes.io/tls` thich has up to 3 entries: tls.crt, tls.key and ca.crt
	// which describe the TLS configuration necessary to connect to the registry.
	// +optional
	TLSConfigSecret corev1.ObjectReference `json:"tlsConfigSecret,omitempty"`

	// EnforceSSLVerify may be set to false to skip verifying the registry's certificate, default is true.
	//+optional
	EnforceSSLVerify bool `json:"enforceSslVerify,omitempty"`
}

type ComponentConfig struct {
	// ExtraEnv is a map of environment variables to pass on to a Kubernetes Component command.
	//+optional
	ExtraEnv map[string]string `json:"extraEnv,omitempty"`

	// ExtraArgs is a map of command line arguments to pass to a Kubernetes Component command.
	//+optional
	ExtraArgs []string `json:"extraArgs,omitempty"`

	// ExtraMounts is a map of volume mounts to be added for the Kubernetes component StaticPod
	//+optional
	ExtraMounts map[string]string `json:"extraMounts,omitempty"`

	// OverrideImage is a string that references a container image to override the default one for the Kubernetes Component
	//+optional
	OverrideImage string `json:"overrideImage,omitempty"`
}

func init() {
	SchemeBuilder.Register(&RKE2Config{}, &RKE2ConfigList{})
}
