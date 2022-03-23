module github.com/anchore/stereoscope

go 1.16

require (
	github.com/anchore/go-testutils v0.0.0-20200925183923-d5f45b0d3c04
	github.com/bmatcuk/doublestar/v4 v4.0.2
	github.com/containerd/containerd v1.5.10
	github.com/docker/cli v20.10.12+incompatible
	// docker/distribution for https://github.com/advisories/GHSA-qq97-vm5h-rrhg
	github.com/docker/distribution v2.8.0+incompatible // indirect
	github.com/docker/docker v20.10.12+incompatible
	github.com/gabriel-vasile/mimetype v1.4.0
	github.com/go-test/deep v1.0.8
	github.com/google/go-containerregistry v0.7.0
	github.com/hashicorp/go-multierror v1.1.0
	github.com/logrusorgru/aurora v0.0.0-20200102142835-e9ef32dff381
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pelletier/go-toml v1.9.3
	github.com/pkg/errors v0.9.1
	github.com/scylladb/go-set v1.0.2
	github.com/sergi/go-diff v1.1.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/afero v1.6.0
	github.com/stretchr/testify v1.7.0
	github.com/wagoodman/go-partybus v0.0.0-20200526224238-eb215533f07d
	github.com/wagoodman/go-progress v0.0.0-20200621122631-1a2120f0695a
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2
)
