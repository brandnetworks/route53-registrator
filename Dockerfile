FROM ubuntu:14.04

WORKDIR /
CMD ["/run.sh"]

ADD run.sh /run.sh
ADD trigger.sh /trigger.sh

