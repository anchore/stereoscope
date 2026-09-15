package oci

import (
	"context"
	"errors"
	"runtime"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/internal/testutil"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/image"
)

func Test_NewProviderFromPath(t *testing.T) {
	//GIVEN
	path := "path"
	generator := file.TempDirGenerator{}
	defer generator.Cleanup()

	//WHEN
	provider := NewDirectoryProvider(&generator, path).(*directoryImageProvider)

	//THEN
	assert.NotNil(t, provider.path)
	assert.NotNil(t, provider.tmpDirGen)
}

func Test_Directory_Provider_no_platform(t *testing.T) {
	//GIVEN
	tests := []struct {
		name                     string
		fixturePath              string
		expectedErr              string
		expectedPlatformMetadata *image.Platform
	}{
		{"fails to read from path", "", "unable to read image from OCI directory path", nil},
		{"fails to read invalid oci manifest", "invalid_file", "unable to parse OCI directory indexManifest", nil},
		{"fails to read valid oci manifest with no images", "no_manifests", "no images found in OCI directory at path", nil},
		{"fails to read an invalid oci directory", "valid_manifest", "EOF", nil},
		// platform metadata is always reported, sourced from the image config rather than the index descriptor
		{"reads a valid oci directory", "valid_oci_dir", "", &image.Platform{Architecture: "amd64", OS: "linux"}},
		{"reads a single image with no platform descriptor", "single_image_no_platform_oci_dir", "", &image.Platform{Architecture: "amd64", OS: "linux"}},
		{"reads a single image referenced by several descriptors", "duplicate_platform_oci_dir", "", &image.Platform{Architecture: "amd64", OS: "linux"}},
		{"ignores buildx attestation manifests", "attestation_oci_dir", "", &image.Platform{Architecture: "amd64", OS: "linux"}},
		{"reads a multiplatform oci directory", "multiplatform_oci_dir", "", &image.Platform{Architecture: runtime.GOARCH, OS: "linux"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.fixturePath == "multiplatform_oci_dir" &&
				!(runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64") {
				t.Skipf("unsupported architecture for test: %s", runtime.GOARCH)
			}
			tmpDirGen := file.NewTempDirGenerator("tempDir")
			defer tmpDirGen.Cleanup()
			path := tc.fixturePath
			if path != "" {
				path = testutil.GetFixturePath(t, tc.fixturePath)
			}
			provider := NewDirectoryProvider(tmpDirGen, path)

			//WHEN
			image, err := provider.Provide(context.Background())

			//THEN
			if tc.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
				assert.Nil(t, image)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, image)
				assert.Equal(t, tc.expectedPlatformMetadata.Architecture, image.Metadata.Architecture)
				assert.Equal(t, tc.expectedPlatformMetadata.Variant, image.Metadata.Variant)
				assert.Equal(t, tc.expectedPlatformMetadata.OS, image.Metadata.OS)
			}

		})
	}
}

