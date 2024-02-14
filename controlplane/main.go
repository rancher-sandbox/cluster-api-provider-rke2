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

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"

	bootstrapv1alpha1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1alpha1"
	bootstrapv1 "github.com/rancher-sandbox/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1alpha1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha1"
	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/internal/controllers"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/consts"
)

var (
	scheme         = runtime.NewScheme()
	setupLog       = ctrl.Log.WithName("setup")
	controllerName = "cluster-api-rke2-control-plane-manager"

	// flags.
	metricsBindAddr             string
	enableLeaderElection        bool
	leaderElectionLeaseDuration time.Duration
	leaderElectionRenewDeadline time.Duration
	leaderElectionRetryPeriod   time.Duration
	watchFilterValue            string
	profilerAddress             string
	concurrencyNumber           int
	syncPeriod                  time.Duration
	webhookPort                 int
	webhookCertDir              string
	healthAddr                  string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(controlplanev1.AddToScheme(scheme))
	utilruntime.Must(controlplanev1alpha1.AddToScheme(scheme))
	utilruntime.Must(bootstrapv1.AddToScheme(scheme))
	utilruntime.Must(bootstrapv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
} //nolint:wsl

// InitFlags initializes the flags.
func InitFlags(fs *pflag.FlagSet) {
	fs.StringVar(&metricsBindAddr, "metrics-bind-addr", ":8080",
		"The address the metric endpoint binds to.")

	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")

	fs.DurationVar(&leaderElectionLeaseDuration, "leader-elect-lease-duration", consts.DefaultLeaderElectLeaseDuration,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)")

	fs.DurationVar(&leaderElectionRenewDeadline, "leader-elect-renew-deadline", consts.DefaultLeaderElectRenewDeadline,
		"Duration that the leading controller manager will retry refreshing leadership before giving up (duration string)")

	fs.DurationVar(&leaderElectionRetryPeriod, "leader-elect-retry-period", consts.DefaultLeaderElectRetryPeriod,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)")

	fs.StringVar(&watchFilterValue, "watch-filter", "",
		fmt.Sprintf("Label value that the controller watches to reconcile cluster-api objects. Label key is always %s. If unspecified, the controller watches for all cluster-api objects.", clusterv1.WatchLabel)) //nolint:lll

	fs.StringVar(&profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")

	fs.IntVar(&concurrencyNumber, "concurrency", 1,
		"Number of core resources to process simultaneously")

	fs.DurationVar(&syncPeriod, "sync-period", consts.DefaultSyncPeriod,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")

	fs.IntVar(&webhookPort, "webhook-port", consts.DefaultWebhookPort, "Webhook Server port")

	fs.StringVar(&webhookCertDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs/",
		"Webhook cert dir, only used when webhook-port is specified.")

	fs.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")
}

func main() {
	klog.InitFlags(nil)
	InitFlags(pflag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(klogr.New())

	if profilerAddress != "" {
		klog.Infof("Profiler listening for requests at %s", profilerAddress)

		go func() {
			klog.Info(http.ListenAndServe(profilerAddress, nil))
		}()
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsBindAddr,
		},
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "rke2-controlplane-manager-leader-election-capi",
		LeaseDuration:    &leaderElectionLeaseDuration,
		RenewDeadline:    &leaderElectionRenewDeadline,
		RetryPeriod:      &leaderElectionRetryPeriod,
		Cache: cache.Options{
			SyncPeriod: &syncPeriod,
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
			},
		},
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port:    webhookPort,
				CertDir: webhookCertDir,
			},
		),
		HealthProbeBindAddress: healthAddr,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	setupChecks(mgr)
	setupReconcilers(ctx, mgr)
	setupWebhooks(mgr)
	//+kubebuilder:scaffold:builder

	setupLog.Info("starting manager")

	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder
	setupLog.Info("starting manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupChecks(mgr ctrl.Manager) {
	if err := mgr.AddReadyzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "unable to create health check")
		os.Exit(1)
	}
}

func setupReconcilers(ctx context.Context, mgr ctrl.Manager) {
	secretCachingClient, err := client.New(mgr.GetConfig(), client.Options{
		HTTPClient: mgr.GetHTTPClient(),
		Cache: &client.CacheOptions{
			Reader: mgr.GetCache(),
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to create secret caching client")
		os.Exit(1)
	}

	// Set up a ClusterCacheTracker to provide to controllers
	// requiring a connection to a remote cluster
	tracker, err := remote.NewClusterCacheTracker(mgr, remote.ClusterCacheTrackerOptions{
		SecretCachingClient: secretCachingClient,
		ControllerName:      controllerName,
		Log:                 &ctrl.Log,
		ClientUncachedObjects: []client.Object{
			&corev1.Secret{},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to create cluster cache tracker")
		os.Exit(1)
	}
	if err := (&remote.ClusterCacheReconciler{
		Client:           mgr.GetClient(),
		Tracker:          tracker,
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, controller.Options{}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ClusterCacheReconciler")
		os.Exit(1)
	}

	if err := (&controllers.RKE2ControlPlaneReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		SecretCachingClient: secretCachingClient,
		Tracker:             tracker,
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RKE2ControlPlane")
		os.Exit(1)
	}
}

func setupWebhooks(mgr ctrl.Manager) {
	if err := (&controlplanev1.RKE2ControlPlane{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "RKE2ControlPlane")
		os.Exit(1)
	}

	if err := (&controlplanev1.RKE2ControlPlaneTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "RKE2ControlPlaneTemplate")
		os.Exit(1)
	}
}
