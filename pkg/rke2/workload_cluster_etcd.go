/*
Copyright 2020 The Kubernetes Authors.

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

package rke2

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	etcdutil "github.com/rancher/cluster-api-provider-rke2/pkg/etcd/util"
)

// ReconcileEtcdMembers iterates over all etcd members and finds members that do not have corresponding nodes.
// If there are any such members, it deletes them from etcd so that etcd does not run etcd health checks on them.
func (w *Workload) ReconcileEtcdMembers(ctx context.Context, nodeNames []string, version semver.Version) ([]string, error) {
	allRemovedMembers := []string{}
	allErrs := []error{}

	// Return early for clusters without an etcd certificate secret
	if w.etcdClientGenerator == nil {
		return allRemovedMembers, nil
	}

	for _, nodeName := range nodeNames {
		removedMembers, errs := w.reconcileEtcdMember(ctx, nodeNames, nodeName, version)
		allRemovedMembers = append(allRemovedMembers, removedMembers...)
		allErrs = append(allErrs, errs...)
	}

	return allRemovedMembers, kerrors.NewAggregate(allErrs)
}

func (w *Workload) reconcileEtcdMember(ctx context.Context, nodeNames []string, nodeName string, _ semver.Version) ([]string, []error) {
	// Create the etcd Client for the etcd Pod scheduled on the Node
	etcdClient, err := w.etcdClientGenerator.ForFirstAvailableNode(ctx, []string{nodeName})
	if err != nil {
		return nil, nil
	}
	defer etcdClient.Close()

	members, err := etcdClient.Members(ctx)
	if err != nil {
		return nil, nil
	}

	// Check if any member's node is missing from workload cluster
	// If any, delete it with best effort
	removedMembers := []string{}
	errs := []error{}
loopmembers:
	for _, member := range members {
		// If this member is just added, it has a empty name until the etcd pod starts. Ignore it.
		if member.Name == "" {
			continue
		}

		for _, nodeName := range nodeNames {
			if strings.Contains(member.Name, nodeName) {
				// We found the matching node, continue with the outer loop.
				continue loopmembers
			}
		}

		// If we're here, the node cannot be found.
		removedMembers = append(removedMembers, member.Name)
		if err := w.removeMemberForNode(ctx, member.Name); err != nil {
			errs = append(errs, err)
		}
	}

	return removedMembers, errs
}

// RemoveEtcdMemberForMachine removes the etcd member from the target cluster's etcd cluster.
// Removing the last remaining member of the cluster is not supported.
func (w *Workload) RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error {
	if machine == nil || machine.Status.NodeRef == nil {
		// Nothing to do, no node for Machine
		return nil
	}

	return w.removeMemberForNode(ctx, machine.Status.NodeRef.Name)
}

func (w *Workload) removeMemberForNode(ctx context.Context, name string) error {
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return err
	}

	if len(controlPlaneNodes.Items) < minimalNodeCount {
		return ErrControlPlaneMinNodes
	}

	// Return early for clusters without an etcd certificate secret
	if w.etcdClientGenerator == nil {
		return nil
	}

	// Exclude node being removed from etcd client node list
	var remainingNodes []string

	for _, n := range controlPlaneNodes.Items {
		if !strings.Contains(name, n.Name) {
			remainingNodes = append(remainingNodes, n.Name)
		}
	}

	etcdClient, err := w.etcdClientGenerator.ForFirstAvailableNode(ctx, remainingNodes)
	if err != nil {
		return errors.Wrap(err, "failed to create etcd client")
	}
	defer etcdClient.Close()

	// List etcd members. This checks that the member is healthy, because the request goes through consensus.
	members, err := etcdClient.Members(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list etcd members using etcd client")
	}

	member := etcdutil.MemberForName(members, name)

	// The member has already been removed, return immediately
	if member == nil {
		return nil
	}

	if err := etcdClient.RemoveMember(ctx, member.ID); err != nil {
		return errors.Wrap(err, "failed to remove member from etcd")
	}

	log.FromContext(ctx).Info(fmt.Sprintf("Removed member: %s", member.Name))

	return nil
}

// ForwardEtcdLeadership forwards etcd leadership to the first follower.
func (w *Workload) ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error {
	if machine == nil || machine.Status.NodeRef == nil {
		return nil
	}

	if leaderCandidate == nil {
		return errors.New("leader candidate cannot be nil")
	}

	if leaderCandidate.Status.NodeRef == nil {
		return errors.New("leader has no node reference")
	}

	// Return early for clusters without an etcd certificate secret
	if w.etcdClientGenerator == nil {
		return nil
	}

	nodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list control plane nodes")
	}

	nodeNames := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	etcdClient, err := w.etcdClientGenerator.ForLeader(ctx, nodeNames)
	if err != nil {
		return errors.Wrap(err, "failed to create etcd client")
	}
	defer etcdClient.Close()

	members, err := etcdClient.Members(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list etcd members using etcd client")
	}

	currentMember := etcdutil.MemberForName(members, machine.Status.NodeRef.Name)
	if currentMember == nil || currentMember.ID != etcdClient.LeaderID {
		// nothing to do, this is not the etcd leader
		return nil
	}

	// Move the leader to the provided candidate.
	nextLeader := etcdutil.MemberForName(members, leaderCandidate.Status.NodeRef.Name)
	if nextLeader == nil {
		return errors.Errorf("failed to get etcd member from node %q", leaderCandidate.Status.NodeRef.Name)
	}

	log.FromContext(ctx).Info(fmt.Sprintf("Moving leader from %s to %s", currentMember.Name, nextLeader.Name))

	if err := etcdClient.MoveLeader(ctx, nextLeader.ID); err != nil {
		return errors.Wrapf(err, "failed to move leader")
	}

	return nil
}

// EtcdMemberStatus contains status information for a single etcd member.
type EtcdMemberStatus struct {
	Name       string
	Responsive bool
}

// EtcdMembers returns the current set of members in an etcd cluster.
//
// NOTE: This methods uses control plane machines/nodes only to get in contact with etcd,
// but then it relies on etcd as ultimate source of truth for the list of members.
// This is intended to allow informed decisions on actions impacting etcd quorum.
func (w *Workload) EtcdMembers(ctx context.Context) ([]string, error) {
	// Return early for clusters without an etcd certificate secret
	if w.etcdClientGenerator == nil {
		return []string{}, nil
	}

	nodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list control plane nodes")
	}

	nodeNames := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}

	etcdClient, err := w.etcdClientGenerator.ForLeader(ctx, nodeNames)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create etcd client")
	}
	defer etcdClient.Close()

	members, err := etcdClient.Members(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list etcd members using etcd client")
	}

	names := []string{}
	for _, member := range members {
		names = append(names, member.Name)
	}

	return names, nil
}
