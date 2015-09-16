# heka image
# installs heka from a deb package
FROM debian:jessie
MAINTAINER Chance Zibolski <chance.zibolski@gmail.com> (@chance)

COPY heka.deb /tmp/heka.deb
RUN apt-get update && apt-get install -y libgeoip1
RUN dpkg -i /tmp/heka.deb && rm /tmp/heka.deb

EXPOSE 4352
ENTRYPOINT ["hekad", "-config=/etc/heka/conf.d"]