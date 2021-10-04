package file

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
)

func TestDeferredPartialReadCloser(t *testing.T) {
	p := "test-fixtures/a-file.txt"
	fh, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	expectedContents, err := ioutil.ReadAll(fh)
	if err != nil {
		t.Fatal(err)
	}

	dReader := newLazyBoundedReadSeekAtCloser(p, 0, int64(len(expectedContents)))

	if dReader.file != nil {
		t.Fatalf("should not have a file, but we do somehow")
	}

	actualContents, err := ioutil.ReadAll(dReader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(expectedContents, actualContents) {
		t.Fatalf("unexpected contents: %s", string(actualContents))
	}

	if dReader.file == nil {
		t.Fatalf("should have a file, but we do not somehow")
	}

	if err := dReader.Close(); err != nil {
		t.Fatal(err)
	}

	if dReader.file != nil {
		t.Fatalf("should not have a file, but we do somehow")
	}
}

func TestDeferredPartialReadCloser_PartialRead(t *testing.T) {
	p := "test-fixtures/a-file.txt"
	fh, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := ioutil.ReadAll(fh)
	if err != nil {
		t.Fatal(err)
	}

	var start, size = 10, 7
	dReader := newLazyBoundedReadSeekAtCloser(p, int64(start), int64(size))

	actualContents, err := ioutil.ReadAll(dReader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(contents[start:start+size], actualContents) {
		t.Fatalf("unexpected contents: %s", string(actualContents))
	}

}
