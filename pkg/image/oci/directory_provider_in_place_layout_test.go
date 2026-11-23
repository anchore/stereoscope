package oci

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
)

// fileState is what "untouched" means for a path in the layout: same mode, and for regular files the same bytes
// and mtime. Directory mtimes are left out: NTFS can report a stale one in the "before" snapshot.
type fileState struct {
	sha256  string
	mode    fs.FileMode
	modTime time.Time
}

func snapshotTree(t testing.TB, root string) map[string]fileState {
	t.Helper()
	out := map[string]fileState{}
	require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		st := fileState{mode: info.Mode()}
		if info.Mode().IsRegular() {
			st.modTime = info.ModTime()
			fh, err := os.Open(p)
			if err != nil {
				return err
			}
			h := sha256.New()
			_, err = io.Copy(h, fh)
			fh.Close()
			if err != nil {
				return err
			}
			st.sha256 = fmt.Sprintf("%x", h.Sum(nil))
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[rel] = st
		return nil
	}))
	return out
}

// layerCacheFiles returns the layer copies (named "<index>-<alg>-<hex>") anywhere under root. Point TMPDIR at
// root before Provide so the image's content cache dir lands there.
func layerCacheFiles(t testing.TB, root string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && layerCacheName.MatchString(d.Name()) {
			out = append(out, p)
		}
		return nil
	}))
	return out
}

var layerCacheName = regexp.MustCompile(`^[0-9]+-sha256-[0-9a-f]{64}$`)

// D5: Read and Cleanup leave every file under the layout exactly as it was, and remove the cache dir.
func Test_Directory_Provider_inPlace_layoutUntouched(t *testing.T) {
	path, layerDigest := writeLayout(t, types.OCIUncompressedLayer, tarBlob(t))
	before := snapshotTree(t, path)

	tmpRoot := t.TempDir()
	t.Setenv("TMPDIR", tmpRoot)
	tmpDirGen := file.NewTempDirGenerator("tempDir")
	defer tmpDirGen.Cleanup()

	subject, err := NewDirectoryProvider(tmpDirGen, path).Provide(context.Background())
	require.NoError(t, err)
	require.Len(t, subject.Layers, 1)

	// the layer was indexed from the layout itself, so the read leaves no copy in the cache dir
	assert.Empty(t, layerCacheFiles(t, tmpRoot), "an in-place layer must not be copied into the cache dir")

	rc, err := subject.Layers[0].OpenPath("/etc/hello.txt")
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, "hello", string(got))

	require.NoError(t, subject.Cleanup())
	require.NoError(t, tmpDirGen.Cleanup())

	assert.Equal(t, before, snapshotTree(t, path), "the layout must be byte-, mode- and mtime-identical after Cleanup")
	_, err = os.Stat(layoutBlobPath(path, layerDigest))
	require.NoError(t, err, "the source blob must still exist")
	left, err := os.ReadDir(tmpRoot)
	require.NoError(t, err)
	assert.Empty(t, left, "Cleanup must remove the cache dir")
}

// D7: the blob is only ever opened for reading, so a read-only layout is indexed in place.
func Test_Directory_Provider_inPlace_readOnlyLayout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not restrict reads on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores permission bits, the test would prove nothing")
	}

	path, layerDigest := writeLayout(t, types.OCIUncompressedLayer, tarBlob(t))
	blobDir := filepath.Dir(layoutBlobPath(path, layerDigest))

	var restore []func()
	require.NoError(t, filepath.WalkDir(blobDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o444)
		if d.IsDir() {
			mode = 0o555
		}
		restore = append(restore, func() { _ = os.Chmod(p, 0o755) })
		return os.Chmod(p, mode)
	}))
	// t.TempDir cleanup needs write permission back to remove the tree
	t.Cleanup(func() {
		for i := len(restore) - 1; i >= 0; i-- {
			restore[i]()
		}
	})

	tmpRoot := t.TempDir()
	t.Setenv("TMPDIR", tmpRoot)
	tmpDirGen := file.NewTempDirGenerator("tempDir")
	defer tmpDirGen.Cleanup()

	subject, err := NewDirectoryProvider(tmpDirGen, path).Provide(context.Background())
	require.NoError(t, err)
	defer func() { require.NoError(t, subject.Cleanup()) }()

	assert.Empty(t, layerCacheFiles(t, tmpRoot), "a read-only layout must still be indexed in place")

	rc, err := subject.Layers[0].OpenPath("/etc/hello.txt")
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, "hello", string(got))
}

// D10: an OCI archive is untarred into a temp dir and indexed in place there; Cleanup removes that dir.
func Test_Archive_Provider_inPlace_tempDirRemoved(t *testing.T) {
	path, _ := writeLayout(t, types.OCIUncompressedLayer, tarBlob(t))
	tarPath := filepath.Join(t.TempDir(), "image.tar")
	tarballFromDir(t, path, tarPath)

	tmpRoot := t.TempDir()
	t.Setenv("TMPDIR", tmpRoot)
	tmpDirGen := file.NewTempDirGenerator("tempDir")
	defer tmpDirGen.Cleanup()

	subject, err := NewArchiveProvider(tmpDirGen, tarPath).Provide(context.Background())
	require.NoError(t, err)
	require.Len(t, subject.Layers, 1)

	assert.Empty(t, layerCacheFiles(t, tmpRoot), "the extracted blob is indexed in place, not copied")

	require.NoError(t, subject.Cleanup())
	require.NoError(t, tmpDirGen.Cleanup())

	left, err := os.ReadDir(tmpRoot)
	require.NoError(t, err)
	var names []string
	for _, e := range left {
		names = append(names, e.Name())
	}
	assert.Empty(t, names, "the extracted layout must not leak")
}

// B5: a blob that is a symlink to a regular file is located (ggcr follows symlinks too).
func Test_layoutBlobLocator_symlinkToRegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "blobs", "sha256"), 0o755))
	target := filepath.Join(t.TempDir(), "real-blob")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o600))
	link := filepath.Join(root, "blobs", "sha256", "aa")
	require.NoError(t, os.Symlink(target, link))

	got, ok := layoutBlobLocator(root)(v1.Hash{Algorithm: "sha256", Hex: "aa"})
	assert.True(t, ok)
	assert.Equal(t, link, got)
}
