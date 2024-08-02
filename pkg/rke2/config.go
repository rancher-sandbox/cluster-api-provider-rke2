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
//nolint:tagliatelle
package rke2

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher/cluster-api-provider-rke2/pkg/consts"
	bsutil "github.com/rancher/cluster-api-provider-rke2/pkg/util"
)

const (
	// DefaultRKE2ConfigLocation is the default location for the RKE2 config file.
	DefaultRKE2ConfigLocation = "/etc/rancher/rke2/config.yaml"

	// DefaultRKE2CloudProviderConfigLocation is the default location for the RKE2 cloud provider config file.
	DefaultRKE2CloudProviderConfigLocation = "/etc/rancher/rke2/cloud-provider-config"

	// DefaultRKE2JoinPort is the default port used for joining nodes to the cluster. It is open on the control plane nodes.
	DefaultRKE2JoinPort = 9345

	// CISNodePreparationScript is the script that is used to prepare a node for CIS compliance.
	CISNodePreparationScript = `#!/bin/bash
set -e

# Adding etcd user if not present
if id etcd &>/dev/null; then
    echo 'user etcd already exists'
else
		useradd -r -c "etcd user" -s /sbin/nologin -M etcd -U
fi

YUM_BASED_PARAM_FILE_FOUND=false
TAR_BASED_PARAM_FILE_FOUND=false

# Using RKE2 generated kernel parameters
if [ -f /usr/share/rke2/rke2-cis-sysctl.conf ]; then
		YUM_BASED_PARAM_FILE_FOUND=true
		cp -f /usr/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/90-rke2-cis.conf
fi

if [ -f /usr/local/share/rke2/rke2-cis-sysctl.conf ]; then
		TAR_BASED_PARAM_FILE_FOUND=true
		cp -f /usr/local/share/rke2/rke2-cis-sysctl.conf /etc/sysctl.d/90-rke2-cis.conf
fi

if [ "$YUM_BASED_PARAM_FILE_FOUND" = false ] && [ "$TAR_BASED_PARAM_FILE_FOUND" = false ]; then
		echo "No kernel parameters file found"
		exit 1
fi

# Applying kernel parameters
sysctl -p /etc/sysctl.d/90-rke2-cis.conf
`
)

