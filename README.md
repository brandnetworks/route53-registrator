Route53 Registrator
===================

Use this to watch for Docker events of a given service and register or deregister CNAMEs in Route53.

To build, run the following.

```
docker build -t brandnetworks/route53-registrator .
```

Running this image requires four arguments.

```
docker run brandnetworks/route53-registrator image region hosted_zone_id domain
```

