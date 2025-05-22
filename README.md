# Performance testing

This repository includes performance test tools of dragonfly.

## Usage

### Installation

```shell
go install github.com/dragonflyoss/perf-tests/cmd/dfbench@latest
```

### Install Dragonfly for testing

Install Dragonfly using Helm chart, refer to [Dragonfly Helm Chart](https://d7y.io/docs/next/getting-started/installation/helm-charts/).

<!-- markdownlint-disable -->

```shell
$ helm repo add dragonfly https://dragonflyoss.github.io/helm-charts/
$ helm install --wait --create-namespace --namespace dragonfly-system dragonfly dragonfly/dragonfly
NAME: dragonfly
LAST DEPLOYED: Thu Apr 18 19:26:39 2024
NAMESPACE: dragonfly-system
STATUS: deployed
REVISION: 1
TEST SUITE: None
NOTES:
1. Get the scheduler address by running these commands:
  export SCHEDULER_POD_NAME=$(kubectl get pods --namespace dragonfly-system -l "app=dragonfly,release=dragonfly,component=scheduler" -o jsonpath={.items[0].metadata.name})
  export SCHEDULER_CONTAINER_PORT=$(kubectl get pod --namespace dragonfly-system $SCHEDULER_POD_NAME -o jsonpath="{.spec.containers[0].ports[0].containerPort}")
  kubectl --namespace dragonfly-system port-forward $SCHEDULER_POD_NAME 8002:$SCHEDULER_CONTAINER_PORT
  echo "Visit http://127.0.0.1:8002 to use your scheduler"

2. Get the dfdaemon port by running these commands:
  export DFDAEMON_POD_NAME=$(kubectl get pods --namespace dragonfly-system -l "app=dragonfly,release=dragonfly,component=dfdaemon" -o jsonpath={.items[0].metadata.name})
  export DFDAEMON_CONTAINER_PORT=$(kubectl get pod --namespace dragonfly-system $DFDAEMON_POD_NAME -o jsonpath="{.spec.containers[0].ports[0].containerPort}")
  You can use $DFDAEMON_CONTAINER_PORT as a proxy port in Node.

3. Configure runtime to use dragonfly:
  https://d7y.io/docs/getting-started/quick-start/kubernetes/
```

<!-- markdownlint-restore -->

Install file server for testing.

```shell
kubectl apply -f https://raw.githubusercontent.com/dragonflyoss/perf-tests/main/tools/file-server/file-server.yaml
```

### Run performance testing

```text
$ dfbench dragonfly
Running benchmark for all size levels by DFGET ...
+-----------------+-------+-------------+-------------+-------------+
| FILE SIZE LEVEL | TIMES |  MIN COST   |  MAX COST   |  AVG COST   |
+-----------------+-------+-------------+-------------+-------------+
| Nano(1B)        | 3     | 528.46ms    | 717.28ms    | 648.17ms    |
+-----------------+-------+-------------+-------------+-------------+
| Micro(1KB)      | 3     | 691.76ms    | 967.26ms    | 798.55ms    |
+-----------------+-------+-------------+-------------+-------------+
| Small(1MB)      | 3     | 671.12ms    | 1250.22ms   | 897.32ms    |
+-----------------+-------+-------------+-------------+-------------+
| Medium(10MB)    | 3     | 716.83ms    | 971.49ms    | 816.14ms    |
+-----------------+-------+-------------+-------------+-------------+
| Large(1GB)      | 3     | 4855.28ms   | 6069.76ms   | 5526.63ms   |
+-----------------+-------+-------------+-------------+-------------+
| XLarge(10GB)    | 3     | 37428.71ms  | 41206.96ms  | 38794.96ms  |
+-----------------+-------+-------------+-------------+-------------+
| XXLarge(30GB)   | 3     | 102279.19ms | 139299.07ms | 118039.78ms |
+-----------------+-------+-------------+-------------+-------------+
```

## Community

Join the conversation and help the community.

- **Slack Channel**: [#dragonfly](https://cloud-native.slack.com/messages/dragonfly/) on [CNCF Slack](https://slack.cncf.io/)
- **Github Discussions**: [Dragonfly Discussion Forum](https://github.com/dragonflyoss/dragonfly/discussions)
- **Developer Group**: <dragonfly-developers@googlegroups.com>
- **Maintainer Group**: <dragonfly-maintainers@googlegroups.com>
- **Github Discussions**: [Dragonfly Discussion Forum](https://github.com/dragonflyoss/dragonfly/discussions)
- **Twitter**: [@dragonfly_oss](https://twitter.com/dragonfly_oss)

## Contributing

You should check out our
[CONTRIBUTING](https://github.com/dragonflyoss/dragonfly/blob/main/CONTRIBUTING.md) and develop the project together.

## Code of Conduct

Please refer to our [Code of Conduct](https://github.com/dragonflyoss/dragonfly/blob/main/CODE_OF_CONDUCT.md).
