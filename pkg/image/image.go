package image

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/scylladb/go-set/strset"
	"github.com/wagoodman/go-partybus"
	"github.com/wagoodman/go-progress"

	async "github.com/anchore/go-sync"
	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/event"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
)

// Image represents a container image.
type Image struct {
	// image is the raw image metadata and content provider from the GCR lib
	image v1.Image
	// tmpDirGen is a dir generator used by Providers. Multiple directories may
	// be created and cleanup must use this to prevent polluting the disk
	tmpDirGen *file.TempDirGenerator
	// contentCacheDir is where all layer tar cache is stored.
	contentCacheDir string
	// Metadata contains select image attributes
	Metadata Metadata
	// Layers contains the rich layer objects in build order
	Layers []*Layer
	// FileCatalog contains all file metadata for all files in all layers
	FileCatalog FileCatalogReader

	SquashedSearchContext filetree.Searcher

	overrideMetadata []AdditionalMetadata
}

type AdditionalMetadata func(*Image) error

func WithTags(tags ...string) AdditionalMetadata {
	return func(image *Image) error {
		existingTags := strset.New()
		for _, t := range image.Metadata.Tags {
			existingTags.Add(t.String())
		}

		for _, t := range tags {
			// it is possible that we are given references that have both a tag and a digest (or only one)
			// we should only be allowing tags (stripping off digests if they are present)
			fields := strings.Split(t, "@")
			withNoDigest := fields[0]
			if !strings.Contains(withNoDigest, ":") {
				continue
			}
			tagObj, err := name.NewTag(withNoDigest)
			if err != nil {
				log.Warnf("unable to parse additional image tag to add %q: %+v", t, err)
				continue
			}
			if !existingTags.Has(tagObj.String()) {
				image.Metadata.Tags = append(image.Metadata.Tags, tagObj)
			}
		}
		return nil
	}
}

func WithManifest(manifest []byte) AdditionalMetadata {
	return func(image *Image) error {
		image.Metadata.RawManifest = manifest
		image.Metadata.ManifestDigest = fmt.Sprintf("sha256:%x", sha256.Sum256(manifest))
		return nil
	}
}

func WithManifestDigest(digest string) AdditionalMetadata {
	return func(image *Image) error {
		image.Metadata.ManifestDigest = digest
		return nil
	}
}

func WithConfig(config []byte) AdditionalMetadata {
	return func(image *Image) error {
		image.Metadata.RawConfig = config
		image.Metadata.ID = fmt.Sprintf("sha256:%x", sha256.Sum256(config))
		return nil
	}
}

func WithRepoDigests(digests ...string) AdditionalMetadata {
	return func(image *Image) error {
		image.Metadata.RepoDigests = append(image.Metadata.RepoDigests, digests...)
		return nil
	}
}

func WithPlatform(platform string) AdditionalMetadata {
	return func(image *Image) error {
		p, err := NewPlatform(platform)
		if err != nil {
			return err
		}
		image.Metadata.Architecture = p.Architecture
		image.Metadata.Variant = p.Variant
		image.Metadata.OS = p.OS
		return nil
	}
}

func WithArchitecture(architecture, variant string) AdditionalMetadata {
	return func(image *Image) error {
		if architecture == "" {
			return nil
		}
		if !isKnownArch(architecture) {
			return fmt.Errorf("unknown architecture: %s", architecture)
		}
		image.Metadata.Architecture = architecture
		image.Metadata.Variant = variant
		return nil
	}
}

func WithOS(o string) AdditionalMetadata {
	return func(image *Image) error {
		if o == "" {
			return nil
		}
		if !isKnownOS(o) {
			return fmt.Errorf("unknown OS: %s", o)
		}
		image.Metadata.OS = o
		return nil
	}
}

// NewImage provides a new (unread) image object.
//
// Deprecated: use New() instead
func NewImage(image v1.Image, tmpDirGen *file.TempDirGenerator, contentCacheDir string, additionalMetadata ...AdditionalMetadata) *Image {
	return New(image, tmpDirGen, contentCacheDir, additionalMetadata...)
}

