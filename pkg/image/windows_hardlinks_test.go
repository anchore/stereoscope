package image

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/filetree"
)

// treeOf builds a layer tree from entries: "p" is a file, "p/" a directory and "p -> t" a symlink.
func treeOf(t *testing.T, entries ...string) *filetree.FileTree {
	t.Helper()
	tree := filetree.New()
	for _, e := range entries {
		var err error
		switch {
		case strings.Contains(e, " -> "):
			p, target, _ := strings.Cut(e, " -> ")
			_, err = tree.AddSymLink(file.Path(p), file.Path(target))
		case strings.HasSuffix(e, "/"):
			_, err = tree.AddDir(file.Path(strings.TrimSuffix(e, "/")))
		default:
			_, err = tree.AddFile(file.Path(e))
		}
		require.NoError(t, err, e)
	}
	return tree
}

func indexOf(tree *filetree.FileTree) func() foldedIndex {
	return func() foldedIndex { return foldPaths(tree) }
}

func TestResolveWindowsLink(t *testing.T) {
	const (
		sys  = "/Files/Windows/System32/x.dll"
		uvm  = "/UtilityVM/Files/Windows/System32/x.dll"
		none = -1
	)
	tests := []struct {
		name string
		link string
		// own is the link's layer, below the layers under it, nearest first
		own   []string
		below [][]string
		// want is which layer of below the link resolves to, or none
		want int
	}{
		{name: "in the layer below", link: "Files/Windows/System32/x.dll", below: [][]string{{sys}}, want: 0},
		{name: "the nearest layer with the target wins", link: "Files/Windows/System32/x.dll", below: [][]string{{sys}, {sys}}, want: 0},
		{name: "found further down", link: "Files/Windows/System32/x.dll", below: [][]string{{"/Files/other.dll"}, {sys}}, want: 1},
		{name: "in no layer below", link: "Files/Windows/System32/x.dll", below: [][]string{{"/Files/other.dll"}}, want: none},
		{name: "no layers below", link: "Files/Windows/System32/x.dll", want: none},
		{name: "a .. that stays inside Files is cleaned", link: "Files/Windows/WinSxS/../System32/x.dll", below: [][]string{{sys}}, want: 0},

		// Files: whiteouts and the link's own layer don't count
		{name: "a whiteout in between is not consulted", link: "Files/Windows/System32/x.dll", below: [][]string{{"/Files/Windows/System32/.wh.x.dll"}, {sys}}, want: 1},
		{name: "a whiteout of a directory in between is not consulted", link: "Files/Windows/System32/x.dll", below: [][]string{{"/Files/Windows/.wh.System32"}, {sys}}, want: 1},
		{name: "the link's own layer is not consulted", link: "Files/Windows/System32/x.dll", own: []string{"/Files/windows/system32/X.DLL"}, below: [][]string{{sys}}, want: 0},

		// what hcsshim refuses
		{name: "a target outside Files", link: "Hives/x", below: [][]string{{"/Hives/x"}}, want: none},
		{name: "a target with a leading separator", link: "/Files/Windows/System32/x.dll", below: [][]string{{sys}}, want: none},
		{name: "a target that climbs out of Files", link: "../Files/Windows/System32/x.dll", below: [][]string{{sys}}, want: none},
		{name: "a target that climbs out of Files from inside", link: "Files/../../Files/Windows/System32/x.dll", below: [][]string{{sys}}, want: none},
		{name: "a target with a backslash", link: `Files/Windows\System32/x.dll`, below: [][]string{{`/Files/Windows\System32/x.dll`}}, want: none},
		{name: "an empty target", link: "", below: [][]string{{sys}}, want: none},
		{name: "a target that is a directory", link: "Files/Windows/System32", below: [][]string{{"/Files/Windows/System32/"}}, want: none},
		{name: "a target that is only a parent of something", link: "Files/Windows/System32", below: [][]string{{sys}}, want: none},

		// what NTFS and safefile do that a literal lookup wouldn't
		{name: "a nearer layer has the name in another case", link: "Files/Windows/System32/x.dll", below: [][]string{{"/Files/windows/system32/X.DLL"}, {sys}}, want: none},
		{name: "the target's layer has the name in two cases", link: "Files/Windows/System32/x.dll", below: [][]string{{sys, "/Files/Windows/System32/X.DLL"}}, want: none},
		{name: "a nearer layer has the name in two cases", link: "Files/Windows/System32/x.dll", below: [][]string{{sys, "/Files/Windows/System32/X.DLL"}, {sys}}, want: none},
		{name: "the target's layer has a directory in two cases", link: "Files/Windows/System32/x.dll", below: [][]string{{sys, "/Files/WINDOWS/"}}, want: none},
		{name: "only the target in another case", link: "Files/Windows/System32/x.dll", below: [][]string{{"/Files/Windows/System32/X.dll"}}, want: none},
		{name: "a symlink on the way in a nearer layer", link: "Files/Windows/System32/x.dll", below: [][]string{{"/Files/WINDOWS -> /elsewhere"}, {sys}}, want: none},
		{name: "a file on the way in a nearer layer", link: "Files/Windows/System32/x.dll", below: [][]string{{"/Files/Windows"}, {sys}}, want: none},
		{name: "a symlink at the target in a nearer layer", link: "Files/Windows/System32/x.dll", below: [][]string{{sys[:len(sys)-len("x.dll")] + "x.dll -> /elsewhere"}, {sys}}, want: 0},

		// the utility VM is the parent's clone, where whiteouts delete and this layer's entries land
		{name: "a utility VM target", link: "UtilityVM/Files/Windows/System32/x.dll", below: [][]string{{uvm}}, want: 0},
		{name: "a utility VM target whited out in between", link: "UtilityVM/Files/Windows/System32/x.dll", below: [][]string{{"/UtilityVM/Files/Windows/System32/.wh.x.dll"}, {uvm}}, want: none},
		{name: "a utility VM target under a directory whited out in between", link: "UtilityVM/Files/Windows/System32/x.dll", below: [][]string{{"/UtilityVM/Files/Windows/.wh.System32"}, {uvm}}, want: none},
		{name: "a utility VM target whited out in another case", link: "UtilityVM/Files/Windows/System32/x.dll", below: [][]string{{"/UtilityVM/Files/Windows/System32/.wh.X.DLL"}, {uvm}}, want: none},
		{name: "a utility VM target the link's own layer has", link: "UtilityVM/Files/Windows/System32/x.dll", own: []string{uvm}, below: [][]string{{uvm}}, want: none},
		{name: "a utility VM target the link's own layer whites out", link: "UtilityVM/Files/Windows/System32/x.dll", own: []string{"/UtilityVM/Files/Windows/System32/.wh.x.dll"}, below: [][]string{{uvm}}, want: none},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var below []func() foldedIndex
			var trees []*filetree.FileTree
			for _, entries := range tt.below {
				tree := treeOf(t, entries...)
				trees = append(trees, tree)
				below = append(below, indexOf(tree))
			}
			got := resolveWindowsLink(tt.link, indexOf(treeOf(t, tt.own...)), below)
			if tt.want == none {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Same(t, literalNode(trees[tt.want], got.RealPath), got, "resolved in layer %d", tt.want)
		})
	}
}

