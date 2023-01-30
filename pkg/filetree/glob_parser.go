package filetree

import (
	"regexp"
	"strings"
)

const (
	searchByGlob searchBasis = iota
	searchByPath
	searchByExtension
	searchByBasename
	searchByBasenameGlob
)

type searchBasis int

func (s searchBasis) String() string {
	switch s {
	case searchByGlob:
		return "glob"
	case searchByPath:
		return "path"
	case searchByExtension:
		return "extension"
	case searchByBasename:
		return "basename"
	case searchByBasenameGlob:
		return "basename-glob"
	}
	return "unknown search basis"
}

type searchRequest struct {
	searchBasis
	value       string
	requirement string
}

func parseGlob(glob string) searchRequest {
	glob = cleanGlob(glob)

	if !strings.ContainsAny(glob, "*?") {
		return searchRequest{
			searchBasis: searchByPath,
			value:       glob,
		}
	}

	basenameSplitAt := strings.LastIndex(glob, "/")

	var basename string
	if basenameSplitAt == -1 {
		// note: this has no glob path prefix, thus no requirement...
		// this can only be a basename, basename glob, or extension
		basename = glob
	} else if basenameSplitAt < len(glob)-1 {
		basename = glob[basenameSplitAt+1:]
	}

	request := parseGlobBasename(basename)

	requirement := glob
	if basenameSplitAt == -1 {
		requirement = ""
	} else if basenameSplitAt < len(glob)-1 {
		requirementSection := glob[:basenameSplitAt]
		switch requirementSection {
		case "**", request.requirement:
			requirement = ""
		}
	}

	request.requirement = requirement

	if request.searchBasis == searchByGlob {
		request.value = glob
		if glob == request.requirement {
			request.requirement = ""
		}
	}

	return request
}

func parseGlobBasename(input string) searchRequest {
	extensionFields := strings.Split(input, "*.")
	if len(extensionFields) == 2 && extensionFields[0] == "" {
		possibleExtension := extensionFields[1]
		if !strings.ContainsAny(possibleExtension, "*?") {
			// special case, this is plain extension
			return searchRequest{
				searchBasis: searchByExtension,
				value:       "." + possibleExtension,
			}
		}
	}

	if !strings.ContainsAny(input, "*?") {
		// special case, this is plain extension
		return searchRequest{
			searchBasis: searchByBasename,
			value:       input,
		}
	}

	if strings.ReplaceAll(strings.ReplaceAll(input, "?", ""), "*", "") == "" {
		// special case, this is a glob that is only asterisks... do not process!
		return searchRequest{
			searchBasis: searchByGlob,
		}
	}

	return searchRequest{
		searchBasis: searchByBasenameGlob,
		value:       input,
	}
}

func cleanGlob(glob string) string {
	glob = strings.TrimSpace(glob)
	glob = removeRedundantCountGlob(glob, '/', 1)
	glob = removeRedundantCountGlob(glob, '*', 2)
	if len(glob) > 1 {
		// input case: /
		// then preserve the slash
		glob = strings.TrimRight(glob, "/")
	}
	// e.g. replace "/bar**/" with "/bar*/"
	glob = simplifyMultipleGlobAsterisks(glob)
	glob = simplifyGlobRecursion(glob)
	return glob
}

func simplifyMultipleGlobAsterisks(glob string) string {
	// this will replace any recursive globs (**) that are not clearly indicating recursive tree searches with a single *

	var sb strings.Builder
	var asteriskBuff strings.Builder
	var withinRecursiveStreak bool

	for idx, c := range glob {
		isAsterisk := c == '*'
		isSlash := c == '/'

		// special case, this is the first character in the glob and it is an asterisk...
		// treat this like a recursive streak
		if idx == 0 && isAsterisk {
			withinRecursiveStreak = true
			asteriskBuff.WriteRune(c)
			continue
		}

		if isAsterisk {
			asteriskBuff.WriteRune(c)
			continue
		}

		if isSlash {
			if withinRecursiveStreak {
				// this is a confirmed recursive streak
				// keep all asterisks!
				sb.WriteString(asteriskBuff.String())
				asteriskBuff.Reset()
			}

			if asteriskBuff.Len() > 0 {
				// this is NOT a recursive streak, but there are asterisks
				// keep only one asterisk
				sb.WriteRune('*')
				asteriskBuff.Reset()
			}

			// this is potentially a new streak...
			withinRecursiveStreak = true
		} else {
			// ... and this is NOT a recursive streak
			if asteriskBuff.Len() > 0 {
				// ... keep only one asterisk, since it's not recursive
				sb.WriteRune('*')
			}
			asteriskBuff.Reset()
			withinRecursiveStreak = false
		}

		sb.WriteRune(c)
	}

	if asteriskBuff.Len() > 0 {
		if withinRecursiveStreak {
			sb.WriteString(asteriskBuff.String())
		} else {
			sb.WriteRune('*')
		}
	}

	return sb.String()
}

var globRecursionRightPattern = regexp.MustCompile(`(\*\*/?)+`)

func simplifyGlobRecursion(glob string) string {
	// this function assumes that all redundant asterisks have been removed (e.g. /****/ -> /**/)
	// and that all seemingly recursive globs have been replaced with a single asterisk (e.g. /bar**/ -> /bar*/)
	glob = globRecursionRightPattern.ReplaceAllString(glob, "**/")
	glob = strings.ReplaceAll(glob, "//", "/")
	if strings.HasPrefix(glob, "/**/") {
		glob = strings.TrimPrefix(glob, "/")
	}
	if len(glob) > 1 {
		// input case: /**
		// then preserve the slash
		glob = strings.TrimRight(glob, "/")
	}
	return glob
}

func removeRedundantCountGlob(glob string, val rune, count int) string {
	var sb strings.Builder

	var streak int
	for _, c := range glob {
		if c == val {
			streak++
			if streak > count {
				continue
			}
		} else {
			streak = 0
		}

		sb.WriteRune(c)
	}
	return sb.String()
}
