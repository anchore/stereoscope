package file

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestResolution_RequestResolutionPath(t *testing.T) {
	tests := []struct {
		name    string
		subject Resolution
		want    []Path
	}{
		{
			name: "empty",
			subject: Resolution{
				LinkResolutions: nil,
			},
			want: nil,
		},
		{
			name: "single ref",
			subject: Resolution{
				RequestPath: "/home/wagoodman/file.txt",
				Reference: &Reference{
					id:       1,
					RealPath: "/home/wagoodman/file.txt",
				},
				LinkResolutions: nil,
			},
			want: []Path{
				"/home/wagoodman/file.txt",
			},
		},
		{
			// /home -> /another/place
			name: "ref with 1 leaf link resolutions",
			subject: Resolution{
				RequestPath: "/home",
				Reference:   &Reference{RealPath: "/another/place"},
				LinkResolutions: []Resolution{
					{
						RequestPath: "/home",
						Reference:   &Reference{RealPath: "/home"},
					},
				},
			},
			want: []Path{
				"/home",
				"/another/place",
			},
		},
		{
			// /home/wagoodman/file.txt -> /place/wagoodman/file.txt -> /1/file.txt -> /2/real-file.txt

			// this is the current state of the filetree
			//	.
			//  ├── 1
			//  │   ├── file.txt -> 2/real-file.txt
			//  │   └── link-to-place -> place
			//  ├── 2
			//  │   └── real-file.txt
			//  ├── home -> link-to-1/link-to-place
			//  ├── link-to-1 -> 1
			//  └── place
			//      └── wagoodman
			//          └── file.txt -> link-to-1/file.txt

			name: "ref with 2 leaf link resolutions",
			subject: Resolution{
				RequestPath: "/home/wagoodman/file.txt",
				Reference:   &Reference{RealPath: "/2/real-file.txt"},
				LinkResolutions: []Resolution{
					{
						RequestPath: "/place/wagoodman/file.txt",
						Reference:   &Reference{RealPath: "/place/wagoodman/file.txt"},
					},
					{
						RequestPath: "/1/file.txt",
						Reference:   &Reference{RealPath: "/1/file.txt"},
					},
				},
			},
			want: []Path{
				"/home/wagoodman/file.txt",  // request
				"/place/wagoodman/file.txt", // real intermediate path
				"/1/file.txt",               // real intermediate path
				"/2/real-file.txt",          // final resolved path on the reference
			},
		},
		{
			// /home/wagoodman/file.txt -> /place/wagoodman/file.txt -> /1/file.txt -> /2/real-file.txt

			// this is the current state of the filetree
			//	.
			//  ├── 1
			//  │   ├── file.txt -> 2/real-file.txt
			//  │   └── link-to-place -> place
			//  ├── home -> link-to-1/link-to-place
			//  ├── link-to-1 -> 1
			//  └── place
			//      └── wagoodman
			//          └── file.txt -> link-to-1/file.txt

			name: "ref with dead link",
			subject: Resolution{
				RequestPath: "/home/wagoodman/file.txt",
				// note: this falls back to the last path that exists which is the behavior for link resolution options:
				//	 []LinkResolutionOption{FollowBasenameLinks, DoNotFollowDeadBasenameLinks}
				Reference: &Reference{RealPath: "/1/file.txt"},
				LinkResolutions: []Resolution{
					{
						RequestPath: "/place/wagoodman/file.txt",
						Reference:   &Reference{RealPath: "/place/wagoodman/file.txt"},
					},
					{
						RequestPath: "/1/file.txt",
						Reference:   &Reference{RealPath: "/1/file.txt"},
					},
					{
						RequestPath: "/2/real-file.txt",
						// nope! it's dead!
						//Reference:   &file.Reference{RealPath: "/2/real-file.txt"},
					},
				},
			},
			want: []Path{
				"/home/wagoodman/file.txt",  // request
				"/place/wagoodman/file.txt", // real intermediate path
				"/1/file.txt",               // real intermediate path
				"/2/real-file.txt",          // final resolved path on the reference (that does not exist)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.subject.RequestResolutionPath(), "RequestResolutionPath()")
		})
	}
}

