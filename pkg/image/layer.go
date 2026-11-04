package image

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/scylladb/go-set/strset"
	"github.com/sylabs/squashfs"
	"github.com/wagoodman/go-partybus"
	"github.com/wagoodman/go-progress"

	"github.com/anchore/stereoscope/internal/bus"
	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/event"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
)

const (
	SingularitySquashFSLayer       types.MediaType = "application/vnd.sylabs.sif.layer.v1.squashfs"
	BuildKitZstdCompressedLayer    types.MediaType = "application/vnd.docker.image.rootfs.diff.tar.zstd"
	BuildKitZstdCompressedLayerAlt types.MediaType = "application/vnd.docker.image.rootfs.diff.tar+zstd" // we're future proofing against a possible media type variation
)

// standardLayerMediaTypes are tar-based layer media types that can be processed with standard tar indexing.
var standardLayerMediaTypes = strset.New(
	string(types.OCILayer),
	string(types.OCIUncompressedLayer),
	string(types.OCIRestrictedLayer),
	string(types.OCIUncompressedRestrictedLayer),
	string(types.OCILayerZStd),
	string(types.DockerLayer),
	string(types.DockerForeignLayer),
	string(types.DockerUncompressedLayer),
	string(BuildKitZstdCompressedLayer),
	string(BuildKitZstdCompressedLayerAlt),
)

// singularityLayerMediaTypes are SquashFS-based layer media types.
var singularityLayerMediaTypes = strset.New(
	string(SingularitySquashFSLayer),
)

// isSupportedLayerMediaType returns true if the given media type is supported for layer processing.
func isSupportedLayerMediaType(mt types.MediaType) bool {
	return standardLayerMediaTypes.Has(string(mt)) || singularityLayerMediaTypes.Has(string(mt))
}

// validateLayerMediaTypes checks all layers have supported media types before processing.
func validateLayerMediaTypes(layers []v1.Layer) error {
	var unsupported []string
	for idx, layer := range layers {
		mt, err := layer.MediaType()
		if err != nil {
			return fmt.Errorf("unable to get media type for layer %d: %w", idx, err)
		}
		if !isSupportedLayerMediaType(mt) {
			unsupported = append(unsupported, fmt.Sprintf("layer %d: %s", idx, mt))
		}
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("unsupported layer media type(s): %s", strings.Join(unsupported, ", "))
	}
	return nil
}

// Layer represents a single layer within a container image.
type Layer struct {
	// layer is the raw layer metadata and content provider from the GCR lib
	layer v1.Layer
	// indexedContent provides index access to the cached and unzipped layer tar
	indexedContent *file.TarIndex
	// Metadata contains select layer attributes
	Metadata LayerMetadata
	// Tree is a filetree that represents the structure of the layer tar contents ("diff tree"). It keeps this
	// layer's whiteout entries, since a merge reads them off the upper tree to know what to remove.
	Tree filetree.Reader
	// SquashedTree is a filetree that represents the combination of this layers diff tree and all diff trees
	// in lower layers relative to this one. It never contains whiteout entries: those are changeset metadata
	// and do not materialize as files in an applied rootfs. It is set for every layer once the image is read.
	// Below the top layer of an image read with WithLazyLayerSquashes it is built on first use, so it is
	// not a *filetree.FileTree and its first call can take a while (see WithLazyLayerSquashes). Entries under a
	// lower-layer symlink are squashed with overlay semantics, which some runtimes disagree with (see
	// filetree.FileTree.Merge), so Tree is the place to look for everything a layer wrote.
	SquashedTree filetree.Reader
	// fileCatalog contains all file metadata for all files in all layers (not just this layer)
	fileCatalog *FileCatalog
	// SquashedSearchContext searches SquashedTree. Over a lazy SquashedTree it is lazy too: neither the tree
	// nor the search context's link index is built until the first search.
	SquashedSearchContext filetree.Searcher
	SearchContext         filetree.Searcher
	// knownDiffID is the layer's diff ID as recorded in the image config, when the image could
	// supply it; it saves computing the diff ID from the layer contents.
	knownDiffID string
	// contentPath is where Fetch materialized the uncompressed layer
	contentPath string
	// readMonitor reports indexing progress for this layer
	readMonitor *progress.Manual
	// deferredHardLinks are this layer's hardlinks naming a file that is not in this layer, and those naming
	// one of these, in tar order. squashLayers adopts them from the layers below on a Windows image (see
	// adoptLowerLayerHardLinks), and they stay here after a standalone Read.
	deferredHardLinks []deferredHardLink
}

// deferredHardLink is one hardlink header waiting for the layers below its own.
type deferredHardLink struct {
	ref file.Reference
	// header is what the tar said, kept since a later header at the same path removes this one's catalog entry
	header file.Metadata
	// via is the index in deferredHardLinks of the link this one names, or -1 when it names a lower layer's file
	via int
}

// NewLayer provides a new, unread layer object.
func NewLayer(layer v1.Layer) *Layer {
	return newLayer(layer, "")
}

