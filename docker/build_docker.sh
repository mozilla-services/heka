#!/bin/bash
docker build -t mozilla/heka_base ..
docker build --rm -t mozilla/heka_build .
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -ti --name heka_build mozilla/heka_build
docker rmi mozilla/heka_build
