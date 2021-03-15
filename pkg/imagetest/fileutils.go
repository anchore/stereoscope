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

func dirHash(t testing.TB, root string) (string, error) {
	t.Helper()
	hasher := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() {
			err := f.Close()
			if err != nil {
				panic(err)
			}
		}()

		if _, err := io.Copy(hasher, f); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}
