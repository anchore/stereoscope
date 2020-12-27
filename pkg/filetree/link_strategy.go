package filetree

type LinkStrategy struct {
	FollowAncestorLinks          bool
	FollowBasenameLinks          bool
	DoNotFollowDeadBasenameLinks bool
}

func (s LinkStrategy) FollowLinks() bool {
	return s.FollowAncestorLinks || s.FollowBasenameLinks
}

//const (
//	DoNotFollowLinks LinkStrategy = 0 << iota
//	FollowAncestorLinks
//	FollowBasenameLinks
//	// TODO: DoNotFollowDeadBasenameLinks
//)
//
//type LinkStrategy uint
//
//func (s LinkStrategy) has(otherFlags ...LinkStrategy) bool {
//	if s == DoNotFollowLinks {
//		return false
//	}
//	if len(otherFlags) == 0 {
//		return false
//	}
//	ret := true
//	for _, f := range otherFlags {
//		ret = ret && s&f != 0 && f&f-1 == 0
//	}
//	return ret
//}
