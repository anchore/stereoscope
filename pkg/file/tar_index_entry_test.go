package file

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/internal/testutil"
)

func writeTestTar(t *testing.T, entries map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "layer.tar")
	fh, err := os.Create(path)
	require.NoError(t, err)
	tw := tar.NewWriter(fh)
	for name, content := range entries {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, fh.Close())
	return path
}

func TestTarIndexEntry_OpenIsPositionalAndConcurrent(t *testing.T) {
	entries := map[string]string{
		"a.txt": "alpha contents",
		"b.txt": "bravo contents which are longer",
		"c.txt": "",
	}
	index, err := NewTarIndex(writeTestTar(t, entries), nil)
	require.NoError(t, err)
	defer index.Close()

	// every entry reads its own bytes through the shared descriptor, concurrently
	var wg sync.WaitGroup
	for name, want := range entries {
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(name, want string) {
				defer wg.Done()
				entries := index.indexByName[name]
				require.Len(t, entries, 1)
				rc := entries[0].Open()
				got, err := io.ReadAll(rc)
				require.NoError(t, err)
				assert.Equal(t, want, string(got), name)
				require.NoError(t, rc.Close())
			}(name, want)
		}
	}
	wg.Wait()

	// the reader is seekable and supports ReadAt, as syft's union reader expects
	r := index.indexByName["b.txt"][0].Open().(interface {
		io.ReadCloser
		io.ReaderAt
		io.Seeker
	})
	buf := make([]byte, 5)
	_, err = r.ReadAt(buf, 6)
	require.NoError(t, err)
	assert.Equal(t, "conte", string(buf))
	_, err = r.Seek(-3, io.SeekEnd)
	require.NoError(t, err)
	rest, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "ger", string(rest))

	// a reader past the index's Close fails rather than reading garbage. Use a fresh reader: the one
	// above is drained, and a drained SectionReader reports EOF without ever touching the descriptor,
	// so it would pass this whether or not Close did anything
	fresh := index.indexByName["b.txt"][0].Open()
	require.NoError(t, index.Close())
	_, err = fresh.Read(buf)
	assert.ErrorIs(t, err, os.ErrClosed)
}

func TestTarIndex_CloseIsIdempotentAndRaceFree(t *testing.T) {
	index, err := NewTarIndex(writeTestTar(t, map[string]string{"a.txt": "alpha contents"}), nil)
	require.NoError(t, err)

	entry := index.indexByName["a.txt"][0]

	// readers and closers overlap deliberately: this is the window Image.Cleanup hits when a consumer
	// cancels mid-scan, and it must be free of data races as well as panics
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				rc := entry.Open()
				_, err := io.ReadAll(rc)
				if err != nil {
					// only tolerable failure is losing the race with Close
					assert.ErrorIs(t, err, os.ErrClosed)
				}
				assert.NoError(t, rc.Close())
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, index.Close())
		}()
	}
	wg.Wait()

	// closing again is a no-op rather than an error, and an entry opened strictly after Close reports
	// os.ErrClosed with the tar path attached rather than a bare "invalid argument"
	require.NoError(t, index.Close())
	_, err = io.ReadAll(entry.Open())
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestTarIndexEntry_OpenDoesNotLeakTheDescriptor(t *testing.T) {
	index, err := NewTarIndex(writeTestTar(t, map[string]string{"a.txt": "alpha contents"}), nil)
	require.NoError(t, err)
	defer index.Close()

	rc := index.indexByName["a.txt"][0].Open()

	// guards against embedding *io.SectionReader again: Outer would hand the caller the index's live
	// *os.File plus this entry's absolute offset, enough to read the whole tar and to close the
	// descriptor out from under every other reader
	_, escaped := rc.(interface {
		Outer() (io.ReaderAt, int64, int64)
	})
	assert.False(t, escaped, "entry reader must not expose the shared descriptor")

	// closing an entry reader is not observable, the index still owns the descriptor
	require.NoError(t, rc.Close())
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "alpha contents", string(got))
}

func TestNewTarIndex_ReleasesTheDescriptorOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a.tar")
	require.NoError(t, os.WriteFile(path, []byte("this is not a tar archive"), 0o600))

	// counting descriptors rather than unlinking the file: on unix os.Remove succeeds with the
	// handle still open, so it would pass whether or not the failure path closed anything
	before := testutil.OpenDescriptorCount(t)

	index, err := NewTarIndex(path, nil)
	require.Error(t, err)
	assert.Nil(t, index, "a failed index must not be handed back half-built")

	// the caller has no index to close, so this path has to release the handle itself
	assert.Equal(t, before, testutil.OpenDescriptorCount(t), "the failed index leaked its tar descriptor")
}
