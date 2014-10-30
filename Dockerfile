# heka_base image
FROM debian:jessie

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

# Install Go 1.3.1
RUN curl -s https://storage.googleapis.com/golang/go1.3.1.linux-amd64.tar.gz -o /tmp/go.tar.gz && \
        echo "3af011cc19b21c7180f2604fd85fbc4ddde97143 /tmp/go.tar.gz" | sha1sum -c && \
        tar -C /usr/local -xzf /tmp/go.tar.gz

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
