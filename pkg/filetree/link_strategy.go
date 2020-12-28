package filetree

const (
	// TODO: add plenty of comments about expected behavior
	followAncestorLinks LinkResolutionOption = iota
	FollowBasenameLinks
	DoNotFollowDeadBasenameLinks
)

type LinkResolutionOption int

type linkResolutionStrategy struct {
	FollowAncestorLinks          bool
	FollowBasenameLinks          bool
	DoNotFollowDeadBasenameLinks bool
}

func newLinkResolutionStrategy(options ...LinkResolutionOption) linkResolutionStrategy {
	s := linkResolutionStrategy{}
	for _, o := range options {
		switch o {
		case FollowBasenameLinks:
			s.FollowBasenameLinks = true
		case DoNotFollowDeadBasenameLinks:
			s.DoNotFollowDeadBasenameLinks = true
		case followAncestorLinks:
			s.FollowAncestorLinks = true
		}
	}
	return s
}

func (s linkResolutionStrategy) FollowLinks() bool {
	return s.FollowAncestorLinks || s.FollowBasenameLinks
}