// newLayer provides a new, unread layer object with the diff ID the image already knows for it,
// if any. See Layer.knownDiffID.
func newLayer(layer v1.Layer, knownDiffID string) *Layer {
	return &Layer{
		layer:       layer,
		knownDiffID: knownDiffID,
	}
}

// newLayers builds the layer objects in manifest order.
//
// The image config already records every layer's diff ID, which saves each layer computing its own
// (for an OCI layout that means decompressing the entire layer just to hash it). When the config
// does not list exactly one per layer we cannot line them up, so let each layer answer for itself.
func newLayers(v1Layers []v1.Layer, diffIDs []v1.Hash) []*Layer {
	if len(diffIDs) != len(v1Layers) {
		diffIDs = nil
	}

	layers := make([]*Layer, len(v1Layers))
	for idx, v1Layer := range v1Layers {
		var knownDiffID string
		if diffIDs != nil {
			knownDiffID = diffIDs[idx].String()
		}
		layers[idx] = newLayer(v1Layer, knownDiffID)
	}
	return layers
}

func (l *Layer) uncompressedCache(uncompressedLayersCacheDir string) (string, error) {
	if uncompressedLayersCacheDir == "" {
		return "", fmt.Errorf("no cache directory given")
	}

	// the digest is what the image claims about the layer, not something we verified, so two layers
	// can claim the same one. Key on the index as well so a layer can never be handed another
	// layer's unpacked content. The digest's colon is swapped out since NTFS reads "name:stream" as
	// an alternate data stream, which a create tolerates but a rename into place does not.
	name := fmt.Sprintf("%d-%s", l.Metadata.Index, strings.ReplaceAll(l.Metadata.Digest, ":", "-"))
	path := path.Join(uncompressedLayersCacheDir, name)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return path, nil
	}

	log.WithFields("index", l.Metadata.Index, "path", path).Trace("start uncompressed layer cache")
	startTime := time.Now()

	rawReader, err := l.layer.Uncompressed()
	if err != nil {
		return "", err
	}
	defer rawReader.Close()

	// write to a unique temp name and rename into place, so this path only ever holds a complete
	// layer: a write that dies partway leaves nothing for a later os.Stat to trust
	fh, err := os.CreateTemp(uncompressedLayersCacheDir, "partial-")
	if err != nil {
		return "", fmt.Errorf("unable to create layer cache path=%q : %w", path, err)
	}
	tmpPath := fh.Name()
	defer func() {
		fh.Close()         // no-op once already closed
		os.Remove(tmpPath) // no-op once renamed away
	}()

	if _, err := io.Copy(fh, rawReader); err != nil {
		return "", fmt.Errorf("unable to populate layer cache path=%q : %w", path, err)
	}
	if err := fh.Close(); err != nil {
		return "", fmt.Errorf("unable to finish layer cache path=%q : %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("unable to place layer cache path=%q : %w", path, err)
	}
	log.WithFields("index", l.Metadata.Index, "path", path, "time", time.Since(startTime)).Trace("completed uncompressed layer cache")

	return path, nil
}

// close releases the layer's unpacked tar. Readers opened from this layer fail afterwards.
//
// unexported on purpose: FileCatalog.Layer hands a *Layer to any consumer, so an exported Close would
// let one of them shut the tar for every other reader of the same image. Layer lifetime belongs to the
// Image that read it.
func (l *Layer) close() error {
	if l == nil || l.indexedContent == nil {
		return nil
	}
	return l.indexedContent.Close()
}

// Read parses information from the underlying layer tar into this struct. This includes layer metadata, the layer
// file tree, and the layer squash tree. It is fetch followed by index.
func (l *Layer) Read(catalog *FileCatalog, idx int, uncompressedLayersCacheDir string) error {
	if err := l.fetch(idx, uncompressedLayersCacheDir); err != nil {
		return err
	}
	return l.index(catalog)
}

// fetch resolves the layer's metadata and materializes its uncompressed contents in the cache
// directory - the network download and decompression, for a registry layer. It does no indexing, so
// an image can keep one layer's download going while other layers are indexed (see Image.Read).
//
// Unexported deliberately: fetch must run before index, and that ordering is not something the
// type can enforce for an outside caller. FileCatalog.Layer hands a *Layer to any consumer, so an
// exported pair would let one of them rebuild a layer out from under every other reader of the
// same image. Layer lifetime belongs to the Image that read it; Layer.Read is the entry point.
func (l *Layer) fetch(idx int, uncompressedLayersCacheDir string) error {
	mediaType, err := l.layer.MediaType()
	if err != nil {
		return err
	}
	if !standardLayerMediaTypes.Has(string(mediaType)) && !singularityLayerMediaTypes.Has(string(mediaType)) {
		return fmt.Errorf("unknown layer media type: %+v", mediaType)
	}

	l.Metadata, err = newLayerMetadata(l.layer, idx, l.knownDiffID)
	if err != nil {
		return err
	}

	log.WithFields("index", l.Metadata.Index, "digest", l.Metadata.Digest, "mediaType", l.Metadata.MediaType).Trace("fetching image layer")
	l.contentPath, err = l.uncompressedCache(uncompressedLayersCacheDir)
	return err
}

