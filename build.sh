#!/usr/bin/env bash

set -e

# docker exec -e TRAVIS_JOB_ID="$TRAVIS_JOB_ID" -e TRAVIS_BRANCH="$TRAVIS_BRANCH"
docker build -t globbie/gnode:$TAG --build-arg COVERALLS_TOKEN=$COVERALLS_TOKEN .

#id=$(docker create globbie/gnode:$TAG)
#docker cp ${id}:/tmp/coverage.out .
#docker rm -v ${id}

