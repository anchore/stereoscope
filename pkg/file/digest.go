package file

import (
	"crypto"
	"fmt"
	"hash"
	"io"
	"strings"
)

// Digest is a checksum of a file's full contents, named by the algorithm that produced it.
type Digest struct {
	Algorithm string
	Value     string
}

// DigestAlgorithmName is the canonical name for a hash algorithm: lowercase with separators
// dropped (crypto.SHA256 → "sha256"). This matches the normalization consumers such as syft
// apply to algorithm names, so precomputed digests can be matched up by name.
func DigestAlgorithmName(h crypto.Hash) string {
	return strings.ReplaceAll(strings.ToLower(h.String()), "-", "")
}

// NewDigests computes all requested hashes over the reader's full contents in a single pass.
// The result preserves the order of the requested hashes.
func NewDigests(hashes []crypto.Hash, contents io.Reader) ([]Digest, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	hashers := make([]hash.Hash, len(hashes))
	writers := make([]io.Writer, len(hashes))
	for idx, h := range hashes {
		if !h.Available() {
			return nil, fmt.Errorf("unavailable digest algorithm: %v", h)
		}
		hashers[idx] = h.New()
		writers[idx] = hashers[idx]
	}

	if _, err := io.Copy(io.MultiWriter(writers...), contents); err != nil {
		return nil, err
	}

	digests := make([]Digest, len(hashes))
	for idx, h := range hashes {
		digests[idx] = Digest{
			Algorithm: DigestAlgorithmName(h),
			Value:     fmt.Sprintf("%x", hashers[idx].Sum(nil)),
		}
	}
	return digests, nil
}
