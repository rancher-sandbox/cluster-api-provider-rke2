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

package util

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"

	controlplanev1 "github.com/rancher-sandbox/cluster-api-provider-rke2/controlplane/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func GetOwnerControlPlane(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*controlplanev1.RKE2ControlPlane, error) {
	logger := log.FromContext(ctx)

	ownerRefs := obj.OwnerReferences
	var cpOwnerRef metav1.OwnerReference
	for _, ownerRef := range ownerRefs {
		logger.V(5).Info("Inside GetOwnerControlPlane", "ownerRef.APIVersion", ownerRef.APIVersion, "ownerRef.Kind", ownerRef.Kind, "cpv1.GroupVersion.Group", controlplanev1.GroupVersion.Group)
		if ownerRef.APIVersion == controlplanev1.GroupVersion.Group+"/"+controlplanev1.GroupVersion.Version && ownerRef.Kind == "RKE2ControlPlane" {
			cpOwnerRef = ownerRef
		}
	}

	logger.Info("GetOwnerControlPlane result:", "cpOwnerRef", cpOwnerRef)
	if (cpOwnerRef != metav1.OwnerReference{}) {
		return GetControlPlaneByName(ctx, c, obj.Namespace, cpOwnerRef.Name)
	}

	return nil, nil
}

// GetControlPlaneByName finds and return a ControlPlane object using the specified params.
func GetControlPlaneByName(ctx context.Context, c client.Client, namespace, name string) (*controlplanev1.RKE2ControlPlane, error) {
	m := &controlplanev1.RKE2ControlPlane{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, m); err != nil {
		return nil, err
	}
	return m, nil
}

// GetClusterByName finds and return a Cluster object using the specified params.
func GetClusterByName(ctx context.Context, c client.Client, namespace, name string) (*clusterv1.Cluster, error) {
	m := &clusterv1.Cluster{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, m); err != nil {
		return nil, err
	}
	return m, nil
}

// Random generates a random string with length size
func Random(size int) (string, error) {
	token := make([]byte, size)
	_, err := rand.Read(token)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(token), err
}

// TokenName returns a token name from the cluster name
func TokenName(clusterName string) string {
	return fmt.Sprintf("%s-token", clusterName)
}

func Rke2ToKubeVersion(rk2Version string) (kubeVersion string, err error) {
	var regexStr string = "v(\\d\\.\\d{2}\\.\\d)\\+rke2r\\d"
	var regex *regexp.Regexp
	regex, err = regexp.Compile(regexStr)
	if err != nil {
		return "", err
	}
	kubeVersion = string(regex.ReplaceAll([]byte(rk2Version), []byte("$1")))

	return kubeVersion, nil
}
