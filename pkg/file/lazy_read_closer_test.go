package file

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeferredReadCloser(t *testing.T) {
	filepath := "test-fixtures/a-file.txt"
	allContent := getFixture(t, filepath)

	dReader := NewLazyReadCloser(filepath)
	require.Nil(t, dReader.file, "should not have a file, but we do somehow")

	actualContents, err := io.ReadAll(dReader)
	require.NotNil(t, dReader.file, "should have a file, but we do not somehow")
	require.NoError(t, err)
	require.Equal(t, allContent, actualContents)

	require.NoError(t, dReader.Close())
	require.Nil(t, dReader.file, "should not have a file, but we do somehow")
}

func TestLazyReader_ReadAt(t *testing.T) {
	filepath := "test-fixtures/a-file.txt"
	allContent := getFixture(t, filepath)

	dReader := NewLazyReadCloser(filepath)
	require.Nil(t, dReader.file, "should not have a file, but we do somehow")

	off := 5
	left := len(allContent) - off
	s := make([]byte, left)
	n, err := dReader.ReadAt(s, int64(off))
	require.NoError(t, err)
	require.Equal(t, left, n)
	require.Equal(t, allContent[off:], s)

	require.NoError(t, dReader.Close())
	require.Nil(t, dReader.file, "should not have a file, but we do somehow")

}

func TestLazyReader_Seek(t *testing.T) {
	filepath := "test-fixtures/a-file.txt"
	allContent := getFixture(t, filepath)

	dReader := NewLazyReadCloser(filepath)
	require.Nil(t, dReader.file, "should not have a file, but we do somehow")

	off := 5
	left := len(allContent) - off
	s := make([]byte, left)
	seek, err := dReader.Seek(int64(off), io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, seek, int64(off))

	n, err := dReader.Read(s)
	require.NoError(t, err)
	require.Equal(t, left, n)
	require.Equal(t, allContent[off:], s)

	require.NoError(t, dReader.Close())
	require.Nil(t, dReader.file, "should not have a file, but we do somehow")
}
