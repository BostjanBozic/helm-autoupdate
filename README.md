# helm-autoupdate

CLI/action to update helm versions in git repositories

# Motivation

You start with a helm release object
```yaml
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: aws-vpc-cni
spec:
  chart:
    spec:
      chart: aws-vpc-cni
      sourceRef:
        kind: HelmRepository
        name: aws
      version: 0.0.1
  interval: 1m0s
  timeout: 10m
  values:
    replicaCount: 1
```

This is fine, but how do you know when to update the helm release to a newer version?  One option is to use `*` like this
```yaml
      sourceRef:
        kind: HelmRepository
        name: aws
      version: "*"
```

But in this case, you don't have any git tracking of what version was released.  What you really want is some automation
that will bump the `version` field when a new helm chart is released.  This is what `helm-autoupdate` is for.

# Usage

First, add a file named `.helm-autoupdate.yaml` in the root of your repository.  Add a `chart` item for each chart you want to update.
The field `filename_regex` is an optional list of filename patterns to scan. If omitted, all files are considered.

```yaml
charts:
- chart:
    name: aws-vpc-cni
    repository: https://aws.github.io/eks-charts
    version: "*"
  identity: aws-vpc-cni
filename_regex:
- .*\.yaml
```

Next, add the YAML comment `# helm:autoupdate:<IDENTITY>` to the `version` line you want tracked, where `<IDENTITY>` matches
the `charts[].identity` value. Identity values may only contain letters, digits, and hyphens (`[a-zA-Z0-9-]`).

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: aws-vpc-cni
spec:
  chart:
    spec:
      chart: aws-vpc-cni
      sourceRef:
        kind: HelmRepository
        name: aws
      version: 0.0.1 # helm:autoupdate:aws-vpc-cni
  interval: 1m0s
  timeout: 10m
  values:
    replicaCount: 1
```

When `helm-autoupdate` runs, it replaces `0.0.1` with the latest version that satisfies the `version` constraint defined in
`.helm-autoupdate.yaml`. Use `"*"` for the absolute latest, or a semver constraint such as `"1.*"` to stay within a major version.

To trigger a run locally, build and execute the binary from the root of your repository:

```bash
cd /tmp
git clone git@github.com:bostjanbozic/helm-autoupdate.git
go build ./cmd/helm-autoupdate
cd -
/tmp/helm-autoupdate
```

For automated updates, see [Example GitHub Actions workflows](#example-github-actions-workflows) below.

You can combine this with GitHub's auto-merge feature and status checks to complete the auto merge.

# Configuration reference

| Field | Required | Description |
|---|---|---|
| `charts[].identity` | yes | Unique name referenced in `# helm:autoupdate:<IDENTITY>` comments. Allowed characters: `[a-zA-Z0-9-]`. |
| `charts[].chart.name` | yes | Helm chart name. |
| `charts[].chart.repository` | yes | Repository URL. Supports `https://`, `oci://`, and `s3://` schemes. |
| `charts[].chart.version` | yes | Semver constraint for the target version. Use `"*"` for latest. |
| `charts[].chart.s3_region` | no | AWS region for S3 repositories. Falls back to the default credential chain if omitted. |
| `charts[].chart.cooldown_days` | no | Fall back to the most recent version outside the cooldown window instead of updating to a version published fewer than N days ago. Never downgrades below the current version. See [Cooldown period](#cooldown-period). |
| `filename_regex` | no | List of regex patterns limiting which files are scanned. All files are scanned if omitted. |

# Cooldown period

The optional `cooldown_days` field per chart prevents updating to a version published less than N days ago — similar to `dependabot cooldown.default-days`. This lets a new release stabilise before it is automatically applied.

```yaml
charts:
- chart:
    name: aws-vpc-cni
    repository: https://aws.github.io/eks-charts
    version: "*"
    cooldown_days: 7
  identity: aws-vpc-cni
```

When a cooldown is active, `helm-autoupdate` does not skip the update entirely. It iterates available versions newest-first and picks the most recent one that is both outside the cooldown window and satisfies the `version` constraint. For example, with `cooldown_days: 7` and versions `1.2.0` (2 days old) and `1.1.0` (10 days old), the tool updates to `1.1.0`. The tool never downgrades: if all versions outside the cooldown window are older than the currently pinned version, the update is skipped.

The publish date is read from the `created` field in the Helm repository's `index.yaml`. OCI tag listings carry no publish date metadata, so `cooldown_days` has no effect on OCI repositories.

# Supported helm backends

This project comes with support for HTTPS, OCI, and [S3](./internal/helm/s3.go) backends.

## OCI

OCI repositories use the `oci://` scheme. No additional configuration is required for public registries.
For private registries, authenticate with `docker login` or equivalent before running `helm-autoupdate`.

Example `.helm-autoupdate.yaml` entry:
```yaml
charts:
- chart:
    name: grafana
    repository: oci://ghcr.io/grafana/helm-charts
    version: "*"
  identity: grafana
```

## S3

S3 repositories require AWS credentials. The AWS region is configured per chart via the optional `s3_region` field. If omitted, the region is resolved from the default AWS credential chain (e.g. `~/.aws/config` or `AWS_REGION` env var).

Example `.helm-autoupdate.yaml` entry:
```yaml
charts:
- chart:
    name: my-chart
    repository: s3://my-bucket/helm
    s3_region: us-west-2
    version: "*"
  identity: my-chart
```

In GitHub Actions, configure AWS credentials before running the action:
```yaml
- name: Configure AWS Credentials
  uses: aws-actions/configure-aws-credentials@v4
  with:
    aws-region: us-west-2
- name: update helm
  uses: bostjanbozic/helm-autoupdate@v2
```

# Example GitHub Actions workflows

```yaml
---
name: Helm Chart auto-update

on:
  schedule:
    - cron: "0 7 * * 1"
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write


jobs:
  update-helm-charts:
    name: Helm Chart auto-update
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4
      - name: Update Helm Charts
        uses: bostjanbozic/helm-autoupdate@v2
      - name: Create Pull Request
        uses: peter-evans/create-pull-request@v8
        with:
          title: Auto-update Helm Charts
          branch: chore/helm-chart-autoupdate
          commit-message: "chore(helm): Auto-update Helm Chart versions"
          body: |
            Update Helm chart versions to latest version
          labels: dependencies
```
