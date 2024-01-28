#!/bin/bash
set -ex

if [[ $DOCKER_REPO_SLUG == "" ]]; then
  echo "must set DOCKER_REPO_SLUG ENV var"
  exit 1
fi

if [[ $TRAVIS_TAG != "" ]]; then
  DOCKER_TAG=$TRAVIS_TAG
elif [[ $TRAVIS_BRANCH != "" ]]; then
  DOCKER_TAG=$TRAVIS_BRANCH
else
  echo "Skipping docker build because this is a pull request (${TRAVIS_PULL_REQUEST})"
  exit 0
fi

IMAGE="${DOCKER_REPO_SLUG}:${TRAVIS_BRANCH}"

cd $TRAVIS_BUILD_DIR
mkdir /tmp/heka
find . -name "*.deb" -exec cp {} /tmp/heka/heka.deb \;
cp docker/Dockerfile.travis /tmp/heka/Dockerfile

docker build -t $IMAGE /tmp/heka
docker login -e="$DOCKER_EMAIL" -u="$DOCKER_USERNAME" -p="$DOCKER_PASSWORD"
docker push $IMAGE