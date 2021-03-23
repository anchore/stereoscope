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

func fileOrDirExists(t testing.TB, filename string) bool {
	t.Helper()
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
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
