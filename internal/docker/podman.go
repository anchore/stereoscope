package docker

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/homedir"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var (
	makePodmanClientOnce sync.Once
	defaultHost          = "ssh://core@localhost:63753/run/user/1000/podman/podman.sock?secure=false"
)

func getPodmanClient() (*client.Client, error) {
	makePodmanClientOnce.Do(func() {
		log.Debug("creating podman client via docker")
		var clientOpts = []client.Opt{
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		}

		host := defaultHost

		_url, err := url.Parse(host)
		if err != nil {
			log.Errorf("error parsing host %s with: %v", host, err)
			return
		}

		podmanSSHKeyPath := filepath.Join(homedir.Get(), ".ssh", "podman-machine-default")
		httpClient, err := sshClient(_url, "", podmanSSHKeyPath)
		if err != nil {
			log.Errorf("failed to make ssh client: %v", err)
			return
		}
		clientOpts = append(clientOpts, func(c *client.Client) error {
			return client.WithHTTPClient(httpClient)(c)
		})
		clientOpts = append(clientOpts, client.WithHost("http://d"))

		dockerClient, err := client.NewClientWithOpts(clientOpts...)
		if err != nil {
			log.Errorf("failed create docker client: %w", err)
			instanceErr = err
			return
		}

		instance = dockerClient
	})

	return instance, instanceErr
}

func sshClient(_url *url.URL, passPhrase string, identity string) (*http.Client, error) {
	var signers []ssh.Signer // order Signers are appended to this list determines which key is presented to server

	if len(identity) > 0 {
		s, err := publicKey(identity, []byte(passPhrase))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse identity %q", identity)
		}

		signers = append(signers, s)
		log.Debugf("SSH Ident Key %q %s %s", identity, ssh.FingerprintSHA256(s.PublicKey()), s.PublicKey().Type())
	}

	if sock, found := os.LookupEnv("SSH_AUTH_SOCK"); found {
		log.Debugf("found SSH_AUTH_SOCK %q, ssh-agent signer(s) enabled", sock)

		c, err := net.Dial("unix", sock)
		if err != nil {
			return nil, err
		}

		agentSigners, err := agent.NewClient(c).Signers()
		if err != nil {
			return nil, err
		}
		signers = append(signers, agentSigners...)

		for _, s := range agentSigners {
			log.Debugf("SSH Agent Key %s %s", ssh.FingerprintSHA256(s.PublicKey()), s.PublicKey().Type())
		}
	}

	var authMethods []ssh.AuthMethod
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
		authMethods = append(authMethods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
			return uniq, nil
		}))
	}

	if pw, found := _url.User.Password(); found {
		authMethods = append(authMethods, ssh.Password(pw))
	}

	port := _url.Port()
	if port == "" {
		port = "22"
	}

	callback := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return nil
	}

	bastion, err := ssh.Dial("tcp",
		net.JoinHostPort(_url.Hostname(), port),
		&ssh.ClientConfig{
			User:            _url.User.Username(),
			Auth:            authMethods,
			HostKeyCallback: callback,
			HostKeyAlgorithms: []string{
				ssh.KeyAlgoRSA,
				ssh.KeyAlgoDSA,
				ssh.KeyAlgoECDSA256,
				ssh.KeyAlgoECDSA384,
				ssh.KeyAlgoECDSA521,
				ssh.KeyAlgoED25519,
			},
			Timeout: 5 * time.Second,
		},
	)
	if err != nil {
		return nil, errors.Wrapf(err, "Connection to bastion host (%s) failed.", _url.String())
	}

	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return bastion.Dial("unix", _url.Path)
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