func Test_Directory_Provider_with_platform(t *testing.T) {
	//GIVEN
	singlePlatformDigest := "sha256:c1ed04a3da941a5dd09b58b16c37f065557863d382ef97995ddac885a8452ebb"
	multiplatformAmd64Digest := "sha256:e7c26a4b4d156fd9947ee82295b7b78acf7aa54b93b8f3e4b9f608179ffb20e8"
	multiplatformArm64Digest := "sha256:5ed07065bcbc6c52e3ad28526557d7b6833613fc79257b1a786de85e37c03b05"
	tests := []struct {
		name string
		// fixturePath is relative to testdata
		fixturePath string
		platform    *image.Platform
		// expectedDigest is the manifest digest of the image that should be selected
		expectedDigest  string
		expectedOS      string
		expectedArch    string
		expectedVariant string
		expectedErr     string
		// expectPlatformMismatch asserts the error is a *image.ErrPlatformMismatch, which is how callers
		// tell "wrong platform" apart from "could not read this input at all"
		expectPlatformMismatch bool
	}{
		{
			name:           "reads a single platform oci directory with correct platform",
			fixturePath:    "valid_oci_dir",
			platform:       &image.Platform{Architecture: "amd64", OS: "linux"},
			expectedDigest: singlePlatformDigest,
			expectedOS:     "linux",
			expectedArch:   "amd64",
		},
		{
			name:                   "rejects a single platform oci directory with a different platform",
			fixturePath:            "valid_oci_dir",
			platform:               &image.Platform{Architecture: "arm64", OS: "linux"},
			expectedErr:            `image platform="linux/amd64" does not match user specified platform="linux/arm64"`,
			expectPlatformMismatch: true,
		},
		{
			name:           "reads a single platform oci directory with no specified platform",
			fixturePath:    "valid_oci_dir",
			platform:       nil,
			expectedDigest: singlePlatformDigest,
			expectedOS:     "linux",
			expectedArch:   "amd64",
		},
		{
			// regression: the index descriptor platform is optional and is absent for images written by
			// `skopeo copy ... oci:<dir>`. The image config is what decides the match.
			name:           "reads a single image with no platform descriptor for a matching platform",
			fixturePath:    "single_image_no_platform_oci_dir",
			platform:       &image.Platform{Architecture: "amd64", OS: "linux"},
			expectedDigest: singlePlatformDigest,
			expectedOS:     "linux",
			expectedArch:   "amd64",
		},
		{
			name:                   "rejects a single image with no platform descriptor for a different platform",
			fixturePath:            "single_image_no_platform_oci_dir",
			platform:               &image.Platform{Architecture: "arm64", OS: "linux"},
			expectedErr:            `image platform="linux/amd64" does not match user specified platform="linux/arm64"`,
			expectPlatformMismatch: true,
		},
		{
			// regression: one image referenced by several index entries is still one image
			name:           "reads a single image referenced by several descriptors",
			fixturePath:    "duplicate_platform_oci_dir",
			platform:       &image.Platform{Architecture: "amd64", OS: "linux"},
			expectedDigest: singlePlatformDigest,
			expectedOS:     "linux",
			expectedArch:   "amd64",
		},
		{
			// regression: buildx attestation manifests must not count towards the image total
			name:           "ignores buildx attestation manifests",
			fixturePath:    "attestation_oci_dir",
			platform:       &image.Platform{Architecture: "amd64", OS: "linux"},
			expectedDigest: singlePlatformDigest,
			expectedOS:     "linux",
			expectedArch:   "amd64",
		},
		{
			// the descriptor claims linux/amd64 but the config says linux/arm64/v8. Trusting the
			// descriptor would report an architecture the image does not have.
			name:                   "rejects an index descriptor that disagrees with the image config",
			fixturePath:            "mismatched_descriptor_oci_dir",
			platform:               &image.Platform{Architecture: "amd64", OS: "linux"},
			expectedErr:            `image platform="linux/arm64/v8" does not match user specified platform="linux/amd64"`,
			expectPlatformMismatch: true,
		},
		{
			// variant comes from the config, not from the (variant-less) request
			name:            "reports the variant the image config declares",
			fixturePath:     "mismatched_descriptor_oci_dir",
			platform:        &image.Platform{Architecture: "arm64", OS: "linux"},
			expectedDigest:  "sha256:0a1332ee2b470d4fbfaae974cded394406b9aae094682e0d4113f27f6c3545fc",
			expectedOS:      "linux",
			expectedArch:    "arm64",
			expectedVariant: "v8",
		},
		{
			name:           "reads a multiplatform oci directory for linux/amd64",
			fixturePath:    "multiplatform_oci_dir",
			platform:       &image.Platform{Architecture: "amd64", OS: "linux"},
			expectedDigest: multiplatformAmd64Digest,
			expectedOS:     "linux",
			expectedArch:   "amd64",
		},
		{
			name:           "reads a multiplatform oci directory for linux/arm64",
			fixturePath:    "multiplatform_oci_dir",
			platform:       &image.Platform{Architecture: "arm64", OS: "linux"},
			expectedDigest: multiplatformArm64Digest,
			expectedOS:     "linux",
			expectedArch:   "arm64",
		},
		{
			name:         "reads a multiplatform oci directory with no specified platform",
			fixturePath:  "multiplatform_oci_dir",
			platform:     nil,
			expectedOS:   "linux",
			expectedArch: runtime.GOARCH,
		},
		{
			name:        "reads a multiplatform oci directory for an unlisted platform",
			fixturePath: "multiplatform_oci_dir",
			platform:    &image.Platform{Architecture: "ppc64le", OS: "linux"},
			expectedErr: "unexpected number of images matching platform \"linux/ppc64le\" in OCI directory (expected 1, found 0)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expectedDigest := tc.expectedDigest
			if tc.fixturePath == "multiplatform_oci_dir" && tc.platform == nil {
				switch runtime.GOARCH {
				case "amd64":
					expectedDigest = multiplatformAmd64Digest
				case "arm64":
					expectedDigest = multiplatformArm64Digest
				default:
					t.Skipf("unsupported architecture for test: %s", runtime.GOARCH)
				}
			}
			tmpDirGen := file.NewTempDirGenerator("tempDir")
			defer tmpDirGen.Cleanup()
			path := testutil.GetFixturePath(t, tc.fixturePath)
			provider := NewDirectoryProviderWithPlatform(tmpDirGen, path, tc.platform)

			//WHEN
			imageResult, err := provider.Provide(context.Background())

			//THEN
			if tc.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
				assert.Nil(t, imageResult)
				var pErr *image.ErrPlatformMismatch
				assert.Equal(t, tc.expectPlatformMismatch, errors.As(err, &pErr))
			} else {
				assert.NoError(t, err)
				require.NotNil(t, imageResult)
				assert.Equal(t, expectedDigest, imageResult.Metadata.ManifestDigest)
				assert.Equal(t, tc.expectedOS, imageResult.Metadata.OS)
				assert.Equal(t, tc.expectedArch, imageResult.Metadata.Architecture)
				assert.Equal(t, tc.expectedVariant, imageResult.Metadata.Variant)
			}
		})
	}
}

