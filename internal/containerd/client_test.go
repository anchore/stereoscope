package containerd

import (
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func Test_getContainerHostAddress(t *testing.T) {
	type args struct {
		containerHostEnvVar string
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
			name: "env vars trump default socket values",
			args: args{
				containerHostEnvVar: "unix:///somewhere/containerd.sock",
				xdgRuntimeDir:       "/xdg-runtime",
				defaultSocketPath:   "/default/containerd.sock",
			},
			want:    "unix:///somewhere/containerd.sock",
			wantErr: assert.NoError,
		},

		{
			name: "attempt candidate socket from xdg runtime dir",
			args: args{
				containerHostEnvVar: "",
				xdgRuntimeDir:       "/xdg-runtime",
				defaultSocketPath:   "/default/containerd.sock",
			},
			want:    "unix:///proc/42/root/run/containerd/containerd.sock",
			wantErr: assert.NoError,
		},
		{
			name: "use default socket candidate last",
			args: args{
				containerHostEnvVar: "",
				xdgRuntimeDir:       "does-not-exist",
				defaultSocketPath:   "/default/containerd.sock",
			},
			want:    "unix:///default/containerd.sock",
			wantErr: assert.NoError,
		},
		{
			name: "use default socket candidate last when child_pid file is empty",
			args: args{
				containerHostEnvVar: "",
				xdgRuntimeDir:       "/xdg-runtime-empty",
				defaultSocketPath:   "/default/containerd.sock",
			},
			want:    "unix:///default/containerd.sock",
			wantErr: assert.NoError,
		},
		{
			name: "use default socket candidate last when child_pid is stale",
			args: args{
				containerHostEnvVar: "",
				xdgRuntimeDir:       "/xdg-runtime-stale",
				defaultSocketPath:   "/default/containerd.sock",
			},
			want:    "unix:///default/containerd.sock",
			wantErr: assert.NoError,
		},
		{
			name: "error when there are no candidates",
			args: args{
				containerHostEnvVar: "",
				xdgRuntimeDir:       "does-not-exist",
				defaultSocketPath:   "does-not-exist",
			},
			wantErr: assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CONTAINERD_ADDRESS", tt.args.containerHostEnvVar)
			fs := afero.NewBasePathFs(afero.NewOsFs(), "test-fixtures")
			got, err := getAddress(fs, tt.args.xdgRuntimeDir, tt.args.defaultSocketPath)
			if !tt.wantErr(t, err, fmt.Sprintf("getAddress(%v)", tt.args.xdgRuntimeDir)) {
				return
			}
			assert.Equalf(t, tt.want, got, "getAddress(%v)", tt.args.xdgRuntimeDir)
		})
	}
}
