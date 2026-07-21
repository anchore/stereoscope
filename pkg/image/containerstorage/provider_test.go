//go:build containers_image_openpgp

package containerstorage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	storagetransport "go.podman.io/image/v5/storage"
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
		sysCtx := p.systemContext()
		require.NotNil(t, sysCtx)
		assert.Empty(t, sysCtx.OSChoice)
		assert.Empty(t, sysCtx.ArchitectureChoice)
		assert.Empty(t, sysCtx.VariantChoice)
	})

	t.Run("platform is passed through to the system context", func(t *testing.T) {
		p := newTestProvider("localhost/myimage:latest", &image.Platform{
			OS:           "linux",
			Architecture: "arm",
			Variant:      "v7",
		})
		sysCtx := p.systemContext()
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
	assert.ErrorContains(t, err, "unable to resolve image from containers-storage")
	assert.ErrorContains(t, err, "invalid reference")
}

func TestProvider_provideFromStore_missingImage(t *testing.T) {
	store := newTestStore(t)

	// a well-formed reference that is not present in the (empty) store
	p := newTestProvider("localhost/does-not-exist:latest", nil)

	img, err := p.provideFromStore(context.Background(), store)

	// a non-nil error with a nil image is what allows source auto-resolution to fall through to the next provider
	require.Error(t, err)
	assert.Nil(t, img)
	assert.ErrorContains(t, err, "unable to resolve image from containers-storage")
}

func TestProvider_additionalMetadata_missingImage(t *testing.T) {
	store := newTestStore(t)

	srcRef, err := storagetransport.Transport.ParseStoreReference(store, "localhost/does-not-exist:latest")
	require.NoError(t, err)

	p := newTestProvider("localhost/does-not-exist:latest", nil)

	// when the image is absent the store lookup and inspect both fail; this must be non-fatal and yield no metadata
	require.NotPanics(t, func() {
		md := p.additionalMetadata(context.Background(), store, srcRef)
		assert.Empty(t, md)
	})
}
