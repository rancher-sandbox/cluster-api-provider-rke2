/*
Copyright 2024 SUSE.

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

package etcd

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"

	"github.com/rancher/cluster-api-provider-rke2/pkg/proxy"
)

const etcdPort = 2379

// ClientFor is an interface for source etcd node selection.
type ClientFor interface {
	ForFirstAvailableNode(ctx context.Context, nodeNames []string) (*Client, error)
	ForLeader(ctx context.Context, nodeNames []string) (*Client, error)
}

// ClientGenerator generates etcd clients that connect to specific etcd members on particular control plane nodes.
type ClientGenerator struct {
	restConfig   *rest.Config
	tlsConfig    *tls.Config
	createClient clientCreator
}

type clientCreator func(ctx context.Context, endpoint string) (*Client, error)

var errEtcdNodeConnection = errors.New("failed to connect to etcd node")

// NewClientGenerator returns a new etcdClientGenerator instance.
func NewClientGenerator(restConfig *rest.Config, tlsConfig *tls.Config, etcdDialTimeout, etcdCallTimeout time.Duration) *ClientGenerator {
	ecg := &ClientGenerator{restConfig: restConfig, tlsConfig: tlsConfig}

	ecg.createClient = func(ctx context.Context, endpoint string) (*Client, error) {
		p := proxy.Proxy{
			Kind:       "pods",
			Namespace:  metav1.NamespaceSystem,
			KubeConfig: ecg.restConfig,
			Port:       etcdPort,
		}

		return NewClient(ctx, ClientConfiguration{
			Endpoint:    endpoint,
			Proxy:       p,
			TLSConfig:   tlsConfig,
			DialTimeout: etcdDialTimeout,
			CallTimeout: etcdCallTimeout,
		})
	}

	return ecg
}

// ForFirstAvailableNode takes a list of nodes and returns a client for the first one that connects.
func (c *ClientGenerator) ForFirstAvailableNode(ctx context.Context, nodeNames []string) (*Client, error) {
	// This is an additional safeguard for avoiding this func to return nil, nil.
	if len(nodeNames) == 0 {
		return nil, errors.New("invalid argument: forLeader can't be called with an empty list of nodes")
	}

	// Loop through the existing control plane nodes.
	var errs []error

	for _, name := range nodeNames {
		endpoint := staticPodName("etcd", name)

		client, err := c.createClient(ctx, endpoint)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		return client, nil
	}

	return nil, errors.Wrap(kerrors.NewAggregate(errs), "could not establish a connection to any etcd node")
}

// ForLeader takes a list of nodes and returns a client to the leader node.
func (c *ClientGenerator) ForLeader(ctx context.Context, nodeNames []string) (*Client, error) {
	// This is an additional safeguard for avoiding this func to return nil, nil.
	if len(nodeNames) == 0 {
		return nil, errors.New("invalid argument: forLeader can't be called with an empty list of nodes")
	}

	// Loop through the existing control plane nodes.
	var errs []error

	for _, nodeName := range nodeNames {
		cl, err := c.getLeaderClient(ctx, nodeName, nodeNames)
		if err != nil {
			if errors.Is(err, errEtcdNodeConnection) {
				errs = append(errs, err)

				continue
			}

			return nil, err
		}

		return cl, nil
	}

	return nil, errors.Wrap(kerrors.NewAggregate(errs), "could not establish a connection to the etcd leader")
}

// getLeaderClient provides an etcd client connected to the leader. It returns an
// errEtcdNodeConnection if there was a connection problem with the given etcd
// node, which should be considered non-fatal by the caller.
func (c *ClientGenerator) getLeaderClient(ctx context.Context, nodeName string, nodeNames []string) (*Client, error) {
	// Get a temporary client to the etcd instance hosted on the node.
	client, err := c.ForFirstAvailableNode(ctx, []string{nodeName})
	if err != nil {
		return nil, kerrors.NewAggregate([]error{err, errEtcdNodeConnection})
	}
	defer client.Close()

	// Get the list of members.
	members, err := client.Members(ctx)
	if err != nil {
		return nil, kerrors.NewAggregate([]error{err, errEtcdNodeConnection})
	}

	// Get the leader member.
	var leaderMember *Member

	for _, member := range members {
		if member.ID == client.LeaderID {
			leaderMember = member

			break
		}
	}

	// If we found the leader, and it is one of the nodes,
	// get a connection to the etcd leader via the node hosting it.
	if leaderMember != nil {
		nodeName := ""

		for _, name := range nodeNames {
			if strings.Contains(leaderMember.Name, name) {
				nodeName = name

				break
			}
		}

		if nodeName == "" {
			return nil, errors.Errorf("etcd leader is reported as %x with name %q, but we couldn't find a corresponding Node in the cluster", leaderMember.ID, leaderMember.Name) //nolint:lll
		}

		client, err = c.ForFirstAvailableNode(ctx, []string{nodeName})

		return client, err
	}

	// If it is not possible to get a connection to the leader via existing nodes,
	// it means that the control plane is an invalid state, with an etcd member - the current leader -
	// without a corresponding node.
	return nil, errors.Errorf("etcd leader is reported as %x, but we couldn't find any matching member", client.LeaderID)
}

func staticPodName(component, nodeName string) string {
	return fmt.Sprintf("%s-%s", component, nodeName)
}
