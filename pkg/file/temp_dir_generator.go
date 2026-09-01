package file

import (
	"errors"
	"os"
	"strings"
	"sync"
)

type TempDirGenerator struct {
	// lock guards every field below it: one generator is shared by all providers in an
	// ImageProviders() call, so concurrent GetImage calls reach NewGenerator and NewDirectory at once
	lock         sync.Mutex
	rootPrefix   string
	rootLocation string
	children     []*TempDirGenerator
}

func NewTempDirGenerator(name string) *TempDirGenerator {
	return &TempDirGenerator{
		rootPrefix: name,
	}
}

func (t *TempDirGenerator) getOrCreateRootLocation() (string, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.rootLocation == "" {
		location, err := os.MkdirTemp("", t.rootPrefix+"-")
		if err != nil {
			return "", err
		}

		t.rootLocation = location
	}
	return t.rootLocation, nil
}

// NewGenerator creates a child generator capable of making sibling temp directories.
func (t *TempDirGenerator) NewGenerator() *TempDirGenerator {
	t.lock.Lock()
	defer t.lock.Unlock()

	gen := NewTempDirGenerator(t.rootPrefix)
	t.children = append(t.children, gen)
	return gen
}

// NewDirectory creates a new temp dir within the generators prefix temp dir.
func (t *TempDirGenerator) NewDirectory(name ...string) (string, error) {
	location, err := t.getOrCreateRootLocation()
	if err != nil {
		return "", err
	}

	return os.MkdirTemp(location, strings.Join(name, "-")+"-")
}

// Cleanup deletes all temp dirs created by this generator and any child generator.
func (t *TempDirGenerator) Cleanup() error {
	t.lock.Lock()
	children, rootLocation := t.children, t.rootLocation
	t.lock.Unlock()

	var errs []error
	// children hold their own locks, so recurse outside of ours
	for _, gen := range children {
		if err := gen.Cleanup(); err != nil {
			errs = append(errs, err)
		}
	}
	if rootLocation != "" {
		if err := os.RemoveAll(rootLocation); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
