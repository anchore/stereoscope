//go:build containers_image_openpgp

// Package containerstorage provides an image.Provider that resolves images from the local containers-storage store
// (as populated by buildah and rootless/rootful podman). It is only compiled when the containers_image_openpgp build
// tag is set, since it depends on the containers/image and containers/storage libraries. Without that tag a stub
// provider is used (see provider_stub.go) that reports the feature is not compiled in.
package containerstorage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/containers/image/v5/copy"
	dockerarchive "github.com/containers/image/v5/docker/archive"
	"github.com/containers/image/v5/signature"
	storagetransport "github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/types"

	// NOTE: github.com/containers/storage has moved to go.podman.io/storage as part of the containers monorepo
	// migration (https://blog.podman.io/2025/08/upcoming-migration-of-three-containers-repositories-to-monorepo/).
	// We intentionally stay on github.com/containers/storage because the released containers/image/v5 (v5.36.2) still
	// imports it; the storage transport above (containers/image/v5/storage) expects a storage.Store from this exact
	// module. Switching to go.podman.io/storage now would pull two incompatible storage modules into the build graph.
	// When containers/image ships a release that imports go.podman.io/storage, bump both together and update this import.
	"github.com/containers/storage"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/image/docker"
)

// Source is the image.Source string used to explicitly select this provider (e.g. "containers-storage:localhost/myimage:latest").
const Source image.Source = image.ContainersStorageSource

// NewProvider creates a new provider able to resolve images from the current user's default containers-storage store.
// This is the store typically populated by buildah and rootless podman (e.g. ~/.local/share/containers/storage for
// rootless users, /var/lib/containers/storage for root). The provider relies on the containers/storage default store
// configuration for the current process/user; it does not probe alternate storage locations.
func NewProvider(tmpDirGen *file.TempDirGenerator, imageStr string, platform *image.Platform) image.Provider {
	return &containersStorageProvider{
		tmpDirGen: tmpDirGen,
		imageStr:  imageStr,
		platform:  platform,
	}
}

// containersStorageProvider is an image.Provider capable of resolving an image from the local containers-storage store
// by copying it into a temporary docker-archive and delegating to the docker archive provider.
type containersStorageProvider struct {
	tmpDirGen *file.TempDirGenerator
	imageStr  string
	platform  *image.Platform
}

func (p *containersStorageProvider) Name() string {
	return Source
}

// Provide resolves the configured image reference from the current user's default containers-storage store. When the
// image is not present (or the store is unavailable) a non-nil error is returned so that source auto-resolution can
// continue to the next provider (e.g. the OCI registry).
func (p *containersStorageProvider) Provide(ctx context.Context) (*image.Image, error) {
	store, err := openDefaultStore()
	if err != nil {
		return nil, err
	}
	return p.provideFromStore(ctx, store)
}

// provideFromStore performs the actual resolution against the given containers-storage store: it copies the image to a
// temporary docker-archive and reuses the docker archive provider to construct the final stereoscope image. It is
// separated from Provide so that store construction can be controlled directly in tests.
func (p *containersStorageProvider) provideFromStore(ctx context.Context, store storage.Store) (*image.Image, error) {
	startTime := time.Now()

	srcRef, err := storagetransport.Transport.ParseStoreReference(store, p.imageStr)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve image from containers-storage: invalid reference %q: %w", p.imageStr, err)
	}

	tempDir, err := p.tmpDirGen.NewDirectory("containers-storage-image")
	if err != nil {
		return nil, err
	}

	archivePath := filepath.Join(tempDir, "image.tar")
	destRef, err := dockerarchive.ParseReference(archivePath)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve image from containers-storage: invalid archive destination %q: %w", archivePath, err)
	}

	policyContext, err := newInsecurePolicyContext()
	if err != nil {
		return nil, fmt.Errorf("unable to resolve image from containers-storage: %w", err)
	}
	defer func() {
		if closeErr := policyContext.Destroy(); closeErr != nil {
			log.Debugf("failed to destroy containers-storage policy context: %v", closeErr)
		}
	}()

	log.WithFields("image", p.imageStr, "archive", archivePath).Trace("copying image from containers-storage to docker archive")

	if _, err := copy.Image(ctx, policyContext, destRef, srcRef, &copy.Options{
		SourceCtx: p.systemContext(),
	}); err != nil {
		return nil, fmt.Errorf("unable to resolve image from containers-storage: %w", err)
	}

	log.WithFields("image", p.imageStr, "time", time.Since(startTime)).Debug("copied image from containers-storage")

	// reuse the existing docker archive provider to construct the final stereoscope image from the generated tar
	return docker.NewArchiveProvider(p.tmpDirGen, archivePath).Provide(ctx)
}

// systemContext builds a containers/image SystemContext carrying the requested platform selection (if any) so that
// multi-arch images stored locally resolve to the requested OS/architecture/variant.
func (p *containersStorageProvider) systemContext() *types.SystemContext {
	sysCtx := &types.SystemContext{}
	if p.platform != nil {
		sysCtx.OSChoice = p.platform.OS
		sysCtx.ArchitectureChoice = p.platform.Architecture
		sysCtx.VariantChoice = p.platform.Variant
	}
	return sysCtx
}

// openDefaultStore opens the containers-storage store described by the default configuration for the current
// process/user. Errors are wrapped so explicit usage surfaces actionable messages (e.g. permission denied) while
// still allowing auto-resolution to fall through to the next provider.
func openDefaultStore() (storage.Store, error) {
	storeOptions, err := storage.DefaultStoreOptions()
	if err != nil {
		return nil, fmt.Errorf("unable to resolve image from containers-storage: failed to load default store options: %w", err)
	}

	store, err := storage.GetStore(storeOptions)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve image from containers-storage: failed to open store: %w", err)
	}
	return store, nil
}

// newInsecurePolicyContext returns a policy context that accepts any image. This is appropriate because copying an
// image out of the local containers-storage store is a local data transfer, not a trust-validation operation.
func newInsecurePolicyContext() (*signature.PolicyContext, error) {
	policy := &signature.Policy{
		Default: []signature.PolicyRequirement{
			signature.NewPRInsecureAcceptAnything(),
		},
	}
	pc, err := signature.NewPolicyContext(policy)
	if err != nil {
		return nil, errors.Join(errors.New("failed to create policy context"), err)
	}
	return pc, nil
}
