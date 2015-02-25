# Route53 Registrator

## What is it

Inspired by [registrator](https://github.com/gliderlabs/registrator), route53-registrator watches for docker events created by a named container, and creates or deletes Route53 CNAME records pointing to the host's public address.

## Limitations

The program uses the ec2 metadata service to retrieve the hostname for the instance. As a result, the program is tied to usage on AWS EC2 instances for now.

## Development

### Setup

- Ensure you have a working golang environment
- `go get` to install dependencies

### Make tasks:

 - `build/container`: 
     - Builds the binary image (compiled only for linux amd64)
     - Builds a Docker container to run the binary
 - `dev`:
     - Runs the latest built docker container, passing AWS credentials as env variables and 
       some sane defaults
 - `release`:
     - Pushes the latest image to the public docker index (it's tied to my account right now)