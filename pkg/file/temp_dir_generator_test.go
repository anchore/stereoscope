package file

import (
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTempDirGenerator(t *testing.T) {
	tests := []struct {
		name            string
		genPrefix       string
		names           []string
		extraGenerators int
	}{
		{
			name:      "3 temp dirs",
			genPrefix: "a-special-prefix",
			names: []string{
				"a",
				"bee",
				"si",
			},
		},
		{
			name:      "3 temp dirs on the root generator + 2 extra generators",
			genPrefix: "b-special-prefix",
			names: []string{
				"a",
				"bee",
				"si",
			},
			extraGenerators: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectedPrefix := path.Join(os.TempDir(), test.genPrefix)

			assert.True(t, !doesGlobExist(t, expectedPrefix+"*"),
				"prefix temp dir already exists before test started")

			root := NewTempDirGenerator(test.genPrefix)

			for _, n := range test.names {
				d, err := root.NewDirectory(n)
				assert.NoError(t, err)
				assert.True(t, doesGlobExist(t, d), "sub-temp dir does not exist (root)")
				assert.Contains(t, d, expectedPrefix)
				assert.NotEmpty(t, root.rootLocation)
				assert.Contains(t, d, root.rootLocation)
			}

			assert.True(t, doesGlobExist(t, expectedPrefix+"*"), "prefix temp dir does not exist")

			var gen *TempDirGenerator
			for i := 0; i < test.extraGenerators; i++ {
				gen = root.NewGenerator()
				for _, n := range test.names {
					d, err := gen.NewDirectory(n)
					assert.NoError(t, err)
					assert.True(t, doesGlobExist(t, d), "sub-temp dir does not exist (sub)")
					assert.Contains(t, d, expectedPrefix)
					assert.NotEmpty(t, gen.rootLocation)
					assert.Contains(t, d, gen.rootLocation)
				}

			}

			assert.NoError(t, root.Cleanup())

			assert.True(t, !doesGlobExist(t, expectedPrefix+"*"), "cleanup did not remove prefix temp dir")

		})
	}
}

func doesGlobExist(t *testing.T, pattern string) bool {
	t.Helper()
	m, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) > 0 {
		return true
	}
	return false
}

func TestTempDirGenerator_ConcurrentUse(t *testing.T) {
	root := NewTempDirGenerator("concurrent")
	defer func() {
		if err := root.Cleanup(); err != nil {
			t.Fatal(err)
		}
	}()

	// stereoscope.GetImage builds a child generator per call off one shared root, so concurrent
	// callers land in NewGenerator and NewDirectory at the same time
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gen := root.NewGenerator()
			if _, err := gen.NewDirectory("content"); err != nil {
				t.Error(err)
			}
			if _, err := root.NewDirectory("root-content"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	if len(root.children) != 16 {
		t.Errorf("expected 16 children, got %d", len(root.children))
	}
}

// Cleanup detaches the root before removing it, so a NewDirectory that races the removal
// builds a fresh root rather than creating a dir inside one being deleted (which would
// leave an orphan behind and fail the removal with ENOTEMPTY).
func TestTempDirGenerator_cleanupDuringNewDirectory(t *testing.T) {
	for i := 0; i < 50; i++ {
		gen := NewTempDirGenerator("cleanup-race-prefix")
		// enough entries that RemoveAll's readdir pass takes a while
		for j := 0; j < 64; j++ {
			_, err := gen.NewDirectory("fill")
			require.NoError(t, err)
		}

		var dirs []string
		var mu sync.Mutex
		var cleanupErr error
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			cleanupErr = gen.Cleanup()
		}()
		go func() {
			defer wg.Done()
			<-start
			for k := 0; k < 16; k++ {
				d, err := gen.NewDirectory("racer")
				require.NoError(t, err, "a NewDirectory racing Cleanup must get a usable dir")
				mu.Lock()
				dirs = append(dirs, d)
				mu.Unlock()
			}
		}()
		close(start)
		wg.Wait()

		require.NoError(t, cleanupErr, "cleanup must not trip over a concurrently created dir")

		// whatever the racer created is still tracked, so a shutdown cleanup reclaims it
		require.NoError(t, gen.Cleanup())
		for _, d := range dirs {
			assert.NoDirExists(t, d, "cleanup left an orphaned dir behind")
			assert.NoDirExists(t, path.Dir(d), "cleanup left an orphaned root behind")
		}
	}
}

// Cleanup is not a one-way door: providers share one generator (see providers.go), and one
// provider cleaning up on failure must not poison the generator for the providers after it.
func TestTempDirGenerator_reuseAfterCleanup(t *testing.T) {
	gen := NewTempDirGenerator("reuse-prefix")
	t.Cleanup(func() { assert.NoError(t, gen.Cleanup()) })

	first, err := gen.NewDirectory("first")
	require.NoError(t, err)
	require.DirExists(t, first)

	require.NoError(t, gen.Cleanup())
	assert.NoDirExists(t, first)

	second, err := gen.NewDirectory("second")
	require.NoError(t, err, "generator must be reusable after cleanup")
	require.DirExists(t, second)
	assert.NotContains(t, second, path.Dir(first), "reuse must start a fresh root")

	// cleanup is idempotent
	require.NoError(t, gen.Cleanup())
	require.NoError(t, gen.Cleanup())
	assert.NoDirExists(t, second)
}
