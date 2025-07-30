package file

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func getFixture(t *testing.T, filepath string) []byte {
	fh, err := os.Open(filepath)
	require.NoError(t, err)
	expectedContents, err := io.ReadAll(fh)
	require.NoError(t, err)

	return expectedContents
}

func TestDeferredPartialReadCloser(t *testing.T) {
	p := "test-fixtures/a-file.txt"
	contents := getFixture(t, p)

	dReader := newLazyBoundedReadCloser(p, 0, int64(len(contents)))
	require.Nil(t, dReader.file)

	actualContents, err := io.ReadAll(dReader)
	require.NoError(t, err)

	require.Equal(t, contents, actualContents)
	require.Nil(t, dReader.reader) // file is closed when reader is nil at EOF

	// test EOF behavior
	ignore := make([]byte, 0, 16)
	eofBytesRead, err := dReader.Read(ignore)
	require.ErrorIs(t, err, io.EOF) // continues to return EOF for later reads
	require.Equal(t, 0, eofBytesRead)

	// able to seek after EOF
	_, err = dReader.Seek(0, io.SeekStart)
	require.NoError(t, err)
	require.NotNil(t, dReader.file) // file is reopened

	secondReadContents, err := io.ReadAll(dReader)
	require.NoError(t, err)
	require.Equal(t, contents, secondReadContents)
	require.Nil(t, dReader.reader) // file is closed when reader is nil at EOF

	require.NoError(t, dReader.Close())
	require.Nil(t, dReader.file, "should not have a file, but we do somehow")

	_, err = io.ReadAll(dReader)
	require.ErrorIs(t, err, os.ErrClosed)
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
	actualContent, err := io.ReadAll(dReader)
	require.NoError(t, err)

	require.Equal(t, content[int(off):], actualContent)
	require.Nil(t, dReader.reader) // file is closed when reader is nil at EOF

	require.NoError(t, dReader.Close())
	require.Nil(t, dReader.file, "should not have a file, but we do somehow")
}

func TestDeferredPartialReadCloser_PartialRead(t *testing.T) {
	p := "test-fixtures/a-file.txt"
	contents := getFixture(t, p)

	var start, size int64 = 10, 7
	dReader := newLazyBoundedReadCloser(p, start, size)

	actualContents, err := io.ReadAll(dReader)
	require.NoError(t, err)
	require.Equal(t, contents[start:start+size], actualContents)
}
