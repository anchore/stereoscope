.PHONY: all
all:
	cd singularity && make

.PHONY: save-cache
save-cache:
	cd singularity && make save-cache

.PHONY: load-cache
load-cache:
	cd singularity && make load-cache

.PHONY: fingerprint
fingerprint:
	@# for all tools, generate
	@rm -f cache.fingerprint
	@cat */cache.fingerprint | sort | md5sum | tee cache.fingerprint
