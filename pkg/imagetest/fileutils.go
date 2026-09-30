package imagetest

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func copyFile(t testing.TB, src, dst string) {
	t.Helper()

	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("could not open src (%s): %+v", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("could not open dst (%s): %+v", dst, err)
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		t.Fatalf("could not copy file (%s -> %s): %+v", src, dst, err)
	}
}

func fileExists(t testing.TB, filename string) bool {
	t.Helper()
	s, err := os.Stat(filename)
	return !os.IsNotExist(err) && !s.IsDir()
}

func dirExists(t testing.TB, filename string) bool {
	t.Helper()
	s, err := os.Stat(filename)
	return !os.IsNotExist(err) && s.IsDir()
}

func dirHash(t testing.TB, root string) string {
	hasher := sha256.New()
	walkFn := func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("unable to walk path=%q : %w", path, err)
		}

		// walk does not provide Lstat info, only stat info...
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("unable to lstat path=%q : %w", path, err)
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("unable to open path=%q : %w", path, err)
		}
		defer func() {
			err := f.Close()
			if err != nil {
				t.Fatalf("unable to close walk root=%q path=%q : %+v", root, path, err)
			}
		}()

		if _, err := io.Copy(hasher, f); err != nil {
			return fmt.Errorf("unable to copy path=%q : %w", path, err)
		}

		return nil
	}
	if err := walk(root, walkFn); err != nil {
		t.Fatalf("unable to hash %q : %+v", root, err)
	}
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func walkEvaluateLinks(root string, virtualPath string, fn filepath.WalkFunc) error {
	symWalkFunc := func(path string, info os.FileInfo, err error) error {
		if relativePath, err := filepath.Rel(root, path); err == nil {
			path = filepath.Join(virtualPath, relativePath)
		} else {
			return err
		}

		if err == nil && info.Mode()&os.ModeSymlink == os.ModeSymlink {
			finalPath, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			info, err := os.Lstat(finalPath)
			if err != nil {
				return fn(path, info, err)
			}
			if info.IsDir() {
				return walkEvaluateLinks(finalPath, path, fn)
			}
		}

		return fn(path, info, err)
	}
	return filepath.Walk(root, symWalkFunc)
}

func walk(root string, fn filepath.WalkFunc) error {
	return walkEvaluateLinks(root, root, fn)
}

// writeFileAtomic writes to a temp file next to path and renames it into place once write succeeds, so concurrent
// readers only ever see no file or a complete one (never a truncated or partially written file).
func writeFileAtomic(path string, write func(w io.Writer) error) (err error) {
	// the temp file must be in the same dir for the rename to be atomic (same filesystem). The suffix keeps it from
	// matching the "*.tar" globs used to collect the fixture cache.
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("unable to create temp file for %q: %w", path, err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	if err = write(tmp); err != nil {
		return err
	}
	// CreateTemp uses 0600, keep the permissions os.Create would have given the file
	if err = tmp.Chmod(0o644); err != nil {
		return fmt.Errorf("unable to set permissions on %q: %w", tmp.Name(), err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("unable to close %q: %w", tmp.Name(), err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("unable to move %q into place: %w", path, err)
	}
	return nil
}
