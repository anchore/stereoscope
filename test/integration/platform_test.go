package integration

import (
	"context"
	"fmt"
	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestPlatformSelection(t *testing.T) {
	imageName := "busybox:1.31"
	tests := []struct {
		source         image.Source
		architecture   string
		os             string
		expectedDigest string
		expectedErr    require.ErrorAssertionFunc
	}{
		{
			source:         image.OciRegistrySource,
			architecture:   "arm64",
			os:             "linux",
			expectedDigest: "sha256:1ee006886991ad4689838d3a288e0dd3fd29b70e276622f16b67a8922831a853", // direct from repo manifest
		},
		{
			source:         image.OciRegistrySource,
			architecture:   "amd64",
			os:             "linux",
			expectedDigest: "sha256:95cf004f559831017cdf4628aaf1bb30133677be8702a8c5f2994629f637a209", // direct from repo manifest
		},
		{
			source:         image.DockerDaemonSource,
			architecture:   "arm64",
			os:             "linux",
			expectedDigest: "sha256:dcd4bbdd7ea2360002c684968429a2105997c3ce5821e84bfc2703c5ec984971", // from generated manifest
		},
		{
			source:         image.DockerDaemonSource,
			architecture:   "amd64",
			os:             "linux",
			expectedDigest: "sha256:79d3cb76a5a8ba402af164ace87bd0f3e0759979f94caf7247745126359711da", // from generated manifest
		},
		{
			source:         image.PodmanDaemonSource,
			architecture:   "arm64",
			os:             "linux",
			expectedDigest: "sha256:dcd4bbdd7ea2360002c684968429a2105997c3ce5821e84bfc2703c5ec984971", // from generated manifest
		},
		{
			source:         image.PodmanDaemonSource,
			architecture:   "amd64",
			os:             "linux",
			expectedDigest: "sha256:79d3cb76a5a8ba402af164ace87bd0f3e0759979f94caf7247745126359711da", // from generated manifest
		},
	}

	for _, tt := range tests {
		platform := fmt.Sprintf("%s/%s", tt.os, tt.architecture)
		t.Run(fmt.Sprintf("%s/%s", tt.source.String(), platform), func(t *testing.T) {
			if tt.expectedErr == nil {
				tt.expectedErr = require.NoError
			}
			platformOpt := stereoscope.WithPlatform(platform)
			img, err := stereoscope.GetImageFromSource(context.TODO(), imageName, tt.source, platformOpt)
			tt.expectedErr(t, err)
			require.NotNil(t, img)

			assert.Equal(t, tt.os, img.Metadata.OS)
			assert.Equal(t, tt.architecture, img.Metadata.Architecture)
			found := false
			if img.Metadata.ManifestDigest == tt.expectedDigest {
				found = true
			}
			for _, d := range img.Metadata.RepoDigests {
				if found {
					break
				}
				if strings.Contains(d, tt.expectedDigest) {
					found = true
				}
			}
			assert.True(t, found, "could not find repo digest that matches the expected digest:\nfound manifest digest: %+v\nfound repo digests: %+v\nexpected digest: %+v", img.Metadata.ManifestDigest, img.Metadata.RepoDigests, tt.expectedDigest)
		})
	}
}
