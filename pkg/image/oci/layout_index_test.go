package oci

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	v1Types "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/internal/testutil"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

// tarDirectory writes the directory as an uncompressed tar, the shape of an oci-archive.
func tarDirectory(t *testing.T, dir string) string {
	t.Helper()

	archivePath := filepath.Join(t.TempDir(), "layout.tar")
	f, err := os.Create(archivePath)
	require.NoError(t, err)
	defer f.Close()

	tw := tar.NewWriter(f)
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == "." {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		blob, err := os.Open(path)
		if err != nil {
			return err
		}
		defer blob.Close()
		_, err = io.Copy(tw, blob)
		return err
	}))
	require.NoError(t, tw.Close())
	return archivePath
}

func Test_LayoutProviders_index(t *testing.T) {
	hostPlatform := v1.Platform{OS: "linux", Architecture: runtime.GOARCH}
	otherPlatform := v1.Platform{OS: "linux", Architecture: otherArch()}

	digestOf := func(t *testing.T, d interface{ Digest() (v1.Hash, error) }) v1.Hash {
		t.Helper()
		h, err := d.Digest()
		require.NoError(t, err)
		return h
	}

	newLayout := func(t *testing.T) layout.Path {
		t.Helper()
		p, err := layout.Write(t.TempDir(), empty.Index)
		require.NoError(t, err)
		return p
	}

	tests := []struct {
		name string
		// setup writes the layout and returns the image that should be selected, along with the index that
		// should be reported for it (nil for none)
		setup func(t *testing.T, p layout.Path) (v1.Image, v1.ImageIndex)
	}{
		{
			// what `skopeo copy --all` and buildx write for a multi-platform image
			name: "an index nested in the layout index",
			setup: func(t *testing.T, p layout.Path) (v1.Image, v1.ImageIndex) {
				index, hostImg := platformIndex(t, v1Types.OCIImageIndex)
				require.NoError(t, p.AppendIndex(index))
				return hostImg, index
			},
		},
		{
			name: "a docker manifest list nested in the layout index",
			setup: func(t *testing.T, p layout.Path) (v1.Image, v1.ImageIndex) {
				index, hostImg := platformIndex(t, v1Types.DockerManifestList)
				require.NoError(t, p.AppendIndex(index))
				return hostImg, index
			},
		},
		{
			name: "the same nested index under two names",
			setup: func(t *testing.T, p layout.Path) (v1.Image, v1.ImageIndex) {
				index, hostImg := platformIndex(t, v1Types.OCIImageIndex)
				require.NoError(t, p.AppendIndex(index, layout.WithAnnotations(map[string]string{"org.opencontainers.image.ref.name": "1.0"})))
				require.NoError(t, p.AppendIndex(index, layout.WithAnnotations(map[string]string{"org.opencontainers.image.ref.name": "latest"})))
				return hostImg, index
			},
		},
		{
			// the index named by the layout is the one a reference would resolve to, not the one below it
			name: "an index nested two deep",
			setup: func(t *testing.T, p layout.Path) (v1.Image, v1.ImageIndex) {
				inner, hostImg := platformIndex(t, v1Types.OCIImageIndex)
				outer := mutate.AppendManifests(mutate.IndexMediaType(empty.Index, v1Types.OCIImageIndex), mutate.IndexAddendum{Add: inner})
				require.NoError(t, p.AppendIndex(outer))
				return hostImg, outer
			},
		},
		{
			name: "a single image listed by the layout index",
			setup: func(t *testing.T, p layout.Path) (v1.Image, v1.ImageIndex) {
				hostImg := randomPlatformImage(t, "linux", runtime.GOARCH)
				require.NoError(t, p.AppendImage(hostImg))
				return hostImg, nil
			},
		},
		{
			// the layout index selects the platform here, but it is not an index of this image: it is not
			// content addressed and no registry would serve it
			name: "platform images listed by the layout index itself",
			setup: func(t *testing.T, p layout.Path) (v1.Image, v1.ImageIndex) {
				hostImg := randomPlatformImage(t, "linux", runtime.GOARCH)
				otherImg := randomPlatformImage(t, "linux", otherArch())
				require.NoError(t, p.AppendImage(otherImg, layout.WithPlatform(otherPlatform)))
				require.NoError(t, p.AppendImage(hostImg, layout.WithPlatform(hostPlatform)))
				return hostImg, nil
			},
		},
		{
			name: "an image both nested in an index and listed by the layout index",
			setup: func(t *testing.T, p layout.Path) (v1.Image, v1.ImageIndex) {
				index, hostImg := platformIndex(t, v1Types.OCIImageIndex)
				require.NoError(t, p.AppendIndex(index))
				require.NoError(t, p.AppendImage(hostImg, layout.WithPlatform(hostPlatform)))
				return hostImg, nil
			},
		},
		{
			name: "an image shared by two nested indexes",
			setup: func(t *testing.T, p layout.Path) (v1.Image, v1.ImageIndex) {
				index, hostImg := platformIndex(t, v1Types.OCIImageIndex)
				second := mutate.AppendManifests(mutate.IndexMediaType(empty.Index, v1Types.OCIImageIndex),
					mutate.IndexAddendum{Add: hostImg, Descriptor: v1.Descriptor{Platform: &hostPlatform}})
				require.NoError(t, p.AppendIndex(index))
				require.NoError(t, p.AppendIndex(second))
				return hostImg, nil
			},
		},
		{
			// the digest reported is taken from the bytes, so bytes that are not the named index are not reported
			name: "a nested index that does not match its digest",
			setup: func(t *testing.T, p layout.Path) (v1.Image, v1.ImageIndex) {
				index, hostImg := platformIndex(t, v1Types.OCIImageIndex)
				require.NoError(t, p.AppendIndex(index))

				blobPath := filepath.Join(string(p), "blobs", "sha256", digestOf(t, index).Hex)
				raw, err := os.ReadFile(blobPath)
				require.NoError(t, err)
				require.NoError(t, os.Chmod(blobPath, 0o600))
				require.NoError(t, os.WriteFile(blobPath, append(raw, '\n'), 0o600))
				return hostImg, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layoutPath := newLayout(t)
			wantImg, wantIndex := tt.setup(t, layoutPath)

			providers := map[string]func(*file.TempDirGenerator) image.Provider{
				"oci-dir": func(g *file.TempDirGenerator) image.Provider {
					return NewDirectoryProvider(g, string(layoutPath))
				},
				"oci-archive": func(g *file.TempDirGenerator) image.Provider {
					return NewArchiveProvider(g, tarDirectory(t, string(layoutPath)))
				},
			}

			for providerName, newProvider := range providers {
				t.Run(providerName, func(t *testing.T) {
					generator := file.NewTempDirGenerator("layout-index")
					t.Cleanup(func() { _ = generator.Cleanup() })

					img, err := newProvider(generator).Provide(context.Background())
					require.NoError(t, err)
					require.NotNil(t, img)
					t.Cleanup(func() { _ = img.Cleanup() })

					// the fields that describe the selected image hold what they always have
					wantManifest, err := wantImg.RawManifest()
					require.NoError(t, err)
					wantMediaType, err := wantImg.MediaType()
					require.NoError(t, err)

					assert.Equal(t, wantManifest, img.Metadata.RawManifest)
					assert.Equal(t, digestOf(t, wantImg).String(), img.Metadata.ManifestDigest)
					assert.Equal(t, wantMediaType, img.Metadata.MediaType)
					assert.Empty(t, img.Metadata.RepoDigests)

					if wantIndex == nil {
						assert.Empty(t, img.Metadata.RawIndex)
						assert.Empty(t, img.Metadata.IndexDigest)
						assert.Empty(t, img.Metadata.IndexMediaType)
						return
					}

					wantRawIndex, err := wantIndex.RawManifest()
					require.NoError(t, err)
					wantIndexMediaType, err := wantIndex.MediaType()
					require.NoError(t, err)

					assert.Equal(t, wantRawIndex, img.Metadata.RawIndex)
					assert.Equal(t, digestOf(t, wantIndex).String(), img.Metadata.IndexDigest)
					assert.Equal(t, fmt.Sprintf("sha256:%x", sha256.Sum256(img.Metadata.RawIndex)), img.Metadata.IndexDigest)
					assert.Equal(t, wantIndexMediaType, img.Metadata.IndexMediaType)
				})
			}
		})
	}
}

