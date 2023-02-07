package file

import (
	"github.com/stretchr/testify/assert"
	"sort"
	"testing"
)

func TestReferenceResolutionVias_Less(t *testing.T) {

	realA := Resolution{

		RequestPath: "/parent/a",
		Reference: &Reference{
			RealPath: "/parent/a",
		},
	}

	realB := Resolution{

		RequestPath: "/parent/b",
		Reference: &Reference{
			RealPath: "/parent/b",
		},
	}

	linkToA := Resolution{

		RequestPath: "/parent-link/a",
		Reference: &Reference{
			RealPath: "/a",
		},
	}

	linkToB := Resolution{
		RequestPath: "/parent-link/b",
		Reference: &Reference{
			RealPath: "/b",
		},
	}

	tests := []struct {
		name    string
		subject []Resolution
		want    []Resolution
	}{
		{
			name: "references to real files are preferred first",
			subject: []Resolution{
				linkToA,
				realA,
			},
			want: []Resolution{
				realA,
				linkToA,
			},
		},
		{
			name: "real files are treated equally by request name",
			subject: []Resolution{
				realB,
				realA,
			},
			want: []Resolution{
				realA,
				realB,
			},
		},
		{
			name: "link files are treated equally by request name",
			subject: []Resolution{
				linkToB,
				linkToA,
			},
			want: []Resolution{
				linkToA,
				linkToB,
			},
		},
		{
			name: "regression",
			subject: []Resolution{
				{

					RequestPath: "/parent-link/file-4.txt",
					Reference: &Reference{
						RealPath: "/parent/file-4.txt",
					},
				},
				{

					RequestPath: "/parent/file-4.txt",
					Reference: &Reference{
						RealPath: "/parent/file-4.txt",
					},
				},
			},
			want: []Resolution{
				{
					RequestPath: "/parent/file-4.txt",
					Reference: &Reference{
						RealPath: "/parent/file-4.txt",
					},
				},
				{

					RequestPath: "/parent-link/file-4.txt",
					Reference: &Reference{
						RealPath: "/parent/file-4.txt",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.Sort(Resolutions(tt.subject))
			assert.Equal(t, tt.want, tt.subject)
		})
	}
}
