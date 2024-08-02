# Cluster API Provider RKE2

![GitHub](https://img.shields.io/github/license/rancher/cluster-api-provider-rke2)

------

## What is Cluster API Provider RKE2

The [Cluster API](https://cluster-api.sigs.k8s.io/) brings declarative, Kubernetes-style APIs to cluster creation, configuration and management.

Cluster API Provider RKE2 is a combination of 2 provider types, a __Cluster API Control Plane Provider__ for provisioning Kubernetes control plane nodes and a __Cluster API Bootstrap Provider__ for bootstrapping Kubernetes on a machine where [RKE2](https://docs.rke2.io/) is used as the Kubernetes distro.

------

## Getting Started
Cluster API Provider RKE2 is compliant with the `clusterctl` contract, which means that `clusterctl` simplifies its deployment to the CAPI Management Cluster. In this Getting Started guide, we will be using the RKE2 Provider with the `docker` provider (also called `CAPD`).

### Management Cluster

In order to use this provider, you need to have a management cluster available to you and have your current KUBECONFIG context set to talk to that cluster. If you do not have a cluster available to you, you can create a `kind` cluster. These are the steps needed to achieve that:
1. Ensure kind is installed (https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
2. Create a special `kind` configuration file if you intend to use the Docker infrastructure provider:

```bash
cat > kind-cluster-with-extramounts.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: capi-test
nodes:
- role: control-plane
  extraMounts:
    - hostPath: /var/run/docker.sock
      containerPath: /var/run/docker.sock
EOF
```

3. Run the following command to create a local kind cluster:

```bash
kind create cluster --config kind-cluster-with-extramounts.yaml
```

4. Check your newly created `kind` cluster :

```bash
kubectl cluster-info
```
and get a similar result to this:

```
Kubernetes control plane is running at https://127.0.0.1:40819
CoreDNS is running at https://127.0.0.1:40819/api/v1/namespaces/kube-system/services/kube-dns:dns/proxy

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

### Setting up clusterctl

#### CAPI >= v1.6.0

No additional steps are required and you can install the RKE2 provider with **clusterctl** directly:

```bash
clusterctl init --bootstrap rke2 --control-plane rke2 --infrastructure docker
```

#### CAPI < v1.6.0

With CAPI & clusterctl versions less than v1.6.0 you need a specific configuration. To do this create a file called `clusterctl.yaml` in the `$HOME/.cluster-api` folder with the following content:

```yaml
providers:
  - name: "rke2"
    url: "https://github.com/rancher/cluster-api-provider-rke2/releases/v0.1.1/bootstrap-components.yaml"
    type: "BootstrapProvider"
  - name: "rke2"
    url: "https://github.com/rancher/cluster-api-provider-rke2/releases/v0.1.1/control-plane-components.yaml"
    type: "ControlPlaneProvider"
``` 
> NOTE: Due to some issue related to how `CAPD` creates Load Balancer healthchecks, it is necessary to use a fork of `CAPD` by providing in the above configuration file the following :

```yaml
  - name: "docker"
    url: "https://github.com/belgaied2/cluster-api/releases/v1.3.3-cabpr-fix/infrastructure-components.yaml"
    type: "InfrastructureProvider"
``` 

This configuration tells clusterctl where to look for provider manifests in order to deploy provider components in the management cluster. 

The next step is to run the `clusterctl init` command:

```bash
clusterctl init --bootstrap rke2 --control-plane rke2 --infrastructure docker:v1.3.3-cabpr-fix
```

This should output something similar to the following:

```
Fetching providers
Installing cert-manager Version="v1.10.1"
Waiting for cert-manager to be available...
Installing Provider="cluster-api" Version="v1.3.3" TargetNamespace="capi-system"
Installing Provider="bootstrap-rke2" Version="v0.1.0-alpha.1" TargetNamespace="rke2-bootstrap-system"
Installing Provider="control-plane-rke2" Version="v0.1.0-alpha.1" TargetNamespace="rke2-control-plane-system"

Your management cluster has been initialized successfully!

You can now create your first workload cluster by running the following:

  clusterctl generate cluster [name] --kubernetes-version [version] | kubectl apply -f -
```

### Create a workload cluster

There are some sample cluster templates available under the `samples` folder. This section assumes you are using CAPI v1.6.0 or higher.

For this `Getting Started` section, we will be using the `docker` samples available under `samples/docker/oneline-default` folder. This folder contains a YAML template file called `rke2-sample.yaml` which contains environment variable placeholders which can be substituted using the [envsubst](https://github.com/a8m/envsubst/releases) tool. We will use `clusterctl` to generate the manifests from these template files.
Set the following environment variables:
- CABPR_NAMESPACE
- CLUSTER_NAME
- CABPR_CP_REPLICAS
- CABPR_WK_REPLICAS
- KUBERNETES_VERSION

for example:

```bash
export CABPR_NAMESPACE=example
export CLUSTER_NAME=capd-rke2-test
export CABPR_CP_REPLICAS=3
export CABPR_WK_REPLICAS=2
export KUBERNETES_VERSION=v1.24.6 
```

The next step is to substitue the values in the YAML using the following commands:

```bash
cat rke2-sample.yaml | clusterctl generate yaml > rke2-docker-example.yaml
```

At this moment, you can take some time to study the resulting YAML, then you can apply it to the management cluster:

```bash
kubectl apply -f rke2-docker-example.yaml
```
and see the following output:
```
namespace/example created
cluster.cluster.x-k8s.io/rke2-test created
dockercluster.infrastructure.cluster.x-k8s.io/rke2-test created
rke2controlplane.controlplane.cluster.x-k8s.io/rke2-test-control-plane created
dockermachinetemplate.infrastructure.cluster.x-k8s.io/controlplane created
machinedeployment.cluster.x-k8s.io/worker-md-0 created
dockermachinetemplate.infrastructure.cluster.x-k8s.io/worker created
rke2configtemplate.bootstrap.cluster.x-k8s.io/rke2-test-agent created
```

### Checking the workload cluster

After waiting several minutes, you can check the state of CAPI machines, by running the following command:

```bash
kubectl get machine -n example
```

and you should see output similar to the following:
```
NAME                                 CLUSTER          NODENAME                                      PROVIDERID                                               PHASE     AGE     VERSION
capd-rke2-test-control-plane-4njnd   capd-rke2-test   capd-rke2-test-control-plane-4njnd            docker:////capd-rke2-test-control-plane-4njnd            Running   5m18s   v1.24.6
capd-rke2-test-control-plane-rccsk   capd-rke2-test   capd-rke2-test-control-plane-rccsk            docker:////capd-rke2-test-control-plane-rccsk            Running   3m1s    v1.24.6
capd-rke2-test-control-plane-v5g8v   capd-rke2-test   capd-rke2-test-control-plane-v5g8v            docker:////capd-rke2-test-control-plane-v5g8v            Running   8m4s    v1.24.6
worker-md-0-6d4944f5b6-k5xxw         capd-rke2-test   capd-rke2-test-worker-md-0-6d4944f5b6-k5xxw   docker:////capd-rke2-test-worker-md-0-6d4944f5b6-k5xxw   Running   8m6s    v1.24.6
worker-md-0-6d4944f5b6-qjbjh         capd-rke2-test   capd-rke2-test-worker-md-0-6d4944f5b6-qjbjh   docker:////capd-rke2-test-worker-md-0-6d4944f5b6-qjbjh   Running   8m6s    v1.24.6
```

You can now get the kubeconfig file for the workload cluster using :

```bash
clusterctl get kubeconfig capd-rke2-test -n example > ~/capd-rke2-test-kubeconfig.yaml
export KUBECONFIG=~/capd-rke2-test-kubeconfig.yaml
``` 

and query the newly created cluster using:

```bash
kubectl cluster-info
```

and see output like this:

```
Kubernetes control plane is running at https://172.18.0.5:6443
CoreDNS is running at https://172.18.0.5:6443/api/v1/namespaces/kube-system/services/rke2-coredns-rke2-coredns:udp-53/proxy

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

:tada: CONGRATULATIONS ! :tada: You created your first RKE2 cluster with CAPD as an infrastructure provider.

### Using ClusterClass for cluster creation

This provider supports using [ClusterClass](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20210526-cluster-class-and-managed-topologies.md), a Cluster API feature that implements an extra level of abstraction on top of the existing Cluster API functionality. The `ClusterClass` object is used to define a collection of template resources (control plane and machine deployment) which are used to generate one or more clusters of the same flavor.

If you are interested in leveraging this functionality, you can refer to the examples [here](./samples/docker/clusterclass/):
- [clusterclass-quick-start.yaml](./samples/docker/clusterclass/clusterclass-quick-start.yaml): creates a sample `ClusterClass` and necessary resources.
- [rke2-sample.yaml](./samples/docker/clusterclass/rke2-sample.yaml): creates a workload cluster using the `ClusterClass`.

As with other sample templates, you will need to set a number environment variables:
- CLUSTER_NAME
- CABPR_CP_REPLICAS
- CABPR_WK_REPLICAS
- KUBERNETES_VERSION
- KIND_IP

for example:

```bash
export CLUSTER_NAME=capd-rke2-clusterclass
export CABPR_CP_REPLICAS=3
export CABPR_WK_REPLICAS=2
export KUBERNETES_VERSION=v1.25.11
export KIND_IP=192.168.20.20
```

**Remember that, since we are using Kind, the value of `KIND_IP` must be an IP address in the range of the `kind` network.**
You can check the range Docker assigns to this network by inspecting it:

```bash
docker network inspect kind
```

The next step is to substitue the values in the YAML using the following commands:

```bash
cat clusterclass-quick-start.yaml | clusterctl generate yaml > clusterclass-example.yaml
```

At this moment, you can take some time to study the resulting YAML, then you can apply it to the management cluster:

```bash
kubectl apply -f clusterclass-example.yaml
```

This will create a new `ClusterClass` template that can be used to provision one or multiple workload clusters of the same flavor.
To do so, you can follow the same procedure and substitute the values in the YAML for the cluster definition:

```bash
cat rke2-sample.yaml | clusterctl generate yaml > rke2-clusterclass-example.yaml
```

And then apply the resulting YAML file to create a cluster from the existing `ClusterClass`.
```bash
kubectl apply -f rke2-clusterclass-example.yaml
```

## Testing the DEV main branch
These instructions are for development purposes initially and will be changed in the future for user facing instructions.

1. Clone the [Cluster API Repo](https://github.com/kubernetes-sigs/cluster-api) into the **GOPATH**

> **Why clone into the GOPATH?** There have been historic issues with code generation tools when they are run outside the go path

2. Fork the [Cluster API Provider RKE2](https://github.com/rancher/cluster-api-provider-rke2) repo
3. Clone your new repo into the **GOPATH** (i.e. `~/go/src/github.com/yourname/cluster-api-provider-rke2`)
4. Ensure **Tilt** and **kind** are installed
5. Create a `tilt-settings.json` file in the root of your forked/cloned `cluster-api` directory.
6. Add the following contents to the file (replace "yourname" with your github account name):

```json
{
    "default_registry": "ghcr.io/yourname",
    "provider_repos": ["../../github.com/yourname/cluster-api-provider-rke2"],
    "enable_providers": ["docker", "rke2-bootstrap", "rke2-control-plane"],
    "kustomize_substitutions": {
        "EXP_MACHINE_POOL": "true",
        "EXP_CLUSTER_RESOURCE_SET": "true"
    },
    "extra_args": {
        "rke2-bootstrap": ["--v=4"],
        "rke2-control-plane": ["--v=4"],
        "core": ["--v=4"]
    },
    "debug": {
        "rke2-bootstrap": {
            "continue": true,
            "port": 30001
        },
        "rke2-control-plane": {
            "continue": true,
            "port": 30002
        }
    }
}
```

> NOTE: Until this [bug](https://github.com/kubernetes-sigs/cluster-api/pull/7482) merged in CAPI you will have to make the changes locally to your clone of CAPI.

7. Open another terminal (or pane) and go to the `cluster-api` directory.
8.  Run the following to create a configuration for kind:

```bash
cat > kind-cluster-with-extramounts.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: capi-test
nodes:
- role: control-plane
  extraMounts:
    - hostPath: /var/run/docker.sock
      containerPath: /var/run/docker.sock
EOF
```

> NOTE: if you are using Docker Desktop v4.13 or above then you will you will encounter issues from here. Until a permanent solution is found its recommended you use v4.12

9. Run the following command to create a local kind cluster:

```bash
kind create cluster --config kind-cluster-with-extramounts.yaml
```

10. Now start tilt by running the following:

```bash
tilt up
```

11. Press the **space** key to see the Tilt web ui and check that everything goes green.

## Known Issues

### When using CAPD  < v1.6.0 unmodified, Cluster creation is stuck after first node and API is not reachable

If you use `docker` as your infrastructure provider without any modification, Cluster creation will stall after provisioning the first node, and the API will not be available using the LB address. This is caused by Load Balancer configuration used in CAPD which is not compatible with RKE2. Therefore, it is necessary to use our own fork of `v1.3.3` by using a specific clusterctl configuration.

## Get in contact
You can get in contact with us via the [#capbr](https://rancher-users.slack.com/archives/C046X0CDKCH) channel on the [Rancher Users Slack](https://slack.rancher.io/).