func TestReferenceResolutionVia_ResolutionReferences(t *testing.T) {
	type fields struct {
		ReferenceResolution Resolution
		LeafLinkResolution  []Resolution
	}
	tests := []struct {
		name    string
		subject Resolution
		want    []Reference
	}{
		{
			name: "empty",
			subject: Resolution{
				LinkResolutions: nil,
			},
			want: nil,
		},
		{
			name: "single ref",
			subject: Resolution{
				RequestPath: "/home/wagoodman/file.txt",
				Reference: &Reference{
					id:       1,
					RealPath: "/home/wagoodman/file.txt",
				},
				LinkResolutions: nil,
			},
			want: []Reference{
				{
					id:       1,
					RealPath: "/home/wagoodman/file.txt",
				},
			},
		},
		{
			// /home -> /another/place
			name: "ref with 1 leaf link resolutions",
			subject: Resolution{
				RequestPath: "/home",
				Reference:   &Reference{RealPath: "/another/place"},
				LinkResolutions: []Resolution{
					{
						RequestPath: "/home",
						Reference:   &Reference{RealPath: "/home"},
					},
				},
			},
			want: []Reference{
				{RealPath: "/home"},
				{RealPath: "/another/place"},
			},
		},
		{
			// /home/wagoodman/file.txt -> /place/wagoodman/file.txt -> /1/file.txt -> /2/real-file.txt

			// this is the current state of the filetree
			//	.
			//  ├── 1
			//  │   ├── file.txt -> 2/real-file.txt
			//  │   └── link-to-place -> place
			//  ├── 2
			//  │   └── real-file.txt
			//  ├── home -> link-to-1/link-to-place
			//  ├── link-to-1 -> 1
			//  └── place
			//      └── wagoodman
			//          └── file.txt -> link-to-1/file.txt

			name: "ref with 2 leaf link resolutions",
			subject: Resolution{
				RequestPath: "/home/wagoodman/file.txt",
				Reference:   &Reference{RealPath: "/2/real-file.txt"},
				LinkResolutions: []Resolution{
					{
						RequestPath: "/place/wagoodman/file.txt",
						Reference:   &Reference{RealPath: "/place/wagoodman/file.txt"},
					},
					{
						RequestPath: "/1/file.txt",
						Reference:   &Reference{RealPath: "/1/file.txt"},
					},
				},
			},
			want: []Reference{
				{RealPath: "/place/wagoodman/file.txt"},
				{RealPath: "/1/file.txt"},
				{RealPath: "/2/real-file.txt"},
			},
		},
		{
			// /home/wagoodman/file.txt -> /place/wagoodman/file.txt -> /1/file.txt -> /2/real-file.txt

			// this is the current state of the filetree
			//	.
			//  ├── 1
			//  │   ├── file.txt -> 2/real-file.txt
			//  │   └── link-to-place -> place
			//  ├── home -> link-to-1/link-to-place
			//  ├── link-to-1 -> 1
			//  └── place
			//      └── wagoodman
			//          └── file.txt -> link-to-1/file.txt

			name: "ref with dead link",
			subject: Resolution{
				RequestPath: "/home/wagoodman/file.txt",
				// note: this falls back to the last path that exists which is the behavior for link resolution options:
				//	 []LinkResolutionOption{FollowBasenameLinks, DoNotFollowDeadBasenameLinks}
				Reference: &Reference{RealPath: "/1/file.txt"},
				LinkResolutions: []Resolution{
					{
						RequestPath: "/place/wagoodman/file.txt",
						Reference:   &Reference{RealPath: "/place/wagoodman/file.txt"},
					},
					{
						RequestPath: "/1/file.txt",
						Reference:   &Reference{RealPath: "/1/file.txt"},
					},
					{
						RequestPath: "/2/real-file.txt",
						// nope! it's dead!
						//Reference:   &file.Reference{RealPath: "/2/real-file.txt"},
					},
				},
			},
			want: []Reference{
				{RealPath: "/place/wagoodman/file.txt"},
				{RealPath: "/1/file.txt"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.subject.ResolutionReferences(), "ResolutionReferences()")

		})
	}
}
