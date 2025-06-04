package filetree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parseGlob(t *testing.T) {

	tests := []struct {
		name string
		glob string
		want []searchRequest
	}{
		{
			name: "relative path",
			glob: "foo/bar/basename.txt",
			want: []searchRequest{
				{
					searchBasis: searchByFullPath,
					indexLookup: "/foo/bar/basename.txt",
					glob:        "/foo/bar/basename.txt",
				},
			},
		},
		{
			name: "absolute path",
			glob: "/foo/bar/basename.txt",
			want: []searchRequest{
				{
					searchBasis: searchByFullPath,
					indexLookup: "/foo/bar/basename.txt",
					glob:        "/foo/bar/basename.txt",
				},
			},
		},
		{
			name: "extension",
			glob: "*.txt",
			want: []searchRequest{
				{
					searchBasis: searchByExtension,
					indexLookup: ".txt",
					glob:        "/*.txt",
				},
			},
		},
		{
			name: "extension with slash",
			glob: "/*.txt",
			want: []searchRequest{
				{
					searchBasis: searchByExtension,
					indexLookup: ".txt",
					glob:        "/*.txt",
				},
			},
		},
		{
			name: "extension anywhere",
			glob: "**/*.txt",
			want: []searchRequest{
				{
					searchBasis: searchByExtension,
					indexLookup: ".txt",
					glob:        "**/*.txt",
				},
			},
		},
		{
			name: "basename glob search with requirement",
			glob: "bas*nam?.txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "bas*nam?.txt",
					glob:        "/bas*nam?.txt",
				},
			},
		},
		{
			name: "extension with path requirement",
			glob: "foo/bar/**/*.txt",
			want: []searchRequest{
				{
					searchBasis: searchByExtension,
					indexLookup: ".txt",
					glob:        "/foo/bar/**/*.txt",
				},
			},
		},
		{
			name: "basename but without a path prefix",
			glob: "basename.txt",
			want: []searchRequest{
				{
					searchBasis: searchByFullPath,
					indexLookup: "/basename.txt",
					glob:        "/basename.txt",
				},
			},
		},
		{
			name: "basename anywhere",
			glob: "**/basename.txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasename,
					indexLookup: "basename.txt",
					glob:        "**/basename.txt",
				},
			},
		},
		{
			name: "basename with requirement",
			glob: "foo/b*/basename.txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasename,
					indexLookup: "basename.txt",
					glob:        "/foo/b*/basename.txt",
				},
			},
		},
		{
			name: "basename glob",
			glob: "basename.*",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "basename.*",
					glob:        "/basename.*",
				},
			},
		},
		{
			name: "basename glob with requirement",
			glob: "**/foo/bar/basename.*",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "basename.*",
					glob:        "**/foo/bar/basename.*",
				},
			},
		},
		{
			name: "basename wildcard glob with requirement",
			glob: "**/foo/bar/basenam?.txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "basenam?.txt",
					glob:        "**/foo/bar/basenam?.txt",
				},
			},
		},
		{
			name: "glob classes within a basename",
			glob: "**/foo/bar/basena[me][me].txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "basena[me][me].txt",
					glob:        "**/foo/bar/basena[me][me].txt",
				},
			},
		},
		{
			name: "glob classes within a the path",
			glob: "**/foo/[bB]ar/basena[me][me].txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "basena[me][me].txt",
					glob:        "**/foo/[bB]ar/basena[me][me].txt",
				},
			},
		},
		{
			name: "alt clobbers basename extraction",
			glob: "**/foo/bar/{nested/basena[me][me].txt,another.txt}",
			want: []searchRequest{
				{
					searchBasis: searchByGlob,
					glob:        "**/foo/bar/{nested/basena[me][me].txt,another.txt}",
				},
			},
		},
		{
			name: "class clobbers basename extraction",
			glob: "**/foo/bar/[me][m/e].txt,another.txt",
			want: []searchRequest{
				{
					searchBasis: searchByGlob,
					glob:        "**/foo/bar/[me][m/e].txt,another.txt",
				},
			},
		},
		{
			name: "match alternative matches in the basename",
			glob: "**/var/lib/rpm/{Packages,Packages.db,rpmdb.sqlite}",
			want: []searchRequest{
				{
					searchBasis: searchByBasename,
					indexLookup: "Packages",
					glob:        "**/var/lib/rpm/{Packages,Packages.db,rpmdb.sqlite}",
				},
				{
					searchBasis: searchByBasename,
					indexLookup: "Packages.db",
					glob:        "**/var/lib/rpm/{Packages,Packages.db,rpmdb.sqlite}",
				},
				{
					searchBasis: searchByBasename,
					indexLookup: "rpmdb.sqlite",
					glob:        "**/var/lib/rpm/{Packages,Packages.db,rpmdb.sqlite}",
				},
			},
		},
		{
			name: "match fallback to glob search on non-simple alternatives",
			glob: "**/var/lib/rpm/{Packa{ges}{GES},Packages.db,rpmdb.sqlite}",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "{Packa{ges}{GES},Packages.db,rpmdb.sqlite}",
					glob:        "**/var/lib/rpm/{Packa{ges}{GES},Packages.db,rpmdb.sqlite}",
				},
			},
		},
		{
			name: "dynamic extraction of basename and basename glob for alternatives",
			glob: "**/var/lib/rpm/{Pack???s,Packages.db,rpm*.sqlite}",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "Pack???s",
					glob:        "**/var/lib/rpm/{Pack???s,Packages.db,rpm*.sqlite}",
				},
				{
					searchBasis: searchByBasename,
					indexLookup: "Packages.db",
					glob:        "**/var/lib/rpm/{Pack???s,Packages.db,rpm*.sqlite}",
				},
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "rpm*.sqlite",
					glob:        "**/var/lib/rpm/{Pack???s,Packages.db,rpm*.sqlite}",
				},
			},
		},
		{
			name: "fallback to full glob search",
			glob: "**/foo/bar/**?/**",
			want: []searchRequest{
				{
					searchBasis: searchByGlob,
					glob:        "**/foo/bar/*?/**",
				},
			},
		},
		{
			name: "use parent basename for directory contents",
			glob: "**/foo/bar/*",
			want: []searchRequest{
				{
					searchBasis: searchBySubDirectory,
					indexLookup: "bar",
					glob:        "**/foo/bar/*",
				},
			},
		},
		// special cases
		{
			name: "empty string",
			glob: "",
			want: []searchRequest{
				{
					searchBasis: searchByFullPath,
					indexLookup: "/",
					glob:        "/",
				},
			},
		},
		{
			name: "only a slash",
			glob: "/",
			want: []searchRequest{
				{
					searchBasis: searchByFullPath,
					indexLookup: "/",
					glob:        "/",
				},
			},
		},
		{
			name: "cleanup to single slash",
			glob: "///",
			want: []searchRequest{
				{
					searchBasis: searchByFullPath,
					indexLookup: "/",
					glob:        "/",
				},
			},
		},
		{
			name: "ends with slash",
			glob: "/foo/b*r/",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "b*r",
					glob:        "/foo/b*r", // note that the slash is removed since this should be a clean path
				},
			},
		},
		{
			name: "ends with *",
			glob: "**/foo/b*r/*",
			want: []searchRequest{
				{
					searchBasis: searchByGlob,
					glob:        "**/foo/b*r/*",
				},
			},
		},
		{
			name: "ends with ***",
			glob: "**/foo/b*r/**",
			want: []searchRequest{
				{
					searchBasis: searchByGlob,
					glob:        "**/foo/b*r/**",
				},
			},
		},
		{
			name: "spaces around everything",
			glob: " /foo/b*r/ .txt ",
			want: []searchRequest{
				{
					searchBasis: searchByBasename,
					indexLookup: " .txt",          // note the space
					glob:        "/foo/b*r/ .txt", // note the space in the middle, but otherwise clean on the front and back
				},
			},
		},
		{
			name: "fallback to full glob search",
			glob: "**/foo/bar/***.*****.******",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "*.*.*",            // note that the basename glob is cleaned up
					glob:        "**/foo/bar/*.*.*", // note that the glob is cleaned up
				},
			},
		},
		{
			name: "odd glob input still honors basename searches",
			glob: "**/foo/**.***.****bar/***thin*.txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "*thin*.txt",                 // note that the basename glob is cleaned up
					glob:        "**/foo/*.*.*bar/*thin*.txt", // note that the glob is cleaned up
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, parseGlob(tt.glob), "parseGlob(%v)", tt.glob)
		})
	}
}

