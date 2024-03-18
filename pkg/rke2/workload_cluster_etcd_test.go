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
	"errors"
	"fmt"
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/etcd"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	etcdfake "github.com/rancher-sandbox/cluster-api-provider-rke2/pkg/etcd/fake"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestRemoveEtcdMemberForMachine(t *testing.T) {
	machine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "cp1",
			},
		},
	}
	cp1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cp1",
			Namespace: "cp1",
			Labels: map[string]string{
				labelNodeRoleControlPlane: "true",
			},
		},
	}
	cp1DiffNS := cp1.DeepCopy()
	cp1DiffNS.Namespace = "diff-ns"

	cp2 := cp1.DeepCopy()
	cp2.Name = "cp2"
	cp2.Namespace = "cp2"

	tests := []struct {
		name                string
		machine             *clusterv1.Machine
		etcdClientGenerator etcd.ClientFor
		objs                []client.Object
		expectErr           bool
	}{
		{
			name:      "does nothing if the machine is nil",
			machine:   nil,
			expectErr: false,
		},
		{
			name: "does nothing if the machine has no node",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					NodeRef: nil,
				},
			},
			expectErr: false,
		},
		{
			name:      "returns an error if there are less than 2 control plane nodes",
			machine:   machine,
			objs:      []client.Object{cp1},
			expectErr: true,
		},
		{
			name:                "returns an error if it fails to create the etcd client",
			machine:             machine,
			objs:                []client.Object{cp1, cp2},
			etcdClientGenerator: &fakeEtcdClientGenerator{forNodesErr: errors.New("no client")},
			expectErr:           true,
		},
		{
			name:    "returns an error if the client errors getting etcd members",
			machine: machine,
			objs:    []client.Object{cp1, cp2},
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &etcdfake.FakeEtcdClient{
						ErrorResponse: errors.New("cannot get etcd members"),
					},
				},
			},
			expectErr: true,
		},
		{
			name:    "returns an error if the client errors removing the etcd member",
			machine: machine,
			objs:    []client.Object{cp1, cp2},
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &etcdfake.FakeEtcdClient{
						ErrorResponse: errors.New("cannot remove etcd member"),
						MemberListResponse: &clientv3.MemberListResponse{
							Members: []*pb.Member{
								{Name: "cp1", ID: uint64(1)},
								{Name: "test-2", ID: uint64(2)},
								{Name: "test-3", ID: uint64(3)},
							},
						},
						AlarmResponse: &clientv3.AlarmResponse{
							Alarms: []*pb.AlarmMember{},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name:    "removes the member from etcd",
			machine: machine,
			objs:    []client.Object{cp1, cp2},
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &etcdfake.FakeEtcdClient{
						MemberListResponse: &clientv3.MemberListResponse{
							Members: []*pb.Member{
								{Name: "cp1", ID: uint64(1)},
								{Name: "test-2", ID: uint64(2)},
								{Name: "test-3", ID: uint64(3)},
							},
						},
						AlarmResponse: &clientv3.AlarmResponse{
							Alarms: []*pb.AlarmMember{},
						},
					},
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.objs...).Build()
			w := &Workload{
				Client:              fakeClient,
				etcdClientGenerator: tt.etcdClientGenerator,
			}
			err := w.RemoveEtcdMemberForMachine(ctx, tt.machine)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestForwardEtcdLeadership(t *testing.T) {
	t.Run("handles errors correctly", func(t *testing.T) {
		tests := []struct {
			name                string
			machine             *clusterv1.Machine
			leaderCandidate     *clusterv1.Machine
			etcdClientGenerator etcd.ClientFor
			k8sClient           client.Client
			expectErr           bool
		}{
			{
				name:      "does nothing if the machine is nil",
				machine:   nil,
				expectErr: false,
			},
			{
				name: "does nothing if machine's NodeRef is nil",
				machine: defaultMachine(func(m *clusterv1.Machine) {
					m.Status.NodeRef = nil
				}),
				expectErr: false,
			},
			{
				name:            "returns an error if the leader candidate is nil",
				machine:         defaultMachine(),
				leaderCandidate: nil,
				expectErr:       true,
			},
			{
				name:    "returns an error if the leader candidate's noderef is nil",
				machine: defaultMachine(),
				leaderCandidate: defaultMachine(func(m *clusterv1.Machine) {
					m.Status.NodeRef = nil
				}),
				expectErr: true,
			},
			{
				name:                "returns an error if it can't retrieve the list of control plane nodes",
				machine:             defaultMachine(),
				leaderCandidate:     defaultMachine(),
				etcdClientGenerator: &fakeEtcdClientGenerator{},
				k8sClient:           &fakeClient{listErr: errors.New("failed to list nodes")},
				expectErr:           true,
			},
			{
				name:                "returns an error if it can't create an etcd client",
				machine:             defaultMachine(),
				leaderCandidate:     defaultMachine(),
				k8sClient:           &fakeClient{},
				etcdClientGenerator: &fakeEtcdClientGenerator{forLeaderErr: errors.New("no etcdClient")},
				expectErr:           true,
			},
			{
				name:            "returns error if it fails to get etcd members",
				machine:         defaultMachine(),
				leaderCandidate: defaultMachine(),
				k8sClient:       &fakeClient{},
				etcdClientGenerator: &fakeEtcdClientGenerator{
					forLeaderClient: &etcd.Client{
						EtcdClient: &etcdfake.FakeEtcdClient{
							ErrorResponse: errors.New("cannot get etcd members"),
						},
					},
				},
				expectErr: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				w := &Workload{
					Client:              tt.k8sClient,
					etcdClientGenerator: tt.etcdClientGenerator,
				}
				err := w.ForwardEtcdLeadership(ctx, tt.machine, tt.leaderCandidate)
				if tt.expectErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
			})
		}
	})

	t.Run("does nothing if the machine is not the leader", func(t *testing.T) {
		g := NewWithT(t)
		fakeEtcdClient := &etcdfake.FakeEtcdClient{
			MemberListResponse: &clientv3.MemberListResponse{
				Members: []*pb.Member{
					{Name: "machine-node", ID: uint64(101)},
				},
			},
			AlarmResponse: &clientv3.AlarmResponse{
				Alarms: []*pb.AlarmMember{},
			},
		}
		etcdClientGenerator := &fakeEtcdClientGenerator{
			forLeaderClient: &etcd.Client{
				EtcdClient: fakeEtcdClient,
				LeaderID:   555,
			},
		}

		w := &Workload{
			Client: &fakeClient{list: &corev1.NodeList{
				Items: []corev1.Node{nodeNamed("leader-node")},
			}},
			etcdClientGenerator: etcdClientGenerator,
		}
		err := w.ForwardEtcdLeadership(ctx, defaultMachine(), defaultMachine())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(fakeEtcdClient.MovedLeader).To(BeEquivalentTo(0))
	})

	t.Run("move etcd leader", func(t *testing.T) {
		tests := []struct {
			name               string
			leaderCandidate    *clusterv1.Machine
			etcdMoveErr        error
			expectedMoveLeader uint64
			expectErr          bool
		}{
			{
				name: "it moves the etcd leadership to the leader candidate",
				leaderCandidate: defaultMachine(func(m *clusterv1.Machine) {
					m.Status.NodeRef.Name = "candidate-node"
				}),
				expectedMoveLeader: 12345,
			},
			{
				name: "returns error if failed to move to the leader candidate",
				leaderCandidate: defaultMachine(func(m *clusterv1.Machine) {
					m.Status.NodeRef.Name = "candidate-node"
				}),
				etcdMoveErr: errors.New("move err"),
				expectErr:   true,
			},
			{
				name: "returns error if the leader candidate doesn't exist in etcd",
				leaderCandidate: defaultMachine(func(m *clusterv1.Machine) {
					m.Status.NodeRef.Name = "some other node"
				}),
				expectErr: true,
			},
		}

		currentLeader := defaultMachine(func(m *clusterv1.Machine) {
			m.Status.NodeRef.Name = "current-leader"
		})
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				fakeEtcdClient := &etcdfake.FakeEtcdClient{
					ErrorResponse: tt.etcdMoveErr,
					MemberListResponse: &clientv3.MemberListResponse{
						Members: []*pb.Member{
							{Name: currentLeader.Status.NodeRef.Name, ID: uint64(101)},
							{Name: "other-node", ID: uint64(1034)},
							{Name: "candidate-node", ID: uint64(12345)},
						},
					},
					AlarmResponse: &clientv3.AlarmResponse{
						Alarms: []*pb.AlarmMember{},
					},
				}

				etcdClientGenerator := &fakeEtcdClientGenerator{
					forLeaderClient: &etcd.Client{
						EtcdClient: fakeEtcdClient,
						// this etcdClient belongs to the machine-node
						LeaderID: 101,
					},
				}

				w := &Workload{
					etcdClientGenerator: etcdClientGenerator,
					Client: &fakeClient{list: &corev1.NodeList{
						Items: []corev1.Node{nodeNamed("leader-node"), nodeNamed("other-node"), nodeNamed("candidate-node")},
					}},
				}
				err := w.ForwardEtcdLeadership(ctx, currentLeader, tt.leaderCandidate)
				if tt.expectErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(fakeEtcdClient.MovedLeader).To(BeEquivalentTo(tt.expectedMoveLeader))
			})
		}
	})
}

