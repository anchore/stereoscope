package docker

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	configTypes "github.com/docker/cli/cli/config/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeCredentials(t *testing.T) {
	// regression test for https://github.com/anchore/grype/issues/254
	// the JSON encoded credentials should NOT escape characters

	user, pass := "dockerusertest", "WL[cC-<sN#K(zk~NVspmw.PL)3K?v"
	// encoded string: expected := base64encode(`{"password":"WL[cC-<sN#K(zk~NVspmw.PL)3K?v","username":"dockerusertest"}\n`)
	// where the problem character is the "<" within the password, which should NOT be encoded to \u003c

	cfg := configTypes.AuthConfig{
		Username: user,
		Password: pass,
	}

	actual, err := encodeCredentials(cfg)
	if err != nil {
		t.Fatalf("unable to encode credentials: %+v", err)
	}
	actualCfgBytes, err := base64.URLEncoding.DecodeString(actual)
	require.NoError(t, err)

	var actualCfg configTypes.AuthConfig
	require.NoError(t, json.Unmarshal(actualCfgBytes, &actualCfg))
	assert.Equal(t, user, actualCfg.Username)
	assert.Equal(t, pass, actualCfg.Password)
}

func Test_authURL(t *testing.T) {
	tests := []struct {
		imageStr   string
		workaround bool
		want       string
		wantErr    require.ErrorAssertionFunc
	}{
		{
			imageStr:   "alpine:latest",
			workaround: true,
			want:       "index.docker.io/v1/",
		},
		{
			imageStr:   "alpine:latest",
			workaround: false,
			want:       "index.docker.io",
		},
		{
			imageStr:   "myhost.io/alpine:latest",
			workaround: true,
			want:       "myhost.io",
		},
		{
			imageStr:   "someone/something:latest",
			workaround: true,
			want:       "index.docker.io/v1/",
		},
		{
			imageStr:   "somewhere.io/someone/something:latest",
			workaround: true,
			want:       "somewhere.io",
		},
		{
			imageStr:   "host.io:5000/image:latest",
			workaround: true,
			want:       "host.io:5000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.imageStr, func(t *testing.T) {
			if tt.wantErr == nil {
				tt.wantErr = require.NoError
			}
			got, err := authURL(tt.imageStr, tt.workaround)
			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
