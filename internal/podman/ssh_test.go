package podman

import (
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/testdata"
)

func TestNewSSHConfig(t *testing.T) {
	paths := []string{
		"./test-fixtures/containers.conf",
		"./test-fixtures/empty.conf",
	}

	const (
		sshAddress = "ssh://core@localhost:45983/run/user/1000/podman/podman.sock"
		sshKeyPath = "/home/jonas/.ssh/podman-machine-default"
	)

	address, identity := getSSHAddress(paths)
	assert.Equal(t, sshAddress, address)
	assert.Equal(t, sshKeyPath, identity)

	expected := &sshClientConfig{
		secure:   true,
		username: "core",
		keyPath:  sshKeyPath,
		host:     "localhost:45983",
		path:     "/run/user/1000/podman/podman.sock",
	}

	conf, err := newSSHConf(address, identity, "")
	assert.NoError(t, err)
	assert.Equal(t, expected, conf)
}

func TestEmptySSHConfig(t *testing.T) {
	paths := []string{
		"./test-fixtures/empty.conf",
	}

	address, identity := getSSHAddress(paths)
	conf, err := newSSHConf(address, identity, "")
	assert.Error(t, err)
	assert.Nil(t, conf)
	assert.ErrorIs(t, err, ErrNoHostAddress)
}

func TestGetSigners(t *testing.T) {
	var allKeyFileNames []string

	t.Cleanup(func() {
		for _, fn := range allKeyFileNames {
			err := os.Remove(fn)
			assert.NoError(t, err)
		}
	})

	for _, tt := range testdata.PEMEncryptedKeys {
		t.Run(tt.Name, func(t *testing.T) {
			kf, err := ioutil.TempFile(os.TempDir(), "key-"+tt.Name)
			assert.NoError(t, err)

			s, err := kf.Write(tt.PEMBytes)
			assert.NoError(t, err)
			assert.NotZero(t, s)
			err = kf.Close()
			assert.NoError(t, err)

			signers, err := getSigners(kf.Name(), tt.EncryptionKey)
			assert.NoError(t, err)
			assert.Len(t, signers, 1)

			allKeyFileNames = append(allKeyFileNames, kf.Name())
		})
	}
}

func TestParsePublicKey(t *testing.T) {
	for _, tt := range testdata.PEMEncryptedKeys {
		t.Run(tt.Name, func(t *testing.T) {
			_, err := getSignerFromPrivateKey(tt.PEMBytes, []byte("incorrect"))
			assert.ErrorIs(t, x509.IncorrectPasswordError, err)

			_, err = getSignerFromPrivateKey(tt.PEMBytes, []byte(tt.EncryptionKey))
			assert.NoError(t, err)
		})
	}

	t.Run("unencrypted keys", func(t *testing.T) {
		for _, k := range testdata.PEMBytes {
			_, err := getSignerFromPrivateKey(k, []byte{})
			assert.NoError(t, err)
		}
	})
}

func TestSSHCallback(t *testing.T) {
	tests := []struct {
		name   string
		config *sshClientConfig
		want   interface{}
	}{
		{
			name: "try to validate host key",
			config: &sshClientConfig{
				host:   "unknown-host.com",
				secure: true},
			want: nil,
		},
		{
			name: "do not validate host key",
			config: &sshClientConfig{
				host:   "unknown-host.com",
				secure: false},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := getSSHCallback(tt.config)
			assert.NotNil(t, cb)
			var a *net.UnixAddr
			var pk *ssh.Certificate
			err := cb(tt.config.host, a, pk)
			assert.Equal(t, tt.want, err)
		})
	}
}

func TestHostKey(t *testing.T) {
	tests := []struct {
		name           string
		knownHostsPath string
		host           string
		keyType        string
		hasPublicKey   bool
	}{
		{
			name:           "known host with public key",
			knownHostsPath: filepath.Join("test-fixtures", "known_hosts"),
			host:           "github.com",
			keyType:        "ssh-rsa",
			hasPublicKey:   true,
		},
		{
			name:           "unknown host",
			knownHostsPath: filepath.Join("test-fixtures", "known_hosts"),
			host:           "doma.in",
			keyType:        "",
			hasPublicKey:   false,
		},
		{
			name:           "file not found",
			knownHostsPath: filepath.Join("test-fixtures", "not-there"),
			host:           "doma.in",
		},
		{
			name:           "file not found",
			knownHostsPath: filepath.Join("test-fixtures", "known_hosts_empty"),
			host:           "doma.in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pk := hostKey(tt.host, tt.knownHostsPath)

			if tt.hasPublicKey {
				assert.Equal(t, tt.keyType, pk.Type())
			} else {
				assert.Nil(t, pk)
			}
		})
	}
}

func Test_newSSHConf(t *testing.T) {
	pass := func(t assert.TestingT, err error, i ...interface{}) bool {
		return true
	}

	type args struct {
		address    string
		identity   string
		passPhrase string
	}
	tests := []struct {
		name    string
		args    args
		want    *sshClientConfig
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "empty address",
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return errors.Is(err, ErrNoHostAddress)
			},
		},
		{
			name: "invalid secure flag",
			args: args{
				address: "ssh://core@localhost:123/file/path/podman.sock?secure=not-a-bool-value",
			},
			want: &sshClientConfig{
				host:     "localhost:123",
				path:     "/file/path/podman.sock",
				secure:   true,
				username: "core",
			},
			wantErr: pass,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newSSHConf(tt.args.address, tt.args.identity, tt.args.passPhrase)
			if !tt.wantErr(t, err, fmt.Sprintf("newSSHConf(%v, %v, %v)", tt.args.address, tt.args.identity, tt.args.passPhrase)) {
				return
			}
			assert.Equalf(t, tt.want, got, "newSSHConf(%v, %v, %v)", tt.args.address, tt.args.identity, tt.args.passPhrase)
		})
	}
}
