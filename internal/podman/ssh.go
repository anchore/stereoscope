package podman

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/docker/docker/pkg/homedir"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type sshClientConfig struct {
	host          string
	path          string
	keyPath       string
	keyPassphrase string
	secure        bool
	username      string
	password      string
}

func newSSHConf(address, identity, passPhrase string) (*sshClientConfig, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("parsing ssh address: %w", err)
	}

	// This flag is meant to control whether the ssh handshake validates
	// the server's host key against the local known keys in .ssh/known_hosts,
	// which is important when talking to remote Podman servers.
	// If no flag is passed, empty string, ParseBool will return an error, setting the value to
	// true, i.e:
	// secure is true unless explicitly set to false
	secure, err := strconv.ParseBool(u.Query().Get("secure"))
	if err != nil {
		// secure by default
		secure = true
	}

	return &sshClientConfig{
		host:          u.Host,
		path:          u.Path,
		keyPath:       identity,
		keyPassphrase: passPhrase,
		secure:        secure,
		username:      u.User.Username(),
	}, nil
}

func getSigners(keyPath, passphrase string) (signers []ssh.Signer, err error) {
	if keyPath != "" {
		s, err := publicKey(keyPath, []byte(passphrase))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse identity %q", keyPath)
		}

		signers = append(signers, s)
	}

	return
}

func getAuthMethods(params *sshClientConfig) ([]ssh.AuthMethod, error) {
	signers, err := getSigners(params.keyPath, params.keyPassphrase) // order Signers are appended to this list determines which key is presented to server
	if err != nil {
		return nil, err
	}

	var methods []ssh.AuthMethod
	if len(signers) > 0 {
		var dedup = make(map[string]ssh.Signer)
		// Dedup signers based on fingerprint, ssh-agent keys override CONTAINER_SSHKEY
		for _, s := range signers {
			fp := ssh.FingerprintSHA256(s.PublicKey())
			if _, found := dedup[fp]; found {
				log.Debugf("dedup SSH Key %s %s", ssh.FingerprintSHA256(s.PublicKey()), s.PublicKey().Type())
			}
			dedup[fp] = s
		}

		var uniq []ssh.Signer
		for _, s := range dedup {
			uniq = append(uniq, s)
		}
		methods = append(methods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
			return uniq, nil
		}))
	}

	if params.password != "" {
		methods = append(methods, ssh.Password(params.password))
	}

	return methods, nil
}

func getSSHCallback(params *sshClientConfig) ssh.HostKeyCallback {
	// nolint: gosec
	cb := ssh.InsecureIgnoreHostKey()
	if !params.secure {
		return cb
	}

	key := hostKey(params.host)
	if key != nil {
		cb = ssh.FixedHostKey(key)
	}

	return cb
}

// NOTE: code inspired by Podman's client: https://github.com/containers/podman/blob/main/pkg/bindings/connection.go#L177
func httpClientOverSSH(params *sshClientConfig) (*http.Client, error) {
	if params == nil {
		return nil, errors.New("empty ssh config")
	}

	authMethods, err := getAuthMethods(params)
	if err != nil {
		return nil, fmt.Errorf("getting ssh auth methods: %w", err)
	}

	bastion, err := ssh.Dial("tcp",
		params.host,
		&ssh.ClientConfig{
			User:            params.username,
			Auth:            authMethods,
			HostKeyCallback: getSSHCallback(params),
			HostKeyAlgorithms: []string{
				ssh.KeyAlgoDSA,
				ssh.KeyAlgoECDSA256,
				ssh.KeyAlgoECDSA384,
				ssh.KeyAlgoECDSA521,
				ssh.KeyAlgoED25519,
				ssh.KeyAlgoRSA,
			},
			Timeout: 5 * time.Second,
		},
	)
	if err != nil {
		return nil, errors.Wrapf(err, "connection to bastion host (%s) failed.", params.host)
	}

	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return bastion.Dial("unix", params.path)
			},
		}}, nil
}

func publicKey(path string, passphrase []byte) (ssh.Signer, error) {
	key, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		if _, ok := err.(*ssh.PassphraseMissingError); !ok {
			return nil, err
		}
		return ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
	}
	return signer, nil
}

func hostKey(host string) ssh.PublicKey {
	// parse OpenSSH known_hosts file
	// ssh or use ssh-keyscan to get initial key
	knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")
	fd, err := os.Open(knownHosts)
	if err != nil {
		log.Errorf("openning known_hosts", err)
		return nil
	}

	// support -H parameter for ssh-keyscan
	hashhost := knownhosts.HashHostname(host)

	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		_, hosts, key, _, _, err := ssh.ParseKnownHosts(scanner.Bytes())
		if err != nil {
			logrus.Errorf("Failed to parse known_hosts: %s", scanner.Text())
			continue
		}

		for _, h := range hosts {
			if h == host || h == hashhost {
				return key
			}
		}
	}

	return nil
}
