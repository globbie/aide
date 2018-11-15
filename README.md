# Gnode

Knowdy library Golang wrapper.

## Build

1. `git submodule update --init --recursive`
2. `./build_knowdy.sh`

## Test

`go test -v ./...`

## Run

```bash
go run -config-path <config-path> -listen-address <address:port>
```

## Config example

See `config/shard.gsl`
