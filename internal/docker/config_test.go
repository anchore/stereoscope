package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
)

func Test_loadConfig(t *testing.T) {
	root := "test-fixtures"

	prepareConfig := func(t *testing.T, context, fname string) *configfile.ConfigFile {
		t.Helper()
		cfg := configfile.New(fname)
		cfg.CurrentContext = context

		return cfg
	}

	tests := []struct {
		name     string
		filename string
		want     *configfile.ConfigFile
		err      error
		wantErr  bool
	}{
		{
			name:     "config file not found",
			filename: "some-nonexisting-file",
			wantErr:  true,
		},
		{
			name:     "config file parsed normally",
			filename: filepath.Join(root, "config0.json"),
			wantErr:  false,
			want:     prepareConfig(t, "colima", filepath.Join(root, "config0.json")),
		},
		{
			name:     "config file cannot be parsed",
			filename: filepath.Join(root, "config1.json"),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadConfig(tt.filename)
			if tt.wantErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_resolveContextName(t *testing.T) {
	type args struct {
		contextOverride string
		config          *configfile.ConfigFile
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "respect contextOverride",
			args: args{
				contextOverride: "contextFromEnvironment",
			},
			want: "contextFromEnvironment",
		},
		{
			name: "returns default context name if config is nil",
			args: args{},
			want: "default",
		},
		{
			name: "returns context from the config",
			args: args{
				config: &configfile.ConfigFile{
					CurrentContext: "colima",
				},
			},
			want: "colima",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveContextName(tt.args.contextOverride, tt.args.config)

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_endpointFromContext(t *testing.T) {
	root := "test-fixtures"

	tmpCtxDir := func(t *testing.T) string {
		t.Helper()
		d, err := os.MkdirTemp("", "tests")
		if err != nil {
			t.Fatalf("cant setup test: %v", err)
		}

		ctxDir := filepath.Join(d, "contexts")
		err = os.MkdirAll(ctxDir, 0o755)
		if err != nil {
			t.Fatalf("cant setup test: %v", err)
		}

		return ctxDir
	}

	readFixture := func(t *testing.T, name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("cant setup test: %v", err)
		}

		return data
	}

	// meta files stored under ~/.docker with names like
	// ~/.docker/contexts/meta/f24fd3749c1368328e2b149bec149cb6795619f244c5b584e844961215dadd16/meta.json
	writeTestMeta := func(t *testing.T, fixture, dir, contextName string) {
		t.Helper()
		data := readFixture(t, fixture)
		dd := digest.FromString(contextName)

		base := filepath.Join(dir, "meta", dd.Encoded())

		err := os.MkdirAll(base, 0o755)
		if err != nil {
			t.Fatalf("cant setup test: %v", err)
		}

		outFname := filepath.Join(base, "meta.json")

		err = os.WriteFile(outFname, data, 0o600)
		if err != nil {
			t.Fatalf("cant setup test: %v", err)
		}

		t.Logf("fixture %s written to: %s", fixture, outFname)
	}

	tests := []struct {
		name    string
		ctxName string
		want    string
		fixture string
		wantErr bool
	}{
		{
			name:    "reads docker host from the meta data",
			want:    "unix:///some_weird_location/.colima/docker.sock",
			ctxName: "colima",
			fixture: "meta0.json",
		},
		{
			name:    "cant read docker host from the meta data",
			want:    "",
			ctxName: "colima",
			fixture: "",
			wantErr: true,
		},
		{
			name:    "invalid endpoint name",
			want:    "",
			ctxName: "colima",
			fixture: "meta1.json",
			wantErr: true,
		},
		{
			name:    "no host defined",
			want:    "",
			ctxName: "colima",
			fixture: "meta2.json",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tmpCtxDir(t)
			if tt.fixture != "" {
				writeTestMeta(t, tt.fixture, dir, tt.ctxName)
			}

			got, err := endpointFromContext(dir, tt.ctxName)
			if tt.wantErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}
