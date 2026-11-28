# Fixed-pool benchmark

Run repeatable load against a fixed dataset while varying request size, worker
count and rate. Each action can be called independently:

| Action | Inputs | Result |
| --- | --- | --- |
| `fixed-pool-prepare data` | Object size B and count N | Object files and a dataset manifest |
| `fixed-pool-prepare plan` | Manifest, request size R and worker count W | Access plan and W non-overlapping shards |
| `fixed-pool-prepare seed-cache` | Manifest, source URL and proxy | Full reads and integrity checks; logs |
| `fixed-pool-worker` | Manifest, plan, source, proxy and load settings | k6 requests and results for one shard |

`data` and `plan` need no HTTP server or Dragonfly. `plan` and `seed-cache` both
consume the manifest and do not depend on each other. Seed preparation is optional;
the tester chooses and maintains the cache conditions for the experiment.

## Choose how to run

| Method | Prerequisites | Workflow |
| --- | --- | --- |
| Native commands | Go to build the commands; k6 on PATH for the worker | [Run natively](#run-natively) |
| Docker | Docker and matching preparation/worker images | [Run with containers](#run-with-containers) |
| Kubernetes | kubectl, a cluster with usable storage, and matching images | [Deployment guide](deploy/README.md) |

Choose one workflow. Image users need no host Go or k6 installation. Obtain
matching images from your registry or [build them locally](#build-and-load-locally);
replace example tags with your actual build. An HTTP source and Dragonfly proxy
must be reachable for Seed preparation and workers. Data and plan generation can
run offline.

## Run natively

Run from the repository root using the Go version in `go.mod`. The worker also
requires k6 on `PATH`.

### 1. Generate a dataset and plan

```bash
go build -mod=readonly -o bin/fixed-pool-prepare ./cmd/fixedpool/prepare
go build -mod=readonly -o bin/fixed-pool-worker ./cmd/fixedpool/worker
./bin/fixed-pool-prepare data --root ./dataset --size 4194304 --count 2
./bin/fixed-pool-prepare plan --manifest ./dataset/dataset-manifest.json \
  --range 65536 --workers 1 --out ./plan
```

This creates two 4 MiB objects and a plan with 128 regions of 64 KiB for one
worker. Paths and B/N/R/W values are examples; choose them for your experiment.
To use an existing dataset, pass its manifest directly to `plan`.

### 2. Arrange the source and optionally prepare the Seed

Deploy an HTTP source serving `dataset/objects/sha256/<digest>` at
`<source-base-url>/objects/sha256/<digest>`, and a reachable Dragonfly proxy.
For example, serve the generated `dataset/` directory as the source's document
root. These commands do not deploy either service. Set their actual addresses:

```bash
export SOURCE_BASE_URL=http://source.example
export HTTP_PROXY=http://seed.example:4001
```

For a warm-pool experiment, prepare the selected objects before running workers:

```bash
./bin/fixed-pool-prepare seed-cache \
  --manifest ./dataset/dataset-manifest.json \
  --source-base-url "$SOURCE_BASE_URL" --proxy "$HTTP_PROXY"
```

Skip this command when preparation is not part of your experiment. Skipping it
does not guarantee a cold cache.

### 3. Run one worker

```bash
export MANIFEST="$PWD/dataset/dataset-manifest.json"
export PLAN="$PWD/plan/request-plan.json"
export RATE=10 DURATION=30s
./bin/fixed-pool-worker ./tools/proxy-bench/fixed-pool/benchmark.js
```

The worker checks its inputs, then issues Range GETs at a target rate of 10
requests/s for 30 seconds. Results appear in the terminal; see [Reading results](#reading-results).

## Run with containers

Run from the repository root. The commands below use the tags from
[local image builds](#build-and-load-locally); substitute your images when using
a registry. To reuse existing outputs, skip their generation and select them
directly.

```bash
export PREPARE_IMAGE=fixed-pool-prepare:local
export WORKER_IMAGE=proxy-bench:local
mkdir -p dataset plan

docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/dataset:/data" \
  "$PREPARE_IMAGE" fixed-pool-prepare data \
  --root /data --size 4194304 --count 2

docker run --rm --user "$(id -u):$(id -g)" \
  -v "$PWD/dataset:/dataset:ro" -v "$PWD/plan:/plan" \
  "$PREPARE_IMAGE" fixed-pool-prepare plan \
  --manifest /dataset/dataset-manifest.json --range 65536 --workers 1 --out /plan
```

The Linux `--user` setting writes outputs as your host user. Without it, the
preparation image uses UID/GID `1000:1000`; provide writable output mounts.
Workers need readable manifests and shards, and do not need writable input mounts.

Arrange an HTTP source serving `dataset/objects/sha256/<digest>` at
`<source-base-url>/objects/sha256/<digest>`. Set addresses reachable from the
actual callers, then optionally prepare the Seed:

```bash
export SOURCE_BASE_URL=http://source.example
export HTTP_PROXY=http://seed.example:4001

docker run --rm -v "$PWD/dataset:/dataset:ro" \
  "$PREPARE_IMAGE" fixed-pool-prepare seed-cache \
  --manifest /dataset/dataset-manifest.json \
  --source-base-url "$SOURCE_BASE_URL" --proxy "$HTTP_PROXY"
```

Run one worker, with an optional native k6 tag:

```bash
docker run --rm \
  -v "$PWD/dataset:/dataset:ro" -v "$PWD/plan:/plan:ro" \
  -e MODE=fixed-pool \
  -e MANIFEST=/dataset/dataset-manifest.json \
  -e PLAN=/plan/request-plan.json \
  -e SOURCE_BASE_URL -e HTTP_PROXY \
  -e RATE=10 -e DURATION=30s \
  "$WORKER_IMAGE" --tag case=baseline
```

`MODE=fixed-pool` selects this workload in the shared image. Without `MODE`,
the image runs the original `repeat` mode; the original modes are described in
[proxy-bench](../README.md#modes). Preparation actions use their subcommands,
not `MODE`.

## Reading results

Preparation commands print the selected inputs, successful outputs and elapsed
time on stdout. Workers print their checked configuration, native k6 progress,
a shared text report and machine-readable results.

The shared text report is the first place to read the outcome. This excerpt
comes from a three-second run with a six-region shard:

```text
proxy-bench (fixed-pool)
  Worker        0
  Consumed      30 plan entries; shard 6 regions (repeats included)
  Requests      30 total, 9.902 req/s, 1 dropped
  Failed        0 of 30 (0.00%), status other than 206 or transport error
  Result        NOT EVALUATED (no thresholds configured)
```

- `Requests` shows the native request count/rate; RATE is the offered target.
- `Failed` counts transport errors and statuses other than 206. Dropped iterations
  never started and are excluded from that request-failure count.
- `Consumed` counts plan entries including repeats; it is not a unique-region count.
- `Result` reports configured k6 thresholds. `NOT EVALUATED` means no thresholds
  were configured, not that request quality passed.

Latency and throughput use the [shared proxy-bench report](../README.md#reading-the-report).
Judge achieved rate, request failures and drops separately. Process completion
alone does not establish request quality or cache conditions. Fixed-pool sets no
default request-failure threshold.

The machine-readable records retain details beyond the text report:

| Record | Purpose |
| --- | --- |
| `PROXY_BENCH_FIXED_POOL_K6_SUMMARY_V1` | Native metrics and final `effective_options` |
| `PROXY_BENCH_FIXED_POOL_WORKER_EXIT_V1` | Observed process exit code, timestamps and interruption flag |

Startup text describes the worker's requested configuration; use
`effective_options`, when a summary is produced, to check the final k6 settings.
The worker does not save report files; redirect output if you need an archive.

Errors and warnings go to stderr. Fixed-pool errors include the action, category
and cause, for example `worker: INPUT_INVALID: RATE is required`. Warnings contain
`warning:`; native k6 diagnostics keep their own format.

## Adjusting and reusing a test

| Change | What to do |
| --- | --- |
| B or N | Generate or select the required dataset, then generate its plan |
| R or W | Generate another plan from the existing manifest |
| Rate, duration or VU settings | Rerun workers with the new environment; reuse the plan |
| Source or proxy address | Set the new address; reuse the plan and prepare the target if the experiment requires it |

### Multiple workers

Start W workers with distinct `WORKER_ID` values from 0 to W-1. Give every worker
the same total `RATE`; the worker divides it in proportion to shard region counts.
Each allocated rate must be at least one request/s. Workers do not coordinate
start times or detect duplicate worker indices.

Do not average per-worker P99 values into a global P99.

## Commands and parameters

Preparation numeric options take decimal integers. Sizes are raw bytes:
use `4194304`, not `4MiB`. Use `fixed-pool-prepare <action> --help` for help.

### data

| Option | Required / Default | Meaning |
| --- | --- | --- |
| `--root` | **Required** | Output directory |
| `--size` | **Required** | Positive object size B in bytes |
| `--count` | **Required** | Positive object count N |
| `--generation-workers` | Default: `1` | Positive generation concurrency |

Generation streams data without holding an entire object in memory. Changing
concurrency does not change the generated contents.

### plan

| Option | Required / Default | Meaning |
| --- | --- | --- |
| `--manifest` | **Required** | Selected dataset manifest |
| `--range` | **Required** | Request size R in bytes; `0 < R <= B` |
| `--workers` | Default: `1` | Worker count W, from 1 to the total region count |
| `--out` | **Required** | Plan output directory |

Only the manifest is needed, not the object bodies. Each planned request has
length R:

- Regions per object: `floor(B / R)`.
- Total regions: `N * floor(B / R)`.
- Excluded tail bytes: `N * (B % R)`; these remain in the object files.

The plan uses a repeatable fixed-seed order and divides the regions into
non-overlapping shards covering the planned set.

### seed-cache

| Option | Required / Default | Meaning |
| --- | --- | --- |
| `--manifest` | **Required** | Selected dataset manifest |
| `--source-base-url` | **Required** | HTTP base URL serving `objects/sha256/<digest>` |
| `--proxy` | **Required** | HTTP proxy origin; no credentials, path, query or fragment |
| `--concurrency` | Default: `1` | Positive concurrency; at most N objects run at once |
| `--request-timeout-ms` | Default: `1800000` | Positive timeout for one complete object GET, in milliseconds |

Every object must return HTTP 200, valid Dragonfly task/server identity headers,
and a complete body matching the manifest's size and SHA-256. The first failure
stops new dispatch; requests already running finish or respond to cancellation.
The command writes no preparation files, and the worker does not require a
historical preparation record.

Success verifies these reads, not continued local cache residency, absence of
external fetches or idle background tasks. A shared proxy Service may reach
multiple Clients; to prepare each Seed, call each direct endpoint separately.
Reading through a shared Service does not ensure every backend caches the pool.

### worker

When calling the native binary, the first argument is the benchmark script.
The container entrypoint supplies this argument automatically. Configure inputs
and load through environment variables:

| Environment | Required / Default | Meaning |
| --- | --- | --- |
| `MANIFEST` | **Required** | Dataset manifest matching the plan's SHA-256 |
| `PLAN` | **Required** | Plan descriptor, with relative shard paths preserved |
| `SOURCE_BASE_URL` | **Required** | HTTP base URL serving `objects/sha256/<digest>` |
| `HTTP_PROXY` | **Required** | HTTP proxy origin, with the restrictions of `seed-cache --proxy` |
| `RATE` | **Required** | Total positive integer rate across workers, at most 2147483647 requests/s |
| `WORKER_ID` | Default: `0` | Worker index in `[0, W)` |
| `DURATION` | Default: `300s` | Run duration, at least one second |
| `TIMEOUT` | Default: `30s` | Per-request timeout |
| `GRACEFUL_STOP` | Default: `TIMEOUT + 5s` | In-flight completion allowance; must exceed `TIMEOUT` |
| `PRE_ALLOCATED_VUS` | Default: `64` | Initially allocated k6 VUs |
| `MAX_VUS` | Default: `512` | Maximum VUs, at least `PRE_ALLOCATED_VUS` |
| `RUN_ID` | Optional | User label: 1..128 ASCII letters/digits/._-, starting with a letter or digit |

Durations use Go duration notation with whole-millisecond precision. Each
iteration consumes one local shard entry, cycles at the end and makes one
Range GET. HTTP 206 is expected; other statuses and transport errors count as
k6 request failures. Connections may be reused, redirects are disabled and
there is no business-level retry. R comes from the plan; an inherited `RANGE`
is ignored with a warning.

Size container resources together with the VU settings: raising MAX_VUS does not
allocate more CPU or memory. See the [k6 sizing guide](https://grafana.com/docs/k6/latest/testing-guides/running-large-tests/).

#### Native k6 options

With the worker environment set, append options after the script for a native
run, or after the image name for a container run:

```bash
./bin/fixed-pool-worker ./tools/proxy-bench/fixed-pool/benchmark.js --tag case=baseline
```

| Setting | Docker | Kubernetes |
| --- | --- | --- |
| `MODE` and worker inputs | `-e NAME=value` before the image name | Container `env` |
| Optional native k6 flags | Arguments after the image name | Container `args` |

The image supplies the worker and script path. Pass only extra k6 flags;
do not repeat `k6 run` or a script filename. For a Job, the corresponding
container fields are:

```yaml
env:
  - name: MODE
    value: fixed-pool
  # Add the other required worker environment variables here.
args: ['--tag', 'case=baseline']
```

This is an input-placement example, not a complete Job. Leave `command` unset
to retain the image entrypoint; omit `args` if no extra k6 options are needed.

The worker checks its own inputs, then forwards these arguments unchanged.
Actual k6 validates the options; an invalid option can fail before any load is
generated, even after the worker reports that its inputs passed validation.

For the same native option, precedence is:

```text
k6 defaults < configuration file < script options < K6_* environment < CLI flags
```

For example, `--no-connection-reuse=true` overrides
`K6_NO_CONNECTION_REUSE=false`. See the [k6 configuration rules](https://grafana.com/docs/k6/latest/using-k6/k6-options/how-to/).
Worker variables construct the `fixed_pool` scenario: `--env RATE=20` does not
change the rate already parsed by Go, and `--duration` is not an alias for
`DURATION`. Native options may override or conflict with the scenario under
k6's rules.

## Output conflicts and failures

### Output layout

Successful outputs can be reused directly. The example layout is:

```text
dataset/                     # Example --root
  dataset-manifest.json
  objects/sha256/<digest>

plan/                        # Example --out
  request-plan.json
  shards/0.json              # 0.json through W-1.json
```

When copying a plan, keep its descriptor and relative `shards/` layout. A worker
needs the manifest, descriptor and only its own shard; other shards need not be
mounted locally.

> **Note:** Copy the manifest unchanged. Its identity includes its exact bytes;
> reformatting the JSON changes its SHA-256 and breaks the plan binding.

### Conflicts and recovery

Use one writer per output location. For `data` and `plan`, the directory must be
absent, empty or contain only the allowed empty skeleton:

| Action | Allowed subdirectories |
| --- | --- |
| `data` | `objects/`, `objects/sha256/` |
| `plan` | `shards/` |

Other directories, files and symbolic links are rejected. Keep the input manifest
outside the plan output directory. Existing outputs are never overwritten.
Ensure enough writable storage: `data` checks free space against B × N without
reserving it. Generated object files are read-only (`0444`).

The manifest or plan descriptor is published last, after all referenced files
succeed. Individual object or shard files do not establish completion. On
failure or cancellation, inspect stderr, fix the cause, then explicitly clean
failed output or choose another directory. Partial files may remain; there is
no automatic rollback or resume. A failed plan does not invalidate the dataset.
If only temporary-name cleanup fails after publication, a warning is printed
and the successful result remains valid.

For a failed `seed-cache`, fix the reported cause and rerun. Dragonfly may reuse
existing cached data; this command does not clear caches or repair tasks/pieces.

### Exit codes

| Exit code | Meaning |
| --- | --- |
| 0 | Operation completed |
| 2 | Invalid input, contract mismatch or output conflict |
| 3 | Execution failure |
| 126 / 127 | Cannot start k6 / k6 not found |
| 130 | Handled cancellation or interruption |

Once k6 starts, its exit code is preserved, including 99 for a failed threshold;
handled interruption returns 130. If k6 fails before producing a summary, read
its diagnostic output and the available exit record.

## Build container images

Run from the repository root with Docker Buildx. Each Dockerfile compiles its
command in a Go builder stage, then copies it into the runtime image. No host Go
toolchain or prebuilt binaries are required for image builds.
The runtime uses the existing k6 base image for workers and Alpine for preparation;
the upstream FileServer image is reused.

### Build and load locally

Build and load one platform into the local Docker image store:

```bash
PLATFORM=linux/amd64 # Use linux/arm64 for an ARM64 container host.
docker buildx build --platform "$PLATFORM" --load \
  -t fixed-pool-prepare:local -f tools/proxy-bench/fixed-pool/Dockerfile.tools .
docker buildx build --platform "$PLATFORM" --load \
  -t proxy-bench:local -f tools/proxy-bench/Dockerfile .
```

### Multi-platform builds and manual publication

The image targets build Linux amd64 and arm64 images; each builder stage
compiles the command for the target architecture:

| Target | Result |
| --- | --- |
| `make docker-build-fixed-pool-prepare` | Build result; loading depends on the selected Buildx driver |
| `make docker-push-fixed-pool-prepare` | Build and push the multi-platform preparation image |
| `make docker-build-proxy-bench` | Build result; loading depends on the selected Buildx driver |
| `make docker-push-proxy-bench` | Build and push the multi-platform worker image |

The two `docker-build-*` targets do not publish images or explicitly load them.
A `docker-container` builder without an explicit output keeps the build result
in its cache. Use the local commands above to obtain runnable local images.
See [Docker build outputs](https://docs.docker.com/build/exporters/).

To publish both images manually, select a registry namespace you can push to:

```bash
export D7Y_REGISTRY=registry.example/your-namespace
make docker-push-proxy-bench D7Y_REGISTRY="$D7Y_REGISTRY"
make docker-push-fixed-pool-prepare D7Y_REGISTRY="$D7Y_REGISTRY"
```

Use the resulting image names and tags in your container or Job configuration.

## Development checks

```bash
go test -race ./pkg/fixedpool/... ./cmd/fixedpool/...
```
