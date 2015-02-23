FROM gliderlabs/alpine

ADD ./stage/route53-registrator /bin/route53-registrator

CMD ["/bin/route53-registrator"]
