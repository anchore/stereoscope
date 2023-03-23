package stereoscope

import (
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/scylladb/go-set/i8set"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_setPlatform(t *testing.T) {

	expectedSources := i8set.New()
	for _, s := range image.AllSources {
		expectedSources.Add(int8(s))
	}
	actualSources := i8set.New()

	tests := []struct {
		name            string
		source          image.Source
		defaultArch     string
		initialPlatform *image.Platform
		wantPlatform    *image.Platform
		wantErr         require.ErrorAssertionFunc
	}{
		// allow defaults ---------------------------------------------------------
		{
			name:            "docker daemon",
			source:          image.DockerDaemonSource,
			defaultArch:     "amd64",
			initialPlatform: nil,
			wantPlatform: &image.Platform{
				Architecture: "amd64",
				OS:           "linux",
			},
		},
		{
			name:        "docker daemon (do not override)",
			source:      image.DockerDaemonSource,
			defaultArch: "amd64",
			initialPlatform: &image.Platform{
				Architecture: "arm64", // not different than default arch
				OS:           "linux",
			},
			wantPlatform: &image.Platform{
				Architecture: "arm64", // note: did not change
				OS:           "linux",
			},
		},
		{
			name:            "podman daemon",
			source:          image.PodmanDaemonSource,
			defaultArch:     "amd64",
			initialPlatform: nil,
			wantPlatform: &image.Platform{
				Architecture: "amd64",
				OS:           "linux",
			},
		},
		{
			name:        "podman daemon (do not override)",
			source:      image.PodmanDaemonSource,
			defaultArch: "amd64",
			initialPlatform: &image.Platform{
				Architecture: "arm64", // not different than default arch
				OS:           "linux",
			},
			wantPlatform: &image.Platform{
				Architecture: "arm64", // note: did not change
				OS:           "linux",
			},
		},
		{
			name:            "OCI registry",
			source:          image.OciRegistrySource,
			defaultArch:     "amd64",
			initialPlatform: nil,
			wantPlatform: &image.Platform{
				Architecture: "amd64",
				OS:           "linux",
			},
		},
		{
			name:        "OCI registry (do not override)",
			source:      image.OciRegistrySource,
			defaultArch: "amd64",
			initialPlatform: &image.Platform{
				Architecture: "arm64", // not different than default arch
				OS:           "linux",
			},
			wantPlatform: &image.Platform{
				Architecture: "arm64", // note: did not change
				OS:           "linux",
			},
		},
		// disallow defaults ---------------------------------------------------------
		{
			name:            "docker tarball",
			source:          image.DockerTarballSource,
			defaultArch:     "amd64",
			initialPlatform: nil,
			wantPlatform:    nil,
		},
		{
			name:        "docker tarball (override fails)",
			source:      image.DockerTarballSource,
			defaultArch: "amd64",
			initialPlatform: &image.Platform{
				Architecture: "amd64",
				OS:           "linux",
			},
			wantErr: require.Error,
		},
		{
			name:            "OCI dir",
			source:          image.OciDirectorySource,
			defaultArch:     "amd64",
			initialPlatform: nil,
			wantPlatform:    nil,
		},
		{
			name:        "OCI dir (override fails)",
			source:      image.OciDirectorySource,
			defaultArch: "amd64",
			initialPlatform: &image.Platform{
				Architecture: "amd64",
				OS:           "linux",
			},
			wantErr: require.Error,
		},
		{
			name:            "OCI tarball",
			source:          image.OciTarballSource,
			defaultArch:     "amd64",
			initialPlatform: nil,
			wantPlatform:    nil,
		},
		{
			name:        "OCI tarball (override fails)",
			source:      image.OciTarballSource,
			defaultArch: "amd64",
			initialPlatform: &image.Platform{
				Architecture: "amd64",
				OS:           "linux",
			},
			wantErr: require.Error,
		},
		{
			name:            "singularity",
			source:          image.SingularitySource,
			defaultArch:     "amd64",
			initialPlatform: nil,
			wantPlatform:    nil,
		},
		{
			name:        "singularity (override fails)",
			source:      image.SingularitySource,
			defaultArch: "amd64",
			initialPlatform: &image.Platform{
				Architecture: "amd64",
				OS:           "linux",
			},
			wantErr: require.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}

			actualSources.Add(int8(tt.source))
			cfg := config{
				Platform: tt.initialPlatform,
			}
			err := setPlatform(tt.source, &cfg, tt.defaultArch)
			tt.wantErr(t, err)
			if err != nil {
				return
			}

			assert.Equal(t, tt.wantPlatform, cfg.Platform)
		})
	}

	diff := i8set.Difference(expectedSources, actualSources)
	if !diff.IsEmpty() {
		t.Errorf("missing test cases for sources: %v", diff.List())
	}
}
