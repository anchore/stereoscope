package file

import (
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func getFixture(t *testing.T, filepath string) []byte {
	fh, err := os.Open(filepath)
	require.NoError(t, err)
	expectedContents, err := ioutil.ReadAll(fh)
	require.NoError(t, err)

	return expectedContents
}

func TestDeferredPartialReadCloser(t *testing.T) {
	p := "test-fixtures/a-file.txt"
	contents := getFixture(t, p)

	dReader := newLazyBoundedReadCloser(p, 0, int64(len(contents)))
	require.Nil(t, dReader.file)

	actualContents, err := ioutil.ReadAll(dReader)
	require.NoError(t, err)

	require.Equal(t, contents, actualContents)
	require.NotNil(t, dReader.file)

	require.NoError(t, dReader.Close())
	require.Nil(t, dReader.file, "should not have a file, but we do somehow")
}

func TestDeferredPartialReadCloser_Seek(t *testing.T) {
	p := "test-fixtures/a-file.txt"
	content := getFixture(t, p)

	dReader := newLazyBoundedReadCloser(p, 0, int64(len(content)))
	require.Nil(t, dReader.file)

	var off int64 = 5
	seek, err := dReader.Seek(off, io.SeekStart)
	require.Equal(t, off, seek)
	require.NoError(t, err)
	actualContent, err := ioutil.ReadAll(dReader)
	require.NoError(t, err)

	require.Equal(t, content[int(off):], actualContent)
	require.NotNil(t, dReader.file)

	require.NoError(t, dReader.Close())
	require.Nil(t, dReader.file, "should not have a file, but we do somehow")
}

func TestDeferredPartialReadCloser_PartialRead(t *testing.T) {
	p := "test-fixtures/a-file.txt"
	contents := getFixture(t, p)

	var start, size int64 = 10, 7
	dReader := newLazyBoundedReadCloser(p, start, size)

	actualContents, err := ioutil.ReadAll(dReader)
	require.NoError(t, err)
	require.Equal(t, contents[start:start+size], actualContents)
}
