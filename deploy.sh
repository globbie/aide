#!/usr/bin/env bash

set -e

docker build -t tmp -f Dockerfile.build .

id=$(docker create tmp)

docker cp ${id}:/tmp/gnode .
docker cp ${id}:/tmp/schemas .
docker cp ${id}:/tmp/coverage.out .

docker rm -v ${id}
docker rmi tmp

docker build -t globbie/gnode:$TAG .

#rm gnode
#rm -rf schemas