// New provides a new (unread) image object.
func New(image v1.Image, tmpDirGen *file.TempDirGenerator, contentCacheDir string, additionalMetadata ...AdditionalMetadata) *Image {
	imgObj := &Image{
		image:            image,
		tmpDirGen:        tmpDirGen,
		contentCacheDir:  contentCacheDir,
		overrideMetadata: additionalMetadata,
	}
	return imgObj
}

func (i *Image) IDs() []string {
	var ids = make([]string, len(i.Metadata.Tags))
	for idx, t := range i.Metadata.Tags {
		ids[idx] = t.String()
	}
	ids = append(ids, i.Metadata.ID)
	return ids
}

func (i *Image) trackReadProgress(metadata Metadata) *progress.Manual {
	prog := progress.NewManual(
		// x2 for read and squash of each layer
		int64(len(metadata.Config.RootFS.DiffIDs) * 2),
	)

	bus.Publish(partybus.Event{
		Type:   event.ReadImage,
		Source: metadata,
		Value:  progress.Progressable(prog),
	})

	return prog
}

func (i *Image) applyOverrideMetadata() error {
	for _, optionFn := range i.overrideMetadata {
		if err := optionFn(i); err != nil {
			return fmt.Errorf("unable to override metadata option: %w", err)
		}
	}
	return nil
}