func TestReconcileEtcdMembers(t *testing.T) {
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ip-10-0-0-1.ec2.internal",
			Namespace: "ns1",
			Labels: map[string]string{
				labelNodeRoleControlPlane: "true",
			},
		},
	}
	node2 := node1.DeepCopy()
	node2.Name = "ip-10-0-0-2.ec2.internal"

	fakeEtcdClient := &etcdfake.FakeEtcdClient{
		MemberListResponse: &clientv3.MemberListResponse{
			Members: []*pb.Member{
				{Name: "ip-10-0-0-1.ec2.internal", ID: uint64(1)},
				{Name: "ip-10-0-0-2.ec2.internal", ID: uint64(2)},
				{Name: "ip-10-0-0-3.ec2.internal", ID: uint64(3)},
			},
		},
		AlarmResponse: &clientv3.AlarmResponse{
			Alarms: []*pb.AlarmMember{},
		},
	}

	tests := []struct {
		name                string
		kubernetesVersion   semver.Version
		objs                []client.Object
		nodes               []string
		etcdClientGenerator etcd.ClientFor
		expectErr           bool
		assert              func(*WithT, client.Client)
	}{
		{
			// the node to be removed is ip-10-0-0-3.ec2.internal since the
			// other two have nodes
			name:              "successfully removes the etcd member without a node for Kubernetes version < 1.22.0",
			kubernetesVersion: semver.MustParse("1.19.1"), // Kubernetes version < 1.22.0 has ClusterStatus
			objs:              []client.Object{node1.DeepCopy(), node2.DeepCopy()},
			nodes:             []string{node1.Name, node2.Name},
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: fakeEtcdClient,
				},
			},
			expectErr: false,
			assert: func(g *WithT, c client.Client) {
				g.Expect(fakeEtcdClient.RemovedMember).To(Equal(uint64(3)))
			},
		},
		{
			// the node to be removed is ip-10-0-0-3.ec2.internal since the
			// other two have nodes
			name:              "successfully removes the etcd member without a node for Kubernetes version >= 1.22.0",
			kubernetesVersion: semver.MustParse("1.22.0"), // Kubernetes version >= 1.22.0 does not have ClusterStatus
			objs:              []client.Object{node1.DeepCopy(), node2.DeepCopy()},
			nodes:             []string{node1.Name, node2.Name},
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: fakeEtcdClient,
				},
			},
			expectErr: false,
			assert: func(g *WithT, c client.Client) {
				g.Expect(fakeEtcdClient.RemovedMember).To(Equal(uint64(3)))
			},
		},
		{
			name:  "return error if there aren't enough control plane nodes",
			objs:  []client.Object{node1.DeepCopy()},
			nodes: []string{node1.Name},
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: fakeEtcdClient,
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewClientBuilder().WithObjects(tt.objs...).Build()
			w := &Workload{
				Client:              fakeClient,
				etcdClientGenerator: tt.etcdClientGenerator,
			}
			ctx := context.TODO()
			_, err := w.ReconcileEtcdMembers(ctx, tt.nodes, tt.kubernetesVersion)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			if tt.assert != nil {
				tt.assert(g, fakeClient)
			}
		})
	}
}