type rke2ServerConfig struct {
	AdvertiseAddress                  string            `json:"advertise-address,omitempty"`
	AuditPolicyFile                   string            `json:"audit-policy-file,omitempty"`
	BindAddress                       string            `json:"bind-address,omitempty"`
	CNI                               []string          `json:"cni,omitempty"`
	CloudControllerManagerExtraEnv    map[string]string `json:"cloud-controller-manager-extra-env,omitempty"`
	CloudControllerManagerExtraMounts map[string]string `json:"cloud-controller-manager-extra-mount,omitempty"`
	CloudProviderConfig               string            `json:"cloud-provider-config,omitempty"`
	CloudProviderName                 string            `json:"cloud-provider-name,omitempty"`
	ClusterDNS                        string            `json:"cluster-dns,omitempty"`
	ClusterDomain                     string            `json:"cluster-domain,omitempty"`
	DisableCloudController            bool              `json:"disable-cloud-controller,omitempty"`
	DisableComponents                 []string          `json:"disable,omitempty"`
	DisableKubeProxy                  bool              `json:"disable-kube-proxy,omitempty"`
	DisableScheduler                  bool              `json:"disable-scheduler,omitempty"`
	EtcdArgs                          []string          `json:"etcd-arg,omitempty"`
	EtcdExtraEnv                      map[string]string `json:"etcd-extra-env,omitempty"`
	EtcdExtraMounts                   map[string]string `json:"etcd-extra-mount,omitempty"`
	EtcdImage                         string            `json:"etcd-image,omitempty"`
	EtcdDisableSnapshots              *bool             `json:"etcd-disable-snapshots,omitempty"`
	EtcdExposeMetrics                 bool              `json:"etcd-expose-metrics,omitempty"`
	EtcdS3                            bool              `json:"etcd-s3,omitempty"`
	EtcdS3AccessKey                   string            `json:"etcd-s3-access-key,omitempty"`
	EtcdS3Bucket                      string            `json:"etcd-s3-bucket,omitempty"`
	EtcdS3Endpoint                    string            `json:"etcd-s3-endpoint,omitempty"`
	EtcdS3EndpointCA                  string            `json:"etcd-s3-endpoint-ca,omitempty"`
	EtcdS3Folder                      string            `json:"etcd-s3-folder,omitempty"`
	EtcdS3Region                      string            `json:"etcd-s3-region,omitempty"`
	EtcdS3SecretKey                   string            `json:"etcd-s3-secret-key,omitempty"`
	EtcdS3SkipSslVerify               bool              `json:"etcd-s3-skip-ssl-verify,omitempty"`
	EtcdSnapshotDir                   string            `json:"etcd-snapshot-dir,omitempty"`
	EtcdSnapshotName                  string            `json:"etcd-snapshot-name,omitempty"`
	EtcdSnapshotRetention             string            `json:"etcd-snapshot-retention,omitempty"`
	EtcdSnapshotScheduleCron          string            `json:"etcd-snapshot-schedule-cron,omitempty"`
	KubeAPIServerArgs                 []string          `json:"kube-apiserver-arg,omitempty"`
	KubeAPIserverExtraEnv             map[string]string `json:"kube-apiserver-extra-env,omitempty"`
	KubeAPIserverExtraMounts          map[string]string `json:"kube-apiserver-extra-mount,omitempty"`
	KubeAPIserverImage                string            `json:"kube-apiserver-image,omitempty"`
	KubeControllerManagerArgs         []string          `json:"kube-controller-manager-arg,omitempty"`
	KubeControllerManagerExtraEnv     map[string]string `json:"kube-controller-manager-extra-env,omitempty"`
	KubeControllerManagerExtraMounts  map[string]string `json:"kube-controller-manager-extra-mount,omitempty"`
	KubeControllerManagerImage        string            `json:"kube-controller-manager-image,omitempty"`
	KubeSchedulerArgs                 []string          `json:"kube-scheduler-arg,omitempty"`
	KubeSchedulerExtraEnv             map[string]string `json:"kube-scheduler-extra-env,omitempty"`
	KubeSchedulerExtraMounts          map[string]string `json:"kube-scheduler-extra-mount,omitempty"`
	KubeSchedulerImage                string            `json:"kube-scheduler-image,omitempty"`
	ServiceNodePortRange              string            `json:"service-node-port-range,omitempty"`
	TLSSan                            []string          `json:"tls-san,omitempty"`

	// We don't expose these fields in the API
	ClusterCIDR string `json:"cluster-cidr,omitempty"`
	ServiceCIDR string `json:"service-cidr,omitempty"`

	// Fields below are missing from our API and RKE2 docs
	AirgapExtraRegistry       string `json:"airgap-extra-registry,omitempty"`
	DisableAPIserver          bool   `json:"disable-apiserver,omitempty"`
	DisableControllerManager  bool   `json:"disable-controller-manager,omitempty"`
	EgressSelectorMode        string `json:"egress-selector-mode,omitempty"`
	EnablePprof               bool   `json:"enable-pprof,omitempty"`
	EnableServiceLoadBalancer bool   `json:"enable-servicelb,omitempty"`
	EtcdS3Insecure            bool   `json:"etcd-s3-insecure,omitempty"`
	EtcdS3Timeout             string `json:"etcd-s3-timeout,omitempty"`
	EtcdSnapshotCompress      string `json:"etcd-snapshot-compress,omitempty"`
	ServicelbNamespace        string `json:"servicelb-namespace,omitempty"`

	rke2AgentConfig `json:",inline"`
}

// ServerConfigOpts is a struct that contains the information needed to generate a RKE2 server config.
type ServerConfigOpts struct {
	Cluster              clusterv1.Cluster
	ControlPlaneEndpoint string
	Token                string
	ServerURL            string
	ServerConfig         controlplanev1.RKE2ServerConfig
	AgentConfig          bootstrapv1.RKE2AgentConfig
	Ctx                  context.Context
	Client               client.Client
	Version              string
}

