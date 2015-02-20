FROM gliderlabs/alpine

ADD ./stage/route53-registrator /bin/route53-registrator

ENTRYPOINT ["/bin/route53-registrator"]
