package oci

import (
	"context"
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
		{"reads a valid oci directory", "valid_oci_dir", "", nil},
		{"reads a multiplatform oci directory", "multiplatform_oci_dir", "", &image.Platform{Architecture: runtime.GOARCH, OS: "linux"}},
	}

	for _, tc := range tests {
		tmpDirGen := file.NewTempDirGenerator("tempDir")
		path := tc.fixturePath
		if path != "" {
			path = testutil.GetFixturePath(t, tc.fixturePath)
		}
		provider := NewDirectoryProvider(tmpDirGen, path)
		t.Run(tc.name, func(t *testing.T) {
			defer tmpDirGen.Cleanup()
			if tc.fixturePath == "multiplatform_oci_dir" &&
				!(runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64") {
				t.Skipf("unsupported architecture for test: %s", runtime.GOARCH)
			}
			//WHEN
			image, err := provider.Provide(context.Background())

			//THEN
			if tc.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
				assert.Nil(t, image)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, image)
				if tc.expectedPlatformMetadata == nil {
					assert.Empty(t, image.Metadata.Architecture)
					assert.Empty(t, image.Metadata.Variant)
					assert.Empty(t, image.Metadata.OS)
				} else {
					assert.Equal(t, tc.expectedPlatformMetadata.Architecture, image.Metadata.Architecture)
					assert.Equal(t, tc.expectedPlatformMetadata.Variant, image.Metadata.Variant)
					assert.Equal(t, tc.expectedPlatformMetadata.OS, image.Metadata.OS)
				}
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
		name           string
		fixturePath    string
		platform       *image.Platform
		expectedDigest string
		expectedOS     string
		expectedArch   string
		expectedErr    string
	}{
		{
			"reads a single platform oci directory with correct platform",
			"valid_oci_dir",
			&image.Platform{Architecture: "amd64", OS: "linux"},
			singlePlatformDigest,
			"linux",
			"amd64",
			"",
		},
		{
			"reads a single platform oci directory with different platform",
			"valid_oci_dir",
			&image.Platform{Architecture: "arm64", OS: "linux"},
			"",
			"",
			"",
			"unexpected number of images matching platform \"linux/arm64\" in OCI directory (expected 1, found 0)",
		},
		{
			"reads a single platform oci directory with no specified platform",
			"valid_oci_dir",
			nil,
			singlePlatformDigest,
			"",
			"",
			"",
		},
		{
			"reads a multiplatform oci directory for linux/amd64", "multiplatform_oci_dir",
			&image.Platform{Architecture: "amd64", OS: "linux"},
			multiplatformAmd64Digest,
			"linux",
			"amd64",
			"",
		},
		{
			"reads a multiplatform oci directory for linux/arm64", "multiplatform_oci_dir",
			&image.Platform{Architecture: "arm64", OS: "linux"},
			multiplatformArm64Digest,
			"linux",
			"arm64",
			"",
		},
		{
			"reads a multiplatform oci directory with no specified platform", "multiplatform_oci_dir",
			nil,
			"",
			"linux",
			runtime.GOARCH,
			"",
		},
		{
			"reads a multiplatform oci directory for an unlisted platform", "multiplatform_oci_dir",
			&image.Platform{Architecture: "ppc64le", OS: "linux"},
			"",
			"",
			"",
			"unexpected number of images matching platform \"linux/ppc64le\" in OCI directory (expected 1, found 0)",
		},
	}

	for _, tc := range tests {
		tmpDirGen := file.NewTempDirGenerator("tempDir")
		path := testutil.GetFixturePath(t, tc.fixturePath)
		provider := NewDirectoryProviderWithPlatform(tmpDirGen, path, tc.platform)
		t.Run(tc.name, func(t *testing.T) {
			defer tmpDirGen.Cleanup()
			if tc.fixturePath == "multiplatform_oci_dir" && tc.platform == nil {
				switch runtime.GOARCH {
				case "amd64":
					tc.expectedDigest = multiplatformAmd64Digest
				case "arm64":
					tc.expectedDigest = multiplatformArm64Digest
				default:
					t.Skipf("unsupported architecture for test: %s", runtime.GOARCH)
				}
			}

			//WHEN
			imageResult, err := provider.Provide(context.Background())

			//THEN
			if tc.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
				assert.Nil(t, imageResult)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, imageResult)
				assert.Equal(t, tc.expectedDigest, imageResult.Metadata.ManifestDigest)
				assert.Equal(t, tc.expectedOS, imageResult.Metadata.OS)
				assert.Equal(t, tc.expectedArch, imageResult.Metadata.Architecture)
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
