IMAGE = localhost/podman-ssh:latest

all: build start

.PHONY: build
build:
	docker build -t $(IMAGE) .

.PHONY: ssh
ssh:
	ssh -o 'StrictHostKeyChecking no' -i ./ssh/id_ed25519 -p 2222 root@localhost

.PHONY: stop
stop:
	docker kill podman

.PHONY: exec
exec:
	docker exec -it podman bash

.PHONY: status
status:
	docker exec -t podman systemctl status sshd podman.socket

.PHONY: start
start:
	docker run --rm \
				-d \
				--name podman \
				-t \
				--privileged \
				-p 2222:22 \
				-v $(shell pwd)/ssh:/root/.ssh/ \
					$(IMAGE)