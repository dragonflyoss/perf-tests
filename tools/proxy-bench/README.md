# proxy-bench

Load-test the Dragonfly dfdaemon proxy with [k6](https://k6.io). Every request goes through the proxy, the
response body is discarded as it arrives, and k6 prints latency, throughput and error rate at the end.

## Prerequisites

- Dragonfly installed in the cluster, see the [root README](../../README.md).
- The test file server: `kubectl apply -f ../file-server/file-server.yaml`.

## Run in Kubernetes

1. Point the Job at the proxy and pick a mode by editing `env` in `proxy-bench.yaml`:

   ```yaml
   - name: HTTP_PROXY
     value: 'http://seed-client:4001'
   - name: MODE
     value: 'repeat' # repeat | random | sequential
   ```

2. Start the Job in the namespace where the proxy and the file server run, the two hostnames above resolve
   there:

   ```shell
   kubectl apply -n dragonfly-system -f proxy-bench.yaml
   ```

3. Follow the logs, the summary is printed when the run ends:

   ```shell
   kubectl logs -n dragonfly-system -f job/proxy-bench
   ```

   The Job is marked `Failed` when more than 1% of the requests failed.

4. Delete the Job before the next run, its name is fixed:

   ```shell
   kubectl delete -n dragonfly-system job/proxy-bench
   ```

## Run locally

Install [k6](https://grafana.com/docs/k6/latest/set-up/install-k6/), then:

```shell
HTTP_PROXY=http://127.0.0.1:4001 k6 run -e MODE=sequential -e RATE=0 -e VUS=256 proxy-bench.js
```

## Modes

| Mode         | What every request does                                                                        |
|--------------|------------------------------------------------------------------------------------------------|
| `repeat`     | Same URL every time, one Dragonfly task, served from the P2P cache.                            |
| `random`     | Unique query string every time, one task per request, always back-to-source.                   |
| `sequential` | Read `FILE_SIZE` bytes head to tail in `CHUNK_SIZE` ranges, cycling through `URL_COUNT` tasks. |

## Knobs

All knobs are environment variables, set them in the Job `env` or with `k6 run -e NAME=value`.

| Name               | Default                       | Description                                       |
|--------------------|-------------------------------|---------------------------------------------------|
| `HTTP_PROXY`       | none                          | Proxy to send requests through.                   |
| `MODE`             | `repeat`                      | `repeat`, `random` or `sequential`.               |
| `TARGET_URL`       | `http://file-server/small` \* | Object to download.                               |
| `RATE`             | `100`                         | Requests per second, `0` runs `VUS` back-to-back. |
| `VUS`              | `64`                          | Max concurrent requests.                          |
| `DURATION`         | `60s`                         | Test duration.                                    |
| `TIMEOUT`          | `30s`                         | Per-request timeout.                              |
| `RANGE`            | none                          | Range header for repeat/random, e.g. `0-1023`.    |
| `FILE_SIZE`        | `1073741824` (1GiB)           | sequential: object size in bytes.                 |
| `CHUNK_SIZE`       | `4194304` (4MiB)              | sequential: bytes per request.                    |
| `STREAMS`          | `128`                         | sequential: readers interleaved round-robin.      |
| `URL_COUNT`        | `32`                          | sequential: number of tasks in the pool.          |
| `SEED_CLIENT_CPUS` | off                           | CPUs of the seed client, prints `cpu_cost`.       |

\* `http://file-server/large` in the sequential mode.

## Reading the report

k6 prints a short report instead of its default summary:

```text
proxy-bench

  Run           sequential via http://seed-client:4001, 60.0s
  Target        http://file-server/large
  Load          256 VUs back-to-back, constant-vus
  Sequential    1 GiB in 4 MiB chunks, 256 per pass, 128 streams over 32 URLs

  Requests      120,481 total, 2008.0 req/s, 0 dropped
  Failed        0 of 120,481 (0.00%)
  Check         ✓ status is 206, 120,481 of 120,481

  Latency (ms)        min      avg      med    p(90)    p(95)    p(99)      max
    request         12.11   127.40   118.62   201.55   233.10   310.27   812.94
    connect          0.00     0.01     0.00     0.00     0.00     0.21     4.10
    first byte       0.31     9.85     6.20    18.71    27.02    61.44   402.80
    download        11.02   117.30   111.90   186.02   214.65   281.13   711.50

  Throughput    470.6 GiB received, 67.36 Gbps
  CPU cost      4 cores / 67.36 Gbps = 0.059 core/Gbps

  Result        PASSED, ✓ http_req_failed rate<0.01
```

| Row          | Meaning                                                                         |
|--------------|---------------------------------------------------------------------------------|
| `Requests`   | Requests sent, rate achieved, iterations dropped because `VUS` could not keep up. |
| `Failed`     | Wrong status code or transport error.                                            |
| `Check`      | Every response carried the expected status, 206 for ranged requests, else 200.  |
| `request`    | Whole request, from first byte sent to last byte received.                       |
| `connect`    | TCP connect, 0 once connections are reused.                                      |
| `first byte` | Request sent to first byte of the response, i.e. proxy latency.                  |
| `download`   | First to last byte of the response body.                                         |
| `Throughput` | Bytes received through the proxy, headers included.                              |
| `CPU cost`   | Seed client cores per Gbps served, only with `SEED_CLIENT_CPUS`.                 |
| `Result`     | `FAILED` and exit code 99 when a threshold is crossed.                           |

## Build the image

```shell
make docker-build-proxy-bench
```

Tag and push it, then update `image` in `proxy-bench.yaml`.