func Test_findAllImages(t *testing.T) {
	//GIVEN
	tests := []struct {
		name                   string
		fixturePath            string
		expectedImagePlatforms map[v1.Hash][]v1.Platform
	}{
		{
			"reads a valid oci directory",
			"valid_oci_dir",
			map[v1.Hash][]v1.Platform{
				{
					Algorithm: "sha256",
					Hex:       "c1ed04a3da941a5dd09b58b16c37f065557863d382ef97995ddac885a8452ebb",
				}: {
					{
						Architecture: "amd64",
						OS:           "linux",
					},
				},
			},
		},
		{
			"reads a multiplatform oci directory",
			"multiplatform_oci_dir",
			map[v1.Hash][]v1.Platform{
				{
					Algorithm: "sha256",
					Hex:       "e7c26a4b4d156fd9947ee82295b7b78acf7aa54b93b8f3e4b9f608179ffb20e8",
				}: {
					{
						Architecture: "amd64",
						OS:           "linux",
					},
				},
				{
					Algorithm: "sha256",
					Hex:       "5ed07065bcbc6c52e3ad28526557d7b6833613fc79257b1a786de85e37c03b05",
				}: {
					{
						Architecture: "arm64",
						OS:           "linux",
					},
				},
			},
		},
	}

	for _, tc := range tests {
		path := testutil.GetFixturePath(t, tc.fixturePath)
		t.Run(tc.name, func(t *testing.T) {
			imageIndex, err := layout.ImageIndexFromPath(path)
			require.NoError(t, err)

			//WHEN
			result, err := findAllImages(imageIndex)
			require.NoError(t, err)
			platformResults := make(map[v1.Hash][]v1.Platform)
			for digest, image := range result {
				platformResults[digest] = image.platforms
			}

			//THEN
			require.Equal(t, tc.expectedImagePlatforms, platformResults)
		})
	}
}

func Test_walkImages(t *testing.T) {
	type walkResult struct {
		digest   v1.Hash
		platform *v1.Platform
	}
	//GIVEN
	tests := []struct {
		name           string
		fixturePath    string
		expectedImages []walkResult
	}{
		{"reads a valid oci directory", "valid_oci_dir", []walkResult{
			{
				digest: v1.Hash{
					Algorithm: "sha256",
					Hex:       "c1ed04a3da941a5dd09b58b16c37f065557863d382ef97995ddac885a8452ebb",
				},
				platform: nil,
			},
			{
				digest: v1.Hash{
					Algorithm: "sha256",
					Hex:       "c1ed04a3da941a5dd09b58b16c37f065557863d382ef97995ddac885a8452ebb",
				},
				platform: &v1.Platform{
					Architecture: "amd64",
					OS:           "linux",
				},
			},
		}},
		{"reads a multiplatform oci directory", "multiplatform_oci_dir", []walkResult{
			{
				digest: v1.Hash{
					Algorithm: "sha256",
					Hex:       "e7c26a4b4d156fd9947ee82295b7b78acf7aa54b93b8f3e4b9f608179ffb20e8",
				},
				platform: &v1.Platform{
					Architecture: "amd64",
					OS:           "linux",
				},
			},
			{
				digest: v1.Hash{
					Algorithm: "sha256",
					Hex:       "5ed07065bcbc6c52e3ad28526557d7b6833613fc79257b1a786de85e37c03b05",
				},
				platform: &v1.Platform{
					Architecture: "arm64",
					OS:           "linux",
				},
			},
		}},
	}

	for _, tc := range tests {
		path := testutil.GetFixturePath(t, tc.fixturePath)
		t.Run(tc.name, func(t *testing.T) {
			imageIndex, err := layout.ImageIndexFromPath(path)
			require.NoError(t, err)

			//WHEN
			var responses []walkResult
			err = walkImages(imageIndex, func(i v1.Image, p *v1.Platform) error {
				digest, err := i.Digest()
				if err != nil {
					return err
				}
				responses = append(responses, walkResult{
					digest:   digest,
					platform: p,
				})
				return nil
			})
			require.NoError(t, err)

			//THEN
			require.Equal(t, tc.expectedImages, responses)
		})
	}
}