// the index of a layer is only built when the lookup reaches it, and the link's own layer only for a utility VM
// target
func TestResolveWindowsLink_FoldsOnlyTheLayersItReaches(t *testing.T) {
	reached := map[string]bool{}
	index := func(name string, entries ...string) func() foldedIndex {
		tree := treeOf(t, entries...)
		return func() foldedIndex {
			reached[name] = true
			return foldPaths(tree)
		}
	}
	got := resolveWindowsLink("Files/x.dll", index("own"), []func() foldedIndex{
		index("nearest", "/Files/x.dll"),
		index("further", "/Files/x.dll"),
	})
	require.NotNil(t, got)
	assert.Equal(t, map[string]bool{"nearest": true}, reached)
}

// a name past the cap is refused before any layer is folded, whatever the layers hold
func TestResolveWindowsLink_LongName(t *testing.T) {
	long := "Files/" + strings.Repeat("a/", maxWindowsLinkName/2) + "x"
	got := resolveWindowsLink(long, nil, []func() foldedIndex{func() foldedIndex {
		t.Fatal("folded a layer for a name past the cap")
		return nil
	}})
	assert.Nil(t, got)
}

func TestFoldPaths(t *testing.T) {
	idx := foldPaths(treeOf(t, "/Files/Windows/x.dll", "/Files/Windows/.wh.gone", "/Files/a", "/Files/A"))

	hit, refused := idx.find([]string{"FILES", "WINDOWS", "X.DLL"}, false)
	assert.False(t, refused)
	require.NotNil(t, hit)
	assert.Equal(t, file.Path("/Files/Windows/x.dll"), hit.RealPath)

	hit, refused = idx.find([]string{"FILES", "MISSING", "X.DLL"}, false)
	assert.False(t, refused, "a miss moves on to the next layer")
	assert.Nil(t, hit)

	_, refused = idx.find([]string{"FILES", "A"}, false)
	assert.True(t, refused, "two names that fold together")

	_, refused = idx.find([]string{"FILES", "WINDOWS", "GONE"}, true)
	assert.True(t, refused, "a whiteout, when whiteouts count")
	hit, refused = idx.find([]string{"FILES", "WINDOWS", "GONE"}, false)
	assert.False(t, refused, "a whiteout, when they don't")
	assert.Nil(t, hit)
}
