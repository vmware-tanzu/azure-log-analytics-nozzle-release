#! /bin/bash

cd "$(git rev-parse --show-toplevel)"/ci/docker

docker build . -t logcache/azure-ci:latest
docker push logcache/azure-ci:latest
