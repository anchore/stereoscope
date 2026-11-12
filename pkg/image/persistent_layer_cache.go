package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anchore/stereoscope/internal/log"
)

// DefaultLayerCacheMaxBytes is the size cap a persistent layer cache gets when none is given.
const DefaultLayerCacheMaxBytes int64 = 20 << 30 // 20 GiB

const (
	// layerCacheFormat names the on-disk layout. Anything under the cache root other than this
	// directory is left alone, so a future layout can live beside this one.
	layerCacheFormat = "v1"
	// layerCacheEdgeBytes is how much of each end of a cached tar the hit check hashes.
	layerCacheEdgeBytes = 64 << 10
	// layerCacheGrace protects recently used entries from eviction: another scan on this host may
	// have just looked one up and not yet opened it.
	layerCacheGrace = 10 * time.Minute
	// layerCacheOrphanAge is how old an in-flight write or a tar without a sidecar must be before
	// eviction assumes its writer died. Younger ones may belong to a scan still running.
	layerCacheOrphanAge     = time.Hour
	layerCachePartialPrefix = "partial-"
	layerCacheTarSuffix     = ".tar"
	layerCacheSidecarSuffix = ".json"
)

// LayerCacheConfig turns on a persistent cache of unpacked (uncompressed) layer tars, shared by
// every image read that names the same directory. A layer is unpacked once per host and reused by
// any later read of an image containing it, which is the common case for images built on a shared
// base. The zero value is off.
type LayerCacheConfig struct {
	// Dir is the cache root. Empty turns the cache off. One OS user per directory: entries are
	// created read-only for their owner and the directory is created 0700.
	Dir string
	// MaxBytes caps the total size of cached tars. Least recently used entries are evicted first.
	// Zero or less means DefaultLayerCacheMaxBytes. A single layer larger than the cap is never cached.
	MaxBytes int64
}

type layerCacheContextKey struct{}

// WithLayerCache returns a context that makes every image read under it use the given persistent
// layer cache. It travels on the context so that it reaches the image read through any library in
// between (syft's GetSource, for one) without each of them growing an option for it. A config with
// an empty Dir returns ctx unchanged.
func WithLayerCache(ctx context.Context, cfg LayerCacheConfig) context.Context {
	if cfg.Dir == "" {
		return ctx
	}
	return context.WithValue(ctx, layerCacheContextKey{}, cfg)
}

// LayerCacheFromContext reports the persistent layer cache config installed by WithLayerCache.
func LayerCacheFromContext(ctx context.Context) (LayerCacheConfig, bool) {
	if ctx == nil {
		return LayerCacheConfig{}, false
	}
	cfg, ok := ctx.Value(layerCacheContextKey{}).(LayerCacheConfig)
	return cfg, ok && cfg.Dir != ""
}

// errLayerCacheBypass means the persistent cache could not take this layer, for a reason that has
// nothing to do with whether the layer itself can be read: the directory is unwritable or full, the
// layer is larger than the cap, or its content does not match the diff ID the image claims for it.
// The caller reads the layer the per-run way instead. A broken cache must never fail a scan that
// would have succeeded without it.
var errLayerCacheBypass = errors.New("persistent layer cache bypassed")

// layerCacheSidecar is written beside each cached tar, after the tar is in place. A tar without one
// is never trusted.
type layerCacheSidecar struct {
	DiffID string `json:"diffID"`
	Size   int64  `json:"size"`
	// Edge is the sha256 of the first and last layerCacheEdgeBytes of the tar (the whole tar when it
	// is no larger than both together). It is a spot check that catches truncation, zeroed ends and
	// a swapped file for two small reads per hit. A same-size change in the middle of the tar is not
	// caught; the content was verified against the diff ID when it was written.
	Edge      string    `json:"edge"`
	WrittenAt time.Time `json:"writtenAt"`
}

// persistentLayerCache is one image read's handle on a persistent layer cache directory. Writers in
// one process are serialized per key; across processes there is no lock, and the write protocol
// (temp file, verify, rename into place, sidecar last) is what keeps readers safe.
type persistentLayerCache struct {
	root     string // the configured directory
	dir      string // root/layerCacheFormat
	maxBytes int64
	now      func() time.Time

	locksMu sync.Mutex
	locks   map[string]*sync.Mutex

	hits      atomic.Int64
	populated atomic.Int64
	bypassed  atomic.Int64
}

// newPersistentLayerCache returns the cache configured on ctx, or nil when there is none.
func newPersistentLayerCache(ctx context.Context) *persistentLayerCache {
	cfg, ok := LayerCacheFromContext(ctx)
	if !ok {
		return nil
	}
	return newPersistentLayerCacheFromConfig(cfg)
}