func newRKE2ServerConfig(opts ServerConfigOpts) (*rke2ServerConfig, []bootstrapv1.File, error) { // nolint:gocyclo
	rke2ServerConfig := &rke2ServerConfig{}
	files := []bootstrapv1.File{}
	rke2ServerConfig.AdvertiseAddress = opts.ServerConfig.AdvertiseAddress

	if opts.ServerConfig.AuditPolicySecret != nil {
		auditPolicySecret := &corev1.Secret{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.ServerConfig.AuditPolicySecret.Name,
			Namespace: opts.ServerConfig.AuditPolicySecret.Namespace,
		}, auditPolicySecret); err != nil {
			return nil, nil, fmt.Errorf("failed to get audit policy secret: %w", err)
		}

		auditPolicy, ok := auditPolicySecret.Data["audit-policy.yaml"]
		if !ok {
			return nil, nil, fmt.Errorf("audit policy secret is missing audit-policy.yaml key")
		}

		rke2ServerConfig.AuditPolicyFile = "/etc/rancher/rke2/audit-policy.yaml"

		files = append(files, bootstrapv1.File{
			Path:        rke2ServerConfig.AuditPolicyFile,
			Content:     string(auditPolicy),
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.DefaultFileMode,
		})
	}

	if opts.Cluster.Spec.ClusterNetwork != nil &&
		opts.Cluster.Spec.ClusterNetwork.Pods != nil &&
		len(opts.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks) > 0 {
		rke2ServerConfig.ClusterCIDR = opts.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks[0]
	}

	if opts.Cluster.Spec.ClusterNetwork != nil &&
		opts.Cluster.Spec.ClusterNetwork.Services != nil &&
		len(opts.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks) > 0 {
		rke2ServerConfig.ServiceCIDR = opts.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0]
	}

	rke2ServerConfig.BindAddress = opts.ServerConfig.BindAddress
	if opts.ServerConfig.CNIMultusEnable {
		rke2ServerConfig.CNI = append([]string{"multus"}, string(opts.ServerConfig.CNI))
	} else if opts.ServerConfig.CNI != "" {
		rke2ServerConfig.CNI = []string{string(opts.ServerConfig.CNI)}
	}

	rke2ServerConfig.ClusterDNS = opts.ServerConfig.ClusterDNS
	rke2ServerConfig.ClusterDomain = opts.ServerConfig.ClusterDomain

	if opts.ServerConfig.CloudProviderConfigMap != nil {
		cloudProviderConfigMap := &corev1.ConfigMap{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.ServerConfig.CloudProviderConfigMap.Name,
			Namespace: opts.ServerConfig.CloudProviderConfigMap.Namespace,
		}, cloudProviderConfigMap); err != nil {
			return nil, nil, fmt.Errorf("failed to get cloud provider config map: %w", err)
		}

		cloudProviderConfig, ok := cloudProviderConfigMap.Data["cloud-config"]
		if !ok {
			return nil, nil, fmt.Errorf("cloud provider config map is missing cloud-config key")
		}

		rke2ServerConfig.CloudProviderConfig = "/etc/rancher/rke2/cloud-provider-config"

		files = append(files, bootstrapv1.File{
			Path:        rke2ServerConfig.CloudProviderConfig,
			Content:     cloudProviderConfig,
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.DefaultFileMode,
		})
	}

	rke2ServerConfig.CloudProviderName = opts.ServerConfig.CloudProviderName
	rke2ServerConfig.DisableComponents = func() []string {
		disabled := []string{}
		for _, plugin := range opts.ServerConfig.DisableComponents.PluginComponents {
			disabled = append(disabled, string(plugin))
		}

		return disabled
	}()

	for _, component := range opts.ServerConfig.DisableComponents.KubernetesComponents {
		switch component {
		case controlplanev1.KubeProxy:
			rke2ServerConfig.DisableKubeProxy = true
		case controlplanev1.Scheduler:
			rke2ServerConfig.DisableScheduler = true
		case controlplanev1.CloudController:
			rke2ServerConfig.DisableCloudController = true
		}
	}

	rke2ServerConfig.EtcdDisableSnapshots = opts.ServerConfig.Etcd.BackupConfig.DisableAutomaticSnapshots
	rke2ServerConfig.EtcdExposeMetrics = opts.ServerConfig.Etcd.ExposeMetrics

	if opts.ServerConfig.Etcd.BackupConfig.S3 != nil {
		rke2ServerConfig.EtcdS3 = true
		awsCredentialsSecret := &corev1.Secret{}

		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.ServerConfig.Etcd.BackupConfig.S3.S3CredentialSecret.Name,
			Namespace: opts.ServerConfig.Etcd.BackupConfig.S3.S3CredentialSecret.Namespace,
		}, awsCredentialsSecret); err != nil {
			return nil, nil, fmt.Errorf("failed to get aws credentials secret: %w", err)
		}

		accessKeyID, ok := awsCredentialsSecret.Data["aws_access_key_id"]

		if !ok {
			return nil, nil, fmt.Errorf("aws credentials secret is missing aws_access_key_id")
		}

		secretAccessKey, ok := awsCredentialsSecret.Data["aws_secret_access_key"]

		if !ok {
			return nil, nil, fmt.Errorf("aws credentials secret is missing aws_secret_access_key")
		}

		rke2ServerConfig.EtcdS3AccessKey = string(accessKeyID)
		rke2ServerConfig.EtcdS3SecretKey = string(secretAccessKey)
		rke2ServerConfig.EtcdS3Bucket = opts.ServerConfig.Etcd.BackupConfig.S3.Bucket
		rke2ServerConfig.EtcdS3Region = opts.ServerConfig.Etcd.BackupConfig.S3.Region
		rke2ServerConfig.EtcdS3Folder = opts.ServerConfig.Etcd.BackupConfig.S3.Folder
		rke2ServerConfig.EtcdS3Endpoint = opts.ServerConfig.Etcd.BackupConfig.S3.Endpoint

		if opts.ServerConfig.Etcd.BackupConfig.S3.EndpointCASecret != nil {
			endpointCAsecret := &corev1.Secret{}
			if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
				Name:      opts.ServerConfig.Etcd.BackupConfig.S3.EndpointCASecret.Name,
				Namespace: opts.ServerConfig.Etcd.BackupConfig.S3.EndpointCASecret.Namespace,
			}, endpointCAsecret); err != nil {
				return nil, nil, fmt.Errorf("failed to get aws credentials secret: %w", err)
			}

			caCert, ok := endpointCAsecret.Data["ca.pem"]
			if !ok {
				return nil, nil, fmt.Errorf("endpoint CA secret is missing ca.pem")
			}

			rke2ServerConfig.EtcdS3EndpointCA = "/etc/rancher/rke2/etcd-s3-ca.crt"

			files = append(files, bootstrapv1.File{
				Path:        rke2ServerConfig.EtcdS3EndpointCA,
				Content:     string(caCert),
				Owner:       consts.DefaultFileOwner,
				Permissions: "0640",
			})
		}

		rke2ServerConfig.EtcdSnapshotDir = opts.ServerConfig.Etcd.BackupConfig.S3.Folder
		rke2ServerConfig.EtcdS3SkipSslVerify = !opts.ServerConfig.Etcd.BackupConfig.S3.EnforceSSLVerify
	} else {
		rke2ServerConfig.EtcdSnapshotDir = opts.ServerConfig.Etcd.BackupConfig.Directory
	}

	rke2ServerConfig.EtcdSnapshotName = opts.ServerConfig.Etcd.BackupConfig.SnapshotName
	rke2ServerConfig.EtcdSnapshotRetention = opts.ServerConfig.Etcd.BackupConfig.Retention
	rke2ServerConfig.EtcdSnapshotScheduleCron = opts.ServerConfig.Etcd.BackupConfig.ScheduleCron

	if opts.ServerConfig.Etcd.CustomConfig != nil {
		rke2ServerConfig.EtcdArgs = opts.ServerConfig.Etcd.CustomConfig.ExtraArgs
		rke2ServerConfig.EtcdImage = opts.ServerConfig.Etcd.CustomConfig.OverrideImage
		rke2ServerConfig.EtcdExtraMounts = opts.ServerConfig.Etcd.CustomConfig.ExtraMounts
		rke2ServerConfig.EtcdExtraEnv = opts.ServerConfig.Etcd.CustomConfig.ExtraEnv
	}

	rke2ServerConfig.ServiceNodePortRange = opts.ServerConfig.ServiceNodePortRange
	rke2ServerConfig.TLSSan = append(opts.ServerConfig.TLSSan, opts.ControlPlaneEndpoint)

	if opts.ServerConfig.KubeAPIServer != nil {
		rke2ServerConfig.KubeAPIServerArgs = opts.ServerConfig.KubeAPIServer.ExtraArgs
		rke2ServerConfig.KubeAPIserverImage = opts.ServerConfig.KubeAPIServer.OverrideImage
		rke2ServerConfig.KubeAPIserverExtraMounts = opts.ServerConfig.KubeAPIServer.ExtraMounts
		rke2ServerConfig.KubeAPIserverExtraEnv = opts.ServerConfig.KubeAPIServer.ExtraEnv
	}

	if opts.ServerConfig.KubeScheduler != nil {
		rke2ServerConfig.KubeSchedulerArgs = opts.ServerConfig.KubeScheduler.ExtraArgs
		rke2ServerConfig.KubeSchedulerImage = opts.ServerConfig.KubeScheduler.OverrideImage
		rke2ServerConfig.KubeSchedulerExtraMounts = opts.ServerConfig.KubeScheduler.ExtraMounts
		rke2ServerConfig.KubeSchedulerExtraEnv = opts.ServerConfig.KubeScheduler.ExtraEnv
	}

	if opts.ServerConfig.KubeControllerManager != nil {
		rke2ServerConfig.KubeControllerManagerArgs = opts.ServerConfig.KubeControllerManager.ExtraArgs
		rke2ServerConfig.KubeControllerManagerImage = opts.ServerConfig.KubeControllerManager.OverrideImage
		rke2ServerConfig.KubeControllerManagerExtraMounts = opts.ServerConfig.KubeControllerManager.ExtraMounts
		rke2ServerConfig.KubeControllerManagerExtraEnv = opts.ServerConfig.KubeControllerManager.ExtraEnv
	}

	if opts.ServerConfig.CloudControllerManager != nil {
		rke2ServerConfig.CloudControllerManagerExtraMounts = opts.ServerConfig.CloudControllerManager.ExtraMounts
		rke2ServerConfig.CloudControllerManagerExtraEnv = opts.ServerConfig.CloudControllerManager.ExtraEnv
	}

	return rke2ServerConfig, files, nil
}