type fakeEtcdClientGenerator struct {
	forNodesClient     *etcd.Client
	forNodesClientFunc func([]string) (*etcd.Client, error)
	forLeaderClient    *etcd.Client
	forNodesErr        error
	forLeaderErr       error
}

func (c *fakeEtcdClientGenerator) ForFirstAvailableNode(_ context.Context, n []string) (*etcd.Client, error) {
	if c.forNodesClientFunc != nil {
		return c.forNodesClientFunc(n)
	}
	return c.forNodesClient, c.forNodesErr
}

func (c *fakeEtcdClientGenerator) ForLeader(_ context.Context, _ []string) (*etcd.Client, error) {
	return c.forLeaderClient, c.forLeaderErr
}

type fakeClient struct {
	client.Client
	list interface{}

	createErr    error
	get          map[string]interface{}
	getCalled    bool
	updateCalled bool
	getErr       error
	patchErr     error
	updateErr    error
	listErr      error
}

func (f *fakeClient) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	f.getCalled = true
	if f.getErr != nil {
		return f.getErr
	}
	item := f.get[key.String()]
	switch l := item.(type) {
	case *corev1.Pod:
		l.DeepCopyInto(obj.(*corev1.Pod))
	case *rbacv1.RoleBinding:
		l.DeepCopyInto(obj.(*rbacv1.RoleBinding))
	case *rbacv1.Role:
		l.DeepCopyInto(obj.(*rbacv1.Role))
	case *appsv1.DaemonSet:
		l.DeepCopyInto(obj.(*appsv1.DaemonSet))
	case *corev1.ConfigMap:
		l.DeepCopyInto(obj.(*corev1.ConfigMap))
	case *corev1.Secret:
		l.DeepCopyInto(obj.(*corev1.Secret))
	case nil:
		return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
	default:
		return fmt.Errorf("unknown type: %s", l)
	}
	return nil
}

func (f *fakeClient) List(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
	if f.listErr != nil {
		return f.listErr
	}
	switch l := f.list.(type) {
	case *clusterv1.MachineList:
		l.DeepCopyInto(list.(*clusterv1.MachineList))
	case *corev1.NodeList:
		l.DeepCopyInto(list.(*corev1.NodeList))
	case *corev1.PodList:
		l.DeepCopyInto(list.(*corev1.PodList))
	default:
		return fmt.Errorf("unknown type: %s", l)
	}
	return nil
}

func (f *fakeClient) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	if f.createErr != nil {
		return f.createErr
	}
	return nil
}

func (f *fakeClient) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
	if f.patchErr != nil {
		return f.patchErr
	}
	return nil
}

func (f *fakeClient) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	f.updateCalled = true
	if f.updateErr != nil {
		return f.updateErr
	}
	return nil
}

func defaultMachine(transforms ...func(m *clusterv1.Machine)) *clusterv1.Machine {
	m := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "machine-node",
			},
		},
	}
	for _, t := range transforms {
		t(m)
	}
	return m
}

func nodeNamed(name string) corev1.Node {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return node
}