func newPersistentLayerCacheFromConfig(cfg LayerCacheConfig) *persistentLayerCache {
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultLayerCacheMaxBytes
	}
	root := filepath.Clean(cfg.Dir)
	return &persistentLayerCache{
		root:     root,
		dir:      filepath.Join(root, layerCacheFormat),
		maxBytes: maxBytes,
		now:      time.Now,
		locks:    make(map[string]*sync.Mutex),
	}
}

// layerCacheKey turns a diff ID into the entry name, or reports that the layer cannot be cached.
// Only sha256 is accepted: it is what the write-time verification computes. The colon is swapped out
// since NTFS reads "name:stream" as an alternate data stream (#705).
func layerCacheKey(diffID string) (string, bool) {
	hexPart, ok := strings.CutPrefix(diffID, "sha256:")
	if !ok || len(hexPart) != sha256.Size*2 {
		return "", false
	}
	if _, err := hex.DecodeString(hexPart); err != nil || strings.ToLower(hexPart) != hexPart {
		return "", false
	}
	return "sha256-" + hexPart, true
}

func (c *persistentLayerCache) tarPath(key string) string {
	return filepath.Join(c.dir, key+layerCacheTarSuffix)
}

func (c *persistentLayerCache) sidecarPath(key string) string {
	return filepath.Join(c.dir, key+layerCacheSidecarSuffix)
}

func (c *persistentLayerCache) lockFor(key string) *sync.Mutex {
	c.locksMu.Lock()
	defer c.locksMu.Unlock()
	mu, ok := c.locks[key]
	if !ok {
		mu = &sync.Mutex{}
		c.locks[key] = mu
	}
	return mu
}

// materialize returns the path of a verified, cached tar for the layer, populating the cache on a
// miss. ok is false when the layer is not eligible or the cache was bypassed; the caller then reads
// the layer the per-run way. An error is a failure to read the layer itself and fails the fetch,
// as it would without the cache.
func (c *persistentLayerCache) materialize(l *Layer) (path string, ok bool, err error) {
	if !standardLayerMediaTypes.Has(string(l.Metadata.MediaType)) {
		return "", false, nil
	}
	key, eligible := layerCacheKey(l.Metadata.Digest)
	if !eligible {
		log.WithFields("index", l.Metadata.Index, "digest", l.Metadata.Digest).Trace("layer is not eligible for the persistent layer cache")
		return "", false, nil
	}

	if p, hit := c.lookup(key, l.Metadata.Digest); hit {
		c.hits.Add(1)
		log.WithFields("index", l.Metadata.Index, "digest", l.Metadata.Digest).Trace("persistent layer cache hit")
		return p, true, nil
	}

	// two layers of one image can share a diff ID (empty layers do): decompress it once
	mu := c.lockFor(key)
	mu.Lock()
	defer mu.Unlock()

	if p, hit := c.lookup(key, l.Metadata.Digest); hit {
		c.hits.Add(1)
		return p, true, nil
	}

	p, err := c.populate(l, key)
	switch {
	case errors.Is(err, errLayerCacheBypass):
		c.bypassed.Add(1)
		log.WithFields("index", l.Metadata.Index, "digest", l.Metadata.Digest, "reason", err).Warn("not using the persistent layer cache for this layer")
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	c.populated.Add(1)
	log.WithFields("index", l.Metadata.Index, "digest", l.Metadata.Digest).Trace("persistent layer cache populated")
	return p, true, nil
}

// lookup reports whether a trustworthy entry exists for key. A damaged entry is removed so it cannot
// poison a later read; an entry with no sidecar yet is left for its writer (or the orphan sweep).
func (c *persistentLayerCache) lookup(key, diffID string) (string, bool) {
	raw, err := os.ReadFile(c.sidecarPath(key))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.WithFields("key", key, "error", err).Debug("unable to read persistent layer cache sidecar")
		}
		return "", false
	}

	var sc layerCacheSidecar
	if err := json.Unmarshal(raw, &sc); err != nil {
		c.invalidate(key, fmt.Sprintf("unreadable sidecar: %v", err))
		return "", false
	}
	if sc.DiffID != diffID {
		c.invalidate(key, fmt.Sprintf("sidecar names diff ID %q", sc.DiffID))
		return "", false
	}

	tarPath := c.tarPath(key)
	edge, size, err := layerCacheEdgeDigest(tarPath)
	if err != nil {
		c.invalidate(key, fmt.Sprintf("unable to read cached tar: %v", err))
		return "", false
	}
	if size != sc.Size {
		c.invalidate(key, fmt.Sprintf("size is %d, expected %d", size, sc.Size))
		return "", false
	}
	if edge != sc.Edge {
		c.invalidate(key, "spot-check digest does not match")
		return "", false
	}

	// the sidecar's mtime is the entry's last use for eviction
	now := c.now()
	_ = os.Chtimes(c.sidecarPath(key), now, now)
	return tarPath, true
}

