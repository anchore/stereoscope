package docker

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	configTypes "github.com/docker/cli/cli/config/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/image"
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

type mockEmitter struct {
	calledEvents []*pullEvent
}

func (m *mockEmitter) onEvent(event *pullEvent) {
	m.calledEvents = append(m.calledEvents, event)
}

func TestHandlePullEventWithMockEmitter(t *testing.T) {
	tests := []struct {
		name          string
		event         *pullEvent
		expectOnEvent bool
		assertFunc    require.ErrorAssertionFunc
	}{
		{
			name: "error in event",
			event: &pullEvent{
				Error: "fetch failed",
			},
			expectOnEvent: false,
			assertFunc: func(t require.TestingT, err error, args ...interface{}) {
				require.Error(t, err)
				var pErr *image.ErrPlatformMismatch
				require.NotErrorAs(t, err, &pErr)
			},
		},
		{
			name: "platform error in event",
			event: &pullEvent{
				Error: "image with reference anchore/test_images:golang was found but its platform (linux/amd64) does not match the specified platform (linux/arm64)",
			},
			expectOnEvent: false,
			assertFunc: func(t require.TestingT, err error, args ...interface{}) {
				require.Error(t, err)
				var pErr *image.ErrPlatformMismatch
				require.ErrorAs(t, err, &pErr)
			},
		},
		{
			name: "digest event",
			event: &pullEvent{
				Status: "Digest: abc123",
			},
			expectOnEvent: false,
			assertFunc:    require.NoError,
		},
		{
			name: "status event",
			event: &pullEvent{
				Status: "Status: Downloaded",
			},
			expectOnEvent: false,
			assertFunc:    require.NoError,
		},
		{
			name: "non-terminal event",
			event: &pullEvent{
				Status: "Downloading layer",
			},
			expectOnEvent: true,
			assertFunc:    require.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockEmitter{}
			err := handlePullEvent(mock, tt.event)
			tt.assertFunc(t, err)

			if tt.expectOnEvent {
				require.Len(t, mock.calledEvents, 1)
				require.Equal(t, tt.event, mock.calledEvents[0])
			} else {
				require.Empty(t, mock.calledEvents)
			}
		})
	}
}
