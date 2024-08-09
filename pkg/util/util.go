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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/version"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
)

const (
	// RKE2_CIS_VERSION_CHANGE is the version where the CIS benchmark changed in RKE2 (because of PSPs).
	RKE2_CIS_VERSION_CHANGE = "v1.25.0"
)

// ErrControlPlaneNotFound is returned when a control plane is not found.
var ErrControlPlaneNotFound = fmt.Errorf("control plane not found")

// GetOwnerControlPlane returns the RKE2ControlPlane object that owns the object passed as parameter.
func GetOwnerControlPlane(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*controlplanev1.RKE2ControlPlane, error) {
	logger := log.FromContext(ctx)

	ownerRefs := obj.OwnerReferences

	var cpOwnerRef metav1.OwnerReference

	for _, ownerRef := range ownerRefs {
		logger.V(5).Info(
			"Inside GetOwnerControlPlane",
			"ownerRef.APIVersion", ownerRef.APIVersion,
			"ownerRef.Kind", ownerRef.Kind,
			"cpv1.GroupVersion.Group", controlplanev1.GroupVersion.Group)

		if ownerRef.APIVersion == controlplanev1.GroupVersion.Group+"/"+controlplanev1.GroupVersion.Version && ownerRef.Kind == "RKE2ControlPlane" {
			cpOwnerRef = ownerRef
		}
	}

	logger.V(5).Info("GetOwnerControlPlane result:", "cpOwnerRef", cpOwnerRef)

	if (cpOwnerRef != metav1.OwnerReference{}) {
		return GetControlPlaneByName(ctx, c, obj.Namespace, cpOwnerRef.Name)
	}

	return nil, ErrControlPlaneNotFound
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

// Random generates a random string with length size.
func Random(size int) (string, error) {
	token := make([]byte, size)

	_, err := rand.Read(token)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(token), err
}

// TokenName returns a token name from the cluster name.
func TokenName(clusterName string) string {
	return fmt.Sprintf("%s-token", clusterName)
}

// Rke2ToKubeVersion converts an RKE2 version to a Kubernetes version.
func Rke2ToKubeVersion(rk2Version string) (kubeVersion string, err error) {
	regexStr := "v(\\d\\.\\d{2}\\.\\d)\\+rke2r\\d"

	var regex *regexp.Regexp

	regex, err = regexp.Compile(regexStr)
	if err != nil {
		return "", err
	}

	kubeVersion = string(regex.ReplaceAll([]byte(rk2Version), []byte("$1")))

	return kubeVersion, nil
}

// IsRKE2Version checks if a string is an RKE2 version.
func IsRKE2Version(rke2Version string) bool {
	regexStr := "v(\\d\\.\\d{2}\\.\\d)\\+rke2r\\d"

	regex, _ := regexp.Compile(regexStr)

	return regex.MatchString(rke2Version)
}

// AppendIfNotPresent appends a string to a slice only if the value does not already exist.
func AppendIfNotPresent(origSlice []string, strItem string) (resultSlice []string) {
	present := false

	for _, item := range origSlice {
		if item == strItem {
			present = true
		}
	}

	if !present {
		return append(origSlice, strItem)
	}

	return origSlice
}

// CompareVersions compares two string version supposing those would begin with 'v' or not.
func CompareVersions(v1 string, v2 string) bool {
	if string(v1[0]) != "v" {
		v1 = "v" + v1
	}

	if string(v2[0]) != "v" {
		v2 = "v" + v2
	}

	return v1 == v2
}

// GetMapKeysAsString returns a comma separated string of keys from a map.
func GetMapKeysAsString(m map[string][]byte) (keys string) {
	for k := range m {
		keys = keys + k + ","
	}
	// remove last comma
	keys = keys[:len(keys)-1]

	return
}

// AtLeastv125 returns true if the RKE2 version is at least v1.25.0.
func AtLeastv125(rke2version string) (bool, error) {
	kubeVersion, err := Rke2ToKubeVersion(rke2version)
	if err != nil {
		return false, err
	}

	parsedVersion := version.MustParseGeneric(kubeVersion)
	if parsedVersion.AtLeast(version.MustParseGeneric(RKE2_CIS_VERSION_CHANGE)) {
		return true, nil
	}

	return false, nil
}

// ProfileCompliant returns true if the CIS profile is compliant.
func ProfileCompliant(profile bootstrapv1.CISProfile, version string) bool {
	isAtLeastv125, err := AtLeastv125(version)
	if err != nil {
		return false
	}

	switch profile {
	case bootstrapv1.CIS:
		return isAtLeastv125
	case bootstrapv1.CIS1_23:
		return isAtLeastv125
	case bootstrapv1.CIS1_5:
		return !isAtLeastv125
	case bootstrapv1.CIS1_6:
		return !isAtLeastv125
	default:
		return false
	}
}