// invalidate removes an entry: sidecar first, so no reader trusts a tar that is half gone.
func (c *persistentLayerCache) invalidate(key, reason string) {
	log.WithFields("key", key, "dir", c.dir, "reason", reason).Warn("discarding a damaged persistent layer cache entry")
	_ = os.Remove(c.sidecarPath(key))
	_ = removeReadOnly(c.tarPath(key))
}

// populate unpacks the layer into the cache. Errors reading the layer are returned as they are;
// anything to do with the cache itself is wrapped in errLayerCacheBypass.
func (c *persistentLayerCache) populate(l *Layer, key string) (string, error) {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return "", fmt.Errorf("%w: unable to create %q: %v", errLayerCacheBypass, c.dir, err)
	}

	rawReader, err := l.layer.Uncompressed()
	if err != nil {
		return "", err
	}
	defer rawReader.Close()

	fh, err := os.CreateTemp(c.dir, layerCachePartialPrefix)
	if err != nil {
		return "", fmt.Errorf("%w: unable to create a cache file: %v", errLayerCacheBypass, err)
	}
	tmpPath := fh.Name()
	defer func() {
		fh.Close()         // no-op once already closed
		os.Remove(tmpPath) // no-op once renamed away
	}()

	// the file and the hash are written through one tracked writer so that a failure writing the
	// cache (disk full) can be told apart from a failure reading the layer (dropped connection)
	digester := sha256.New()
	sink := &cacheSink{w: io.MultiWriter(fh, digester), limit: c.maxBytes}
	if _, err := io.Copy(sink, rawReader); err != nil {
		if sink.err != nil {
			return "", fmt.Errorf("%w: %v", errLayerCacheBypass, sink.err)
		}
		return "", err
	}
	if err := fh.Close(); err != nil {
		return "", fmt.Errorf("%w: unable to finish cache file: %v", errLayerCacheBypass, err)
	}

	// the diff ID is only what the image claims; nothing is kept under a name its content does not match
	if computed := "sha256:" + hex.EncodeToString(digester.Sum(nil)); computed != l.Metadata.Digest {
		return "", fmt.Errorf("%w: layer content digest %s does not match diff ID %s", errLayerCacheBypass, computed, l.Metadata.Digest)
	}

	if err := os.Chmod(tmpPath, 0o444); err != nil {
		return "", fmt.Errorf("%w: %v", errLayerCacheBypass, err)
	}
	tarPath := c.tarPath(key)
	if err := os.Rename(tmpPath, tarPath); err != nil {
		// on Windows a rename onto an existing (read-only, or open) entry fails; another scan won the
		// race, so take its entry if it is complete
		if p, hit := c.lookup(key, l.Metadata.Digest); hit {
			return p, nil
		}
		return "", fmt.Errorf("%w: unable to place cache file: %v", errLayerCacheBypass, err)
	}

	edge, size, err := layerCacheEdgeDigest(tarPath)
	if err != nil {
		return "", fmt.Errorf("%w: unable to read back cache file: %v", errLayerCacheBypass, err)
	}
	if err := c.writeSidecar(key, layerCacheSidecar{DiffID: l.Metadata.Digest, Size: size, Edge: edge, WrittenAt: c.now().UTC()}); err != nil {
		// the tar stays behind without a sidecar: never trusted, swept once it is old
		return "", fmt.Errorf("%w: %v", errLayerCacheBypass, err)
	}
	return tarPath, nil
}

func (c *persistentLayerCache) writeSidecar(key string, sc layerCacheSidecar) error {
	raw, err := json.Marshal(sc)
	if err != nil {
		return err
	}
	fh, err := os.CreateTemp(c.dir, layerCachePartialPrefix)
	if err != nil {
		return err
	}
	tmpPath := fh.Name()
	defer func() {
		fh.Close()
		os.Remove(tmpPath)
	}()
	if _, err := fh.Write(raw); err != nil {
		return err
	}
	if err := fh.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, c.sidecarPath(key))
}

// cacheSink records why a write failed, and stops a layer larger than the cap from filling the disk.
type cacheSink struct {
	w       io.Writer
	limit   int64
	written int64
	err     error
}

func (s *cacheSink) Write(p []byte) (int, error) {
	if s.written+int64(len(p)) > s.limit {
		s.err = fmt.Errorf("layer is larger than the cache size cap of %d bytes", s.limit)
		return 0, s.err
	}
	n, err := s.w.Write(p)
	s.written += int64(n)
	if err != nil {
		s.err = err
	}
	return n, err
}

