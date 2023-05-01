package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/image"
)

func TestPlatformSelection(t *testing.T) {
	/*
	   All digests were obtained by:
	   $ docker image pull --platform $PLATFORM busybox:1.31
	   $ docker image inspect busybox:1.31 | jq -r '.[0].Id'
	*/
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
			expectedDigest: "sha256:19d689bc58fd64da6a46d46512ea965a12b6bfb5b030400e21bc0a04c4ff155e",
		},
		{
			source:         image.OciRegistrySource,
			architecture:   "s390x",
			os:             "linux",
			expectedDigest: "sha256:5bf065699d2e6ddb8b5f7e30f02edc3cfe15d7400e7101b5b505d61fde01f84c",
		},
		{
			source:         image.OciRegistrySource,
			architecture:   "amd64",
			os:             "linux",
			expectedDigest: "sha256:1c35c441208254cb7c3844ba95a96485388cef9ccc0646d562c7fc026e04c807",
		},
		{
			source:         image.DockerDaemonSource,
			architecture:   "arm64",
			os:             "linux",
			expectedDigest: "sha256:19d689bc58fd64da6a46d46512ea965a12b6bfb5b030400e21bc0a04c4ff155e",
		},
		{
			source:         image.DockerDaemonSource,
			architecture:   "amd64",
			os:             "linux",
			expectedDigest: "sha256:1c35c441208254cb7c3844ba95a96485388cef9ccc0646d562c7fc026e04c807",
		},
		{
			source:         image.DockerDaemonSource,
			architecture:   "s390x",
			os:             "linux",
			expectedDigest: "sha256:5bf065699d2e6ddb8b5f7e30f02edc3cfe15d7400e7101b5b505d61fde01f84c",
		},
		{
			source:         image.PodmanDaemonSource,
			architecture:   "arm64",
			os:             "linux",
			expectedDigest: "sha256:19d689bc58fd64da6a46d46512ea965a12b6bfb5b030400e21bc0a04c4ff155e",
		},
		{
			source:         image.PodmanDaemonSource,
			architecture:   "amd64",
			os:             "linux",
			expectedDigest: "sha256:1c35c441208254cb7c3844ba95a96485388cef9ccc0646d562c7fc026e04c807",
		},
		{
			source:         image.ContainerdDaemonSource,
			architecture:   "arm64",
			os:             "linux",
			expectedDigest: "sha256:19d689bc58fd64da6a46d46512ea965a12b6bfb5b030400e21bc0a04c4ff155e",
		},
		{
			source:         image.ContainerdDaemonSource,
			architecture:   "amd64",
			os:             "linux",
			expectedDigest: "sha256:1c35c441208254cb7c3844ba95a96485388cef9ccc0646d562c7fc026e04c807",
		},
	}

	for _, tt := range tests {
		platform := fmt.Sprintf("%s/%s", tt.os, tt.architecture)
		t.Run(fmt.Sprintf("%s/%s", tt.source.String(), platform), func(t *testing.T) {
			if tt.expectedErr == nil {
				tt.expectedErr = require.NoError
			}
			platformOpt := stereoscope.WithPlatform(platform)
			img, err := stereoscope.GetImageFromSource(context.TODO(), imageName, tt.source, "", platformOpt)
			tt.expectedErr(t, err)
			require.NotNil(t, img)

			assertArchAndOs(t, img, tt.os, tt.architecture)
			assert.Equal(t, tt.expectedDigest, img.Metadata.ID)
		})
	}
}

func TestDigestThatNarrowsToOnePlatform(t *testing.T) {
	// This digest is busybox:1.31 on linux/s390x
	// Test assumes that the host running these tests _isn't_ linux/s390x, but the behavior
	// should be the same regardless.
	imageStrWithDigest := "busybox:1.31@sha256:91c15b1ba6f408a648be60f8c047ef79058f26fa640025f374281f31c8704387"
	tests := []struct {
		name   string
		source image.Source
	}{
		{
			name:   "docker",
			source: image.DockerDaemonSource,
		},
		{
			name:   "registry",
			source: image.OciRegistrySource,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := stereoscope.GetImageFromSource(context.TODO(), imageStrWithDigest, tt.source, "")
			assert.NoError(t, err)
			assertArchAndOs(t, img, "linux", "s390x")
		})
	}
}

func TestDefaultPlatformWithOciRegistry(t *testing.T) {
	img, err := stereoscope.GetImageFromSource(context.TODO(), "busybox:1.31", image.OciRegistrySource, "")
	require.NoError(t, err)
	assertArchAndOs(t, img, "linux", runtime.GOARCH)
}

func assertArchAndOs(t *testing.T, img *image.Image, os string, architecture string) {
	type platform struct {
		Architecture string `json:"architecture"`
		Os           string `json:"os"`
	}
	var got platform
	err := json.Unmarshal(img.Metadata.RawConfig, &got)
	require.NoError(t, err)
	assert.Equal(t, os, got.Os)
	assert.Equal(t, architecture, got.Architecture)
}
