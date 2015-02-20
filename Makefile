NAME=route53-registrator
VERSION=$(shell cat VERSION)
AWS_ACCESS_KEY=$(echo AWS_ACCESS_KEY_ID)
AWS_SECRET_ACCESS_KEY=$(echo AWS_SECRET_ACCESS_KEY)

dev:
	docker build -f Dockerfile.dev -t $(NAME):dev .
	docker run --rm \
		-v /var/run/docker.sock:/tmp/docker.sock \
		-e AWS_ACCESS_KEY_ID=$(AWS_ACCESS_KEY)\
		-e AWS_SECRET_ACCESS_KEY=$(AWS_SECRET_ACCESS_KEY) \
		$(NAME):dev /bin/r53-registrator -metadata-address=192.168.59.103:5000

