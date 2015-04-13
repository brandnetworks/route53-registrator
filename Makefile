NAME=rtux/route53-registrator

dev:
	docker run \
		-e DOCKER_HOST=http://192.168.59.103:2375 \
		-e AWS_ACCESS_KEY_ID=$(AWS_ACCESS_KEY_ID) \
		-e AWS_SECRET_ACCESS_KEY=$(AWS_SECRET_ACCESS_KEY) \
		$(NAME) /bin/route53-registrator -metadata=192.168.59.103:5000 -container=test -zone=Z1P7DHMHEAX6O3 -cname=cluster-registry.realtime.bnservers.com -logtostderr=1


build/container: clean stage/route53-registrator Dockerfile
	docker build --no-cache -t $(NAME) .
	touch build/container

build/route53-registrator: *.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/route53-registrator

stage/route53-registrator: build/route53-registrator
	mkdir -p stage
	cp build/route53-registrator stage/route53-registrator

release:
	docker tag -f route53-registrator rtux/route53-registrator
	docker push rtux/route53-registrator

.PHONY: clean build/container 
clean:
	rm -rf build ; rm -rf stage
