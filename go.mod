module github.com/anchore/stereoscope

go 1.19

require (
	github.com/GoogleCloudPlatform/docker-credential-gcr v2.0.5+incompatible
	github.com/adrg/xdg v0.4.0
	github.com/anchore/go-logger v0.0.0-20220728155337-03b66a5207d8
	github.com/anchore/go-testutils v0.0.0-20200925183923-d5f45b0d3c04
	github.com/awslabs/amazon-ecr-credential-helper/ecr-login v0.0.0-20220517224237-e6f29200ae04
	github.com/becheran/wildmatch-go v1.0.0
	github.com/bmatcuk/doublestar/v4 v4.0.2
	github.com/containerd/containerd v1.7.0
	github.com/docker/cli v20.10.20+incompatible
	github.com/docker/docker v20.10.24+incompatible
	github.com/gabriel-vasile/mimetype v1.4.0
	github.com/go-test/deep v1.0.8
	github.com/google/go-cmp v0.5.9
	github.com/google/go-containerregistry v0.13.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/logrusorgru/aurora v0.0.0-20200102142835-e9ef32dff381
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pelletier/go-toml v1.9.5
	github.com/pkg/errors v0.9.1
	// pinned to pull in 386 arch fix: https://github.com/scylladb/go-set/commit/cc7b2070d91ebf40d233207b633e28f5bd8f03a5
	github.com/scylladb/go-set v1.0.3-0.20200225121959-cc7b2070d91e
	github.com/sergi/go-diff v1.2.0
	github.com/spf13/afero v1.6.0
	github.com/stretchr/testify v1.8.2
	github.com/sylabs/sif/v2 v2.8.1
	github.com/sylabs/squashfs v0.6.1
	github.com/wagoodman/go-partybus v0.0.0-20200526224238-eb215533f07d
	github.com/wagoodman/go-progress v0.0.0-20230925121702-07e42b3cdba0
	golang.org/x/crypto v0.1.0
)

require (
	cloud.google.com/go/compute v1.19.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20230106234847-43070de90fa1 // indirect
	github.com/AdamKorcz/go-118-fuzz-build v0.0.0-20221215162035-5330a85ea652 // indirect
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/Microsoft/hcsshim v0.10.0-rc.7 // indirect
	github.com/aws/aws-sdk-go-v2 v1.7.1 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.3.1 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.3.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.4.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecrpublic v1.4.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.2.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.3.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.6.0 // indirect
	github.com/aws/smithy-go v1.6.0 // indirect
	github.com/containerd/cgroups v1.1.0 // indirect
	github.com/containerd/continuity v0.3.0 // indirect
	github.com/containerd/fifo v1.1.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.12.1 // indirect
	github.com/containerd/ttrpc v1.2.1 // indirect
	github.com/containerd/typeurl/v2 v2.1.0 // indirect
	github.com/cyphar/filepath-securejoin v0.2.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	// docker/distribution for https://github.com/advisories/GHSA-qq97-vm5h-rrhg
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.7.0 // indirect
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.16.4 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/moby/sys/sequential v0.5.0 // indirect
	github.com/moby/sys/signal v0.7.0 // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.0-rc2.0.20221005185240-3a7f492d3f1b
	github.com/opencontainers/runc v1.1.5 // indirect
	github.com/opencontainers/runtime-spec v1.1.0-rc.1 // indirect
	github.com/opencontainers/selinux v1.11.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/therootcompany/xz v1.0.1 // indirect
	github.com/ulikunitz/xz v0.5.10 // indirect
	github.com/vbatts/tar-split v0.11.2 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/otel v1.14.0 // indirect
	go.opentelemetry.io/otel/trace v1.14.0 // indirect
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/net v0.9.0 // indirect
	golang.org/x/oauth2 v0.6.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/sys v0.7.0 // indirect
	golang.org/x/term v0.7.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	golang.org/x/tools v0.8.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230403163135-c38d8f061ccd // indirect
	google.golang.org/grpc v1.54.0 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require github.com/kr/pretty v0.3.0 // indirect
