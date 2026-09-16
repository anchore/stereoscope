package filetree

import "fmt"

type UnionFileTree struct {
	trees []ReadWriter
}

func NewUnionFileTree() *UnionFileTree {
	return &UnionFileTree{
		trees: make([]ReadWriter, 0),
	}
}

func (u *UnionFileTree) PushTree(t ReadWriter) {
	u.trees = append(u.trees, t)
}

func (u *UnionFileTree) Squash() (ReadWriter, error) {
	if len(u.trees) == 0 {
		return New(), nil
	}

	var squashedTree ReadWriter
	var err error
	for layerIdx, refTree := range u.trees {
		if layerIdx == 0 {
			squashedTree, err = refTree.Copy()
			if err != nil {
				return nil, err
			}
			// every tree above this one is merged, which applies and drops its whiteout entries, but the
			// lowest tree is only copied. Whiteouts are changeset metadata and never materialize as files
			// in an applied rootfs, so strip them here.
			if err = removeWhiteouts(squashedTree); err != nil {
				return nil, err
			}
			continue
		}

		if err = squashedTree.Merge(refTree); err != nil {
			return nil, fmt.Errorf("unable to squash layer=%d : %w", layerIdx, err)
		}
	}
	return squashedTree, nil
}

// removeWhiteouts drops any whiteout marker from the given tree. Note that markers cannot be dropped when a
// layer tree is built, since Merge reads them off the upper tree to know what to remove.
func removeWhiteouts(t ReadWriter) error {
	for _, p := range t.AllRealPaths() {
		if !p.IsWhiteout() {
			continue
		}
		if err := t.RemovePath(p); err != nil {
			return fmt.Errorf("unable to remove whiteout marker (path=%s): %w", p, err)
		}
	}
	return nil
}
