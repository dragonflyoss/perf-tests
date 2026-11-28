# Fixed-pool in Kubernetes

These six ordinary YAML examples run a small dataset through the
[fixed-pool benchmark](../README.md). They do not manage experiments or distribute
files. Choose images built from the same revision; see
[image builds](../README.md#build-container-images).

| File | Purpose | Values to choose |
| --- | --- | --- |
| [storage.yaml](storage.yaml) | Optional source and metadata PVCs | StorageClass, capacities and access modes |
| [data.yaml](data.yaml) | Generate objects and their manifest | Preparation image, B and N |
| [file-server.yaml](file-server.yaml) | Serve generated objects with the existing FileServer image | Source storage, resources and Service exposure |
| [plan.yaml](plan.yaml) | Generate a plan from a manifest | Preparation image, R and W |
| [seed.yaml](seed.yaml) | Optional full reads through Dragonfly | Preparation image, source and proxy addresses |
| [worker.yaml](worker.yaml) | Run one shard through k6 | Worker image, source/proxy, RATE, DURATION and WORKER_ID |

Work on local copies and replace `replace-with-build-tag` image references before
applying. B/N/R/W and RATE/DURATION are test specifications; PVC and container
resources are environment settings. The examples use two 4 MiB objects, 64 KiB
requests, one worker and 10 requests/s for 30 seconds. These are function-check
values, not capacity recommendations. Paths, volume names and mount references
are wiring; change them together if choosing another layout.

Run the commands below from the feature directory. From the repository root:

```bash
cd tools/proxy-bench/fixed-pool
```

Resources have no namespace or active node selector. Use `kubectl -n` to select
the namespace. For measured runs, choose load nodes that do not compete with the
Seed. Placement must respect the PVCs: local volumes bind to one node;
ReadWriteOnce does not provide cross-node file sharing. Provide independent
metadata copies or suitable shared storage for workers on different nodes.

## 1. Generate the data

Create a namespace, then apply storage only if you need new PVCs:

```bash
export NS=benchmark
kubectl create namespace "$NS"
kubectl -n "$NS" apply -f deploy/storage.yaml
kubectl -n "$NS" apply -f deploy/data.yaml
kubectl -n "$NS" wait --for=condition=complete job/fixed-pool-data --timeout=300s
kubectl -n "$NS" logs job/fixed-pool-data
kubectl -n "$NS" get pvc
```

Continue after the Job succeeds. Its log identifies B/N, the manifest path and
SHA-256. If waiting times out, inspect the Job, Pod events and logs before
continuing. Large datasets require a longer wait and adequate disk space.
The source PVC contains `/data/dataset/objects/sha256/<digest>` and
`/data/dataset/dataset-manifest.json`. Data generation needs no source address.

With a `WaitForFirstConsumer` StorageClass, the input PVC may still be Pending:
it has no consumer yet. Starting an input-volume consumer permits binding;
waiting alone does not. Omitted `storageClassName` selects the cluster default.
An explicit empty string selects classless static PVs. PVC capacity does not
necessarily impose a directory quota; verify actual storage capacity too.

## 2. Publish the source and place the manifest

```bash
kubectl -n "$NS" apply -f deploy/file-server.yaml
kubectl -n "$NS" rollout status statefulset/fixed-pool-file-server --timeout=300s
```

The StatefulSet mounts `dataset/objects` from PVC `fixed-pool-source` at Nginx's
`/usr/share/nginx/html/objects`. Service `fixed-pool-http-service` exposes port 80.
Object URLs are `<SOURCE_BASE_URL>/objects/sha256/<digest>`; the manifest is not
published over HTTP. Recreate the FileServer Pod after replacing the mounted
dataset directory. Changing only R/W or RATE does not require recreating it.

ClusterIP is the default. If the Seed cannot reach cluster Service addresses,
choose a reachable source endpoint. For example, change `spec.type` to `NodePort`,
apply again, then read the allocated port:

```bash
kubectl -n "$NS" get service fixed-pool-http-service
kubectl -n "$NS" get service fixed-pool-http-service \
  -o jsonpath='{.spec.ports[0].nodePort}{"\n"}'
```

`nodePort` is absent for ClusterIP and automatically allocated for NodePort when
not specified. Use `http://<reachable-node-address>:<allocated-port>` as the source
address in both seed.yaml and worker.yaml. Within a reachable cluster network,
`http://fixed-pool-http-service` works in this namespace. A Ready source Pod does
not prove that an external Seed can reach its address.

Place the selected manifest, unchanged, at `/inputs/dataset-manifest.json` on the
input PVC, or adapt the consumer paths and mounts. Use your own transfer method.
For a same-node example, the following temporary file-access Pod mounts both PVCs;
it is an operator copy step, not a benchmark component. It is unsuitable when the
two PVCs are already bound to different nodes.

```bash
export PREPARE_IMAGE=your-registry/fixed-pool-prepare:your-build-tag
kubectl -n "$NS" apply -f - <<EOF_POD
apiVersion: v1
kind: Pod
metadata:
  name: fixed-pool-files
spec:
  restartPolicy: Never
  automountServiceAccountToken: false
  securityContext:
    runAsNonRoot: true
    fsGroup: 1000
    seccompProfile: {type: RuntimeDefault}
  containers:
  - name: files
    image: $PREPARE_IMAGE
    command: ["/bin/sleep", "3600"]
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities: {drop: [ALL]}
    volumeMounts:
    - {name: source, mountPath: /data, readOnly: true}
    - {name: inputs, mountPath: /inputs}
  volumes:
  - name: source
    persistentVolumeClaim: {claimName: fixed-pool-source, readOnly: true}
  - name: inputs
    persistentVolumeClaim: {claimName: fixed-pool-inputs}
EOF_POD
kubectl -n "$NS" wait --for=condition=Ready pod/fixed-pool-files --timeout=120s
kubectl -n "$NS" exec fixed-pool-files -- \
  cp -n /data/dataset/dataset-manifest.json /inputs/dataset-manifest.json
kubectl -n "$NS" exec fixed-pool-files -- sha256sum /inputs/dataset-manifest.json
kubectl -n "$NS" delete pod fixed-pool-files
```

Compare the displayed digest with the data Job's log to confirm your selection.
`cp -n` preserves an existing destination; it does not confirm that the existing
file is the intended one. Do not reformat the manifest: plans bind its exact bytes.

## 3. Generate the plan; optionally prepare the Seed

```bash
kubectl -n "$NS" apply -f deploy/plan.yaml
kubectl -n "$NS" wait --for=condition=complete job/fixed-pool-plan --timeout=300s
kubectl -n "$NS" logs job/fixed-pool-plan
```

The input PVC now contains `plan/request-plan.json` and `plan/shards/0.json`
through `W-1.json`; see [plan inputs and outputs](../README.md#plan).

For an experiment that requires preparation, configure the source and proxy in
seed.yaml, then run:

```bash
kubectl -n "$NS" apply -f deploy/seed.yaml
kubectl -n "$NS" wait --for=condition=complete job/fixed-pool-seed --timeout=3600s
kubectl -n "$NS" logs job/fixed-pool-seed
```

The plan and Seed Jobs can run independently. See [seed-cache](../README.md#seed-cache)
for preparation checks and the meaning of success. On failure, read its log,
correct the problem, delete that Job and rerun it; see
[failure handling](../README.md#output-conflicts-and-failures).

Wait timeouts above limit the operator's wait, not the running Job. Adjust them
for dataset size; the per-object HTTP timeout is a separate Seed CLI option.

## 4. Run the worker and read the results

Choose `SOURCE_BASE_URL` and `HTTP_PROXY` in worker.yaml for this run. They are
explicit worker inputs, even if the optional Seed Job used the same addresses.

```bash
kubectl -n "$NS" apply -f deploy/worker.yaml
kubectl -n "$NS" logs -f job/fixed-pool-worker
kubectl -n "$NS" get job fixed-pool-worker
```

Stdout/stderr go directly to Pod logs; save them locally when needed.
Job Complete does not imply that RATE or request quality met your target.
See [Reading results](../README.md#reading-results) for interpreting the report.

To repeat, delete only the worker Job, adjust its load values if needed, and apply
it again. Keep data and plan; other changes follow the
[reuse rules](../README.md#adjusting-and-reusing-a-test).
Deleting Jobs does not remove PVC contents.

For W workers, create W differently named Jobs with distinct WORKER_ID values,
the appropriate input mounts and the [multi-worker settings](../README.md#multiple-workers).
Distribute files and arrange overlapping execution windows yourself.

The fixed resource names describe one deployment. Use another namespace or change
related names/references consistently for additional deployments. Edit before
creating Jobs; existing Job specs cannot be changed by applying edited arguments.
