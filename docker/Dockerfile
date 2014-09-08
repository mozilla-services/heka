# heka_build image
# Uses heka base which includes a built checkout of heka, and then builds a dpkg
# and builds a new image which installs that dpkg
FROM mozilla/heka_base

RUN mkdir -p /heka_docker
RUN cd /heka/build && make deb
RUN find /heka/build -name "*.deb" -exec cp {} /heka_docker/heka.deb \;
COPY Dockerfile.final /heka_docker/Dockerfile

RUN wget -nv https://get.docker.io/builds/Linux/x86_64/docker-latest -O /docker && chmod +x /docker

CMD /docker build -t mozilla/heka /heka_docker
