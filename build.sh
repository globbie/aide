#!/usr/bin/env bash

set -e

docker build -t globbie/aide:$TAG --build-arg TRAVIS_JOB_ID=$TRAVIS_JOB_ID --build-arg TRAVIS_BRANCH=$TRAVIS_BRANCH .

id=$(docker create globbie/aide:$TAG)
docker cp ${id}:/usr/bin/aide .
docker rm -v ${id}

