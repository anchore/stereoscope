module github.com/anchore/stereoscope

go 1.16

require (
	github.com/GoogleCloudPlatform/docker-credential-gcr v2.0.5+incompatible
	github.com/anchore/go-testutils v0.0.0-20200925183923-d5f45b0d3c04
	github.com/awslabs/amazon-ecr-credential-helper/ecr-login v0.0.0-20220517224237-e6f29200ae04
	github.com/bmatcuk/doublestar/v4 v4.0.2
	github.com/containerd/containerd v1.5.13
	github.com/docker/cli v20.10.12+incompatible
	// docker/distribution for https://github.com/advisories/GHSA-qq97-vm5h-rrhg
	github.com/docker/distribution v2.8.0+incompatible // indirect
	github.com/docker/docker v20.10.12+incompatible
	github.com/gabriel-vasile/mimetype v1.4.0
	github.com/go-test/deep v1.0.8
	github.com/google/go-containerregistry v0.7.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/logrusorgru/aurora v0.0.0-20200102142835-e9ef32dff381
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pelletier/go-toml v1.9.3
	github.com/pkg/errors v0.9.1
	// pinned to pull in 386 arch fix: https://github.com/scylladb/go-set/commit/cc7b2070d91ebf40d233207b633e28f5bd8f03a5
	github.com/scylladb/go-set v1.0.3-0.20200225121959-cc7b2070d91e
	github.com/sergi/go-diff v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/afero v1.6.0
	github.com/stretchr/testify v1.7.0
	github.com/sylabs/sif/v2 v2.7.2
	github.com/sylabs/squashfs v0.6.1
	github.com/wagoodman/go-partybus v0.0.0-20200526224238-eb215533f07d
	github.com/wagoodman/go-progress v0.0.0-20200621122631-1a2120f0695a
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
)