// Read parses information from the underlying image tar into this struct. This includes image metadata, layer
// metadata, layer file trees, and layer squash trees (which implies the image squash tree).
//
// The context bounds the read: cancelling it abandons work that has not started and stops the
// layer pools from picking up more.
func (i *Image) Read(ctx context.Context) error {
	var err error
	i.Metadata, err = readImageMetadata(i.image)
	if err != nil {
		return err
	}

	// override any metadata with what the user has provided manually
	if err = i.applyOverrideMetadata(); err != nil {
		return err
	}

	startTime := time.Now()
	lapTime := startTime

	v1Layers, err := i.image.Layers()
	if err != nil {
		return err
	}

	// validate all layer media types before processing
	if err := validateLayerMediaTypes(v1Layers); err != nil {
		return err
	}

	log.WithFields("digest", i.Metadata.ID, "mediaType", i.Metadata.MediaType, "tags", i.Metadata.Tags).Debug("reading image")

	// let consumers know of a monitorable event (image save + copy stages)
	readProg := i.trackReadProgress(i.Metadata)
	// deferred rather than completed at the end of the squash: every early return between here and
	// there used to leave a consumer's bar stuck at whatever it had reached
	defer readProg.SetCompleted()

	fileCatalog := NewFileCatalog()

	// this rebuilds every layer, so release what a previous Read left open. Deferred to here rather
	// than the top of Read so that a failure before this point leaves the existing layers usable
	for _, closeErr := range i.closeLayers() {
		log.WithFields("error", closeErr).Trace("unable to release a layer tar from a previous read")
	}
	i.Layers = nil

	// the config already records every layer's diff ID, which saves each layer computing its own
	// (for an OCI layout that means decompressing the entire layer just to hash it). When the
	// config does not list exactly one per layer we cannot line them up, so let each layer answer.
	diffIDs := i.Metadata.Config.RootFS.DiffIDs
	if len(diffIDs) != len(v1Layers) {
		diffIDs = nil
	}

	// fetch, index and squash all run concurrently from here; see readLayers for how the stages
	// are bounded and squashLayers for why consuming in manifest order keeps it correct
	layers := make([]*Layer, len(v1Layers))
	for idx, v1Layer := range v1Layers {
		var knownDiffID string
		if diffIDs != nil {
			knownDiffID = diffIDs[idx].String()
		}
		layers[idx] = newLayer(v1Layer, knownDiffID)
	}

	// squash runs alongside the read stages rather than after them. It consumes layers in
	// manifest order, blocking on each layer's gate until that layer is indexed, so layer 0 is
	// being squashed while layer 3 is still downloading.
	gates := newLayerGates(len(layers))
	squashDone := make(chan error, 1)
	// capture what the goroutine logs, and send last: once Read receives from squashDone it may
	// return, and a caller is free to call Read again, which rewrites i.Metadata
	squashDigest := i.Metadata.ID
	go func() {
		squashStart := time.Now()
		err := i.squashLayers(layers, gates, readProg)
		log.WithFields("digest", squashDigest, "time", time.Since(squashStart)).Trace("completed image squash")
		squashDone <- err
	}()

	readErr := i.readLayers(ctx, layers, fileCatalog, readProg, gates)

	// release any layer the read never reached, so squash cannot block on a gate that will
	// never open (a cancelled context stops the stages from starting queued work)
	gates.releaseAll()
	squashErr := <-squashDone

	// i.Layers is not assigned until every one of these succeeds, so each failure releases the
	// local slice rather than i.Layers - the caller has an error and may never reach Cleanup, and
	// a half-built layer set must not be left visible either: accessors like SquashedTree read the
	// last layer, which here was never squashed.
	if readErr != nil {
		for _, closeErr := range closeLayers(layers) {
			log.WithFields("error", closeErr).Trace("unable to release a layer tar after a failed read")
		}
		return readErr
	}
	if err := ctx.Err(); err != nil {
		// checked before squashErr because cancellation is the root cause a caller wants to see:
		// the stages stop starting queued work, so neither raises a per-layer failure and the
		// squash just finds layers that were never indexed
		for _, closeErr := range closeLayers(layers) {
			log.WithFields("error", closeErr).Trace("unable to release a layer tar after a cancelled read")
		}
		return fmt.Errorf("unable to read image: %w", err)
	}
	if squashErr != nil {
		for _, closeErr := range closeLayers(layers) {
			log.WithFields("error", closeErr).Trace("unable to release a layer tar after a failed squash")
		}
		return squashErr
	}
	i.Layers = layers

	log.WithFields("digest", i.Metadata.ID, "time", time.Since(lapTime)).Trace("completed image layer read and squash")
	lapTime = time.Now()

	i.FileCatalog = fileCatalog
	// the top layer's squash IS the image squash, and squashLayers already built a search context
	// over that same tree and index. Rebuilding it here walked every symlink and hardlink in the
	// whole catalog a second time, which with the squash now overlapped was the largest piece of
	// serial work left in Read.
	if len(i.Layers) > 0 {
		i.SquashedSearchContext = i.Layers[len(i.Layers)-1].SquashedSearchContext
	} else {
		i.SquashedSearchContext = filetree.NewSearchContext(i.SquashedTree(), i.FileCatalog)
	}

	log.WithFields("digest", i.Metadata.ID, "time", time.Since(lapTime)).Trace("completed image search context")
	log.WithFields("digest", i.Metadata.ID, "mediaType", i.Metadata.MediaType, "tags", i.Metadata.Tags, "time", time.Since(startTime)).Info("completed image read")

	return nil
}

