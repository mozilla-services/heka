# heka_base image
FROM golang:1.4

MAINTAINER Chance Zibolski <chance.zibolski@gmail.com> (@chance)

RUN     apt-get update && \
        apt-get install -yq --no-install-recommends \
        build-essential \
        ca-certificates \
        cmake \
        debhelper \
        fakeroot \
        libgeoip-dev \
        libgeoip1 \
        golang-goprotobuf-dev \
        patch \
        ruby-dev \
        protobuf-compiler \
        python-sphinx \
        wget

WORKDIR /heka

ENV CTEST_OUTPUT_ON_FAILURE 1
ENV BUILD_DIR   /heka/build
ENV GOPATH      $BUILD_DIR/heka
ENV GOBIN       $GOPATH/bin
ENV PATH        $PATH:$GOBIN

EXPOSE 4352

COPY . /heka