func Test_LayoutProviders_index_fixtures(t *testing.T) {
	tests := []struct {
		name            string
		provider        func(*testing.T, *file.TempDirGenerator) image.Provider
		wantIndexDigest string
	}{
		{
			// index.json names one nested index, which holds the amd64 and arm64 images
			name: "oci-dir written by containerd for a multi-platform image",
			provider: func(t *testing.T, g *file.TempDirGenerator) image.Provider {
				return NewDirectoryProvider(g, testutil.GetFixturePath(t, "multiplatform_oci_dir"))
			},
			wantIndexDigest: "sha256:a1a8d08faa5fe3d2ccc0ba54874e39dede9a17120f883c5cb56f2fd9309067c8",
		},
		{
			name: "oci-dir of a single image",
			provider: func(t *testing.T, g *file.TempDirGenerator) image.Provider {
				return NewDirectoryProvider(g, testutil.GetFixturePath(t, "valid_oci_dir"))
			},
		},
		{
			name: "oci-archive of a single image",
			provider: func(t *testing.T, g *file.TempDirGenerator) image.Provider {
				return NewArchiveProvider(g, testutil.GetFixturePath(t, "valid-oci.tar"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantIndexDigest != "" && runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
				t.Skipf("unsupported architecture for test: %s", runtime.GOARCH)
			}
			generator := file.NewTempDirGenerator("layout-index")
			t.Cleanup(func() { _ = generator.Cleanup() })

			img, err := tt.provider(t, generator).Provide(context.Background())
			require.NoError(t, err)
			require.NotNil(t, img)
			t.Cleanup(func() { _ = img.Cleanup() })

			assert.Equal(t, tt.wantIndexDigest, img.Metadata.IndexDigest)
			if tt.wantIndexDigest == "" {
				assert.Empty(t, img.Metadata.RawIndex)
				assert.Empty(t, img.Metadata.IndexMediaType)
				return
			}
			assert.Equal(t, fmt.Sprintf("sha256:%x", sha256.Sum256(img.Metadata.RawIndex)), img.Metadata.IndexDigest)
			assert.Equal(t, v1Types.OCIImageIndex, img.Metadata.IndexMediaType)
		})
	}
}
