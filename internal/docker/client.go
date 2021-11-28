package docker

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
	"strings"
	"sync"
	"time"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/homedir"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	once        sync.Once
	instance    *client.Client
	instanceErr error
)

func GetClient(srcName string) (*client.Client, error) {
	if srcName == "PodmanDaemon" {
		return getPodmanClient()
	}

	once.Do(func() {
		var clientOpts = []client.Opt{
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		}

		host := os.Getenv("DOCKER_HOST")

		if strings.HasPrefix(host, "ssh") && strings.Contains(host, "podman.sock") {
			_url, err := url.Parse(host)
			if err != nil {
				log.Errorf("error parsing host %s with: %v", host, err)
				return
			}

			secure, err := strconv.ParseBool(_url.Query().Get("secure"))
			if err != nil {
				secure = false
			}

			httpClient, err := sshClient(_url, secure, "", "/Users/jonas/.ssh/podman-machine-default")
			if err != nil {
				log.Errorf("failed to make ssh client: %v", err)
				return
			}
			clientOpts = append(clientOpts, func(c *client.Client) error {
				return client.WithHTTPClient(httpClient)(c)
			})
			clientOpts = append(clientOpts, client.WithHost("http://d"))
		} else if strings.HasPrefix(host, "ssh") {
			var helper *connhelper.ConnectionHelper
			var err error

			helper, err = connhelper.GetConnectionHelper(host)

			if err != nil {
				log.Errorf("failed to fetch docker connection helper: %w", err)
				instanceErr = err
				return
			}
			clientOpts = append(clientOpts, func(c *client.Client) error {
				httpClient := &http.Client{
					Transport: &http.Transport{
						DialContext: helper.Dialer,
					},
				}
				return client.WithHTTPClient(httpClient)(c)
			})
			clientOpts = append(clientOpts, client.WithHost(helper.Host))
			clientOpts = append(clientOpts, client.WithDialContext(helper.Dialer))
		}

		if os.Getenv("DOCKER_TLS_VERIFY") != "" && os.Getenv("DOCKER_CERT_PATH") == "" {
			err := os.Setenv("DOCKER_CERT_PATH", "~/.docker")
			if err != nil {
				log.Errorf("failed create docker client: %w", err)
				instanceErr = err
				return
			}
		}
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

func sshClient(_url *url.URL, secure bool, passPhrase string, identity string) (*http.Client, error) {
	var signers []ssh.Signer // order Signers are appended to this list determines which key is presented to server

	if len(identity) > 0 {
		s, err := publicKey(identity, []byte(passPhrase))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse identity %q", identity)
		}

		signers = append(signers, s)
		logrus.Debugf("SSH Ident Key %q %s %s", identity, ssh.FingerprintSHA256(s.PublicKey()), s.PublicKey().Type())
	}

	if sock, found := os.LookupEnv("SSH_AUTH_SOCK"); found {
		logrus.Debugf("Found SSH_AUTH_SOCK %q, ssh-agent signer(s) enabled", sock)

		c, err := net.Dial("unix", sock)
		if err != nil {
			return nil, err
		}

		agentSigners, err := agent.NewClient(c).Signers()
		if err != nil {
			return nil, err
		}
		signers = append(signers, agentSigners...)

		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			for _, s := range agentSigners {
				logrus.Debugf("SSH Agent Key %s %s", ssh.FingerprintSHA256(s.PublicKey()), s.PublicKey().Type())
			}
		}
	}

	var authMethods []ssh.AuthMethod
	if len(signers) > 0 {
		var dedup = make(map[string]ssh.Signer)
		// Dedup signers based on fingerprint, ssh-agent keys override CONTAINER_SSHKEY
		for _, s := range signers {
			fp := ssh.FingerprintSHA256(s.PublicKey())
			if _, found := dedup[fp]; found {
				logrus.Debugf("Dedup SSH Key %s %s", ssh.FingerprintSHA256(s.PublicKey()), s.PublicKey().Type())
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

	if secure {
		host := _url.Hostname()
		if port != "22" {
			host = fmt.Sprintf("[%s]:%s", host, port)
		}
		key := hostKey(host)
		if key != nil {
			callback = ssh.FixedHostKey(key)
		}
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

func hostKey(host string) ssh.PublicKey {
	// parse OpenSSH known_hosts file
	// ssh or use ssh-keyscan to get initial key
	knownHosts := filepath.Join(homedir.Get(), ".ssh", "known_hosts")
	fd, err := os.Open(knownHosts)
	if err != nil {
		logrus.Error(err)
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
