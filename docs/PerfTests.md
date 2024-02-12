# Perf tests

## Test results

grafana/d/mHq4sS2Sk/middleware-perf-tests

Memory and cpu pprof is collected for each test run and stored in the build artifacts.

## Run tests

https://github.com/anyproto/anytype-heart/actions/workflows/build.yml

Runner: `self-hosted`

Run perf test times: `> 0`

## Run tests locally

```
cd cmd/perftester/
go run main.go <runs amount>
```

You'll get the artifacts locally.

You must have the prometheus key PROM_KEY and password PROM_PASSWORD in your environment variables or just comment sending metrics in grpc.go.

## Perf tests schedule

Main branch every day at midnight