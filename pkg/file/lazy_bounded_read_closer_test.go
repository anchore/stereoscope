package file

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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

	dReader := newLazyBoundedReadCloser(p, 0, int64(len(expectedContents)))

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
	dReader := newLazyBoundedReadCloser(p, int64(start), int64(size))

	actualContents, err := ioutil.ReadAll(dReader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(contents[start:start+size], actualContents) {
		t.Fatalf("unexpected contents: %s", string(actualContents))
	}

}

func Test_lazyBoundedReadCloser_Read(t *testing.T) {
	type fields struct {
		path   string
		file   *os.File
		reader *io.SectionReader
		start  int64
		size   int64
	}
	type args struct {
		b []byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int
		wantErr assert.ErrorAssertionFunc
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &lazyBoundedReadCloser{
				path:   tt.fields.path,
				file:   tt.fields.file,
				reader: tt.fields.reader,
				start:  tt.fields.start,
				size:   tt.fields.size,
			}
			got, err := d.Read(tt.args.b)
			if !tt.wantErr(t, err, fmt.Sprintf("Read(%v)", tt.args.b)) {
				return
			}
			assert.Equalf(t, tt.want, got, "Read(%v)", tt.args.b)
		})
	}
}
