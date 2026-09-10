# file-bench

Benchmark concurrent downloads through Dragonfly. Every selected peer runs `dfget` for the same file at the
same time, and `dfbench` prints the download latency, the failure rate and how much of the traffic went back to
source, came from remote peers or was hit in the local cache.

## Prerequisites

- Dragonfly installed in the cluster, see the [root README](../../README.md).
- The test file server: `kubectl apply -f ../file-server/file-server.yaml`.

## Run in Kubernetes

1. Store your kubeconfig in a secret, the Jobs drive the peers with `kubectl exec` and read it from the standard
   `KUBECONFIG` variable. Create it in the namespace the Jobs run in:

   ```shell
   kubectl create secret generic file-bench-kubeconfig -n dragonfly-system --from-file=config=$HOME/.kube/config
   ```

2. Pick the file and the number of peers by editing `args` in `file-bench.yaml`:

   ```yaml
   - '--file=1g' # 1b | 1k | 1m | 4m | 10m | 1g | 2g | 4g | 10g | 20g | 30g
   - '--peers=0' # 0 means every peer pod
   ```

3. Start the Job in the namespace where the secret is:

   ```shell
   kubectl apply -n dragonfly-system -f file-bench.yaml
   ```

4. Follow the logs, the report is printed when the run ends:

   ```shell
   kubectl logs -n dragonfly-system -f job/file-bench
   ```

   The Job is marked `Failed` when more than 1% of the downloads failed.

5. Clear the peer caches before the next run, so it starts cold again. The cleanup Job disables `storage.keep`
   in the dfdaemon config of the peers and seed peers and restarts them, it finishes when both rollouts are
   done:

   ```shell
   kubectl apply -n dragonfly-system -f file-bench-cleanup.yaml
   kubectl logs -n dragonfly-system -f job/file-bench-cleanup
   ```

6. Delete the Jobs before the next run, their names are fixed:

   ```shell
   kubectl delete -n dragonfly-system job/file-bench job/file-bench-cleanup
   ```

## Run locally

Install `dfbench`, it uses the current `kubectl` context:

```shell
go install github.com/dragonflyoss/perf-tests/cmd/dfbench@latest
dfbench file-bench --namespace dragonfly-system --file 1g --peers 10 --timeout 2h
dfbench file-bench cleanup --namespace dragonfly-system
```

## Knobs

All knobs are flags, set them in the Job `args` or on the command line.
`dfbench` runs `kubectl exec` over SPDY, the WebSocket client of kubectl before 1.34 fails some concurrent
execs with `Unknown stream id 1, discarding message`. Set `KUBECTL_REMOTE_COMMAND_WEBSOCKETS=true` to stream
over WebSockets anyway.

| Flag                    | Default                 | Description                                                    |
|-------------------------|-------------------------|----------------------------------------------------------------|
| `--namespace`           | `dragonfly-system`      | Namespace of the peers and the file server.                    |
| `--peer-label`          | `component=client`      | Label selector of the peer pods, `kubectl -l` syntax.          |
| `--seed-peer-label`     | `component=seed-client` | Label selector of the seed peer pods.                          |
| `--peer-container`      | `client`                | Name of the dfdaemon container in the peer pods.               |
| `--seed-peer-container` | `seed-client`           | Name of the dfdaemon container in the seed peer pods.          |
| `--file`                | `1g`                    | File server path to download, see the file server image.       |
| `--file-server`         | none                    | File server base URL, defaults to the one in `--namespace`.    |
| `--output-dir`          | `/tmp`                  | Directory in the peer pods to download to, see below.          |
| `--peers`               | `0`                     | Number of peers to download on, sorted by name, `0` is all.    |
| `--metrics-port`        | `4002`                  | Metrics port of the dfdaemon to read the traffic.              |
| `--peer-configmap`      | none                    | cleanup: dfdaemon configmap of the peers, else found by label. |
| `--seed-peer-configmap` | none                    | cleanup: dfdaemon configmap of the seed peers, else by label.  |
| `--timeout`             | `30m`                   | Timeout of the whole run, raise it for large files.            |
| `--kubeconfig`          | none                    | Kubeconfig to use, defaults to `$KUBECONFIG` like kubectl.     |
| `--log-level`           | `info`                  | `debug` prints every `kubectl` command.                        |

`dfget` hard links the downloaded file to `--output-dir` when the directory is on the same filesystem as the
dfdaemon storage, `/var/lib/dragonfly` by default, and copies it otherwise. The copy of a large file takes a
while and counts in the download latency, so point `--output-dir` at the storage filesystem, e.g.
`/var/lib/dragonfly/dfbench`, unless the copy is what you want to measure. The directory is created if missing.

## Reading the report

```text
file-bench

  Run             1g on 10 peers by dfget, 14.2s
  Target          http://file-server.dragonfly-system.svc/1g?tag=dfget&uuid=8d2b...

  Downloads       10 total, 10 succeeded
  Failed          0 of 10 (0.00%)

  Latency (ms)          min      avg      med    p(90)    p(95)    p(99)      max
    download        5102.44  5231.18  5188.90  6011.72  6320.15  6480.31  6480.31

  Traffic         10.0 GiB total, 1.0 GiB back-to-source, 9.0 GiB remote peer, 0 B local peer
  Back to source  10.00%
  Metrics         13 of 13 peers and seed peers read

  Result          PASSED, ✓ download failed rate<0.01
```

| Row              | Meaning                                                                                    |
|------------------|--------------------------------------------------------------------------------------------|
| `Run`            | File, number of peers and wall-clock time of the run.                                      |
| `Target`         | Download URL, the `uuid` makes every run a fresh Dragonfly task shared by all peers.       |
| `Downloads`      | `dfget` runs started and how many exited successfully.                                     |
| `Failed`         | `dfget` runs that exited with an error, their cost is left out of the latency row.         |
| `download`       | Time of `dfget` on each peer, measured in the pod, `kubectl exec` overhead excluded.       |
| `Traffic`        | Bytes the peers and seed peers downloaded during the run, read from dfdaemon metrics.      |
| `Back to source` | Share of the traffic fetched from the file server, the rest came from the P2P network.     |
| `Metrics`        | Peers whose metrics were read before and after the run, the traffic leaves out the others. |
| `Result`         | `FAILED` and a non-zero exit code when more than 1% of the downloads failed.               |

## Build the image

```shell
make docker-build-file-bench
```

Tag and push it, then update `image` in `file-bench.yaml` and `file-bench-cleanup.yaml`.