// index builds the layer's file tree and catalog entries from the contents fetch materialized.
// See fetch for why this is not exported.
func (l *Layer) index(catalog *FileCatalog) error {
	if l.contentPath == "" {
		// no index to report: Metadata is only populated by fetch, so it is zeroed here
		return fmt.Errorf("layer contents have not been fetched")
	}
	tree := filetree.New()
	l.Tree = tree
	l.fileCatalog = catalog
	l.deferredHardLinks = nil

	// published here rather than in fetch: the monitor counts tar entries, which is entirely
	// index-stage work. Publishing it at fetch time also meant a layer that failed to fetch left a
	// monitor behind that nothing would ever complete, and that the event order followed fetch
	// completion rather than the layer order consumers expect.
	l.readMonitor = trackReadProgress(l.Metadata)

	// l.Metadata.MediaType was already validated against these same sets by fetch (and, before
	// that, validateLayerMediaTypes for every layer up front), so there is no unknown case left
	// to handle here.
	var readErr error
	switch {
	case standardLayerMediaTypes.Has(string(l.Metadata.MediaType)):
		readErr = l.indexStandardImageLayer(tree)
	case singularityLayerMediaTypes.Has(string(l.Metadata.MediaType)):
		readErr = l.indexSingularityImageLayer(tree)
	}
	if readErr != nil {
		return readErr
	}

	startTime := time.Now()
	l.SearchContext = filetree.NewSearchContext(l.Tree, l.fileCatalog.Index)
	log.WithFields("index", l.Metadata.Index, "time", time.Since(startTime)).Trace("completed layer search context")

	return nil
}

func (l *Layer) indexStandardImageLayer(tree *filetree.FileTree) error {
	var err error
	startTime := time.Now()
	l.indexedContent, err = file.NewTarIndex(
		l.contentPath,
		layerTarIndexer(tree, l.fileCatalog, &l.Metadata.Size, l, l.readMonitor),
	)
	if err != nil {
		return fmt.Errorf("failed to read layer=%q tar : %w", l.Metadata.Digest, err)
	}
	log.WithFields("index", l.Metadata.Index, "digest", l.Metadata.Digest, "mediaType", l.Metadata.MediaType, "time", time.Since(startTime)).Trace("completed indexing image layer")

	l.readMonitor.SetCompleted()
	return nil
}

func (l *Layer) indexSingularityImageLayer(tree *filetree.FileTree) error {
	startTime := time.Now()
	if err := file.WalkSquashFS(l.contentPath, squashfsVisitor(tree, l.fileCatalog, &l.Metadata.Size, l, l.readMonitor)); err != nil {
		return fmt.Errorf("failed to walk layer=%q squashfs : %w", l.Metadata.Digest, err)
	}
	log.WithFields("index", l.Metadata.Index, "digest", l.Metadata.Digest, "mediaType", l.Metadata.MediaType, "time", time.Since(startTime)).Trace("completed indexing image layer")

	l.readMonitor.SetCompleted()
	return nil
}

// OpenPath reads the file contents for the given path from the underlying layer blob, relative to the layers "diff tree".
// An error is returned if there is no file at the given path and layer or the read operation cannot continue.
func (l *Layer) OpenPath(path file.Path) (io.ReadCloser, error) {
	return fetchReaderByPath(l.Tree, l.fileCatalog, path)
}

// OpenPathFromSquash reads the file contents for the given path from the underlying layer blob, relative to the layers squashed file tree.
// An error is returned if there is no file at the given path and layer or the read operation cannot continue.
func (l *Layer) OpenPathFromSquash(path file.Path) (io.ReadCloser, error) {
	if l.SquashedTree == nil {
		return nil, l.errNoSquash()
	}
	return fetchReaderByPath(l.SquashedTree, l.fileCatalog, path)
}

// FileContents reads the file contents for the given path from the underlying layer blob, relative to the layers "diff tree".
// An error is returned if there is no file at the given path and layer or the read operation cannot continue.
//
// Deprecated: use OpenPath() instead.
func (l *Layer) FileContents(path file.Path) (io.ReadCloser, error) {
	return fetchReaderByPath(l.Tree, l.fileCatalog, path)
}

// FileContentsFromSquash reads the file contents for the given path from the underlying layer blob, relative to the layers squashed file tree.
// An error is returned if there is no file at the given path and layer or the read operation cannot continue.
//
// Deprecated: use OpenPathFromSquash() instead.
func (l *Layer) FileContentsFromSquash(path file.Path) (io.ReadCloser, error) {
	return l.OpenPathFromSquash(path)
}

