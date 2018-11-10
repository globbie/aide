#!/usr/bin/env bash

docker build -t tmp -f Dockerfile.build .
if [ $? -ne 0 ]; then
    echo "docker build failed"
    exit 1
fi

id=$(docker create tmp)
docker cp ${id}:/tmp/gnode .
if [ $? -ne 0 ]; then
    echo "docker copy binary failed"
    exit 1
fi

docker cp ${id}:/tmp/schemas .
if [ $? -ne 0 ]; then
    echo "docker copy schemas failed"
    exit 1
fi

docker rm -v ${id}
if [ $? -ne 0 ]; then
    echo "docker remove container failed"
    exit 1
fi

docker rmi tmp
if [ $? -ne 0 ]; then
    echo "docker remove temporary image failed"
    exit 1
fi

docker build -t globbie/gnode .
if [ $? -ne 0 ]; then
    echo "docker build gnode image failed"
    exit 1
fi

docker push globbie/gnode
if [ $? -ne 0 ]; then
    echo "docker push gnode image failed"
    exit 1
fi

rm gnode
rm -rf schemas
