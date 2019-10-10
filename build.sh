#!/usr/bin/env bash

set -e

docker build -t globbie/gnode:$TAG --build-arg COVERALLS_TOKEN=$COVERALLS_TOKEN .

id=$(docker create globbie/gnode:$TAG)
docker cp ${id}:/usr/bin/gnode .
docker rm -v ${id}

