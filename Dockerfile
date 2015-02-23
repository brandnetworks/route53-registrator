FROM scratch

ADD ./stage/route53-registrator /bin/route53-registrator
ADD ca-bundle.crt /etc/ssl/ca-bundle.pem

CMD ["/bin/route53-registrator"]
