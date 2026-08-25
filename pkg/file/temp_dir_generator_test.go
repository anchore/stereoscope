package file

import (
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestTempDirGenerator_concurrent(t *testing.T) {
	root := NewTempDirGenerator("concurrent-prefix")
	t.Cleanup(func() { _ = root.Cleanup() })

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			gen := root.NewGenerator()
			if _, err := gen.NewDirectory("d"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
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
