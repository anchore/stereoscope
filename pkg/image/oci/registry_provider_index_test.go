package oci

import (
	"context"
	"crypto/sha256"
	"fmt"
	"runtime"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

// randomPlatformImage is a small image whose config claims the given platform, as the providers check the
// config rather than trust the index descriptor.
func randomPlatformImage(t *testing.T, os, arch string) v1.Image {
	t.Helper()

	img, err := random.Image(256, 1)
	require.NoError(t, err)

	cfg, err := img.ConfigFile()
	require.NoError(t, err)
	cfg.OS = os
	cfg.Architecture = arch

	img, err = mutate.ConfigFile(img, cfg)
	require.NoError(t, err)
	return img
}

// otherArch is an architecture that is never the host's, so that a platform has to be picked from an index.
func otherArch() string {
	if runtime.GOARCH == "s390x" {
		return "ppc64le"
	}
	return "s390x"
}

// platformIndex is an index of the given media type holding one image for the host architecture and one for
// another, returning the index along with the host's image.
func platformIndex(t *testing.T, mediaType v1Types.MediaType) (v1.ImageIndex, v1.Image) {
	t.Helper()

	hostImg := randomPlatformImage(t, "linux", runtime.GOARCH)
	otherImg := randomPlatformImage(t, "linux", otherArch())

	index := mutate.AppendManifests(mutate.IndexMediaType(empty.Index, mediaType),
		mutate.IndexAddendum{
			Add:        otherImg,
			Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: otherArch()}},
		},
		mutate.IndexAddendum{
			Add:        hostImg,
			Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: runtime.GOARCH}},
		},
	)
	return index, hostImg
}

func Test_RegistryProvider_retainsIndex(t *testing.T) {
	registryHost := makeRegistry(t)
	const repo = "retain-index"

	nameOpts := []name.Option{name.Insecure}

	pushIndex := func(t *testing.T, tag string, mediaType v1Types.MediaType) (v1.ImageIndex, v1.Image) {
		t.Helper()
		index, hostImg := platformIndex(t, mediaType)
		ref, err := name.ParseReference(fmt.Sprintf("%s/%s:%s", registryHost, repo, tag), nameOpts...)
		require.NoError(t, err)
		require.NoError(t, remote.WriteIndex(ref, index))
		return index, hostImg
	}

	ociIndex, ociHostImg := pushIndex(t, "oci", v1Types.OCIImageIndex)
	dockerIndex, dockerHostImg := pushIndex(t, "docker", v1Types.DockerManifestList)

	singleImg := randomPlatformImage(t, "linux", runtime.GOARCH)
	singleRef, err := name.ParseReference(fmt.Sprintf("%s/%s:single", registryHost, repo), nameOpts...)
	require.NoError(t, err)
	require.NoError(t, remote.Write(singleRef, singleImg))

	digestOf := func(t *testing.T, d interface{ Digest() (v1.Hash, error) }) string {
		t.Helper()
		h, err := d.Digest()
		require.NoError(t, err)
		return h.String()
	}

	tests := []struct {
		name string
		// ref is what follows the repository in the reference handed to the provider
		ref      string
		platform *image.Platform
		// index is the index the reference resolves to, nil when it names an image manifest directly
		index v1.ImageIndex
		// img is the image that should be provided
		img v1.Image
	}{
		{
			name:  "tag of an OCI index",
			ref:   ":oci",
			index: ociIndex,
			img:   ociHostImg,
		},
		{
			name:     "tag of an OCI index with an explicit platform",
			ref:      ":oci",
			platform: &image.Platform{OS: "linux", Architecture: runtime.GOARCH},
			index:    ociIndex,
			img:      ociHostImg,
		},
		{
			name:  "tag of a docker manifest list",
			ref:   ":docker",
			index: dockerIndex,
			img:   dockerHostImg,
		},
		{
			name:  "digest of an index",
			ref:   "@" + digestOf(t, ociIndex),
			index: ociIndex,
			img:   ociHostImg,
		},
		{
			name: "tag of a single manifest",
			ref:  ":single",
			img:  singleImg,
		},
		{
			// a child named by its own digest was not selected from the index, even though one holds it
			name: "digest of a manifest that an index holds",
			ref:  "@" + digestOf(t, ociHostImg),
			img:  ociHostImg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := file.TempDirGenerator{}
			t.Cleanup(func() { _ = generator.Cleanup() })

			provider := NewRegistryProvider(&generator, image.RegistryOptions{InsecureUseHTTP: true},
				fmt.Sprintf("%s/%s%s", registryHost, repo, tt.ref), tt.platform)

			img, err := provider.Provide(context.Background())
			require.NoError(t, err)
			require.NotNil(t, img)
			t.Cleanup(func() { _ = img.Cleanup() })

			// the fields that describe the selected image hold what they always have
			wantManifest, err := tt.img.RawManifest()
			require.NoError(t, err)
			wantMediaType, err := tt.img.MediaType()
			require.NoError(t, err)
			wantConfigName, err := tt.img.ConfigName()
			require.NoError(t, err)

			assert.Equal(t, wantManifest, img.Metadata.RawManifest)
			assert.Equal(t, digestOf(t, tt.img), img.Metadata.ManifestDigest)
			assert.Equal(t, wantMediaType, img.Metadata.MediaType)
			assert.Equal(t, wantConfigName.String(), img.Metadata.ID)
			assert.Equal(t, "linux", img.Metadata.OS)
			assert.Equal(t, runtime.GOARCH, img.Metadata.Architecture)

			if tt.index == nil {
				assert.Empty(t, img.Metadata.RawIndex)
				assert.Empty(t, img.Metadata.IndexDigest)
				assert.Empty(t, img.Metadata.IndexMediaType)
				assert.Equal(t, []string{fmt.Sprintf("%s/%s@%s", registryHost, repo, digestOf(t, tt.img))}, img.Metadata.RepoDigests)
				return
			}

			wantIndex, err := tt.index.RawManifest()
			require.NoError(t, err)
			wantIndexMediaType, err := tt.index.MediaType()
			require.NoError(t, err)
			wantIndexDigest := digestOf(t, tt.index)

			assert.Equal(t, wantIndex, img.Metadata.RawIndex, "the index must be byte for byte what the registry served")
			assert.Equal(t, wantIndexDigest, img.Metadata.IndexDigest)
			assert.Equal(t, fmt.Sprintf("sha256:%x", sha256.Sum256(img.Metadata.RawIndex)), img.Metadata.IndexDigest)
			assert.Equal(t, wantIndexMediaType, img.Metadata.IndexMediaType)

			assert.NotEqual(t, img.Metadata.IndexDigest, img.Metadata.ManifestDigest)
			assert.Equal(t, []string{fmt.Sprintf("%s/%s@%s", registryHost, repo, wantIndexDigest)}, img.Metadata.RepoDigests)
		})
	}
}
