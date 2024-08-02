# CAPRKE2 Releases

## Release Cadence

- CAPRKE2 minor versions (v0.**2**.0 versus v0.**1**.0) are released every 3-4 months.
- CAPRKE2 patch versions (v0.2.**2** versus v0.2.**1**) are released as often as weekly or once a month. 

## Release Process

1. Clone the repository locally: 

```bash
git clone git@github.com:rancher/cluster-api-provider-rke2.git
```

2. Depending on whether you are cutting a minor or patch release, the process varies.

    * If you are cutting a new minor release:

        Create a new release branch (i.e release-X) and push it to the upstream repository.

        ```bash
            # Note: `upstream` must be the remote pointing to `github.com:rancher/cluster-api-provider-rke2`.
            git checkout -b release-0.2
            git push -u upstream release-0.2
            # Export the tag of the minor release to be cut, e.g.:
            export RELEASE_TAG=v0.2.0
        ```
    * If you are cutting a patch release from an existing release branch:

        Use existing release branch.

        ```bash
            git checkout upstream/release-0.2
            # Export the tag of the patch release to be cut, e.g.:
            export RELEASE_TAG=v0.2.1
        ```
3. Create a signed/annotated tag and push it:

```bash
# Create tags locally
git tag -s -a ${RELEASE_TAG} -m ${RELEASE_TAG}

# Push tags
git push upstream ${RELEASE_TAG}
```

This will trigger a [release GitHub action](https://github.com/rancher/cluster-api-provider-rke2/blob/main/.github/workflows/release.yml) that creates a release with RKE2 provider components.

## Versioning

Cluster API Provider RKE2 follows [semantic versioning](https://semver.org/) specification.

Example versions:
- Pre-release: `v0.2.0-alpha.1`
- Minor release: `v0.2.0`
- Patch release: `v0.2.1`
- Major release: `v2.0.0`

With the v0 release of our codebase, we provide the following guarantees:

- A (*minor*) release CAN include:
  - Introduction of new API versions, or new Kinds.
  - Compatible API changes like field additions, deprecation notices, etc.
  - Breaking API changes for deprecated APIs, fields, or code.
  - Features, promotion or removal of feature gates.
  - And more!

- A (*patch*) release SHOULD only include backwards compatible set of bugfixes.

### Backporting

Any backport MUST not be breaking for either API or behavioral changes.

It is generally not accepted to submit pull requests directly against release branches (release-X). However, backports of fixes or changes that have already been merged into the main branch may be accepted to all supported branches:

- Critical bugs fixes, security issue fixes, or fixes for bugs without easy workarounds.
- Dependency bumps for CVE (usually limited to CVE resolution; backports of non-CVE related version bumps are considered exceptions to be evaluated case by case)
- Cert-manager version bumps (to avoid having releases with cert-manager versions that are out of support, when possible)
- Changes required to support new Kubernetes versions, when possible. See supported Kubernetes versions for more details.
- Changes to use the latest Go patch version to build controller images.
- Improvements to existing docs (the latest supported branch hosts the current version of the book)

**Note:** We generally do not accept backports to Cluster API Provider RKE2 release branches that are out of support.

## Branches

Cluster API Provider RKE2 has two types of branches: the main branch and release-X branches.

The main branch is where development happens. All the latest and greatest code, including breaking changes, happens on main.

The release-X branches contain stable, backwards compatible code. On every major or minor release, a new branch is created. It is from these branches that minor and patch releases are tagged. In some cases, it may be necessary to open PRs for bugfixes directly against stable branches, but this should generally not be the case.

### Support and guarantees

Cluster API Provider RKE2 maintains the most recent release/releases for all supported APIs. Support for this section refers to the ability to backport and release patch versions; [backport policy](#backporting) is defined above.

- The API version is determined from the GroupVersion defined in the top-level `bootstrap/api/` and `controlplane/api/` packages.
- For the current stable API version (v1beta1) we support the two most recent minor releases; older minor releases are immediately unsupported when a new major/minor release is available.