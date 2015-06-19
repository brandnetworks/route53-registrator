FROM ubuntu:14.04

WORKDIR /
ENTRYPOINT ["/run.sh"]

RUN apt-get update &&\
	apt-get install -y -q awscli curl apt-transport-https ca-certificates &&\
	apt-key adv --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9 &&\
	echo deb https://get.docker.com/ubuntu docker main > /etc/apt/sources.list.d/docker.list &&\
	apt-get update &&\
	apt-get install -y -q lxc-docker-1.5.0

ADD run.sh /run.sh
ADD trigger.sh /trigger.sh

