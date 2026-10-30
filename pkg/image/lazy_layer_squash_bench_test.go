package image

import (
	"archive/tar"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"

	"github.com/anchore/stereoscope/pkg/file"
)

// many-layer image for comparing eager and lazy layer squashes. Layer 0 has benchFilesPerLayer files,
// every layer above rewrites benchOverwrites of the files below it, adds benchNewFiles new ones and a
// few symlinks, so the image squash ends up with about 2000 + 39*500 files.
const (
	benchLayerCount    = 40
	benchFilesPerLayer = 2000
	benchOverwrites    = 1500
	benchNewFiles      = benchFilesPerLayer - benchOverwrites
	benchLinksPerLayer = 5
)

func benchFilePath(n int) string {
	return fmt.Sprintf("usr/d%02d/f%05d", n%50, n)
}

func benchLazySquashLayers(b *testing.B) []v1.Layer {
	b.Helper()
	dir := b.TempDir()
	reg, sym := byte(tar.TypeReg), byte(tar.TypeSymlink)

	var layers []v1.Layer
	total := 0
	for idx := 0; idx < benchLayerCount; idx++ {
		var entries []tarEntry
		if idx == 0 {
			for n := 0; n < benchFilesPerLayer; n++ {
				entries = append(entries, tarEntry{path: benchFilePath(n), typeFlag: reg, contents: "v0"})
			}
			total = benchFilesPerLayer
		} else {
			// a rolling window over the files below, so each layer rewrites a different slice
			for j := 0; j < benchOverwrites; j++ {
				n := (idx*benchOverwrites + j) % total
				entries = append(entries, tarEntry{path: benchFilePath(n), typeFlag: reg, contents: fmt.Sprintf("v%d", idx)})
			}
			for j := 0; j < benchNewFiles; j++ {
				entries = append(entries, tarEntry{path: benchFilePath(total + j), typeFlag: reg, contents: fmt.Sprintf("v%d", idx)})
			}
			total += benchNewFiles
			for j := 0; j < benchLinksPerLayer; j++ {
				entries = append(entries, tarEntry{
					path:     fmt.Sprintf("usr/link-%d-%d", idx, j),
					typeFlag: sym,
					linkPath: "/" + benchFilePath(j*7),
				})
			}
		}
		layers = append(layers, benchWriteLayer(b, filepath.Join(dir, fmt.Sprintf("layer-%02d.tar", idx)), entries))
	}
	return layers
}

// benchWriteLayer is layerFromTarEntries for a benchmark.
func benchWriteLayer(b *testing.B, tarPath string, entries []tarEntry) v1.Layer {
	b.Helper()
	fh, err := os.Create(tarPath)
	if err != nil {
		b.Fatal(err)
	}
	tw := tar.NewWriter(fh)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.path, Typeflag: e.typeFlag, Linkname: e.linkPath, Mode: 0o644, Size: int64(len(e.contents))}
		if err := tw.WriteHeader(hdr); err != nil {
			b.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.contents)); err != nil {
			b.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		b.Fatal(err)
	}
	if err := fh.Close(); err != nil {
		b.Fatal(err)
	}
	layer, err := tarball.LayerFromFile(tarPath)
	if err != nil {
		b.Fatal(err)
	}
	return layer
}

// heapSampler polls HeapInuse while the timed work runs, since the runtime keeps no peak.
// polling every 5ms can miss a short spike, which is good enough to compare eager vs lazy.
func heapSampler() (stop func() uint64) {
	var peak atomic.Uint64
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		var ms runtime.MemStats
		t := time.NewTicker(5 * time.Millisecond)
		defer t.Stop()
		for {
			runtime.ReadMemStats(&ms)
			if ms.HeapInuse > peak.Load() {
				peak.Store(ms.HeapInuse)
			}
			select {
			case <-done:
				return
			case <-t.C:
			}
		}
	}()
	return func() uint64 {
		close(done)
		<-finished
		return peak.Load()
	}
}

// retainedHeap is the live heap after a full GC.
func retainedHeap() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func BenchmarkLazyLayerSquash(b *testing.B) {
	layers := benchLazySquashLayers(b)
	v1Img, err := mutate.AppendLayers(empty.Image, layers...)
	if err != nil {
		b.Fatal(err)
	}

	touchSquashed := func(b *testing.B, img *Image) {
		if len(img.SquashedTree().AllFiles()) == 0 {
			b.Fatal("empty image squash")
		}
		if _, err := img.SquashedSearchContext.SearchByPath("/usr/link-1-0"); err != nil {
			b.Fatal(err)
		}
	}
	touchAllLayers := func(b *testing.B, img *Image) {
		for _, l := range img.Layers {
			if len(l.SquashedTree.AllFiles()) == 0 {
				b.Fatal("empty layer squash")
			}
			if _, err := l.SquashedSearchContext.SearchByPath("/usr/link-1-0"); err != nil {
				b.Fatal(err)
			}
		}
	}

	cases := []struct {
		name  string
		lazy  bool
		touch func(*testing.B, *Image)
	}{
		{name: "squashed/eager", touch: touchSquashed},
		{name: "squashed/lazy", lazy: true, touch: touchSquashed},
		{name: "all-layers/eager", touch: touchAllLayers},
		{name: "all-layers/lazy", lazy: true, touch: touchAllLayers},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			ctx := context.Background()
			if tc.lazy {
				ctx = WithLazyLayerSquashes(ctx)
			}
			var readNs, retainedRead, retainedTouch, peak float64
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				img := New(v1Img, file.NewTempDirGenerator("bench"), b.TempDir())
				base := retainedHeap()
				stopSampler := heapSampler()
				b.StartTimer()

				start := time.Now()
				if err := img.Read(ctx); err != nil {
					b.Fatal(err)
				}
				readNs += float64(time.Since(start).Nanoseconds())

				b.StopTimer()
				retainedRead += float64(int64(retainedHeap()) - int64(base))
				b.StartTimer()

				tc.touch(b, img)

				b.StopTimer()
				peak += float64(stopSampler())
				retainedTouch += float64(int64(retainedHeap()) - int64(base))
				runtime.KeepAlive(img)
				if err := img.Cleanup(); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
			}
			const mb = 1 << 20
			N := float64(b.N)
			b.ReportMetric(readNs/N/1e6, "read-ms")
			b.ReportMetric(retainedRead/N/mb, "retained-read-MB")
			b.ReportMetric(retainedTouch/N/mb, "retained-touch-MB")
			b.ReportMetric(peak/N/mb, "peak-inuse-MB")
		})
	}
}