// readLayers drives the two stages of a layer read: a fetch stage (download + decompress into the
// cache) and an index stage (tar walk into the file catalog) that runs alongside it, so the
// previous layer is indexed while the next is still being fetched.
//
// Each stage is bounded by its own go-sync executor pulled from the context, which is what lets a
// caller fold stereoscope into a process-wide concurrency budget: install executors under
// LayerFetchExecutor and LayerIndexExecutor and they win. Read fills in the default for whichever
// stage the caller left out, so the two bounds stay independent - the registry provider installs a
// fetch executor of one and leaves indexing at the default, which a single fused bound cannot
// express.
//
// Errors from both stages are joined. Panics inside either stage are captured as errors rather
// than taking down the process, and a cancelled context stops either stage from starting more
// work.
func (i *Image) readLayers(ctx context.Context, layers []*Layer, fileCatalog *FileCatalog, readProg *progress.Manual, gates *layerGates) error {
	ctx = withLayerExecutors(ctx, len(layers))

	idxs := make([]int, len(layers))
	for n := range idxs {
		idxs[n] = n
	}

	// fetched hands layer indexes from the fetch stage to the index stage. Buffered to the layer
	// count so a fetch worker can never block on the handoff, which keeps the two stages free of
	// any ordering dependency on each other.
	fetched := make(chan int, len(layers))
	fetchDone := make(chan error, 1)

	fetchCtx := ctx
	go func() {
		err := async.Collect(&fetchCtx, LayerFetchExecutor, async.ToSeq(idxs),
			func(idx int) (int, error) {
				if err := layers[idx].fetch(idx, i.contentCacheDir); err != nil {
					// the index stage will never see this layer, so open its gate here
					gates.done(idx, false)
					return idx, fmt.Errorf("failed to fetch layer %d: %w", idx, err)
				}
				fetched <- idx
				return idx, nil
			}, nil)
		close(fetched)
		fetchDone <- err
	}()

	indexCtx := ctx
	indexErr := async.Collect(&indexCtx, LayerIndexExecutor, seqOfChannel(fetched),
		func(idx int) (int, error) {
			err := layers[idx].index(fileCatalog)
			// open the gate either way: squash decides what to do with the outcome
			gates.done(idx, err == nil)
			if err != nil {
				return idx, fmt.Errorf("failed to index layer %d: %w", idx, err)
			}
			readProg.Increment()
			return idx, nil
		},
		// the accumulator is serialised by Collect, so this needs no lock of its own
		func(idx int, _ int) {
			i.Metadata.Size += layers[idx].Metadata.Size
		})

	return errors.Join(<-fetchDone, indexErr)
}

// LayerFetchExecutor and LayerIndexExecutor name the go-sync executors bounding each stage of a
// layer read. Install one under either name to take over that stage's concurrency:
//
//	ctx = sync.SetContextExecutor(ctx, image.LayerFetchExecutor, sync.NewExecutor(2))
//
// A caller-supplied executor always wins; Read only fills in what is missing.
const (
	LayerFetchExecutor = "layer-fetch"
	LayerIndexExecutor = "layer-index"
)

// withLayerExecutors fills in the stage executors this image should use for any the caller has not
// supplied. Never bounded at zero: a zero-concurrency go-sync executor runs inline on the caller's
// goroutine, which would collapse the two stages back into one and lose the overlap.
func withLayerExecutors(ctx context.Context, layers int) context.Context {
	for _, name := range []string{LayerFetchExecutor, LayerIndexExecutor} {
		if async.HasContextExecutor(ctx, name) {
			continue
		}
		// go-sync resolves a missing named executor to ExecutorDefault, so a host that installed
		// one to express a single process-wide budget already has an answer for this stage and we
		// should not talk over it. Only fill in when there is nothing at all, because the last
		// fallback go-sync offers is an inline serial executor, which would collapse the two
		// stages into one and lose the overlap.
		if async.HasContextExecutor(ctx, async.ExecutorDefault) {
			continue
		}
		ctx = async.SetContextExecutor(ctx, name, async.NewExecutor(layerReadWorkers(layers)))
	}
	return ctx
}

// layerGates lets the squash stage consume layers in manifest order while the read stages
// complete them in any order: squash blocks on gate idx until the read stages are done with that
// layer, and learns from the gate whether it is safe to squash.
//
// Release is idempotent so the read stages and the caller's cleanup pass can both call it. A
// layer the read never reached is released as not-ok, which is what stops squash blocking
// forever on a cancelled read.
type layerGates struct {
	wg   []sync.WaitGroup
	once []sync.Once
	ok   []atomic.Bool
}

func newLayerGates(layers int) *layerGates {
	g := &layerGates{
		wg:   make([]sync.WaitGroup, layers),
		once: make([]sync.Once, layers),
		ok:   make([]atomic.Bool, layers),
	}
	for idx := range g.wg {
		g.wg[idx].Add(1)
	}
	return g
}

