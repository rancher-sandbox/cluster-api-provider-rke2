<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Deprecate kubebuilder defaults](#deprecate-kubebuilder-defaults)
  - [Context](#context)
  - [Decision](#decision)
  - [Consequences](#consequences)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Deprecate kubebuilder defaults
<!-- A short and clear title which is prefixed with the ADR number -->

<!-- TODO: remove the following disable link checker comment before committing your ADR -->
<!-- markdown-link-check-disable-next-line -->
- Status: accepted
- Date: 2024-05-21
- Authors: @alexander-demicev
- Deciders: @Danil-Grigorev @furkatgofurov7 @salasberryfin @mjura @yiannistri

## Context
<!-- What is the context of the decision and what's the motivation -->

Enforcing default values for fields in CRD definitions can cause problem with the GitOps approach. It can lead to a situation where defined resources in Git repository differs from the actual state of applied resources in the cluster. This can lead to unexpected behavior like state drift, unless is manually handled.

## Decision
<!-- What is the decision that has been made -->

Kubebuilder defaulting annotations were [deprecated in CAPRKE2 API](https://github.com/rancher/cluster-api-provider-rke2/commit/86025754c0993e6e0d549110cc7f38687ac420e3). As a result of this deprecation
CRD definitions don't include default values for some fields.

## Consequences
<!-- Whats the result or impact of this decision. Does anything need to change and are new GitHub issues created as a result -->

Users have to specify the following fields manually when creating RKE2ControlPlane resources:

- `spec.rolloutStrategy`:

```yaml
  rolloutStrategy:
    type: "RollingUpdate"
    rollingUpdate:
      maxSurge: 1 # can be 0
```

- `spec.registrationMethod`:

```yaml
  registrationMethod:
    type: "control-plane-endpoint" # other supported values are "internal-only-ips", "external-only-ips", "address", "internal-first"
```
`"control-plane-endpoint"` is the recommended value for `registrationMethod` field as other can cause issue with scaling the cluster and should be used with caution.