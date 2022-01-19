package podman

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	conf, err := NewSSHConf(address, identity, "")
	assert.NoError(t, err)
	assert.True(t, conf.secure)
	assert.Equal(t, "core", conf.username)
	assert.Empty(t, conf.password)
	assert.Equal(t, sshKeyPath, conf.keyPath)
	assert.Empty(t, conf.keyPassphrase)
	assert.Equal(t, "localhost:45983", conf.host)
	assert.Equal(t, "/run/user/1000/podman/podman.sock", conf.path)
}

//func TestGetSigners(t *testing.T) {
//	// key generated with: 'ssh-keygen -N "12345" -t ed25519
//	err := os.Setenv("CONTAINER_SSHKEY", "./test-fixtures/key-file.pub")
//	assert.NoError(t, err)
//
//	host, err := url.Parse("http://:bla@server.com/")
//	assert.NoError(t, err)
//
//	signers, err := getSigners(host)
//	assert.NoError(t, err)
//	assert.NotEmpty(t, signers)
//}
