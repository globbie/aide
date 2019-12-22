# Aide

[![Build Status](https://travis-ci.org/globbie/aide.svg?branch=master)](https://travis-ci.org/globbie/aide)

[![Coverage Status](https://coveralls.io/repos/github/globbie/aide/badge.svg?branch=master)](https://coveralls.io/github/globbie/aide?branch=master)

AI Decision Executor

## Build

```bash
git submodule update --init --recursive
./build_knowdy.sh
```

## Test

```bash
go test -v ./...
```

## Run

```bash
go run -config-path <config-path> -listen-address <address:port>
```

## Config example

See `config/shard.gsl`

## Run via Docker

```bash
docker run -p 8081:8081 globbie/aide
```

If you want to build your own Docker image, please see `build.sh` script.
