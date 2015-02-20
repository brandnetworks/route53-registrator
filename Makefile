NAME=route53-registrator

dev:
	docker run \
		-e DOCKER_HOST=http://192.168.59.103:2376 \
		-e AWS_ACCESS_KEY_ID=$(AWS_ACCESS_KEY_ID) \
		-e AWS_SECRET_ACCESS_KEY=$(AWS_SECRET_ACCESS_KEY) \
		$(NAME) /bin/route53-registrator -metadata-address=192.168.59.103:5000


build/container: stage/route53-registrator Dockerfile
	docker build --no-cache -t $(NAME) .
	touch build/container

build/route53-registrator: *.go
	GOOS=linux GOARCH=amd64 go build -o build/route53-registrator

stage/route53-registrator: build/route53-registrator
	mkdir -p stage
	cp build/route53-registrator stage/route53-registrator

release:
	docker tag route53-registrator rtux/route53-registrator
	docker push rtux/route53-registrator

.PHONY: clean build/container 
clean:
	rm -rf build
