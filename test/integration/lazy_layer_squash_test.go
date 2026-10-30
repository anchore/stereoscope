//go:build !windows

package integration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
	"github.com/anchore/stereoscope/pkg/image"
	"github.com/anchore/stereoscope/pkg/imagetest"
)

// describeLayerSquash renders everything a caller can observe of one layer's squash (each path's metadata
// and link resolution, regular file contents, and glob and MIME searches) without file reference IDs, which
// differ between two reads of the same image.
func describeLayerSquash(t *testing.T, img *image.Image, idx int) []string {
	t.Helper()
	layer := img.Layers[idx]
	squash := layer.SquashedTree

	meta := func(ref *file.Reference) string {
		if ref == nil {
			return "<nil>"
		}
		e, err := img.FileCatalog.Get(*ref)
		if err != nil {
			return "<no entry: " + err.Error() + ">"
		}
		m := e.Metadata
		var mode, size string
		if m.FileInfo != nil {
			mode, size = m.Mode().String(), fmt.Sprint(m.Size())
		}
		return fmt.Sprintf("%s|type=%v|link=%s|mode=%s|size=%s|uid=%d|gid=%d|mime=%s",
			ref.RealPath, m.Type, m.LinkDestination, mode, size, m.UserID, m.GroupID, m.MIMEType)
	}
	resolution := func(r *file.Resolution) string {
		if r == nil {
			return "<nil>"
		}
		s := fmt.Sprintf("%s -> %s", r.RequestPath, meta(r.Reference))
		for _, lr := range r.LinkResolutions {
			s += fmt.Sprintf(" via(%s -> %s)", lr.RequestPath, meta(lr.Reference))
		}
		return s
	}

	var out []string
	paths := squash.AllRealPaths()
	slices.Sort(paths)
	for _, p := range paths {
		ok, r, err := squash.File(p)
		out = append(out, fmt.Sprintf("File(%s)=%v %s err=%v", p, ok, resolution(r), err))
		ok, r, err = squash.File(p, filetree.FollowBasenameLinks)
		out = append(out, fmt.Sprintf("FileFollow(%s)=%v %s err=%v", p, ok, resolution(r), err))
		if !ok || !r.HasReference() {
			continue
		}
		if e, err := img.FileCatalog.Get(*r.Reference); err != nil || e.Metadata.Type != file.TypeRegular {
			continue
		}
		rc, err := layer.OpenPathFromSquash(p)
		require.NoErrorf(t, err, "layer %d: open %s", idx, p)
		h := sha256.New()
		_, err = io.Copy(h, rc)
		require.NoError(t, rc.Close())
		require.NoError(t, err)
		out = append(out, fmt.Sprintf("Open(%s)=%x", p, h.Sum(nil)))
	}
	out = append(out, fmt.Sprintf("AllFiles=%d", len(squash.AllFiles(file.AllTypes()...))))

	search := func(kind string, results []file.Resolution, err error) {
		require.NoErrorf(t, err, "layer %d: %s search", idx, kind)
		var lines []string
		for i := range results {
			lines = append(lines, kind+" "+resolution(&results[i]))
		}
		slices.Sort(lines)
		out = append(out, lines...)
	}
	glob, err := layer.SquashedSearchContext.SearchByGlob("**/*", filetree.FollowBasenameLinks)
	search("glob", glob, err)
	mime, err := layer.SquashedSearchContext.SearchByMIMEType("text/plain", "application/x-executable", "application/x-sharedlib")
	search("mime", mime, err)
	return out
}

func TestImage_lazyLayerSquashesMatchEagerOnFixtureImages(t *testing.T) {
	// the synthetic fixtures in pkg/image pin the cases one at a time; this checks the same promise on real
	// images built by docker, whose layers mix links, hardlinks and opaque directories the way builds do
	for _, name := range []string{"image-symlinks", "image-hardlinks", "image-opaque-directory"} {
		t.Run(name, func(t *testing.T) {
			request := imagetest.PrepareFixtureImage(t, "docker-archive", name)

			eager, err := stereoscope.GetImage(context.Background(), request)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, eager.Cleanup()) })
			lazy, err := stereoscope.GetImage(image.WithLazyLayerSquashes(context.Background()), request)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, lazy.Cleanup()) })

			require.Len(t, lazy.Layers, len(eager.Layers))
			// top down, so the first touch of a high layer builds the chain below it
			for idx := len(eager.Layers) - 1; idx >= 0; idx-- {
				assert.Equalf(t, describeLayerSquash(t, eager, idx), describeLayerSquash(t, lazy, idx), "layer %d", idx)
			}
		})
	}
}
