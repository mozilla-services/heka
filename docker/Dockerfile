# heka_build image
# Uses heka base which includes a built checkout of heka, and then builds a dpkg
# and builds a new image which installs that dpkg
FROM mozilla/heka_base

RUN mkdir -p /heka_docker
RUN cd /heka/build && make deb
RUN find /heka/build -name "*.deb" -exec cp {} /heka_docker/heka.deb \;
COPY Dockerfile.final /heka_docker/Dockerfile

RUN curl -sSL https://get.docker.io/builds/Linux/x86_64/docker-1.2.0 -o /tmp/docker && \
    echo "540459bc5d9f1cac17fe8654891814314db15e77 /tmp/docker" | sha1sum -c - && \
    mv /tmp/docker /usr/local/bin/docker && \
    chmod +x /usr/local/bin/docker

CMD docker build -t mozilla/heka /heka_docker