func Test_parseGlobBasename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []searchRequest
	}{
		{
			name:  "empty string",
			input: "",
			want: []searchRequest{
				{
					searchBasis: searchByBasename,
				},
			},
		},
		{
			name:  "everything-ish",
			input: "*?",
			want: []searchRequest{
				{
					searchBasis: searchByGlob,
				},
			},
		},
		{
			name:  "everything recursive",
			input: "**",
			want: []searchRequest{
				{
					searchBasis: searchByGlob,
				},
			},
		},
		{
			name:  "simple basename",
			input: "basename.txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasename,
					indexLookup: "basename.txt",
				},
			},
		},
		{
			name:  "basename with prefix glob",
			input: "*basename.txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "*basename.txt",
				},
			},
		},
		{
			name:  "basename with pattern",
			input: "bas*nam?.txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "bas*nam?.txt",
				},
			},
		},
		{
			name:  "extension",
			input: "*.txt",
			want: []searchRequest{
				{
					searchBasis: searchByExtension,
					indexLookup: ".txt",
				},
			},
		},
		{
			name:  "possible extension that should be searched by glob",
			input: "*.*.txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "*.*.txt",
				},
			},
		},
		{
			name:  "tricky basename",
			input: ".txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasename,
					indexLookup: ".txt",
				},
			},
		},
		{
			name:  "basename glob with extension",
			input: "*thin*.txt",
			want: []searchRequest{
				{
					searchBasis: searchByBasenameGlob,
					indexLookup: "*thin*.txt",
				},
			},
		},
		{
			name:  "basename alternates",
			input: "{Packages,Packages.db,rpmdb.sqlite}",
			want: []searchRequest{
				{
					searchBasis: searchByBasename,
					indexLookup: "Packages",
				},
				{
					searchBasis: searchByBasename,
					indexLookup: "Packages.db",
				},
				{
					searchBasis: searchByBasename,
					indexLookup: "rpmdb.sqlite",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, parseGlobBasename(tt.input, ""), "parseGlobBasename(%v)", tt.input)
		})
	}
}

