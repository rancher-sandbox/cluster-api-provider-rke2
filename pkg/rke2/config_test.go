package rke2

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha1"
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("RKE2ServerConfig", func() {
	var opts *RKE2ServerConfigOpts

	BeforeEach(func() {
		opts = &RKE2ServerConfigOpts{
			Cluster: v1beta1.Cluster{
				Spec: v1beta1.ClusterSpec{
					ClusterNetwork: &v1beta1.ClusterNetwork{
						Pods: &v1beta1.NetworkRanges{
							CIDRBlocks: []string{
								"192.168.0.0/16",
							},
						},
						Services: &v1beta1.NetworkRanges{
							CIDRBlocks: []string{
								"192.169.0.0/16",
							},
						},
					},
				},
			},
			ControlPlaneEndpoint: "testendpoint",
			Ctx:                  context.Background(),
			Client: fake.NewClientBuilder().WithObjects(
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string][]byte{
						"aws_access_key_id":     []byte("test_id"),
						"aws_secret_access_key": []byte("test_secret"),
						"ca.pem":                []byte("test_ca"),
						"audit-policy.yaml":     []byte("test_audit"),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string]string{
						"cloud-config":                 "test_cloud_config",
						"credential-config.yaml":       "test_credential_config",
						"credential-provider-binaries": "test_credential_provider_binaries",
						"resolv.conf":                  "test_resolv_conf",
					},
				},
			).Build(),
			ServerConfig: controlplanev1.RKE2ServerConfig{
				AdvertiseAddress: "testaddress",
				AuditPolicySecret: &corev1.ObjectReference{
					Name:      "test",
					Namespace: "test",
				},

				BindAddress:   "testbindaddress",
				CNI:           controlplanev1.Cilium,
				ClusterDNS:    "testdns",
				ClusterDomain: "testdomain",
				CloudProviderConfigMap: &corev1.ObjectReference{
					Name:      "test",
					Namespace: "test",
				},
				CloudProviderName: "testcloud",
				DisableComponents: controlplanev1.DisableComponents{
					PluginComponents: []controlplanev1.DisabledPluginComponent{
						controlplanev1.CoreDNS,
					},
					KubernetesComponents: []controlplanev1.DisabledKubernetesComponent{
						controlplanev1.KubeProxy,
						controlplanev1.Scheduler,
						controlplanev1.CloudController,
					},
				},
				Etcd: controlplanev1.EtcdConfig{
					ExposeMetrics: true,
					BackupConfig: controlplanev1.EtcdBackupConfig{
						S3: &controlplanev1.EtcdS3{
							S3CredentialSecret: corev1.ObjectReference{
								Name:      "test",
								Namespace: "test",
							},
							EndpointCASecret: &corev1.ObjectReference{
								Name:      "test",
								Namespace: "test",
							},
							Bucket:           "testbucket",
							Region:           "testregion",
							Endpoint:         "testendpoint",
							EnforceSSLVerify: true,
						},
						Directory:    "testdir",
						SnapshotName: "testsnapshot",
						Retention:    "testretention",
						ScheduleCron: "testschedule",
					},
					CustomConfig: &bootstrapv1.ComponentConfig{
						ExtraArgs:     []string{"testarg"},
						OverrideImage: "testimage",
						ExtraMounts:   map[string]string{"testmount": "testmount"},
						ExtraEnv:      map[string]string{"testenv": "testenv"},
					},
				},
				ServiceNodePortRange: "testrange",
				TLSSan:               []string{"testsan"},
				KubeAPIServer: &bootstrapv1.ComponentConfig{
					ExtraArgs:     []string{"testarg"},
					OverrideImage: "testimage",
					ExtraMounts:   map[string]string{"testmount": "testmount"},
					ExtraEnv:      map[string]string{"testenv": "testenv"},
				},
				KubeScheduler: &bootstrapv1.ComponentConfig{
					ExtraArgs:     []string{"testarg"},
					OverrideImage: "testimage",
					ExtraMounts:   map[string]string{"testmount": "testmount"},
					ExtraEnv:      map[string]string{"testenv": "testenv"},
				},
				KubeControllerManager: &bootstrapv1.ComponentConfig{
					ExtraArgs:     []string{"testarg"},
					OverrideImage: "testimage",
					ExtraEnv:      map[string]string{"testenv": "testenv"},
					ExtraMounts:   map[string]string{"testmount": "testmount"},
				},
				CloudControllerManager: &bootstrapv1.ComponentConfig{
					ExtraArgs:     []string{"testarg"},
					OverrideImage: "testimage",
					ExtraEnv:      map[string]string{"testenv": "testenv"},
					ExtraMounts:   map[string]string{"testmount": "testmount"},
				},
			},
		}
	})

	It("should succefully generate a server config", func() {
		rke2ServerConfig, files, err := newRKE2ServerConfig(*opts)
		Expect(err).ToNot(HaveOccurred())

		serverConfig := opts.ServerConfig
		Expect(rke2ServerConfig.AdvertiseAddress).To(Equal(serverConfig.AdvertiseAddress))
		Expect(rke2ServerConfig.AuditPolicyFile).To(Equal("/etc/rancher/rke2/audit-policy.yaml"))
		Expect(rke2ServerConfig.BindAddress).To(Equal(serverConfig.BindAddress))
		Expect(rke2ServerConfig.CNI).To(Equal(string(serverConfig.CNI)))
		Expect(rke2ServerConfig.ClusterCIDR).To(Equal("192.168.0.0/16"))
		Expect(rke2ServerConfig.ServiceCIDR).To(Equal("192.169.0.0/16"))
		Expect(rke2ServerConfig.ClusterDNS).To(Equal(serverConfig.ClusterDNS))
		Expect(rke2ServerConfig.ClusterDomain).To(Equal(serverConfig.ClusterDomain))
		Expect(rke2ServerConfig.CloudProviderConfig).To(Equal("/etc/rancher/rke2/cloud-provider-config"))
		Expect(rke2ServerConfig.CloudProviderName).To(Equal(opts.ServerConfig.CloudProviderName))
		Expect(rke2ServerConfig.DisableComponents).To(Equal([]string{string(controlplanev1.CoreDNS)}))
		Expect(rke2ServerConfig.DisableKubeProxy).To(BeTrue())
		Expect(rke2ServerConfig.DisableCloudController).To(BeTrue())
		Expect(rke2ServerConfig.DisableScheduler).To(BeTrue())
		//Expect(rke2ServerConfig.EtcdDisableSnapshots).To(BeFalse())
		Expect(rke2ServerConfig.EtcdExposeMetrics).To(Equal(serverConfig.Etcd.ExposeMetrics))
		Expect(rke2ServerConfig.EtcdS3).To(BeTrue())
		Expect(rke2ServerConfig.EtcdS3AccessKey).To(Equal("test_id"))
		Expect(rke2ServerConfig.EtcdS3SecretKey).To(Equal("test_secret"))
		Expect(rke2ServerConfig.EtcdS3Bucket).To(Equal(serverConfig.Etcd.BackupConfig.S3.Bucket))
		Expect(rke2ServerConfig.EtcdS3Region).To(Equal(serverConfig.Etcd.BackupConfig.S3.Region))
		Expect(rke2ServerConfig.EtcdS3Folder).To(Equal(serverConfig.Etcd.BackupConfig.S3.Folder))
		Expect(rke2ServerConfig.EtcdS3Endpoint).To(Equal(serverConfig.Etcd.BackupConfig.S3.Endpoint))
		Expect(rke2ServerConfig.EtcdS3EndpointCA).To(Equal("/etc/rancher/rke2/etcd-s3-ca.crt"))
		Expect(rke2ServerConfig.EtcdSnapshotDir).To(Equal(serverConfig.Etcd.BackupConfig.Directory))
		Expect(rke2ServerConfig.EtcdSnapshotName).To(Equal(serverConfig.Etcd.BackupConfig.SnapshotName))
		Expect(rke2ServerConfig.EtcdSnapshotRetention).To(Equal(serverConfig.Etcd.BackupConfig.Retention))
		Expect(rke2ServerConfig.EtcdSnapshotScheduleCron).To(Equal(serverConfig.Etcd.BackupConfig.ScheduleCron))
		Expect(rke2ServerConfig.EtcdS3SkipSslVerify).To(BeFalse())
		Expect(rke2ServerConfig.EtcdArgs).To(Equal(serverConfig.Etcd.CustomConfig.ExtraArgs))
		Expect(rke2ServerConfig.EtcdImage).To(Equal(serverConfig.Etcd.CustomConfig.OverrideImage))
		Expect(rke2ServerConfig.EtcdExtraMounts).To(Equal(serverConfig.Etcd.CustomConfig.ExtraMounts))
		Expect(rke2ServerConfig.EtcdExtraEnv).To(Equal(serverConfig.Etcd.CustomConfig.ExtraEnv))
		Expect(rke2ServerConfig.ServiceNodePortRange).To(Equal(rke2ServerConfig.ServiceNodePortRange))
		Expect(rke2ServerConfig.TLSSan).To(Equal(append(serverConfig.TLSSan, opts.ControlPlaneEndpoint)))
		Expect(rke2ServerConfig.KubeAPIServerArgs).To(Equal(serverConfig.KubeAPIServer.ExtraArgs))
		Expect(rke2ServerConfig.KubeAPIserverImage).To(Equal(serverConfig.KubeAPIServer.OverrideImage))
		Expect(rke2ServerConfig.KubeAPIserverExtraMounts).To(Equal(serverConfig.KubeAPIServer.ExtraMounts))
		Expect(rke2ServerConfig.KubeAPIserverExtraEnv).To(Equal(serverConfig.KubeAPIServer.ExtraEnv))
		Expect(rke2ServerConfig.KubeSchedulerArgs).To(Equal(serverConfig.KubeScheduler.ExtraArgs))
		Expect(rke2ServerConfig.KubeSchedulerImage).To(Equal(serverConfig.KubeScheduler.OverrideImage))
		Expect(rke2ServerConfig.KubeSchedulerExtraMounts).To(Equal(serverConfig.KubeScheduler.ExtraMounts))
		Expect(rke2ServerConfig.KubeSchedulerExtraEnv).To(Equal(serverConfig.KubeScheduler.ExtraEnv))
		Expect(rke2ServerConfig.KubeControllerManagerArgs).To(Equal(serverConfig.KubeControllerManager.ExtraArgs))
		Expect(rke2ServerConfig.KubeControllerManagerImage).To(Equal(serverConfig.KubeControllerManager.OverrideImage))
		Expect(rke2ServerConfig.KubeControllerManagerExtraMounts).To(Equal(serverConfig.KubeControllerManager.ExtraMounts))
		Expect(rke2ServerConfig.KubeControllerManagerExtraEnv).To(Equal(serverConfig.KubeControllerManager.ExtraEnv))
		Expect(rke2ServerConfig.CloudControllerManagerExtraMounts).To(Equal(serverConfig.CloudControllerManager.ExtraMounts))
		Expect(rke2ServerConfig.CloudControllerManagerExtraEnv).To(Equal(serverConfig.CloudControllerManager.ExtraEnv))

		Expect(files).To(HaveLen(3))

		Expect(files[0].Path).To(Equal(rke2ServerConfig.AuditPolicyFile))
		Expect(files[0].Content).To(Equal("test_audit"))
		Expect(files[0].Owner).To(Equal("root:root"))
		Expect(files[0].Permissions).To(Equal("0644"))

		Expect(files[1].Path).To(Equal(rke2ServerConfig.CloudProviderConfig))
		Expect(files[1].Content).To(Equal("test_cloud_config"))
		Expect(files[1].Owner).To(Equal("root:root"))
		Expect(files[1].Permissions).To(Equal("0644"))

		Expect(files[2].Path).To(Equal(rke2ServerConfig.EtcdS3EndpointCA))
		Expect(files[2].Content).To(Equal("test_ca"))
		Expect(files[2].Owner).To(Equal("root:root"))
		Expect(files[2].Permissions).To(Equal("0640"))
	})
})

