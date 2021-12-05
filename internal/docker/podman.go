package docker

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/homedir"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

var (
	// Podman supports the following connections
	// rootless "unix://run/user/$UID/podman/podman.sock" (Default)
	// rootfull "unix://run/podman/podman.sock (Default)
	// remote rootless ssh://engineering.lab.company.com/run/user/1000/podman/podman.sock
	// remote rootfull ssh://root@10.10.1.136:22/run/podman/podman.sock

	localRootlessPath     = "/run/user/1000/podman/podman.sock"
	defaultRemoteRootless = fmt.Sprintf("ssh://core@localhost:63753%s?secure=false", localRootlessPath)
	defaultLocalRootless  = fmt.Sprintf("unix://%s", localRootlessPath)
)

func configClientPerPlatform() (string, clientMaker) {
	switch runtime.GOOS {
	case "windows", "darwin":
		return defaultRemoteRootless, sshClient
	default:
		return defaultLocalRootless, unixClient
	}
}

func GetClientForPodman() (*client.Client, error) {
	log.Debug("creating podman client via docker")
	var clientOpts = []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	host, makeClient := configClientPerPlatform()

	if v, found := os.LookupEnv("CONTAINER_HOST"); found && v != "" {
		log.Debugf("using $CONTAINER_HOST: %s", v)
		host = v
	}

	if v, found := os.LookupEnv("PODMAN_HOST"); found && v != "" {
		log.Debugf("using $PODMAN_HOST: %s", v)
		host = v
	}

	hostURL, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("error parsing host %s with: %w", host, err)
	}

	httpClient, err := makeClient(hostURL)
	if err != nil {
		return nil, fmt.Errorf("making http client: %w", err)
	}

	clientOpts = append(clientOpts, func(c *client.Client) error {
		return client.WithHTTPClient(httpClient)(c)
	})
	clientOpts = append(clientOpts, client.WithHost("http://d"))

	dockerClient, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed create docker client: %w", err)
	}

	return dockerClient, err
}

type clientMaker func(*url.URL) (*http.Client, error)

func unixClient(hostURL *url.URL) (*http.Client, error) {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", hostURL.Path)
			},
			DisableCompression: true,
		},
	}, nil
}

// NOTE: code inspired by Podman's client: https://github.com/containers/podman/blob/main/pkg/bindings/connection.go#L177
func sshClient(hostURL *url.URL) (*http.Client, error) {
	identity := filepath.Join(homedir.Get(), ".ssh", "podman-machine-default")
	if v, found := os.LookupEnv("CONTAINER_SSHKEY"); found && len(identity) == 0 {
		log.Debugf("using $CONTAINER_SSHKEY: %s", v)
		identity = v
	}

	var signers []ssh.Signer // order Signers are appended to this list determines which key is presented to server

	if len(identity) > 0 {
		passPhrase, _ := hostURL.User.Password()
		s, err := publicKey(identity, []byte(passPhrase))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse identity %q", identity)
		}

		signers = append(signers, s)
		log.Debugf("SSH Ident Key %q %s %s", identity, ssh.FingerprintSHA256(s.PublicKey()), s.PublicKey().Type())
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

	if pw, found := hostURL.User.Password(); found {
		authMethods = append(authMethods, ssh.Password(pw))
	}

	port := hostURL.Port()
	if port == "" {
		port = "22"
	}

	callback := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return nil
	}

	bastion, err := ssh.Dial("tcp",
		net.JoinHostPort(hostURL.Hostname(), port),
		&ssh.ClientConfig{
			User:            hostURL.User.Username(),
			Auth:            authMethods,
			HostKeyCallback: callback,
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
		return nil, errors.Wrapf(err, "Connection to bastion host (%s) failed.", hostURL.String())
	}

	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return bastion.Dial("unix", hostURL.Path)
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
