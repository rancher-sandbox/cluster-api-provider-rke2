package rke2

import (
	"fmt"
	"path"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// +kubebuilder:scaffold:imports
	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	"github.com/rancher/cluster-api-provider-rke2/test/helpers"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	testEnv *helpers.TestEnvironment
	ctx     = ctrl.SetupSignalHandler()
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	setup()
	defer teardown()
	RunSpecs(t, "RKE2 Suite")
}

func setup() {
	utilruntime.Must(clusterv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(bootstrapv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(controlplanev1.AddToScheme(scheme.Scheme))

	testEnvConfig := helpers.NewTestEnvironmentConfiguration([]string{
		path.Join("bootstrap", "config", "crd", "bases"),
		path.Join("controlplane", "config", "crd", "bases"),
	},
	)
	var err error
	testEnv, err = testEnvConfig.Build()
	if err != nil {
		panic(err)
	}
	go func() {
		fmt.Println("Starting the manager")
		if err := testEnv.StartManager(ctx); err != nil {
			panic(fmt.Sprintf("Failed to start the envtest manager: %v", err))
		}
	}()
}

func teardown() {
	if err := testEnv.Stop(); err != nil {
		panic(fmt.Sprintf("Failed to stop envtest: %v", err))
	}
}
