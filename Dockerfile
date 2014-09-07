# heka_base image
FROM ubuntu:14.04

MAINTAINER Chance Zibolski <chance.zibolski@gmail.com> (@chance)

RUN     apt-get update && \
        apt-get install -yq --no-install-recommends \
        build-essential \
        bzr \
        ca-certificates \
        cmake \
        curl \
        git \
        golang-goprotobuf-dev\
        make \
        mercurial \
        patch \
        ruby-dev \
        protobuf-compiler \
        python-sphinx \
        wget

# Install Go 1.3
RUN curl -s https://storage.googleapis.com/golang/go1.3.1.linux-amd64.tar.gz | tar -v -C /usr/local -xz

WORKDIR /heka

ENV GOROOT /usr/local/go
ENV PATH $PATH:/usr/local/go/bin:/go/bin

ENV CTEST_OUTPUT_ON_FAILURE 1
ENV BUILD_DIR   /heka/build
ENV GOPATH      $BUILD_DIR/heka
ENV GOBIN       $GOPATH/bin
ENV PATH        $PATH:$GOBIN
# Build faster
ENV NUM_JOBS    10

EXPOSE 4352

COPY . /heka
RUN ./build.sh
