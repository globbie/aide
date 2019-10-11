#!/usr/bin/env bash

set -e

docker build -t globbie/gnode:$TAG --build-arg TRAVIS_JOB_ID=$TRAVIS_JOB_ID --build-arg TRAVIS_BRANCH=$TRAVIS_BRANCH .

id=$(docker create globbie/gnode:$TAG)
docker cp ${id}:/usr/bin/gnode .
docker rm -v ${id}

