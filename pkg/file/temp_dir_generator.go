package file

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	"github.com/hashicorp/go-multierror"
)

type TempDirGenerator struct {
	tempDir []string
	lock    *sync.Mutex
}

func NewTempDirGenerator() TempDirGenerator {
	return TempDirGenerator{
		tempDir: make([]string, 0),
		lock:    &sync.Mutex{},
	}
}

// NewTempDir creates an empty dir in the platform temp dir
func (t *TempDirGenerator) NewTempDir() (string, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	dir, err := ioutil.TempDir("", "stereoscope-cache")
	if err != nil {
		return "", fmt.Errorf("could not create temp dir: %w", err)
	}

	t.tempDir = append(t.tempDir, dir)
	return dir, nil
}

func (t *TempDirGenerator) Cleanup() error {
	t.lock.Lock()
	defer t.lock.Unlock()

	var allErrors error
	for _, dir := range t.tempDir {
		err := os.RemoveAll(dir)
		if err != nil {
			allErrors = multierror.Append(allErrors, err)
		}
	}
	return allErrors
}