type rke2AgentConfig struct {
	ContainerRuntimeEndpoint      string            `json:"container-runtime-endpoint,omitempty"`
	CloudProviderConfig           string            `json:"cloud-provider-config,omitempty"`
	CloudProviderName             string            `json:"cloud-provider-name,omitempty"`
	DataDir                       string            `json:"data-dir,omitempty"`
	ImageCredentialProviderConfig string            `json:"image-credential-provider-config,omitempty"`
	ImageCredentialProviderBinDir string            `json:"image-credential-provider-bin-dir,omitempty"`
	KubeProxyArgs                 []string          `json:"kube-proxy-arg,omitempty"`
	KubeProxyExtraEnv             map[string]string `json:"kube-proxy-extra-env,omitempty"`
	KubeProxyExtraMounts          map[string]string `json:"kube-proxy-extra-mount,omitempty"`
	KubeProxyImage                string            `json:"kube-proxy-image,omitempty"`
	KubeletArgs                   []string          `json:"kubelet-arg,omitempty"`
	KubeletPath                   string            `json:"kubelet-path,omitempty"`
	LbServerPort                  int               `json:"lb-server-port,omitempty"`
	NodeLabels                    []string          `json:"node-label,omitempty"`
	NodeTaints                    []string          `json:"node-taint,omitempty"`
	Profile                       string            `json:"profile,omitempty"`
	ProtectKernelDefaults         bool              `json:"protect-kernel-defaults,omitempty"`
	ResolvConf                    string            `json:"resolv-conf,omitempty"`
	RuntimeImage                  string            `json:"runtime-image,omitempty"`
	Selinux                       bool              `json:"selinux,omitempty"`
	Server                        string            `json:"server,omitempty"`
	Snapshotter                   string            `json:"snapshotter,omitempty"`
	Token                         string            `json:"token,omitempty"`

	// We don't expose these in the API
	PauseImage                     string `json:"pause-image,omitempty"`
	PodSecurityAdmissionConfigFile string `json:"pod-security-admission-config-file,omitempty"` // new flag, not present in the RKE2 docs yet
	PrivateRegistry                string `json:"private-registry,omitempty"`

	NodeExternalIp string `json:"node-external-ip,omitempty"`
	NodeIp         string `json:"node-ip,omitempty"`
	NodeName       string `json:"node-name,omitempty"`
}

