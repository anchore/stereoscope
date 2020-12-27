package filetree

//type UnionFileTree struct {
//	trees []*FileTree
//}
//
//func NewUnionFileTree() *UnionFileTree {
//	return &UnionFileTree{
//		trees: make([]*FileTree, 0),
//	}
//}
//
//func (u *UnionFileTree) PushTree(t *FileTree) {
//	u.trees = append(u.trees, t)
//}
//
//func (u *UnionFileTree) Squash() (*FileTree, error) {
//	switch len(u.trees) {
//	case 0:
//		return NewFileTree(), nil
//	case 1:
//		return u.trees[0].copy()
//	}
//
//	var squashedTree *FileTree
//	var err error
//	for layerIdx, refTree := range u.trees {
//		if layerIdx == 0 {
//			squashedTree, err = refTree.copy()
//			if err != nil {
//				return nil, err
//			}
//			continue
//		}
//
//		squashedTree.merge(refTree)
//	}
//	return squashedTree, nil
//}
