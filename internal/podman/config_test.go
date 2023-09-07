package podman

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_findUnixAddress(t *testing.T) {
	tests := []struct {
		name   string
		config containersConfig
		want   string
	}{
		{
			name: "use active service",
			config: containersConfig{
				Engine: engine{
					ActiveService: "default",
					ServiceDestinations: map[string]serviceDestination{
						"default": {
							URI: "unix://jonas@:22/run/user/1000/podman/podman.sock",
						},
					},
				},
			},
			want: "unix:///run/user/1000/podman/podman.sock",
		},
		{
			name: "no active service",
			config: containersConfig{
				Engine: engine{
					ActiveService: "",
					ServiceDestinations: map[string]serviceDestination{
						"default": {
							URI: "unix://jonas@:22/run/user/1000/podman/podman.sock",
						},
					},
				},
			},
			want: "unix:///run/user/1000/podman/podman.sock",
		},
		{
			name: "no unix service",
			config: containersConfig{
				Engine: engine{
					ActiveService: "",
					ServiceDestinations: map[string]serviceDestination{
						"default": {
							URI: "ssh://jonas@:22",
						},
					},
				},
			},
		},
		{
			name:   "no configuration",
			config: containersConfig{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, findUnixAddress(tt.config))
		})
	}
}

func Test_findSSHConnectionInfo(t *testing.T) {
	tests := []struct {
		name     string
		config   containersConfig
		address  string
		identity string
	}{
		{
			name: "use active service",
			config: containersConfig{
				Engine: engine{
					ActiveService: "default",
					ServiceDestinations: map[string]serviceDestination{
						"default": {
							URI:      "ssh://core@localhost:45983/run/user/1000/podman/podman.sock",
							Identity: "~/.ssh/podman-machine-default",
						},
					},
				},
			},
			address:  "ssh://core@localhost:45983/run/user/1000/podman/podman.sock",
			identity: "~/.ssh/podman-machine-default",
		},
		{
			name: "no active service",
			config: containersConfig{
				Engine: engine{
					ActiveService: "",
					ServiceDestinations: map[string]serviceDestination{
						"default": {
							URI:      "ssh://core@localhost:45983/run/user/1000/podman/podman.sock",
							Identity: "~/.ssh/podman-machine-default",
						},
					},
				},
			},
			address:  "ssh://core@localhost:45983/run/user/1000/podman/podman.sock",
			identity: "~/.ssh/podman-machine-default",
		},
		{
			name: "no ssh service",
			config: containersConfig{
				Engine: engine{
					ActiveService: "",
					ServiceDestinations: map[string]serviceDestination{
						"default": {
							URI: "unix:///place",
						},
					},
				},
			},
		},
		{
			name:   "no configuration",
			config: containersConfig{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualAddress, actualIdentity := findSSHConnectionInfo(tt.config)
			assert.Equalf(t, tt.address, actualAddress, "findSSHConnectionInfo(%v)", tt.config)
			assert.Equalf(t, tt.identity, actualIdentity, "findSSHConnectionInfo(%v)", tt.config)
		})
	}
}

func Test_configPrecedence(t *testing.T) {
	root := "test-fixtures"

	type args struct {
		paths []string
	}
	tests := []struct {
		name            string
		args            args
		wantUnixAddress string
		wantSSHAddress  string
	}{
		{
			name: "low precedence",
			args: args{paths: []string{
				filepath.Join(root, "conf1.conf"),
			}},
			wantUnixAddress: "unix:///low/precedence/1234/podman/podman.sock",
			wantSSHAddress:  "ssh://core@localhost:45983/low/precedence/1234/podman/podman.sock",
		},
		{
			name: "medium precedence",
			args: args{paths: []string{
				filepath.Join(root, "conf1.conf"),
				filepath.Join(root, "conf2.conf"),
			}},
			wantUnixAddress: "unix:///medium/precedence/1234/podman/podman.sock",
			wantSSHAddress:  "ssh://core@localhost:45983/medium/precedence/1234/podman/podman.sock",
		},
		{
			name: "high precedence",
			args: args{paths: []string{
				filepath.Join(root, "conf1.conf"),
				filepath.Join(root, "conf2.conf"),
				filepath.Join(root, "conf3.conf"),
			}},
			wantUnixAddress: "unix:///high/precedence/1234/podman/podman.sock",
			wantSSHAddress:  "ssh://core@localhost:45983/high/precedence/1234/podman/podman.sock",
		},
		{
			name: "order of paths sets precedence",
			args: args{paths: []string{
				filepath.Join(root, "conf3.conf"),
				filepath.Join(root, "conf1.conf"),
				filepath.Join(root, "conf2.conf"),
			}},
			wantUnixAddress: "unix:///medium/precedence/1234/podman/podman.sock",
			wantSSHAddress:  "ssh://core@localhost:45983/medium/precedence/1234/podman/podman.sock",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.wantUnixAddress, getUnixSocketAddressFromConfig(tt.args.paths), "getUnixSocketAddressFromConfig(%v)", tt.args.paths)
		})
	}
}
