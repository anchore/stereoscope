all: build start

.PHONY: build
build:
	docker build -t podman-ssh:latest .

.PHONY: ssh
ssh:
	ssh -o 'StrictHostKeyChecking no' -i ./ssh/id_rsa -p 2222 root@localhost

.PHONY: stop
stop:
	docker kill podman

.PHONY: exec
exec:
	docker exec -it podman bash

.PHONY: start
start:
	docker run --rm \
				-d \
				-it \
				--privileged \
				-p 2222:22 \
				-v $(shell pwd)/ssh:/root/.ssh/ \
					podman-ssh:latest