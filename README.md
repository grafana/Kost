# Kost

Kost is a tool built at Grafana Labs to estimate the cost of workloads running in our k8s clusters.

> [!CAUTION]
> This is still highly experimental and somewhat tightly coupled to how Grafana Labs manages and monitors our k8s infrastructure.
> We'd love to support other teams and organizations, but do not have the bandwidth to implement it.
> If you want to adopt the tool, please fill out an issue and connect with us!


## Requirements

`kost` is somewhat tightly coupled to how Grafana Labs manages our k8s environments.
Specifically, the following assumptions need to be met:
- K8s resources defined in a standalone repository
  - We use [jsonnet](https://jsonnet.org/) to define resources + [tanka](https://tanka.dev/) to generate the k8s manifest files and commit them to a `kube-manifest` repo + [flux](https://fluxcd.io/) to deploy them to clusters
- Mimir to store [opencost](https://github.com/opencost/opencost) + [cloudcost-exporter](https://github.com/grafana/cloudcost-exporter) metrics for cost data
- GitHub Actions to detect changes and run the cost report

While these are what we use internally and require, in theory the bot should work so long as you have:
1. Two manifest files that you can compare changes
2. Prometheus compliant backend with cost metrics
3. A CI system to run the bot when changes happen


## Prerequisites for local development

- HTTP access to a prometheus server that has OpenCost metrics available
- Clone [grafana/kube-manifests](github.com/grafana/kube-manifests)
- Get a token for Mimir with Viewer privileges
   - dev: https://grafana-dev.com/orgs/raintank/api-keys
   - ops: https://grafana-ops.com/orgs/raintank/api-keys
- Create a yaml file with basic auth creds to use. Replace password with the token you created earlier.

```yaml
basic_auth:
  username: <user>
  password: <password>
```

## Running

There are two entrypoints that you can run:
- estimator
- bot

Estimator is a simple cli that accepts two manifest files and a set of clusters to generate the cost estimator for.
Bot is what is ran in GitHub Actions today and requires the `kube-manifest` repository to be available locally.

## Estimator

To check the cost on a single cluster, run the following command:
```shell
go run ./cmd/estimator/ \
  -from $PWD/pkg/costmodel/testdata/resource/Deployment.json \
  -to $PWD/pkg/costmodel/testdata/resource/Deployment-more-requests.json \
  -http.config.file /tmp/dev.yaml \
  -prometheus.address $PROMETHEUS_ADDRESS \
  <cluster>
```

To check the cost across multiple clusters, run the following command:
```shell
go run ./cmd/estimator/ \
  -from $PWD/pkg/costmodel/testdata/resource/Deployment.json \
  -to $PWD/pkg/costmodel/testdata/resource/Deployment-more-requests.json \
  -http.config.file /tmp/dev.yaml \
  -prometheus.address $PROMETHEUS_ADDRESS \
  <cluster-1> <cluster-2>
```

## Kost(bot)

Set the following environment variables:

- `KUBE_MANIFESTS_PATH`: path to `grafana/kube-manifests`
- `HTTP_CONFIG_FILE`: path to configuration created in [Prereqs](#prerequisites)
- `PROMETHEUS_ADDRESS`: mimir endpoint
- `GITHUB_PULL_REQUEST`: GitHub PR to create comment on
- `GITHUB_EVENT_NAME`: set to `pull_request`
- `GITHUB_TOKEN`: set to a token that is able to comment on PRs
- `CI`: set to `true`

```
go run ./cmd/bot/
```
## Debugging

- checkout the change in `kube-manifests` that you want to generate a cost estimate report for.
This also includes to set `master` to the right hash.
