package tree

import (
	"github.com/anchore/stereoscope/pkg/file"
	"path"
)

type UnionFileTree struct {
	trees []*FileTree
}

func NewUnionTree() *UnionFileTree {
	return &UnionFileTree{
		trees: make([]*FileTree, 0),
	}
}

func (u *UnionFileTree) PushTree(t *FileTree) {
	u.trees = append(u.trees, t)
}

func (u *UnionFileTree) Squash() (*FileTree, error) {
	switch len(u.trees) {
	case 0:
		return NewFileTree(), nil
	case 1:
		return u.trees[0].Copy()
	}

	var squashedTree *FileTree
	var err error
	for layerIdx, refTree := range u.trees {
		if layerIdx == 0 {
			squashedTree, err = refTree.Copy()
			if err != nil {
				return nil, err
			}
			continue
		}

		conditions := WalkConditions{
			ShouldContinueBranch: refTree.ConditionFn(func(f file.Reference) bool {
				return !f.Path.IsWhiteout()
			}),
			ShouldVisit: refTree.ConditionFn(func(f file.Reference) bool {
				return !f.Path.IsDirWhiteout()
			}),
		}

		visitor := refTree.VisitorFn(func(f file.Reference) {

			// opaque whiteouts must be processed first
			opaqueWhiteoutChild := file.Path(path.Join(string(f.Path), file.OpaqueWhiteout))
			if refTree.HasPath(opaqueWhiteoutChild) {
				err := squashedTree.RemoveChildPaths(f.Path)
				if err != nil {
					// TODO: replace
					panic(err)
				}

				return
			}

			if f.Path.IsWhiteout() {
				lowerPath, err := f.Path.UnWhiteoutPath()
				if err != nil {
					// TODO: replace
					panic(err)
				}

				err = squashedTree.RemovePath(lowerPath)
				if err != nil {
					// TODO: replace
					panic(err)
				}
			} else {
				if !squashedTree.HasPath(f.Path) {
					_, err := squashedTree.AddPath(f.Path)
					if err != nil {
						// TODO: replace
						panic(err)
					}
				}
				err := squashedTree.SetFile(f)
				if err != nil {
					// TODO: replace
					panic(err)
				}
			}
		})

		w := NewDepthFirstWalkerWithConditions(refTree.Reader(), visitor, conditions)
		w.WalkAll()

	}
	return squashedTree, nil
}
