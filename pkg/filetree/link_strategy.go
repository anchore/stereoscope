package filetree

type LinkStrategy struct {
	FollowAncestorLinks          bool
	FollowBasenameLinks          bool
	DoNotFollowDeadBasenameLinks bool
}

func (s LinkStrategy) FollowLinks() bool {
	return s.FollowAncestorLinks || s.FollowBasenameLinks
}
