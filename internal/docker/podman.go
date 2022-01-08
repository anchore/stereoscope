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
	"time"

	"github.com/anchore/stereoscope/internal/log"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/homedir"
	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func getAddressFromConfig(containerConfigPath string) (string, error) {
	configBytes, err := ioutil.ReadFile(containerConfigPath)
	if err != nil {
		return "", fmt.Errorf("openning podman config file: %w", err)
	}

	config, err := toml.LoadBytes(configBytes)
	if err != nil {
		return "", fmt.Errorf("loading podman config: %w", err)
	}

	activeService := config.Get("engine.active_service").(string)
	return config.Get(fmt.Sprintf("engine.service_destinations.%s.uri", activeService)).(string), nil
}

// TODO: podman not always has clean configs in all platforms it supports
// we should fail nicely when errors appear
// For example: on linux the engine address is jonas@:22/run/user/1000/podman/podman.sock
// which is not valid AFAIK after testing with go and curl
func getPodmanAddress() (string, error) {
	configPath := filepath.Join(homedir.Get(), ".config", "containers", "containers.conf")
	return getAddressFromConfig(configPath)
}

func podmanOverSSH() (*client.Client, error) {
	var clientOpts = []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	host, err := getPodmanAddress()
	if err != nil {
		return nil, err
	}
	makeClient := sshClient

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
	// This http path is defined by podman's docs: https://github.com/containers/podman/blob/main/pkg/api/server/docs.go#L31-L34
	clientOpts = append(clientOpts, client.WithHost("http://d"))

	dockerClient, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed create remote client for podman: %w", err)
	}

	return dockerClient, err
}

func podmanViaUnixSocket() (*client.Client, error) {
	// the last option can overwrite previous options
	var clientOpts = []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	addr, err := getPodmanAddress()
	if err != nil {
		return nil, fmt.Errorf("getting podman unix socket address: %w", err)
	}

	clientOpts = append(clientOpts, client.WithHost(addr))

	c, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed create local client for podman: %w", err)
	}

	return c, err
}

func GetClientForPodman() (*client.Client, error) {
	c, err := podmanViaUnixSocket()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.TODO(), time.Second*3)
	defer cancel()
	_, err = c.Ping(ctx)
	if err == nil {
		return c, nil
	}

	return podmanOverSSH()
}

func getSSHKey() string {
	identity := filepath.Join(homedir.Get(), ".ssh", "podman-machine-default")
	if v, found := os.LookupEnv("CONTAINER_SSHKEY"); found && len(identity) == 0 {
		log.Debugf("using $CONTAINER_SSHKEY: %s", v)
		return v
	}

	return identity
}

func getSigners(hostURL *url.URL) (signers []ssh.Signer, err error) {
	identity := getSSHKey()
	if len(identity) > 0 {
		passPhrase, _ := hostURL.User.Password()
		s, err := publicKey(identity, []byte(passPhrase))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse identity %q", identity)
		}

		signers = append(signers, s)
	}

	return
}

// NOTE: code inspired by Podman's client: https://github.com/containers/podman/blob/main/pkg/bindings/connection.go#L177
func sshClient(hostURL *url.URL) (*http.Client, error) {
	signers, err := getSigners(hostURL) // order Signers are appended to this list determines which key is presented to server
	if err != nil {
		return nil, err
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

	callback := ssh.InsecureIgnoreHostKey()
	secure, err := strconv.ParseBool(hostURL.Query().Get("secure"))
	if err != nil {
		secure = false
	}

	if secure {
		host := hostURL.Hostname()
		if port != "22" {
			host = fmt.Sprintf("[%s]:%s", host, port)
		}
		key := hostKey(host)
		if key != nil {
			callback = ssh.FixedHostKey(key)
		}
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