// AgentConfigOpts is a struct that holds the information needed to generate the rke2 server config.
type AgentConfigOpts struct {
	ServerURL              string
	Token                  string
	AgentConfig            bootstrapv1.RKE2AgentConfig
	Ctx                    context.Context
	Client                 client.Client
	CloudProviderName      string
	CloudProviderConfigMap *corev1.ObjectReference
	Version                string
}

func newRKE2AgentConfig(opts AgentConfigOpts) (*rke2AgentConfig, []bootstrapv1.File, error) {
	rke2AgentConfig := &rke2AgentConfig{}
	files := []bootstrapv1.File{}
	rke2AgentConfig.ContainerRuntimeEndpoint = opts.AgentConfig.ContainerRuntimeEndpoint

	if opts.AgentConfig.CISProfile != "" {
		if !bsutil.ProfileCompliant(opts.AgentConfig.CISProfile, opts.Version) {
			return nil, nil, fmt.Errorf("profile %q is not supported for version %q", opts.AgentConfig.CISProfile, opts.Version)
		}

		files = append(files, bootstrapv1.File{
			Path:        "/opt/rke2-cis-script.sh",
			Content:     CISNodePreparationScript,
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.FileModeRootExecutable,
		})
		rke2AgentConfig.Profile = string(opts.AgentConfig.CISProfile)
	}

	if opts.CloudProviderConfigMap != nil {
		cloudProviderConfigMap := &corev1.ConfigMap{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.CloudProviderConfigMap.Name,
			Namespace: opts.CloudProviderConfigMap.Namespace,
		}, cloudProviderConfigMap); err != nil {
			return nil, nil, fmt.Errorf("failed to get cloud provider config map: %w", err)
		}

		cloudProviderConfig, ok := cloudProviderConfigMap.Data["cloud-config"]

		if !ok {
			return nil, nil, fmt.Errorf("cloud provider config map is missing cloud-config key")
		}

		rke2AgentConfig.CloudProviderConfig = DefaultRKE2CloudProviderConfigLocation

		files = append(files, bootstrapv1.File{
			Path:        rke2AgentConfig.CloudProviderConfig,
			Content:     cloudProviderConfig,
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.DefaultFileMode,
		})
	}

	rke2AgentConfig.CloudProviderName = opts.CloudProviderName
	rke2AgentConfig.DataDir = opts.AgentConfig.DataDir

	if opts.AgentConfig.ImageCredentialProviderConfigMap != nil {
		imageCredentialProviderCM := &corev1.ConfigMap{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.AgentConfig.ImageCredentialProviderConfigMap.Name,
			Namespace: opts.AgentConfig.ImageCredentialProviderConfigMap.Namespace,
		}, imageCredentialProviderCM); err != nil {
			return nil, nil, fmt.Errorf("failed to get image credential provider config map: %w", err)
		}

		credentialConfig, ok := imageCredentialProviderCM.Data["credential-config.yaml"]

		if !ok {
			return nil, nil, fmt.Errorf("image credential provider config map is missing config.yaml")
		}

		credentialProviderBinaries, ok := imageCredentialProviderCM.Data["credential-provider-binaries"]
		if !ok {
			return nil, nil, fmt.Errorf("image credential provider config map is missing credential-provider-binaries")
		}

		rke2AgentConfig.ImageCredentialProviderBinDir = credentialProviderBinaries
		rke2AgentConfig.ImageCredentialProviderConfig = "/etc/rancher/rke2/credential-config.yaml"

		files = append(files, bootstrapv1.File{
			Path:        rke2AgentConfig.ImageCredentialProviderConfig,
			Content:     credentialConfig,
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.DefaultFileMode,
		})
	}

	rke2AgentConfig.KubeletPath = opts.AgentConfig.KubeletPath
	if opts.AgentConfig.Kubelet != nil {
		rke2AgentConfig.KubeletArgs = opts.AgentConfig.Kubelet.ExtraArgs
	}

	rke2AgentConfig.LbServerPort = opts.AgentConfig.LoadBalancerPort
	rke2AgentConfig.NodeLabels = opts.AgentConfig.NodeLabels
	rke2AgentConfig.NodeTaints = opts.AgentConfig.NodeTaints
	rke2AgentConfig.ProtectKernelDefaults = opts.AgentConfig.ProtectKernelDefaults

	if opts.AgentConfig.ResolvConf != nil {
		resolvConfCM := &corev1.ConfigMap{}
		if err := opts.Client.Get(opts.Ctx, types.NamespacedName{
			Name:      opts.AgentConfig.ResolvConf.Name,
			Namespace: opts.AgentConfig.ResolvConf.Namespace,
		}, resolvConfCM); err != nil {
			return nil, nil, fmt.Errorf("failed to get resolv.conf config map: %w", err)
		}

		resolvConf, ok := resolvConfCM.Data["resolv.conf"]
		if !ok {
			return nil, nil, fmt.Errorf("resolv conf config map is missing resolv.conf")
		}

		rke2AgentConfig.ResolvConf = "/etc/rancher/rke2/resolv.conf"

		files = append(files, bootstrapv1.File{
			Path:        rke2AgentConfig.ResolvConf,
			Content:     resolvConf,
			Owner:       consts.DefaultFileOwner,
			Permissions: consts.DefaultFileMode,
		})
	}

	rke2AgentConfig.RuntimeImage = opts.AgentConfig.RuntimeImage
	rke2AgentConfig.Selinux = opts.AgentConfig.EnableContainerdSElinux
	rke2AgentConfig.Server = opts.ServerURL
	rke2AgentConfig.Snapshotter = opts.AgentConfig.Snapshotter

	if opts.AgentConfig.KubeProxy != nil {
		rke2AgentConfig.KubeProxyArgs = opts.AgentConfig.KubeProxy.ExtraArgs
		rke2AgentConfig.KubeProxyImage = opts.AgentConfig.KubeProxy.OverrideImage
		rke2AgentConfig.KubeProxyExtraMounts = opts.AgentConfig.KubeProxy.ExtraMounts
		rke2AgentConfig.KubeProxyExtraEnv = opts.AgentConfig.KubeProxy.ExtraEnv
	}

	rke2AgentConfig.Token = opts.Token

	return rke2AgentConfig, files, nil
}

