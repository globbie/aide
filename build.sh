#!/usr/bin/env bash

set -e

docker build -t globbie/gnode:$TAG .

id=$(docker create globbie/gnode:$TAG)
docker cp ${id}:/tmp/coverage.out .
docker rm -v ${id}