var _ = Describe("RKE2 Agent Config", func() {
	var opts *RKE2AgentConfigOpts

	BeforeEach(func() {
		opts = &RKE2AgentConfigOpts{
			ServerURL: "testurl",
			Ctx:       context.Background(),
			Token:     "testtoken",
			Client: fake.NewClientBuilder().WithObjects(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
					Data: map[string]string{
						"credential-config.yaml":       "test_credential_config",
						"credential-provider-binaries": "test_credential_provider_binaries",
						"resolv.conf":                  "test_resolv_conf",
					},
				},
			).Build(),
			AgentConfig: bootstrapv1.RKE2AgentConfig{
				ContainerRuntimeEndpoint: "testendpoint",
				DataDir:                  "testdir",
				ImageCredentialProviderConfigMap: &corev1.ObjectReference{
					Name:      "test",
					Namespace: "test",
				},
				KubeletPath: "testpath",
				Kubelet: &bootstrapv1.ComponentConfig{
					ExtraArgs: []string{"testarg"},
				},
				LoadBalancerPort:      1234,
				NodeLabels:            []string{"testlabel"},
				NodeTaints:            []string{"testtaint"},
				CISProfile:            bootstrapv1.CIS1_23,
				ProtectKernelDefaults: true,
				ResolvConf: &corev1.ObjectReference{
					Name:      "test",
					Namespace: "test",
				},
				RuntimeImage:            "testimage",
				EnableContainerdSElinux: true,
				Snapshotter:             "testsnapshotter",
				KubeProxy: &bootstrapv1.ComponentConfig{
					ExtraArgs:     []string{"testarg"},
					OverrideImage: "testimage",
					ExtraEnv:      map[string]string{"testenv": "testenv"},
					ExtraMounts:   map[string]string{"testmount": "testmount"},
				},
			},
		}
	})

	It("should succefully generate an agent config", func() {
		agentConfig, files, err := newRKE2AgentConfig(*opts)
		Expect(err).ToNot(HaveOccurred())

		Expect(agentConfig.ContainerRuntimeEndpoint).To(Equal(opts.AgentConfig.ContainerRuntimeEndpoint))
		Expect(agentConfig.DataDir).To(Equal(opts.AgentConfig.DataDir))
		Expect(agentConfig.ImageCredentialProviderConfig).To(Equal("/etc/rancher/rke2/credential-config.yaml"))
		Expect(agentConfig.ImageCredentialProviderBinDir).To(Equal("test_credential_provider_binaries"))
		Expect(agentConfig.KubeletPath).To(Equal(opts.AgentConfig.KubeletPath))
		Expect(agentConfig.KubeletArgs).To(Equal(opts.AgentConfig.Kubelet.ExtraArgs))
		Expect(agentConfig.LbServerPort).To(Equal(opts.AgentConfig.LoadBalancerPort))
		Expect(agentConfig.NodeLabels).To(Equal(opts.AgentConfig.NodeLabels))
		Expect(agentConfig.NodeTaints).To(Equal(opts.AgentConfig.NodeTaints))
		Expect(agentConfig.Profile).To(Equal(string(opts.AgentConfig.CISProfile)))
		Expect(agentConfig.ProtectKernelDefaults).To(Equal(opts.AgentConfig.ProtectKernelDefaults))
		Expect(agentConfig.ResolvConf).To(Equal("/etc/rancher/rke2/resolv.conf"))
		Expect(agentConfig.RuntimeImage).To(Equal(opts.AgentConfig.RuntimeImage))
		Expect(agentConfig.Selinux).To(Equal(opts.AgentConfig.EnableContainerdSElinux))
		Expect(agentConfig.Server).To(Equal(opts.ServerURL))
		Expect(agentConfig.Snapshotter).To(Equal(opts.AgentConfig.Snapshotter))
		Expect(agentConfig.KubeProxyArgs).To(Equal(opts.AgentConfig.KubeProxy.ExtraArgs))
		Expect(agentConfig.KubeProxyImage).To(Equal(opts.AgentConfig.KubeProxy.OverrideImage))
		Expect(agentConfig.KubeProxyExtraMounts).To(Equal(opts.AgentConfig.KubeProxy.ExtraMounts))
		Expect(agentConfig.KubeProxyExtraEnv).To(Equal(opts.AgentConfig.KubeProxy.ExtraEnv))
		Expect(agentConfig.Token).To(Equal(opts.Token))

		Expect(files).To(HaveLen(2))

		Expect(files[0].Path).To(Equal(agentConfig.ImageCredentialProviderConfig))
		Expect(files[0].Content).To(Equal("test_credential_config"))
		Expect(files[0].Owner).To(Equal("root:root"))
		Expect(files[0].Permissions).To(Equal("0644"))

		Expect(files[1].Path).To(Equal(agentConfig.ResolvConf))
		Expect(files[1].Content).To(Equal("test_resolv_conf"))
		Expect(files[1].Owner).To(Equal("root:root"))
		Expect(files[1].Permissions).To(Equal("0644"))
	})
})
