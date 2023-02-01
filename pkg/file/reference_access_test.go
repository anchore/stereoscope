package file

import (
	"github.com/stretchr/testify/assert"
	"sort"
	"testing"
)

func TestReferenceAccessVias_Less(t *testing.T) {

	realA := ReferenceAccessVia{
		ReferenceAccess: ReferenceAccess{
			RequestPath: "/parent/a",
			Reference: &Reference{
				RealPath: "/parent/a",
			},
		},
	}

	realB := ReferenceAccessVia{
		ReferenceAccess: ReferenceAccess{
			RequestPath: "/parent/b",
			Reference: &Reference{
				RealPath: "/parent/b",
			},
		},
	}

	linkToA := ReferenceAccessVia{
		ReferenceAccess: ReferenceAccess{
			RequestPath: "/parent-link/a",
			Reference: &Reference{
				RealPath: "/a",
			},
		},
	}

	linkToB := ReferenceAccessVia{
		ReferenceAccess: ReferenceAccess{
			RequestPath: "/parent-link/b",
			Reference: &Reference{
				RealPath: "/b",
			},
		},
	}

	tests := []struct {
		name    string
		subject []ReferenceAccessVia
		want    []ReferenceAccessVia
	}{
		{
			name: "references to real files are preferred first",
			subject: []ReferenceAccessVia{
				linkToA,
				realA,
			},
			want: []ReferenceAccessVia{
				realA,
				linkToA,
			},
		},
		{
			name: "real files are treated equally by request name",
			subject: []ReferenceAccessVia{
				realB,
				realA,
			},
			want: []ReferenceAccessVia{
				realA,
				realB,
			},
		},
		{
			name: "link files are treated equally by request name",
			subject: []ReferenceAccessVia{
				linkToB,
				linkToA,
			},
			want: []ReferenceAccessVia{
				linkToA,
				linkToB,
			},
		},
		{
			name: "regression",
			subject: []ReferenceAccessVia{
				{
					ReferenceAccess: ReferenceAccess{
						RequestPath: "/parent-link/file-4.txt",
						Reference: &Reference{
							RealPath: "/parent/file-4.txt",
						},
					},
				},
				{
					ReferenceAccess: ReferenceAccess{
						RequestPath: "/parent/file-4.txt",
						Reference: &Reference{
							RealPath: "/parent/file-4.txt",
						},
					},
				},
			},
			want: []ReferenceAccessVia{
				{
					ReferenceAccess: ReferenceAccess{
						RequestPath: "/parent/file-4.txt",
						Reference: &Reference{
							RealPath: "/parent/file-4.txt",
						},
					},
				},
				{
					ReferenceAccess: ReferenceAccess{
						RequestPath: "/parent-link/file-4.txt",
						Reference: &Reference{
							RealPath: "/parent/file-4.txt",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort.Sort(ReferenceAccessVias(tt.subject))
			assert.Equal(t, tt.want, tt.subject)
		})
	}
}
