#!/bin/bash
docker build -t ecnahc515/heka_base ..
docker build --rm -t ecnahc515/heka_build .
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -ti --name heka_build ecnahc515/heka_build
docker rmi ecnahc515/heka_build
