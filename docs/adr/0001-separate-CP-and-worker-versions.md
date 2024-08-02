- [1. Separate Control Plane and Worker Versions](#1-separate-control-plane-and-worker-versions)
  - [Context](#context)
  - [Decision](#decision)
  - [Consequences](#consequences)


# 1. Separate Control Plane and Worker Versions

- Status: accepted
- Date: 2024-05-20
- Authors: @Danil-Grigorev
- Deciders: @alexander-demicev @furkatgofurov7 @salasberryfin @mjura @yiannistri

## Context

In the context of Cluster API, having separate worker and control plane versions is a valid and supported scenario, particularly useful during upgrades.

Three specific scenarios highlight the use-cases for separate version management:

1. **Separate CP and workers upgrade**: When a control plane node is upgraded to a new Kubernetes version while the worker nodes remain on an
older version, or vice versa. In this situation, it's essential to manage the state of the cluster, including the different versions of the workers and control plane nodes.
2. **Failed upgrade**: A situation where some worker or control plane machine wasn't upgraded successfully and is
stuck in the previous version. The cluster remains functional, but the desired agent version doesn't declare the state of all
machines. This requires a manual downgrade of the affected machine templates version, but should not force downgrade 
separate group of machines (CP or workers).
3. **ClusterClass usage**: When a ClusterClass is used as a template to declare a cluster, the version field
inside the `MachineDeployment` template doesn't hold true, but instead the `AgentConfig` `spec.version` is used.
In this case, the template becomes useless for declaring the version of the control plane or worker nodes.

## Decision

To follow the upstream approach we remove the `AgentConfig` `spec.version` field in favor of `MachineDeployment` version 
and the `RKE2ControlPlane` version fields. Existing `AgentConfig` version will be transferred by conversion webhooks to `v1beta1.RKE2ControlPlane` resource.

## Consequences

`AgentConfig` version is removed, so the `RKE2ControlPlane` and `MachineDeployment` should declare valid versions, following `RKE2` naming [pattern](https://github.com/rancher/rke2/releases).

For users affected by the [#315](https://github.com/rancher/cluster-api-provider-rke2/issues/315) this will require 2 step process:
1. Check that version defined in `MachineDeployment` matches rke2 releases: [https://github.com/rancher/rke2/releases](https://github.com/rancher/rke2/releases)
2. Force re-rollout of all worker nodes to the version currently set in `MachineDeployment` or upgrade workers to the new version.
