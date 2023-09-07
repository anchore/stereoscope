package podman

import (
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func Test_getContainerHostAddress(t *testing.T) {
	type args struct {
		containerHostEnvVar string
		configPaths         []string
		xdgRuntimeDir       string
		defaultSocketPath   string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "env vars > config",
			args: args{
				containerHostEnvVar: "unix:///somewhere/podman.sock",
				configPaths: []string{
					"containers.conf",
				},
				xdgRuntimeDir:     "/xdg-runtime",
				defaultSocketPath: "/default/podman.sock",
			},
			want:    "unix:///somewhere/podman.sock",
			wantErr: assert.NoError,
		},
		{
			name: "config > candidates",
			args: args{
				containerHostEnvVar: "",
				configPaths: []string{
					"containers-relative.conf",
				},
				xdgRuntimeDir:     "/xdg-runtime",
				defaultSocketPath: "/default/podman.sock",
			},
			want:    "unix:///user/podman.sock",
			wantErr: assert.NoError,
		},
		{
			name: "attempt candidate socket from xdg runtime dir",
			args: args{
				containerHostEnvVar: "",
				configPaths:         []string{},
				xdgRuntimeDir:       "/xdg-runtime",
				defaultSocketPath:   "/default/podman.sock",
			},
			want:    "unix:///xdg-runtime/podman/podman.sock",
			wantErr: assert.NoError,
		},
		{
			name: "use default socket candidate last",
			args: args{
				containerHostEnvVar: "",
				configPaths:         []string{},
				xdgRuntimeDir:       "does-not-exist",
				defaultSocketPath:   "/default/podman.sock",
			},
			want:    "unix:///default/podman.sock",
			wantErr: assert.NoError,
		},
		{
			name: "error when there are no candidates",
			args: args{
				containerHostEnvVar: "",
				configPaths:         []string{},
				xdgRuntimeDir:       "does-not-exist",
				defaultSocketPath:   "does-not-exist",
			},
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CONTAINER_HOST", tt.args.containerHostEnvVar)
			fs := afero.NewBasePathFs(afero.NewOsFs(), "test-fixtures")
			got, err := getContainerHostAddress(fs, tt.args.configPaths, tt.args.xdgRuntimeDir, tt.args.defaultSocketPath)
			if !tt.wantErr(t, err, fmt.Sprintf("getContainerHostAddress(%v, %v)", tt.args.configPaths, tt.args.xdgRuntimeDir)) {
				return
			}
			assert.Equalf(t, tt.want, got, "getContainerHostAddress(%v, %v)", tt.args.configPaths, tt.args.xdgRuntimeDir)
		})
	}
}