// done opens a layer's gate, reporting whether that layer is indexed and safe to squash.
func (g *layerGates) done(idx int, ok bool) {
	g.once[idx].Do(func() {
		g.ok[idx].Store(ok)
		g.wg[idx].Done()
	})
}

// wait blocks until the layer's gate opens and reports whether it is safe to squash.
func (g *layerGates) wait(idx int) bool {
	g.wg[idx].Wait()
	return g.ok[idx].Load()
}

// releaseAll opens every gate that is still closed, so nothing is left waiting on a layer the
// read stages never got to.
func (g *layerGates) releaseAll() {
	for idx := range g.wg {
		g.done(idx, false)
	}
}

// seqOfChannel adapts a channel to an iter.Seq so a go-sync Collect can consume a stage's output
// as it is produced rather than waiting for all of it.
func seqOfChannel[T any](ch <-chan T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range ch {
			if !yield(v) {
				return
			}
		}
	}
}

// layerReadWorkers is the default bound for a layer-read stage when neither a named executor nor
// ExecutorDefault is in the context: the number of CPUs, capped at 8, and never more workers than
// there are layers.
//
// The cap is measured, not a guess. Both stages are throughput-bound rather than latency-bound -
// decompress and write, then read back and walk - so oversubscribing past a handful of workers
// costs more in contention than it buys in overlap. On a 40-layer image over 12 cores, where the
// cap actually binds: 4 workers 718ms, 8 workers 679ms, 12 workers 694ms, 24 workers 715ms,
// 48 workers 746ms. Going wider is slower, so this is not the place to scale with NumCPU alone.
// Layer count is usually the real ceiling anyway; a 12-layer image cannot use more than 12.
func layerReadWorkers(layers int) int {
	n := runtime.NumCPU()
	if n > 8 {
		n = 8
	}
	if n > layers {
		n = layers
	}
	if n < 1 {
		n = 1
	}
	return n
}

// squash generates a squash tree for each layer in the image. For instance, layer 2 squash =
// squash(layer 0, layer 1, layer 2), layer 3 squash = squash(layer 0, layer 1, layer 2, layer 3), and so on.
func (i *Image) squashLayers(layers []*Layer, gates *layerGates, prog *progress.Manual) error {
	var lastSquashTree filetree.ReadWriter

	for idx, layer := range layers {
		if !gates.wait(idx) {
			// the read stages gave up on this layer, so there is no tree to squash. Report it
			// rather than returning nil and trusting the read stages to raise something: if their
			// error is ever lost, a nil return here lets Read carry on and build the image search
			// context over a layer that was never squashed, which panics on the nil tree.
			return fmt.Errorf("unable to squash: layer %d was not indexed", idx)
		}
		if idx == 0 {
			lastSquashTree = layer.Tree.(filetree.ReadWriter)
			layer.SquashedTree = layer.Tree
			layer.SquashedSearchContext = filetree.NewSearchContext(layer.SquashedTree, layer.fileCatalog.Index)
			continue
		}

		var unionTree = filetree.NewUnionFileTree()
		unionTree.PushTree(lastSquashTree)
		unionTree.PushTree(layer.Tree.(filetree.ReadWriter))

		squashedTree, err := unionTree.Squash()
		if err != nil {
			return fmt.Errorf("failed to squash tree %d: %w", idx, err)
		}

		layer.SquashedTree = squashedTree
		layer.SquashedSearchContext = filetree.NewSearchContext(layer.SquashedTree, layer.fileCatalog.Index)
		lastSquashTree = squashedTree

		prog.Increment()
	}

	return nil
}

// SquashedTree returns the pre-computed image squash file tree.
func (i *Image) SquashedTree() filetree.Reader {
	layerCount := len(i.Layers)

	if layerCount == 0 {
		return filetree.New()
	}

	topLayer := i.Layers[layerCount-1]
	return topLayer.SquashedTree
}

// OpenPathFromSquash fetches file contents for a single path, relative to the image squash tree.
// If the path does not exist an error is returned.
func (i *Image) OpenPathFromSquash(path file.Path) (io.ReadCloser, error) {
	return fetchReaderByPath(i.SquashedTree(), i.FileCatalog, path)
}

