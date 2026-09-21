//go:build containers_image_openpgp

package containerstorage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	digest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/copy"
	dockerarchive "go.podman.io/image/v5/docker/archive"
	storagetransport "go.podman.io/image/v5/storage"
	"go.podman.io/image/v5/types"
	"go.podman.io/storage"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

// newTestStore builds an isolated containers-storage store backed by the cross-platform "vfs" graph driver rooted under
// a temp directory. If the store cannot be created in the current environment the test is skipped rather than failed.
func newTestStore(t *testing.T) storage.Store {
	t.Helper()

	root := t.TempDir()
	runRoot := t.TempDir()

	store, err := storage.GetStore(storage.StoreOptions{
		GraphRoot:       root,
		RunRoot:         runRoot,
		GraphDriverName: "vfs",
	})
	if err != nil {
		t.Skipf("unable to create test containers-storage store in this environment: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.Shutdown(true)
	})
	return store
}

// newStoreImage builds a small random image and copies it into store under ref, so tests can exercise a real,
// populated containers-storage store without requiring buildah or podman to be installed.
func newStoreImage(t *testing.T, store storage.Store, ref string) {
	t.Helper()

	img, err := random.Image(512, 1)
	require.NoError(t, err)

	tag, err := name.NewTag(ref)
	require.NoError(t, err)

	archivePath := filepath.Join(t.TempDir(), "src.tar")
	require.NoError(t, tarball.WriteToFile(archivePath, tag, img))

	srcRef, err := dockerarchive.ParseReference(archivePath)
	require.NoError(t, err)

	destRef, err := storagetransport.Transport.ParseStoreReference(store, ref)
	require.NoError(t, err)

	policyContext, err := newInsecurePolicyContext()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, policyContext.Destroy()) })

	// containers/image otherwise stages big blobs under /var/tmp, which isn't writable in every test environment.
	sysCtx := &types.SystemContext{BigFilesTemporaryDir: t.TempDir()}
	_, err = copy.Image(context.Background(), policyContext, destRef, srcRef, &copy.Options{DestinationCtx: sysCtx})
	require.NoError(t, err)
}

func newTestProvider(imageStr string, platform *image.Platform) *containersStorageProvider {
	return &containersStorageProvider{
		tmpDirGen: file.NewTempDirGenerator("containers-storage-test"),
		imageStr:  imageStr,
		platform:  platform,
	}
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider(file.NewTempDirGenerator("test"), "localhost/myimage:latest", nil)
	assert.Equal(t, image.ContainersStorageSource, p.Name())
	assert.Equal(t, "containers-storage", p.Name())
}

func TestProvider_systemContext(t *testing.T) {
	t.Run("nil platform yields no platform choices", func(t *testing.T) {
		p := newTestProvider("localhost/myimage:latest", nil)
		sysCtx := p.systemContext("/tmp/big-files")
		require.NotNil(t, sysCtx)
		assert.Empty(t, sysCtx.OSChoice)
		assert.Empty(t, sysCtx.ArchitectureChoice)
		assert.Empty(t, sysCtx.VariantChoice)
		assert.Equal(t, "/tmp/big-files", sysCtx.BigFilesTemporaryDir)
	})

	t.Run("platform is passed through to the system context", func(t *testing.T) {
		p := newTestProvider("localhost/myimage:latest", &image.Platform{
			OS:           "linux",
			Architecture: "arm",
			Variant:      "v7",
		})
		sysCtx := p.systemContext("/tmp/big-files")
		require.NotNil(t, sysCtx)
		assert.Equal(t, "linux", sysCtx.OSChoice)
		assert.Equal(t, "arm", sysCtx.ArchitectureChoice)
		assert.Equal(t, "v7", sysCtx.VariantChoice)
	})
}

func TestNewInsecurePolicyContext(t *testing.T) {
	pc, err := newInsecurePolicyContext()
	require.NoError(t, err)
	require.NotNil(t, pc)
	require.NoError(t, pc.Destroy())
}

func TestProvider_provideFromStore_invalidReference(t *testing.T) {
	store := newTestStore(t)

	// a reference containing whitespace is not a valid image reference
	p := newTestProvider("not a valid reference", nil)

	img, err := p.provideFromStore(context.Background(), store)

	require.Error(t, err)
	assert.Nil(t, img)
	assert.ErrorContains(t, err, "invalid containers-storage reference")
}

func TestProvider_provideFromStore_missingImage(t *testing.T) {
	store := newTestStore(t)

	// a well-formed reference that is not present in the (empty) store
	p := newTestProvider("localhost/does-not-exist:latest", nil)

	img, err := p.provideFromStore(context.Background(), store)

	// a non-nil error with a nil image is what allows source auto-resolution to fall through to the next provider
	require.Error(t, err)
	assert.Nil(t, img)
}

func TestProvider_provideFromStore_success(t *testing.T) {
	store := newTestStore(t)
	newStoreImage(t, store, "localhost/myimage:latest")

	// deliberately give a bare, untagged reference: the store only knows the image by its normalized name
	// ("localhost/myimage:latest"), so this also exercises tagAndDigestMetadata resolving via the normalized
	// srcRef rather than a raw string match against p.imageStr.
	p := newTestProvider("localhost/myimage", nil)

	img, err := p.provideFromStore(context.Background(), store)
	require.NoError(t, err)
	require.NotNil(t, img)
	t.Cleanup(func() {
		require.NoError(t, img.Cleanup())
	})

	var tags []string
	for _, tag := range img.Metadata.Tags {
		tags = append(tags, tag.String())
	}
	assert.Contains(t, tags, "localhost/myimage:latest")
	assert.NotEmpty(t, img.Metadata.RepoDigests)
}

func TestProvider_additionalMetadata_missingImage(t *testing.T) {
	store := newTestStore(t)

	srcRef, err := storagetransport.Transport.ParseStoreReference(store, "localhost/does-not-exist:latest")
	require.NoError(t, err)

	p := newTestProvider("localhost/does-not-exist:latest", nil)

	// when the image is absent, resolving tags/digests and inspecting the config both fail; this must be
	// non-fatal and yield no metadata
	require.NotPanics(t, func() {
		md := p.additionalMetadata(context.Background(), srcRef, p.systemContext(t.TempDir()))
		assert.Empty(t, md)
	})
}

func Test_repoDigests(t *testing.T) {
	digestStr := "sha256:" + strings.Repeat("a", 64)

	t.Run("no digest yields no repo digests", func(t *testing.T) {
		assert.Empty(t, repoDigests(&storage.Image{Names: []string{"localhost/myimage:latest"}}))
	})

	t.Run("pairs each name with the canonical digest", func(t *testing.T) {
		got := repoDigests(&storage.Image{
			Names:  []string{"localhost/myimage:latest", "localhost/myimage:v1"},
			Digest: digest.Digest(digestStr),
		})
		assert.Equal(t, []string{
			"localhost/myimage@" + digestStr,
			"localhost/myimage@" + digestStr,
		}, got)
	})

	t.Run("unparsable names are skipped, not fatal", func(t *testing.T) {
		got := repoDigests(&storage.Image{
			Names:  []string{"not a valid reference"},
			Digest: digest.Digest(digestStr),
		})
		assert.Empty(t, got)
	})
}
