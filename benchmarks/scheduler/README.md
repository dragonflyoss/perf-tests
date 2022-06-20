# Scheduler Benckmark

Provide scheduler performance test solution and related metics.

## Deployment

Scheduler can be set to disable seed peer mode,
so that only the manager and scheduler can be deployed for performance testing.

Deploying Dragonfly can refer to [install-dragonfly](https://d7y.io/docs/setup/install/source).

## Run performance tests

Performance test parameters can be controlled by setting environment variables.

| Name                                 | Description                   | Default          |
| ------------------------------------ | ----------------------------- | ---------------- |
| DRAGONFLY_TEST_SCHEDULER_HOST        | Scheduler GRPC server host    | `localhost:8002` |
| DRAGONFLY_TEST_SCHEDULER_PROTOSET    | Scheduler grpc protoset path  | `../bundle.pb`   |
| DRAGONFLY_TEST_SCHEDULER_INSECURE    | Enable grpc insecure mode     | false            |
| DRAGONFLY_TEST_SCHEDULER_CONCURRENCY | Number of concurrent requests | 100              |

Run performance tests.

```shell
go run main.go
```

## Analyze performance

You can use [pprof](https://go.dev/blog/pprof) to analyze golang performance.
First scheduler should enable listening port for `pprof`,
then collect performance data and launch the performance visualization page.

```shell
go tool pprof -http=":8080" "http://dragonfly-scheduler:18066/debug/pprof/profile?seconds=30"
```