// FileContentsFromSquash fetches file contents for a single path, relative to the image squash tree.
// If the path does not exist an error is returned.
//
// Deprecated: use OpenPathFromSquash() instead.
func (i *Image) FileContentsFromSquash(path file.Path) (io.ReadCloser, error) {
	return fetchReaderByPath(i.SquashedTree(), i.FileCatalog, path)
}

// FilesByMIMETypeFromSquash returns file references for files that match at least one of the given MIME types.
//
// Deprecated: please use SquashedSearchContext().SearchByMIMEType() instead.
func (i *Image) FilesByMIMETypeFromSquash(mimeTypes ...string) ([]file.Reference, error) {
	var refs []file.Reference
	refVias, err := i.SquashedSearchContext.SearchByMIMEType(mimeTypes...)
	if err != nil {
		return nil, err
	}
	for _, refVia := range refVias {
		if refVia.HasReference() {
			refs = append(refs, *refVia.Reference)
		}
	}
	return refs, nil
}

// OpenReference fetches file contents for a single file reference, regardless of the source layer.
// If the path does not exist an error is returned.
func (i *Image) OpenReference(ref file.Reference) (io.ReadCloser, error) {
	return i.FileCatalog.Open(ref)
}

// FileContentsByRef fetches file contents for a single file reference, regardless of the source layer.
// If the path does not exist an error is returned.
//
// Deprecated: please use OpenReference() instead.
func (i *Image) FileContentsByRef(ref file.Reference) (io.ReadCloser, error) {
	return i.FileCatalog.Open(ref)
}

// ResolveLinkByLayerSquash resolves a symlink for the given file reference relative to the result from
// the layer squash of the given layer index argument.
// If the given file reference is not a link type, or is a unresolvable (dead) link, then the given file reference is returned.
func (i *Image) ResolveLinkByLayerSquash(ref file.Reference, layer int, options ...filetree.LinkResolutionOption) (*file.Resolution, error) {
	allOptions := append([]filetree.LinkResolutionOption{filetree.FollowBasenameLinks}, options...)
	_, resolvedRef, err := i.Layers[layer].SquashedTree.File(ref.RealPath, allOptions...)
	return resolvedRef, err
}

// ResolveLinkByImageSquash resolves a symlink for the given file reference relative to the result from the image squash.
// If the given file reference is not a link type, or is a unresolvable (dead) link, then the given file reference is returned.
func (i *Image) ResolveLinkByImageSquash(ref file.Reference, options ...filetree.LinkResolutionOption) (*file.Resolution, error) {
	allOptions := append([]filetree.LinkResolutionOption{filetree.FollowBasenameLinks}, options...)
	_, resolvedRef, err := i.Layers[len(i.Layers)-1].SquashedTree.File(ref.RealPath, allOptions...)
	return resolvedRef, err
}

// closeLayers releases every layer tar descriptor in the given slice. Free-standing so a layer
// set that never became i.Layers (an in-flight Read that failed) can be released too.
func closeLayers(layers []*Layer) []error {
	var errs []error
	for _, l := range layers {
		if err := l.close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// closeLayers releases every layer tar descriptor this image is holding open.
func (i *Image) closeLayers() []error {
	return closeLayers(i.Layers)
}

// Cleanup removes all temporary files created from parsing the image. Future calls to image will not function correctly after this call.
func (i *Image) Cleanup() error {
	if i == nil {
		return nil
	}
	// descriptors before dirs: on Windows an open handle makes RemoveAll fail outright
	errs := i.closeLayers()
	if i.tmpDirGen != nil {
		if err := i.tmpDirGen.Cleanup(); err != nil {
			errs = append(errs, err)
		}

		if i.contentCacheDir != "" {
			if _, err := os.Stat(i.contentCacheDir); !os.IsNotExist(err) {
				if err := os.RemoveAll(i.contentCacheDir); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}
