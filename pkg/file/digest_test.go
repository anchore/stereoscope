package file

import (
	"crypto"
	"crypto/md5" //nolint:gosec
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDigests(t *testing.T) {
	digests, err := NewDigests([]crypto.Hash{crypto.SHA256, crypto.MD5}, strings.NewReader("first file"))
	require.NoError(t, err)

	// values are the full-content hashes, named canonically, in the requested order
	assert.Equal(t, []Digest{
		{Algorithm: "sha256", Value: fmt.Sprintf("%x", sha256.Sum256([]byte("first file")))},
		{Algorithm: "md5", Value: fmt.Sprintf("%x", md5.Sum([]byte("first file")))}, //nolint:gosec
	}, digests)
}

func TestNewDigests_EmptyInputAndNoHashes(t *testing.T) {
	digests, err := NewDigests([]crypto.Hash{crypto.SHA256}, strings.NewReader(""))
	require.NoError(t, err)
	// empty files still digest: the empty-input hash, not an absent one
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", digests[0].Value)

	digests, err = NewDigests(nil, strings.NewReader("anything"))
	require.NoError(t, err)
	assert.Nil(t, digests)
}

func TestNewDigests_UnavailableAlgorithm(t *testing.T) {
	_, err := NewDigests([]crypto.Hash{crypto.MD4}, strings.NewReader("first file"))
	require.Error(t, err)
}

func TestDigestAlgorithmName(t *testing.T) {
	assert.Equal(t, "sha256", DigestAlgorithmName(crypto.SHA256))
	assert.Equal(t, "sha1", DigestAlgorithmName(crypto.SHA1))
	assert.Equal(t, "md5", DigestAlgorithmName(crypto.MD5))
}
