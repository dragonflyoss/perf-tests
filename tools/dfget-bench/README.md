# dfget-bench

Benchmark one dfdaemon under concurrent `dfget` load. `dfbench` picks one peer pod and runs `--concurrency`
`dfget` in it at the same time, one `kubectl exec` each, all downloading a file of the file server through the
local dfdaemon, then prints the download latency, the failure rate, the traffic of the dfdaemon and its throughput.

Unlike [file-bench](../file-bench/README.md), which runs one `dfget` on every peer, this benchmark runs many
`dfget` on a single peer, so it measures how a single dfdaemon holds up rather than the P2P network.

## Prerequisites

- Dragonfly installed in the cluster, see the [root README](../../README.md).
- The test file server: `kubectl apply -f ../file-server/file-server.yaml`.

## Run in Kubernetes

1. Store your kubeconfig in a secret, the Job drives the peer with `kubectl exec` and reads it from the standard
   `KUBECONFIG` variable. Create it in the namespace the Job runs in:

   ```shell
   kubectl create secret generic dfget-bench-kubeconfig -n dragonfly-system --from-file=config=$HOME/.kube/config
   ```

2. Pick the file, the concurrency and the mode by editing `args` in `dfget-bench.yaml`:

   ```yaml
   - '--file=1g' # 1b | 1k | 1m | 4m | 10m | 1g | 2g | 4g | 10g | 20g | 30g
   - '--concurrency=10'
   - '--mode=repeat' # repeat | random | fixed
   ```

3. Start the Job in the namespace where the secret is:

   ```shell
   kubectl apply -n dragonfly-system -f dfget-bench.yaml
   ```

4. Follow the logs, the report is printed when the run ends:

   ```shell
   kubectl logs -n dragonfly-system -f job/dfget-bench
   ```

   The Job is marked `Failed` when more than 1% of the `dfget` failed.

5. Clear the peer caches before the next run, so it starts cold again. The cleanup Job disables `storage.keep`
   in the dfdaemon config of the peers and seed peers and restarts them, it finishes when both rollouts are
   done:

   ```shell
   kubectl apply -n dragonfly-system -f dfget-bench-cleanup.yaml
   kubectl logs -n dragonfly-system -f job/dfget-bench-cleanup
   ```

6. Delete the Jobs before the next run, their names are fixed:

   ```shell
   kubectl delete -n dragonfly-system job/dfget-bench job/dfget-bench-cleanup
   ```