// GenerateInitControlPlaneConfig generates the rke2 server and agent config for the init control plane node.
func GenerateInitControlPlaneConfig(opts ServerConfigOpts) (*rke2ServerConfig, []bootstrapv1.File, error) {
	if opts.Token == "" {
		return nil, nil, fmt.Errorf("token is required")
	}

	rke2ServerConfig, serverFiles, err := newRKE2ServerConfig(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rke2 server config: %w", err)
	}

	rke2AgentConfig, agentFiles, err := newRKE2AgentConfig(AgentConfigOpts{
		AgentConfig: opts.AgentConfig,
		Client:      opts.Client,
		Ctx:         opts.Ctx,
		Token:       opts.Token,
		Version:     opts.Version,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rke2 agent config: %w", err)
	}

	rke2ServerConfig.rke2AgentConfig = *rke2AgentConfig

	return rke2ServerConfig, append(serverFiles, agentFiles...), nil
}

// GenerateJoinControlPlaneConfig generates the rke2 agent config for joining a control plane node.
func GenerateJoinControlPlaneConfig(opts ServerConfigOpts) (*rke2ServerConfig, []bootstrapv1.File, error) {
	if opts.ServerURL == "" {
		return nil, nil, fmt.Errorf("server url is required")
	}

	if opts.Token == "" {
		return nil, nil, fmt.Errorf("token is required")
	}

	rke2ServerConfig, serverFiles, err := newRKE2ServerConfig(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rke2 server config: %w", err)
	}

	rke2AgentConfig, agentFiles, err := newRKE2AgentConfig(AgentConfigOpts{
		AgentConfig: opts.AgentConfig,
		Client:      opts.Client,
		Ctx:         opts.Ctx,
		ServerURL:   opts.ServerURL,
		Token:       opts.Token,
		Version:     opts.Version,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rke2 agent config: %w", err)
	}

	rke2ServerConfig.rke2AgentConfig = *rke2AgentConfig

	return rke2ServerConfig, append(serverFiles, agentFiles...), nil
}

// GenerateWorkerConfig generates the rke2 agent config and files.
func GenerateWorkerConfig(opts AgentConfigOpts) (*rke2AgentConfig, []bootstrapv1.File, error) {
	if opts.ServerURL == "" {
		return nil, nil, fmt.Errorf("server url is required")
	}

	if opts.Token == "" {
		return nil, nil, fmt.Errorf("token is required")
	}

	rke2AgentConfig, agentFiles, err := newRKE2AgentConfig(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rke2 agent config: %w", err)
	}

	return rke2AgentConfig, agentFiles, nil
}
