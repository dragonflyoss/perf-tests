# image-bench

Benchmark concurrent image pulls through Dragonfly. Every selected peer node pulls the same image by containerd
at the same time, driven by a pod pinned to the node, and `dfbench` prints the pull latency read from the kubelet
`Pulled` events, the failure rate and how much of the traffic went back to source, came from remote peers or was
hit in the local cache.

## Prerequisites

- Dragonfly installed in the cluster, see the [root README](../../README.md).
- The containerd `certs.d` of the nodes routes the registry of the image through the dfdaemon proxy.
- The benchmark images pushed to that registry, see `make docker-push-image-bench-images`. They are built from
  [build/images/image-bench](../../build/images/image-bench) and hold nothing but random data layers:

  | Tag          | Size   | Layers       |
  |--------------|--------|--------------|
  | `v1-1gb-4`   | 1 GiB  | 256 MiB x 4  |
  | `v1-2gb-8`   | 2 GiB  | 256 MiB x 8  |
  | `v1-4gb-8`   | 4 GiB  | 512 MiB x 8  |
  | `v1-10gb-10` | 10 GiB | 1 GiB x 10   |
  | `v1-20gb-4`  | 20 GiB | 5 GiB x 4    |

## Run in Kubernetes

1. Store your kubeconfig in a secret, the Jobs create pods and read the dfdaemon metrics with `kubectl` and read
   it from the standard `KUBECONFIG` variable. Create it in the namespace the Jobs run in:

   ```shell
   kubectl create secret generic image-bench-kubeconfig -n dragonfly-system --from-file=config=$HOME/.kube/config
   ```

2. Pick the image and the number of peer nodes by editing `args` in `image-bench.yaml`:

   ```yaml
   - '--image=dragonflyoss/image-bench:v1-1gb-4' # any registry, e.g. ghcr.io/dragonflyoss/image-bench:v1-10gb-10
   - '--peers=0' # 0 means every peer node
   ```

3. Start the Job in the namespace where the secret is:

   ```shell
   kubectl apply -n dragonfly-system -f image-bench.yaml
   ```

4. Follow the logs, the report is printed when the run ends:

   ```shell
   kubectl logs -n dragonfly-system -f job/image-bench
   ```

   The Job is marked `Failed` when more than 1% of the pulls failed.

5. Clear the nodes and the peer caches before the next run, so it starts cold again. Set the same `--image` in
   `image-bench-cleanup.yaml`. The cleanup Job deletes the pull pods, removes the image from containerd on
   every peer node with a privileged pod running `crictl`, disables `storage.keep` in the dfdaemon config of the
   peers and seed peers and restarts them, it finishes when both rollouts are done:

   ```shell
   kubectl apply -n dragonfly-system -f image-bench-cleanup.yaml
   kubectl logs -n dragonfly-system -f job/image-bench-cleanup
   ```

6. Delete the Jobs before the next run, their names are fixed:

   ```shell
   kubectl delete -n dragonfly-system job/image-bench job/image-bench-cleanup
   ```

## Run locally

Install `dfbench`, it uses the current `kubectl` context:

```shell
go install github.com/dragonflyoss/perf-tests/cmd/dfbench@latest
dfbench image-bench --namespace dragonfly-system --image ghcr.io/dragonflyoss/image-bench:v1-1gb-4 \
  --peers 10 --timeout 2h
dfbench image-bench cleanup --namespace dragonfly-system --image ghcr.io/dragonflyoss/image-bench:v1-1gb-4
```

## Knobs

All knobs are flags, set them in the Job `args` or on the command line.

| Flag                  | Default                             | Description                                        |
|-----------------------|-------------------------------------|----------------------------------------------------|
| `--namespace`         | `dragonfly-system`                  | Namespace of the peers, the pods run in it too.    |
| `--image`             | `dragonflyoss/image-bench:v1-1gb-4` | Image to pull, any registry.                       |
| `--peers`             | `0`                                 | Peer pods to pull on, sorted by name, `0` is all.  |
| `--cleanup-image`     | `dragonflyoss/image-bench:latest`   | cleanup: image of the cleanup pods, has `crictl`.  |
| `--containerd-socket` | `/run/containerd/containerd.sock`   | cleanup: containerd socket of the nodes.           |
| `--timeout`           | `30m`                               | Timeout of the whole run, raise it for big images. |
| `--kubeconfig`        | none                                | Kubeconfig to use, defaults to `$KUBECONFIG`.      |
| `--log-level`         | `info`                              | `debug` prints every `kubectl` command.            |

## How it works

`dfbench image-bench` lists the peer pods, takes the node of each one and creates a pull pod per node in one
`kubectl create`, pinned with `nodeName`, `imagePullPolicy: Always` and `restartPolicy: Never`. The kubelet
pulls the image by containerd, which goes through the dfdaemon proxy on the node. The pull cost is the duration
in the `Pulled` event of the pod, `Successfully pulled image "..." in 12.3s`. A pod whose container waits with
`ErrImagePull`, `ImagePullBackOff` or `InvalidImageName` counts as failed. The images hold no executable, so the
container is expected to fail to start once the image is pulled, only the pull is measured. The pull pods are
deleted when the run ends.

## Reading the report

```text
image-bench

  Run             10 peers by containerd, 18.4s
  Target          ghcr.io/dragonflyoss/image-bench:v1-1gb-4

  Pulls           10 total, 10 succeeded
  Failed          0 of 10 (0.00%)

  Latency (ms)          min      avg      med    p(90)    p(95)    p(99)      max
    pull           12102.00 13231.00 13188.00 15011.00 15320.00 16480.00 16480.00

  Traffic         10.0 GiB total, 1.0 GiB back-to-source, 9.0 GiB remote peer, 0 B local peer
  Back to source  10.00%

  Result          PASSED, ✓ pull failed rate<0.01
```

| Row              | Meaning                                                                                    |
|------------------|--------------------------------------------------------------------------------------------|
| `Run`            | Number of peer nodes and wall-clock time of the run.                                       |
| `Target`         | Image reference pulled by every node, the same tag is pulled again on every run.           |
| `Pulls`          | Pull pods created and how many got a `Pulled` event.                                       |
| `Failed`         | Pods whose image pull failed, their cost is left out of the latency row.                   |
| `pull`           | Pull duration reported by the kubelet on each node, `kubectl` polling overhead excluded.   |
| `Traffic`        | Bytes the peers and seed peers downloaded during the run, read from dfdaemon metrics.      |
| `Back to source` | Share of the traffic fetched from the registry, the rest came from the P2P network.        |
| `Result`         | `FAILED` and a non-zero exit code when more than 1% of the pulls failed.                   |

## Build the image

```shell
make docker-build-image-bench
```

Tag and push it, then update `image` in `image-bench.yaml` and `image-bench-cleanup.yaml`.