func Test_cleanGlob(t *testing.T) {
	tests := []struct {
		name string
		glob string
		want string
	}{
		{
			name: "empty string",
			glob: "",
			want: "/",
		},
		{
			name: "remove spaces from glob edges",
			glob: " **/foo/ **/ bar.txt  ",
			want: "**/foo/ */ bar.txt",
		},
		{
			name: "simplify slashes",
			glob: "///foo/////**///**////",
			want: "/foo/**",
		},
		{
			name: "simplify larger recursive glob",
			glob: "**/foo/**/*/***/*bar**/***.*****.******",
			want: "**/foo/**/*/**/*bar*/*.*.*",
		},
		{
			name: "simplify glob prefix",
			glob: "***/foo.txt",
			want: "**/foo.txt",
		},
		{
			name: "simplify glob within multiple path",
			glob: "bar**/ba**r*/***/**/bar***/**/foo.txt",
			want: "/bar*/ba*r*/**/bar*/**/foo.txt",
		},
		{
			name: "simplify prefix and suffix glob",
			glob: "***/foo/**/****",
			want: "**/foo/**",
		},
		{
			name: "simplify multiple recursive requests",
			glob: "/**/**/foo/**/**",
			want: "**/foo/**",
		},
		{
			name: "simplify slashes and asterisks",
			glob: "/***/****///foo/**//****////",
			want: "**/foo/**",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, cleanGlob(tt.glob), "cleanGlob(%v)", tt.glob)
		})
	}
}

func Test_removeRedundantCountGlob(t *testing.T) {
	type args struct {
		glob  string
		val   rune
		count int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "empty string",
			args: args{
				glob:  "",
				val:   '*',
				count: 1,
			},
			want: "",
		},
		{
			name: "simplify on edges and body",
			args: args{
				glob:  "**/foo/***/****",
				val:   '*',
				count: 2,
			},
			want: "**/foo/**/**",
		},
		{
			name: "simplify slashes",
			args: args{
				glob:  "///something/**///here?/*/will//work///",
				val:   '/',
				count: 1,
			},
			want: "/something/**/here?/*/will/work/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, removeRedundantCountGlob(tt.args.glob, tt.args.val, tt.args.count), "removeRedundantCountGlob(%v, %v, %v)", tt.args.glob, tt.args.val, tt.args.count)
		})
	}
}

func Test_simplifyMultipleGlobAsterisks(t *testing.T) {
	tests := []struct {
		name string
		glob string
		want string
	}{
		{
			name: "simplify glob suffix",
			glob: "foo/.***",
			want: "foo/.*",
		},
		{
			name: "simplify glob within path",
			glob: "**/bar**/foo.txt",
			want: "**/bar*/foo.txt",
		},
		{
			name: "simplify glob within multiple path",
			glob: "bar**/ba**r*/**/**/bar**/**/foo.txt",
			want: "bar*/ba*r*/**/**/bar*/**/foo.txt",
		},
		{
			name: "simplify glob within path prefix",
			glob: "bar**/foo.txt",
			want: "bar*/foo.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, simplifyMultipleGlobAsterisks(tt.glob), "simplifyMultipleGlobAsterisks(%v)", tt.glob)
		})
	}
}

func Test_simplifyGlobRecursion(t *testing.T) {
	tests := []struct {
		name string
		glob string
		want string
	}{
		{
			name: "single instance with slash prefix",
			glob: "/**",
			want: "**",
		},
		{
			name: "single instance with slash suffix",
			glob: "**/",
			want: "**",
		},
		{
			name: "no slash prefix",
			glob: "**/**/fo*o/**/**",
			want: "**/fo*o/**",
		},
		{
			name: "within body",
			glob: "/fo*o/**/**/bar",
			want: "/fo*o/**/bar",
		},
		{
			name: "with slash prefix",
			glob: "/**/**/foo/**/**",
			want: "**/foo/**",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, simplifyGlobRecursion(tt.glob), "simplifyGlobRecursion(%v)", tt.glob)
		})
	}
}