In the `repeat` and `random` modes every run downloads fresh Dragonfly tasks, the `uuid` in the URL changes, so
the cleanup is optional. The `fixed` mode downloads the same tasks on every run, clean up when a run should start
cold, and skip it between the two runs of a P2P measurement, see [Modes](#modes).

## Run locally

Install `dfbench`, it uses the current `kubectl` context:

```shell
go install github.com/dragonflyoss/perf-tests/cmd/dfbench@latest
dfbench dfget-bench --namespace dragonfly-system --file 1g --concurrency 50 --mode random --timeout 2h
dfbench dfget-bench cleanup --namespace dragonfly-system
```

## Modes

| Mode     | What every `dfget` does                                                                   |
|----------|-------------------------------------------------------------------------------------------|
| `repeat` | Same URL, one Dragonfly task shared by all `dfget`, one back-to-source download.          |
| `random` | Own `uuid` in the URL, one task per `dfget`, every one of them goes back-to-source.       |
| `fixed`  | `r=<i>` in the URL, no `uuid`, the `i`-th `dfget` downloads the same task on every run.   |

The `fixed` mode measures P2P between two peers: run it on one peer, its `dfget` go back-to-source, then run it
again with `--pod` on another peer, its `dfget` download the same `r=0` to `r=<concurrency-1>` tasks from the
first peer, and the report shows the traffic as remote peer instead of back-to-source.

## Knobs

All knobs are flags, set them in the Job `args` or on the command line.
`dfbench` runs `kubectl exec` over SPDY, the WebSocket client of kubectl before 1.34 fails some concurrent
execs with `Unknown stream id 1, discarding message`. Set `KUBECTL_REMOTE_COMMAND_WEBSOCKETS=true` to stream
over WebSockets anyway.

| Flag                    | Default                 | Description                                                     |
|-------------------------|-------------------------|-----------------------------------------------------------------|
| `--namespace`           | `dragonfly-system`      | Namespace of the peers and the file server.                     |
| `--peer-label`          | `component=client`      | Label selector of the peer pods, the first pod by name is used. |
| `--seed-peer-label`     | `component=seed-client` | Label selector of the seed peer pods, for the cleanup.          |
| `--peer-container`      | `client`                | Name of the dfdaemon container in the peer pod.                 |
| `--pod`                 | none                    | Peer pod to download on, overrides `--peer-label`.              |
| `--concurrency`         | `10`                    | Number of `dfget` started at once in the pod.                   |
| `--mode`                | `repeat`                | `repeat`, `random` or `fixed`, see above.                       |
| `--file`                | `1g`                    | File server path to download, see the file server image.        |
| `--file-server`         | none                    | File server base URL, defaults to the one in `--namespace`.     |
| `--output-dir`          | `/tmp`                  | Directory in the peer pod to download to, see below.            |
| `--metrics-port`        | `4002`                  | Metrics port of the dfdaemon to read the traffic.               |
| `--peer-configmap`      | none                    | cleanup: dfdaemon configmap of the peers, else found by label.  |
| `--seed-peer-configmap` | none                    | cleanup: dfdaemon configmap of the seed peers, else by label.   |
| `--timeout`             | `30m`                   | Timeout of the whole run, raise it for large files.             |
| `--kubeconfig`          | none                    | Kubeconfig to use, defaults to `$KUBECONFIG` like kubectl.      |
| `--log-level`           | `info`                  | `debug` prints every `kubectl` command.                         |

`dfget` hard links the downloaded file to `--output-dir` when the directory is on the same filesystem as the
dfdaemon storage, `/var/lib/dragonfly` by default, and copies it otherwise. With many concurrent `dfget` the
copies compete for the disk, so point `--output-dir` at the storage filesystem, e.g. `/var/lib/dragonfly/dfbench`,
unless the copy is what you want to measure. The directory is created if missing. Mind the disk in the `random`
mode: every `dfget` stores its own copy of the file in the dfdaemon cache.

## Reading the report

```text
dfget-bench

  Run             1g by 50 dfget on client-0, random, 41.3s
  Target          http://file-server.dragonfly-system.svc/1g?tag=dfget&uuid=8d2b...

  Downloads       50 total, 50 succeeded
  Failed          0 of 50 (0.00%)

  Latency (ms)          min      avg      med    p(90)    p(95)    p(99)      max
    download       12041.10 31207.55 32011.42 39320.01 40012.74 41100.30 41100.30

  Traffic         50.0 GiB total, 50.0 GiB back-to-source, 0 B remote peer, 0 B local peer
  Back to source  100.00%
  Throughput      10.40 Gbps downloaded by the dfdaemon
  Metrics         1 of 1 peer read

  Result          PASSED, ✓ download failed rate<0.01
```

| Row              | Meaning                                                                                   |
|------------------|-------------------------------------------------------------------------------------------|
| `Run`            | File, number of `dfget`, pod, mode and wall-clock time of the run.                        |
| `Target`         | URL of the first `dfget`, the others differ by `uuid` in `random` and by `r` in `fixed`.  |
| `Downloads`      | `dfget` started and how many exited successfully.                                         |
| `Failed`         | `dfget` that exited with an error, their cost is left out of the latency row.             |
| `download`       | Time of each `dfget`, measured in the pod, `kubectl exec` overhead excluded.              |
| `Traffic`        | Bytes the dfdaemon downloaded during the run, read from its metrics.                      |
| `Back to source` | Share of the traffic fetched from the file server, the rest came from the P2P network.    |
| `Throughput`     | Traffic over the wall-clock time of the run.                                              |
| `Metrics`        | Whether the dfdaemon metrics were read before and after the run, else the traffic is 0.   |
| `Result`         | `FAILED` and a non-zero exit code when more than 1% of the `dfget` failed.                |

## Build the image

```shell
make docker-build-dfget-bench
```

Tag and push it, then update `image` in `dfget-bench.yaml` and `dfget-bench-cleanup.yaml`.
