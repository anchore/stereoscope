CACHE_DIR = ../cache
CACHE_FILE_NAME = singularity-image.tar
CACHE_FILE = $(CACHE_DIR)/$(CACHE_FILE_NAME)
TAG = localhost/singularity:dev

.PHONY: all
all:
	@docker inspect $(TAG) > /dev/null && echo "$(TAG) already exists" || (make build && make save-cache)

.PHONY: build
build:
	docker build -t $(TAG) .

.PHONY: save-cache
save-cache: $(CACHE_DIR)
	docker image save $(TAG) -o $(CACHE_FILE)

.PHONY: load-cache
load-cache:
	docker image load -i $(CACHE_FILE)

$(CACHE_DIR):
	mkdir -p $(CACHE_DIR)

# note: this is used by CI to determine if the tool image should be rebuilt/refreshed
.PHONY: fingerprint
fingerprint:
	find Dockerfile -type f -exec md5sum {} + | sort | md5sum | tee cache.fingerprint
