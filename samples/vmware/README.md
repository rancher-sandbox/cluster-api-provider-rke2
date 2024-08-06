# Example manifests

This config includes a kubevip loadbalancer on the controlplane nodes. The VIP of the loadbalancer for the Kubernetes API is set by the CONTROL_PLANE_ENDPOINT_IP.

Prerequisites:

- VM template to be used for the cluster machine should be present in the vSphere environment.
- If airgapped environment is required then the VM template should already include RKE2 binaries as described in the [docs](https://docs.rke2.io/install/airgap#tarball-method). CAPRKE2 is using the tarball method to install RKE2 on the machines.
Any additional images like vSphere CPI image should be present in the local environment too.

To initialize Cluster API Provider vSphere, clusterctl requires the following variables, which should be set in ~/.cluster-api/clusterctl.yaml as the following:

```bash
## -- Controller settings -- ##
VSPHERE_USERNAME: "<username>"                                # The username used to access the remote vSphere endpoint
VSPHERE_PASSWORD: "<password>"                                # The password used to access the remote vSphere endpoint

## -- Required workload cluster default settings -- ##
VSPHERE_SERVER: "10.0.0.1"                                    # The vCenter server IP or FQDN
VSPHERE_DATACENTER: "SDDC-Datacenter"                         # The vSphere datacenter to deploy the management cluster on
VSPHERE_DATASTORE: "DefaultDatastore"                         # The vSphere datastore to deploy the management cluster on
VSPHERE_NETWORK: "VM Network"                                 # The VM network to deploy the management cluster on
VSPHERE_RESOURCE_POOL: "*/Resources"                          # The vSphere resource pool for your VMs
VSPHERE_FOLDER: "vm"                                          # The VM folder for your VMs. Set to "" to use the root vSphere folder
VSPHERE_TEMPLATE: "ubuntu-1804-kube-v1.17.3"                  # The VM template to use for your management cluster.
CONTROL_PLANE_ENDPOINT_IP: "192.168.9.230"                    # the IP that kube-vip is going to use as a control plane endpoint
VSPHERE_TLS_THUMBPRINT: "..."                                 # sha256 thumbprint of the vcenter certificate: openssl x509 -sha256 -fingerprint -in ca.crt -noout
EXP_CLUSTER_RESOURCE_SET: "true"                              # This enables the ClusterResourceSet feature that we are using to deploy CSI
VSPHERE_SSH_AUTHORIZED_KEY: "ssh-rsa AAAAB3N..."              # The public ssh authorized key on all machines in this cluster.
                                                              #  Set to "" if you don't want to enable SSH, or are using another solution.
"CPI_IMAGE_K8S_VERSION": "v1.30.0"                            # The version of the vSphere CPI image to be used by the CPI workloads
                                                              #  Keep this close to the minimum Kubernetes version of the cluster being created.
```

Then run the following command to generate the RKE2 cluster manifests:

```bash
clusterctl generate cluster --from https://github.com/rancher/cluster-api-provider-rke2/blob/main/samples/vmware/cluster-template.yaml -n example-vsphere rke2-vsphere > vsphere-rke2-clusterctl.yaml
```

```bash
kubectl apply -f vsphere-rke2-clusterctl.yaml
```