// FilesByMIMEType returns file references for files that match at least one of the given MIME types relative to each layer tree.
//
// Deprecated: use SearchContext().SearchByMIMEType() instead.
func (l *Layer) FilesByMIMEType(mimeTypes ...string) ([]file.Reference, error) {
	var refs []file.Reference
	refVias, err := l.SearchContext.SearchByMIMEType(mimeTypes...)
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

// FilesByMIMETypeFromSquash returns file references for files that match at least one of the given MIME types relative to the squashed file tree representation.
//
// Deprecated: use SquashedSearchContext().SearchByMIMEType() instead.
func (l *Layer) FilesByMIMETypeFromSquash(mimeTypes ...string) ([]file.Reference, error) {
	if l.SquashedSearchContext == nil {
		return nil, l.errNoSquash()
	}
	var refs []file.Reference
	refVias, err := l.SquashedSearchContext.SearchByMIMEType(mimeTypes...)
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

// errNoSquash is returned by squash accessors on a layer without one. Image.Read sets one on every layer,
// so this only happens for a layer that was not read as part of an image (Layer.Read alone does not squash).
func (l *Layer) errNoSquash() error {
	return fmt.Errorf("layer %d has no squashed tree (squashes are built by Image.Read)", l.Metadata.Index)
}

// adoptHardLinkInode rewrites metadata for a hardlink tar header to describe the file it names, and
// returns that file's opener. Returns false if that file is not in this layer (a malformed archive),
// leaving the entry to be indexed from its own header.
//
// a hardlink is a second name for an inode already in this layer, not a path to resolve later, and
// its header carries no data section:
//
//   - adopted from the target: size, MIME type, type, link destination, and the file type bits of
//     the mode, which tar does not record on a link header. A hardlink naming a symlink is itself a
//     symlink.
//   - kept from this header: permission bits, uid, gid, mtime, which tar already records correctly.
//
// "inode" is an analogy. There is no such object here: each name keeps its own file.Reference, ID
// and catalog entry, and what is shared is the opener plus a copy of the values above. Pointing both
// names at one reference would model the filesystem more literally, but it would leave a node's
// RealPath disagreeing with its reference's, and drop the second name as a location callers can
// address at all.
//
// Binding here rather than at resolution time is what makes the name immune to later layers: the
// catalog is keyed by file.ID and is never squashed or merged, so replacing or deleting the other
// name cannot change what this one refers to, which is what the filesystem does.
//
// where this model bends the truth:
//
//   - on disk neither name is "the hardlink". Which one a tar marks is an artifact of directory walk
//     order (alphabetical in practice), so it is often the one a person would call the original.
//   - adopting erases that mark, so the names come out symmetric. But nothing can then answer "is
//     this path also known as something else", the way nlink would.
//   - we trust the headers. Data on a link header with an empty regular header would be adopted
//     backwards and serve nothing.
//   - when adoption fails, or is undone by the caller because the adopted type collides with a node
//     already at that path, that one name keeps TypeHardLink, size 0 and an empty data section: the
//     lopsided picture this function exists to avoid.
//   - only this layer is searched. A name that is not in it is deferred, and adopted from the layers
//     below only on a Windows image (see adoptLowerLayerHardLinks).
func adoptHardLinkInode(ft filetree.Reader, fileCatalog *FileCatalog, metadata *file.Metadata) (file.Opener, bool) {
	// a hardlink's link name refers to an earlier member of this same archive, relative to its root.
	// the lookup below is not literal: a miss retries following ancestor symlinks, which is what
	// link(2) does when extracting the archive for real
	linkPath := file.Path(path.Clean(file.DirSeparator + metadata.LinkDestination))

	// an empty link name cleans to the tree root, which would otherwise adopt the root directory
	if metadata.LinkDestination == "" || linkPath == file.Path(file.DirSeparator) {
		return nil, false
	}

	exists, resolution, err := ft.File(linkPath)
	if err != nil || !exists || resolution == nil || resolution.Reference == nil {
		return nil, false
	}

	target, err := fileCatalog.Get(*resolution.Reference)
	if err != nil {
		return nil, false
	}

	// link(2) refuses a directory, so an archive naming one is malformed. adopting it anyway would
	// describe this name as a directory that cannot be listed or walked.
	// a target that is still a hardlink is one that failed to adopt, and taking its description would
	// spread that failure while reporting success, down to its link destination
	if target.Type == file.TypeDirectory || target.Type == file.TypeHardLink {
		return nil, false
	}

	opener := fileCatalog.opener(resolution.ID())
	if opener == nil {
		return nil, false
	}

	bindHardLink(target.Metadata, metadata)
	return opener, true
}

// bindHardLink rewrites metadata, a hardlink header's, to describe target, the file it names (see
// adoptHardLinkInode for what is adopted and what is kept).
func bindHardLink(target file.Metadata, metadata *file.Metadata) {
	metadata.FileInfo = file.ManualInfo{
		NameValue: path.Base(metadata.Path),
		SizeValue: target.Size(),
		// permission bits stay from this header, but the file type bits come from the target: tar
		// records no type bits on a link header, and two names for one inode share a mode
		ModeValue:    metadata.Mode()&^fs.ModeType | target.Mode()&fs.ModeType,
		ModTimeValue: metadata.ModTime(),
		// deliberately the un-adopted header: it is the escape hatch to what the tar actually said,
		// so it still reports TypeLink, size 0 and this name's own link name
		SysValue: metadata.Sys(),
	}
	metadata.MIMEType = target.MIMEType
	metadata.Type = target.Type
	metadata.LinkDestination = target.LinkDestination
}

// literalNode is the node at exactly p in ft, following no links, or nil.
func literalNode(ft filetree.Reader, p file.Path) *filenode.FileNode {
	n, _ := ft.TreeReader().Node(filenode.IDByPath(p)).(*filenode.FileNode)
	return n
}

// hasNonDirAncestor reports whether some ancestor of p in ft is not a directory. The tree can hold a literal
// child under a symlink node, and anything that resolves such a path follows the symlink instead.
func hasNonDirAncestor(ft filetree.Reader, p file.Path) bool {
	for _, c := range p.ConstituentPaths() {
		n := literalNode(ft, c)
		if n == nil {
			// every node has its parent in the tree, so nothing deeper is there either
			return false
		}
		if n.FileType != file.TypeDirectory {
			return true
		}
	}
	return false
}

// removeLiteralLeaf removes the node at exactly p from ft, and its reference from the catalog, when that node has
// no children. It leaves the tree alone when there is no such node, when p is the root, or when an ancestor of p
// is not a directory, since RemovePath would follow that ancestor and remove some other path.
func removeLiteralLeaf(ft filetree.ReadWriter, fileCatalog *FileCatalog, p file.Path) error {
	existing := literalNode(ft, p)
	if existing == nil || p == file.Path(file.DirSeparator) || len(ft.TreeReader().Children(existing)) > 0 || hasNonDirAncestor(ft, p) {
		return nil
	}
	if err := ft.RemovePath(p); err != nil {
		return err
	}
	if existing.Reference != nil {
		fileCatalog.remove(*existing.Reference)
	}
	return nil
}

// removeEarlierEntry clears the way for a tar entry over whatever an earlier entry of the same layer put at its
// path. Extraction applies entries in order and, unless both are directories, removes whatever is at the path
// before writing the next one there (containerd and docker pkg/archive). Doing the same here gives the header its
// own reference, so the later entry wins even with the same type, and the replaced reference leaves the catalog.
//
// a node with children is kept (see removeLiteralLeaf): those children may have been written through a lower
// symlink at this path, so a rootfs applier still has them under the symlink's target. There, and under a
// non-directory ancestor, the builder reuses the node or refuses a retype and fails the read, as before.
func removeEarlierEntry(ft filetree.ReadWriter, fileCatalog *FileCatalog, metadata file.Metadata) error {
	p := file.Path(metadata.Path).Normalize()
	existing := literalNode(ft, p)
	if existing == nil || (existing.FileType == file.TypeDirectory && metadata.Type == file.TypeDirectory) {
		return nil
	}
	return removeLiteralLeaf(ft, fileCatalog, p)
}

func layerTarIndexer(ft filetree.ReadWriter, fileCatalog *FileCatalog, size *int64, layerRef *Layer, monitor *progress.Manual) file.TarIndexVisitor {
	builder := filetree.NewBuilder(ft, fileCatalog.Index)
	// deferredByID is the index in layerRef.deferredHardLinks of each deferred reference, so a later link can
	// tell that the name it follows is one of them
	deferredByID := make(map[file.ID]int)

	return func(index file.TarIndexEntry) error {
		var err error
		var entry = index.ToTarFileEntry()

		var contents = index.Open()
		defer func() {
			if err := contents.Close(); err != nil {
				log.Warnf("unable to close file while indexing layer: %+v", err)
			}
		}()
		metadata := file.NewMetadata(entry.Header, contents)

		// first, as extraction does: an earlier entry at this path is gone before this one is written, even
		// for a hardlink naming it
		if err = removeEarlierEntry(ft, fileCatalog, metadata); err != nil {
			return err
		}

		// a hardlink names a file already present in this layer; describe it as that file and read
		// its contents through that file's opener rather than this header's empty data section
		var hardLinkOpener file.Opener
		var preAdoptionMetadata *file.Metadata
		deferVia, deferred := -1, false
		if metadata.Type == file.TypeHardLink {
			original := metadata
			var adopted bool
			hardLinkOpener, adopted = adoptHardLinkInode(ft, fileCatalog, &metadata)
			if adopted {
				// keep the un-adopted description to fall back to: adoption changes the node type this
				// entry claims, which can conflict with what an earlier header put at the same path
				preAdoptionMetadata = &original
			} else if layerRef != nil {
				deferVia, deferred = deferHardLink(ft, deferredByID, metadata)
			}
		}

		// note: the tar header name is independent of surrounding structure, for example, there may be a tar header entry
		// for /some/path/to/file.txt without any entries to constituent paths (/some, /some/path, /some/path/to ).
		// This is ok, and the FileTree will account for this by automatically adding directories for non-existing
		// constituent paths. If later there happens to be a tar header entry for an already added constituent path
		// the FileNode will be updated with the new file.Reference. If there is no tar header entry for constituent
		// paths the FileTree is still structurally consistent (all paths can be iterated even though there may not have
		// been a tar header entry for part of the given path).
		//
		// In summary: the set of all FileTrees can have NON-leaf nodes that don't exist in the FileCatalog, but
		// the FileCatalog should NEVER have entries that don't appear in one (or more) FileTree(s).
		ref, err := builder.Add(metadata)
		if err != nil {
			if preAdoptionMetadata == nil {
				return err
			}
			// only an adopted hardlink can be described here as something other than what its own
			// header says, so a malformed archive should cost this one entry its adoption rather than
			// the whole image. errors on the un-adopted retry are still fatal, as they were before.
			log.WithFields("path", metadata.Path, "linkName", metadata.LinkDestination, "error", err).
				Trace("adopted hardlink conflicts with an existing entry, indexing it from its own header")

			metadata = *preAdoptionMetadata
			hardLinkOpener = nil

			ref, err = builder.Add(metadata)
			if err != nil {
				return err
			}
		}

		if size != nil {
			// what this entry contributes to the layer blob, which is its own header's data section.
			// NOT metadata.Size(), which for an adopted hardlink is the size of the file it names
			*(size) += entry.Header.Size
		}
		opener := hardLinkOpener
		if opener == nil {
			opener = func() (io.ReadCloser, error) {
				return index.Open(), nil
			}
		}
		fileCatalog.addImageReferences(ref.ID(), layerRef, opener)
		if deferred {
			deferredByID[ref.ID()] = len(layerRef.deferredHardLinks)
			layerRef.deferredHardLinks = append(layerRef.deferredHardLinks, deferredHardLink{ref: *ref, header: metadata, via: deferVia})
		}

		if monitor != nil {
			monitor.Increment()
		}
		return nil
	}
}

// deferHardLink decides whether a hardlink header that indexing could not adopt waits for the layers below. It
// does when nothing in this layer is at its name yet (via -1), or when what is there is a link of this layer that
// is itself deferred (via is that link's index). Anything else this layer already has at the name, a directory or
// an unadoptable link, is what Windows links to, never a lower file, so the header is left as it is.
func deferHardLink(ft filetree.Reader, deferred map[file.ID]int, metadata file.Metadata) (via int, ok bool) {
	if metadata.LinkDestination == "" {
		return -1, false
	}
	n := literalNode(ft, file.Path(path.Clean(file.DirSeparator+metadata.LinkDestination)))
	if n == nil {
		// trace, not warn: a per-header warning would be noisy on any archive that trips this
		log.WithFields("path", metadata.Path, "linkName", metadata.LinkDestination).
			Trace("hardlink names a file that is not in this layer, deferring it to the layers below")
		return -1, true
	}
	if n.Reference != nil {
		if i, ok := deferred[n.Reference.ID()]; ok {
			return i, true
		}
	}
	return -1, false
}

// a Windows layer keeps its filesystem under Files/ and the utility VM's under UtilityVM/Files/. hcsshim only
// resolves a link into a lower layer when both its name and its target are under one of them.
const (
	windowsFilesPrefix          = "Files/"
	windowsUtilityVMFilesPrefix = "UtilityVM/Files/"
)

// underWindowsFiles reports whether p, a tar name without a leading separator, is somewhere hcsshim links.
func underWindowsFiles(p string) bool {
	return strings.HasPrefix(p, windowsFilesPrefix) || strings.HasPrefix(p, windowsUtilityVMFilesPrefix)
}

// adoptLowerLayerHardLinks adopts the hardlinks of layer that name a file in a lower layer, given below: every
// layer under it, bottom first. Only Windows images have these. Their update layers link WinSxS names to System32
// files that only the base layer carries. Linux builders keep a hardlink and its target in one layer, so callers
// pass no layers below for a Linux image and its links stay as their headers describe them.
//
// This follows hcsshim's AddLink, which is what imports a Windows layer, not a squash. A link whose target is not
// in its own layer is looked up in each parent layer's own files, nearest first, and the first layer that has the
// path wins. Whiteouts in between are not consulted, and no reparse point (symlink) is followed: one on the way
// fails the import. NTFS matches names case-insensitively, so a nearer layer with the name in a different case is
// the one Windows links to, and is refused here rather than skipped.
//
// A utility VM target is different: hcsshim clones the parent's whole utility VM into this layer, where whiteouts
// delete files and this layer's own entries land too. There a whiteout on the way, or anything this layer has at
// the name, refuses the link.
//
// Everything hcsshim would refuse, and every shape this can't be sure of, keeps the link's own header:
// TypeHardLink with no contents, as before.
//
// A link naming another deferred link of this layer gets what that link got, and fails with it. Links are in tar
// order, so a chain of any depth resolves in one pass.
//
// Each adopted name's link node is replaced in this layer's tree by a node of the adopted type, since lookups
// would otherwise follow the link to its target's path. The old reference leaves the catalog, and the layer's
// search context is rebuilt. It must run before the layer is merged into the squash.
func adoptLowerLayerHardLinks(below []*Layer, layer *Layer) error {
	pending := layer.deferredHardLinks
	layer.deferredHardLinks = nil
	if len(below) == 0 || len(pending) == 0 {
		return nil
	}

	tree, ok := layer.Tree.(filetree.ReadWriter)
	if !ok {
		return fmt.Errorf("layer %d tree is not writable", layer.Metadata.Index)
	}
	builder := filetree.NewBuilder(tree, layer.fileCatalog.Index)
	lookup := newLowerLayerLookup(layer, below)

	type boundLink struct {
		metadata file.Metadata
		opener   file.Opener
	}
	bound := make(map[int]boundLink, len(pending))

	adopted := 0
	for i, d := range pending {
		metadata := d.header
		var opener file.Opener
		switch {
		case !underWindowsFiles(strings.TrimPrefix(string(d.ref.RealPath), file.DirSeparator)):
			// hcsshim refuses the link
		case d.via >= 0:
			if target, ok := bound[d.via]; ok {
				bindHardLink(target.metadata, &metadata)
				opener = target.opener
			}
		default:
			opener = lookup.adopt(layer.fileCatalog, &metadata)
		}

		// the node must still be this link's: a later header at the path has its own reference. And replacing a
		// node with children would drop them, as would RemovePath under a non-directory, which follows it
		node := literalNode(tree, d.ref.RealPath)
		if opener == nil || node == nil || node.Reference == nil || node.Reference.ID() != d.ref.ID() ||
			len(tree.TreeReader().Children(node)) > 0 || hasNonDirAncestor(tree, d.ref.RealPath) {
			log.WithFields("path", metadata.Path, "linkName", metadata.LinkDestination).
				Trace("hardlink names no file hcsshim would link from the layers below, indexing it from its own header")
			continue
		}
		bound[i] = boundLink{metadata: metadata, opener: opener}

		if err := tree.RemovePath(d.ref.RealPath); err != nil {
			return fmt.Errorf("unable to replace hardlink %q: %w", d.ref.RealPath, err)
		}
		newRef, err := builder.Add(metadata)
		if err != nil {
			return fmt.Errorf("unable to replace hardlink %q: %w", d.ref.RealPath, err)
		}
		layer.fileCatalog.remove(d.ref)
		layer.fileCatalog.addImageReferences(newRef.ID(), layer, opener)
		adopted++
	}

	if adopted > 0 {
		layer.SearchContext = filetree.NewSearchContext(layer.Tree, layer.fileCatalog.Index)
	}
	log.WithFields("index", layer.Metadata.Index, "adopted", adopted, "pending", len(pending)).
		Trace("adopted hardlinks naming files in lower layers")
	return nil
}

// lowerLayerLookup finds a hardlink's target in the layers below the way hcsshim does (see
// adoptLowerLayerHardLinks). It folds each layer's paths once, on first use, since most links are found in the
// nearest layer that could have them and never reach the others.
type lowerLayerLookup struct {
	// own is the layer the links are in
	own *Layer
	// below is nearest first
	below  []*Layer
	folded map[*Layer]map[foldedName]foldedNode
}

// foldedName is one upper-cased path component inside a folded directory. Looking a name up one component at a
// time keeps the work linear in its length, where hashing every prefix would cost its depth times its length.
type foldedName struct {
	dir  int
	name string
	// whiteout is set for the whiteout of name, rather than name itself
	whiteout bool
}

// foldedNode is what a foldedName names: its node, or nil when two paths fold together, and its own folded id.
type foldedNode struct {
	node *filenode.FileNode
	id   int
}

func newLowerLayerLookup(own *Layer, bottomFirst []*Layer) *lowerLayerLookup {
	below := make([]*Layer, len(bottomFirst))
	for i, l := range bottomFirst {
		below[len(bottomFirst)-1-i] = l
	}
	return &lowerLayerLookup{own: own, below: below, folded: make(map[*Layer]map[foldedName]foldedNode)}
}

// maxWindowsLinkName bounds the link names this resolves, and so the work per link and layer.
// ponytail: real WinSxS names are a few hundred bytes. Raise it if a real image has longer ones
const maxWindowsLinkName = 4096

// adopt rewrites metadata to describe the file its link name resolves to in the layers below and returns that
// file's opener, or nil when hcsshim would not link it, or not surely to that file.
func (l *lowerLayerLookup) adopt(fileCatalog *FileCatalog, metadata *file.Metadata) file.Opener {
	// hcsshim cleans the name without a root, so ".." can climb out of Files/ and fail the import, and it reads "\"
	// as a separator where a tar name does not
	name := path.Clean(metadata.LinkDestination)
	if !underWindowsFiles(name) || strings.Contains(name, `\`) || len(name) > maxWindowsLinkName {
		return nil
	}
	layers := l.below
	uvm := strings.HasPrefix(name, windowsUtilityVMFilesPrefix)
	if uvm {
		layers = append([]*Layer{l.own}, l.below...)
	}
	want := file.Path(file.DirSeparator + name)
	// ponytail: Unicode simple case folding, which NTFS's upcase table follows outside a few scripts. A name that
	// folds differently there is matched in the wrong layer
	components := strings.Split(strings.ToUpper(name), file.DirSeparator)

	for _, layer := range layers {
		folded := l.foldedPaths(layer)
		dir := 0
		var hit *filenode.FileNode
		for i, c := range components {
			if _, ok := folded[foldedName{dir: dir, name: c, whiteout: true}]; ok && uvm {
				return nil
			}
			f, ok := folded[foldedName{dir: dir, name: c}]
			if !ok {
				// nothing deeper is in this layer
				break
			}
			// nil is a name this layer has in more than one case
			if f.node == nil {
				return nil
			}
			if i == len(components)-1 {
				hit = f.node
				break
			}
			// a non-directory on the way is a reparse point or a file, and either fails the lookup with an error
			// that stops the import, rather than a miss that moves on to the next layer
			if f.node.FileType != file.TypeDirectory {
				return nil
			}
			dir = f.id
		}
		if hit == nil {
			continue
		}
		// a utility VM target this layer has may have been written before or after the link, which the tree
		// doesn't order
		if hit.RealPath != want || hit.Reference == nil || layer == l.own {
			return nil
		}
		target, err := fileCatalog.Get(*hit.Reference)
		if err != nil || target.Type == file.TypeDirectory || target.Type == file.TypeHardLink {
			return nil
		}
		opener := fileCatalog.opener(hit.Reference.ID())
		if opener != nil {
			bindHardLink(target.Metadata, metadata)
		}
		return opener
	}
	return nil
}

// foldedPaths indexes every path in layer's tree by its upper-cased components (see foldedName).
func (l *lowerLayerLookup) foldedPaths(layer *Layer) map[foldedName]foldedNode {
	if m, ok := l.folded[layer]; ok {
		return m
	}
	nodes := layer.Tree.TreeReader().Nodes()
	// ids numbers each folded directory path; the root is 0
	ids := map[string]int{file.DirSeparator: 0}
	id := func(p string) int {
		if i, ok := ids[p]; ok {
			return i
		}
		ids[p] = len(ids)
		return ids[p]
	}
	whiteoutPrefix := strings.ToUpper(file.WhiteoutPrefix)
	m := make(map[foldedName]foldedNode, len(nodes))
	for _, n := range nodes {
		fn, ok := n.(*filenode.FileNode)
		if !ok || fn == nil || fn.RealPath == file.DirSeparator {
			continue
		}
		p := strings.ToUpper(string(fn.RealPath))
		i := strings.LastIndex(p, file.DirSeparator)
		parent, base := p[:i], p[i+1:]
		if parent == "" {
			parent = file.DirSeparator
		}
		k := foldedName{dir: id(parent), name: base}
		if _, dup := m[k]; dup {
			fn = nil
		}
		m[k] = foldedNode{node: fn, id: id(p)}
		if strings.HasPrefix(base, whiteoutPrefix) {
			m[foldedName{dir: k.dir, name: strings.TrimPrefix(base, whiteoutPrefix), whiteout: true}] = foldedNode{}
		}
	}
	l.folded[layer] = m
	return m
}

// squashfsReader implements an io.ReadCloser that reads a file from within a SquashFS filesystem.
type squashfsReader struct {
	fs.File
	backingFile *os.File
}

// newSquashfsFileReader returns a io.ReadCloser that reads the file at path within the SquashFS
// filesystem at sqfsPath.
func newSquashfsFileReader(sqfsPath, path string) (io.ReadCloser, error) {
	f, err := os.Open(sqfsPath)
	if err != nil {
		return nil, err
	}

	fsys, err := squashfs.NewReader(f)
	if err != nil {
		return nil, err
	}

	r, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}

	return &squashfsReader{
		File:        r,
		backingFile: f,
	}, nil
}

// Close closes the SquashFS file as well as the backing filesystem.
func (f *squashfsReader) Close() error {
	if err := f.File.Close(); err != nil {
		return err
	}

	return f.backingFile.Close()
}

func squashfsVisitor(ft filetree.Writer, fileCatalog *FileCatalog, size *int64, layerRef *Layer, monitor *progress.Manual) file.SquashFSVisitor {
	builder := filetree.NewBuilder(ft, fileCatalog.Index)

	return func(fsys fs.FS, sqfsPath, path string) error {
		ff, err := fsys.Open(path)
		if err != nil {
			return err
		}
		defer ff.Close()

		f, ok := ff.(*squashfs.File)
		if !ok {
			return errors.New("unexpected file type from squashfs")
		}

		metadata, err := file.NewMetadataFromSquashFSFile(path, f)
		if err != nil {
			return err
		}

		fileReference, err := builder.Add(metadata)
		if err != nil {
			return err
		}

		if size != nil {
			*(size) += metadata.Size()
		}
		fileCatalog.addImageReferences(fileReference.ID(), layerRef, func() (io.ReadCloser, error) {
			return newSquashfsFileReader(sqfsPath, path)
		})

		monitor.Increment()
		return nil
	}
}

func trackReadProgress(metadata LayerMetadata) *progress.Manual {
	p := &progress.Manual{}

	bus.Publish(partybus.Event{
		Type:   event.ReadLayer,
		Source: metadata,
		Value:  progress.Monitorable(p),
	})

	return p
}
