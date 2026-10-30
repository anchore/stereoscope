package filetree

import (
	"errors"
	"fmt"
	"sync"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree/filenode"
	"github.com/anchore/stereoscope/pkg/tree"
)

var _ Reader = (*lazyReader)(nil)
var _ Searcher = (*lazySearcher)(nil)

// NewLazyReader returns a Reader that calls build the first time any of its methods is called and then
// delegates to the tree build returned. This lets a caller hand out a tree that may never be needed
// without paying to build it up front.
//
// build runs at most once, even when the reader is used from many goroutines at once: the first caller
// runs it and every other caller blocks until it is done, then all of them see the same tree. That
// first call does all of the work and cannot be cancelled, since none of the Reader methods take a
// context.
//
// If build fails (returns an error, returns no tree, or panics) it is not retried. Every method that can
// return an error returns that failure, now and on every later call, and the methods that cannot
// (AllFiles, AllRealPaths, HasPath and TreeReader) behave as if the tree were empty. The failure is
// logged once and returned wrapped, so compare it with errors.Is or errors.As rather than ==. A build
// that returns another lazy reader's failure unchanged is not logged again.
//
// build must not call methods on the reader it is building: the call would wait on itself forever. It
// should return a tree made with New (or a copy of one) if the reader is going to be passed to
// NewSearchContext.
func NewLazyReader(build func() (ReadWriter, error)) Reader {
	// OnceValues drops build once it has run, along with whatever it captured
	return &lazyReader{realize: sync.OnceValues(func() (t ReadWriter, err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("building lazy file tree panicked: %v", p)
			}
			if err == nil && isNilTree(t) {
				err = errors.New("building lazy file tree returned no tree")
			}
			if err != nil {
				// a build that returned, as is, the failure of a lazy reader below it (a layer squash
				// copying the one under it) was logged there. Anything it wrapped or joined is new
				if _, logged := err.(*loggedBuildError); !logged {
					log.WithFields("error", err).Warn("unable to build lazy file tree")
					err = &loggedBuildError{err}
				}
				t = New()
			}
		}()
		return build()
	})}
}

type lazyReader struct {
	// realize builds the tree on first use. The tree is never nil, so the methods that cannot report an
	// error always have something to delegate to.
	realize func() (ReadWriter, error)
}

// loggedBuildError is a failed build's error once realize has logged it, so that whatever passes it on (a
// lazy reader built from this one, a search context over it) doesn't log it again.
type loggedBuildError struct{ error }

func (e *loggedBuildError) Unwrap() error { return e.error }

// isNilTree reports whether t holds no tree, including a nil *FileTree in a non-nil interface, which would
// otherwise slip past a nil check and panic on first use.
func isNilTree(t ReadWriter) bool {
	ft, ok := t.(*FileTree)
	return t == nil || ok && ft == nil
}

func (r *lazyReader) AllFiles(types ...file.Type) []file.Reference {
	t, _ := r.realize()
	return t.AllFiles(types...)
}

func (r *lazyReader) TreeReader() tree.Reader {
	t, _ := r.realize()
	return t.TreeReader()
}

func (r *lazyReader) File(path file.Path, options ...LinkResolutionOption) (bool, *file.Resolution, error) {
	t, err := r.realize()
	if err != nil {
		return false, nil, err
	}
	return t.File(path, options...)
}

func (r *lazyReader) FilesByGlob(query string, options ...LinkResolutionOption) ([]file.Resolution, error) {
	t, err := r.realize()
	if err != nil {
		return nil, err
	}
	return t.FilesByGlob(query, options...)
}

func (r *lazyReader) AllRealPaths() []file.Path {
	t, _ := r.realize()
	return t.AllRealPaths()
}

func (r *lazyReader) ListPaths(dir file.Path) ([]file.Path, error) {
	t, err := r.realize()
	if err != nil {
		return nil, err
	}
	return t.ListPaths(dir)
}

func (r *lazyReader) HasPath(path file.Path, options ...LinkResolutionOption) bool {
	t, _ := r.realize()
	return t.HasPath(path, options...)
}

func (r *lazyReader) Walk(fn func(path file.Path, f filenode.FileNode) error, conditions *WalkConditions) error {
	t, err := r.realize()
	if err != nil {
		return err
	}
	return t.Walk(fn, conditions)
}

func (r *lazyReader) Copy() (ReadWriter, error) {
	t, err := r.realize()
	if err != nil {
		return nil, err
	}
	return t.Copy()
}

// lazySearcher is what NewSearchContext returns for a lazy reader: building the link resolution index
// walks every link in the index, so it waits for the first search, which also realizes the tree.
type lazySearcher struct {
	get func() (Searcher, error)
}

func newLazySearcher(tree *lazyReader, index IndexReader) *lazySearcher {
	return &lazySearcher{get: sync.OnceValues(func() (_ Searcher, err error) {
		defer func() {
			// only a failure of the search context itself is logged here: realize logs its own
			if p := recover(); p != nil {
				err = fmt.Errorf("building lazy search context panicked: %v", p)
				log.WithFields("error", err).Warn("unable to build lazy search context")
			}
		}()
		t, err := tree.realize()
		if err != nil {
			return nil, err
		}
		return NewSearchContext(t, index), nil
	})}
}

func (s *lazySearcher) SearchByPath(path string, options ...LinkResolutionOption) (*file.Resolution, error) {
	sc, err := s.get()
	if err != nil {
		return nil, err
	}
	return sc.SearchByPath(path, options...)
}

func (s *lazySearcher) SearchByGlob(patterns string, options ...LinkResolutionOption) ([]file.Resolution, error) {
	sc, err := s.get()
	if err != nil {
		return nil, err
	}
	return sc.SearchByGlob(patterns, options...)
}

func (s *lazySearcher) SearchByMIMEType(mimeTypes ...string) ([]file.Resolution, error) {
	sc, err := s.get()
	if err != nil {
		return nil, err
	}
	return sc.SearchByMIMEType(mimeTypes...)
}