// layerCacheEdgeDigest hashes the first and last layerCacheEdgeBytes of the file (all of it when it
// is no larger than both together) and returns that with the file's size.
func layerCacheEdgeDigest(path string) (string, int64, error) {
	fh, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil {
		return "", 0, err
	}
	size := info.Size()

	h := sha256.New()
	if size <= 2*layerCacheEdgeBytes {
		if _, err := io.Copy(h, fh); err != nil {
			return "", 0, err
		}
	} else {
		if _, err := io.Copy(h, io.NewSectionReader(fh, 0, layerCacheEdgeBytes)); err != nil {
			return "", 0, err
		}
		if _, err := io.Copy(h, io.NewSectionReader(fh, size-layerCacheEdgeBytes, layerCacheEdgeBytes)); err != nil {
			return "", 0, err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

// removeReadOnly removes a cache tar. Entries are made read-only, and Windows refuses to delete a
// read-only file, so the mode is relaxed first.
func removeReadOnly(path string) error {
	_ = os.Chmod(path, 0o644)
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// contains reports whether path is the cache root or inside it.
func (c *persistentLayerCache) contains(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	root, err := filepath.Abs(c.root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func (c *persistentLayerCache) logSummary() {
	log.WithFields("hits", c.hits.Load(), "populated", c.populated.Load(), "bypassed", c.bypassed.Load(), "dir", c.root).Debug("persistent layer cache")
}

// evict sweeps abandoned writes and orphans, then removes least recently used entries until the
// cache is within its cap. Entries used within layerCacheGrace are never evicted, so the total can
// stay above the cap while every remaining entry is that fresh. Every failure is logged and skipped:
// eviction is housekeeping and never fails a read.
func (c *persistentLayerCache) evict() {
	dirEntries, err := os.ReadDir(c.dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.WithFields("dir", c.dir, "error", err).Warn("unable to list the persistent layer cache")
		}
		return
	}
	now := c.now()

	tars := map[string]fs.FileInfo{}
	sidecars := map[string]fs.FileInfo{}
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue // removed under us
		}
		name := de.Name()
		switch {
		case strings.HasPrefix(name, layerCachePartialPrefix):
			if now.Sub(info.ModTime()) > layerCacheOrphanAge {
				c.removeLogged(filepath.Join(c.dir, name), os.Remove)
			}
		case strings.HasSuffix(name, layerCacheTarSuffix):
			tars[strings.TrimSuffix(name, layerCacheTarSuffix)] = info
		case strings.HasSuffix(name, layerCacheSidecarSuffix):
			sidecars[strings.TrimSuffix(name, layerCacheSidecarSuffix)] = info
		}
	}

	// a sidecar is written after its tar, so one without a tar is an entry already being removed
	for key := range sidecars {
		if _, ok := tars[key]; !ok {
			c.removeLogged(c.sidecarPath(key), os.Remove)
			delete(sidecars, key)
		}
	}

	type lruEntry struct {
		key  string
		size int64
		used time.Time
	}
	var total int64
	var lru []lruEntry
	for key, info := range tars {
		sc, ok := sidecars[key]
		if !ok {
			// a writer between its rename and its sidecar, or one that died there
			if now.Sub(info.ModTime()) > layerCacheOrphanAge && c.removeLogged(c.tarPath(key), removeReadOnly) {
				continue
			}
			total += info.Size()
			continue
		}
		total += info.Size()
		lru = append(lru, lruEntry{key: key, size: info.Size(), used: sc.ModTime()})
	}
	sort.Slice(lru, func(i, j int) bool { return lru[i].used.Before(lru[j].used) })

	bytesBefore, removed := total, 0
	for _, e := range lru {
		if total <= c.maxBytes {
			break
		}
		if now.Sub(e.used) < layerCacheGrace {
			continue
		}
		if !c.removeLogged(c.sidecarPath(e.key), os.Remove) {
			continue
		}
		if !c.removeLogged(c.tarPath(e.key), removeReadOnly) {
			// the sidecar is gone, so the tar is an orphan the next sweep picks up
			continue
		}
		total -= e.size
		removed++
	}
	log.WithFields("dir", c.dir, "entries", len(lru), "bytesBefore", bytesBefore, "bytesAfter", total, "removed", removed, "maxBytes", c.maxBytes).Debug("persistent layer cache eviction")
}

func (c *persistentLayerCache) removeLogged(path string, remove func(string) error) bool {
	if err := remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// on Windows an entry another scan has open cannot be removed; it goes next time
		log.WithFields("path", path, "error", err).Debug("unable to remove a persistent layer cache file")
		return false
	}
	return true
}
