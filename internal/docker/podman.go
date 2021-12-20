package docker

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
	// remote rootless ssh://engineering.lab.company.com/run/user/$UID/podman/podman.sock
	// remote rootfull ssh://root@10.10.1.136:22/run/podman/podman.sock

	localRootlessPathTemplate = "/run/user/%d/podman/podman.sock"
	defaultRemoteRootless     = "ssh://core@localhost:63753%s?secure=false"
	defaultLocalRootless      = fmt.Sprintf("unix://%s", getLocalSocketPath())
)

func getLocalSocketPath() string {
	return fmt.Sprintf(localRootlessPathTemplate, os.Getuid())
}

func getRemoteRootlessAddress() (string, error) {
	uid, err := getRemoteUID()
	if err != nil {
		return "", err
	}

	sock := fmt.Sprintf(localRootlessPathTemplate, uid)
	return fmt.Sprintf(defaultRemoteRootless, sock), nil
}

func getRemoteUID() (int, error) {
	// TODO: expose these ssh params to caller. End user should have settings for them
	cmd := exec.Command("ssh", "-i", "~/.ssh/podman-machine-default", "-l", "core", "-p", "63753", "--", "localhost", "id -u")
	out := &bytes.Buffer{}
	cmd.Stdout = out

	err := cmd.Run()
	if err != nil {
		return 0, fmt.Errorf("getting remote user ID: %w", err)
	}
	vet := strings.TrimSuffix(out.String(), "\n")
	vet = strings.TrimSpace(vet)

	uid, err := strconv.Atoi(vet)
	if err != nil {
		return 0, fmt.Errorf("converting reponse: %w", err)
	}

	log.Debugf("remote user ID is: %d", uid)
	return uid, nil
}

// TODO(jonas): defaultRemoteRootless includes the UID from a remote users,
// which might be different of the user's UID in the running host.
// We should resolve the remote UID then add it to path of the container client
func podmanOverSSH() (*client.Client, error) {
	log.Debug("using docker client to connect to podman over ssh")
	var clientOpts = []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	host, err := getRemoteRootlessAddress()
	if err != nil {
		return nil, err
	}
	makeClient := sshClient

	if v, found := os.LookupEnv("CONTAINER_HOST"); found && v != "" {
		log.Debugf("using $CONTAINER_HOST: %s", v)
		host = v
	}

	log.Debugf("host: %q", host)

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
		return nil, fmt.Errorf("failed create remote client for podman: %w", err)
	}

	return dockerClient, err
}

func podmanViaUnixSocket() (*client.Client, error) {
	log.Debug("using docker client to connect to podman over local unix socket")

	// the last option can overwrite previous options
	var clientOpts = []client.Opt{
		client.WithAPIVersionNegotiation(),
	}
	// NOTE 0: default unix socket path, least precedence
	clientOpts = append(clientOpts, client.WithHost(defaultLocalRootless))

	// NOTE 1: CONTAINER_HOST can overwrite defaultLocalRootless
	// var name is a Podman conversion: https://github.com/containers/podman/blob/main/pkg/bindings/connection.go#L72
	if v, found := os.LookupEnv("CONTAINER_HOST"); found && v != "" {
		log.Debugf("using $CONTAINER_HOST: %s", v)
		clientOpts = append(clientOpts, client.WithHost(v))
	}

	dockerClient, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed create local client for podman: %w", err)
	}

	return dockerClient, err
}

func GetClientForPodman() (*client.Client, error) {
	log.Debug("creating podman client")

	// TODO: how should detection work here?
	switch runtime.GOOS {
	case "windows", "darwin":
		return podmanOverSSH()
	default:
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
}

type clientMaker func(*url.URL) (*http.Client, error)

func getSSHKey() string {
	identity := filepath.Join(homedir.Get(), ".ssh", "podman-machine-default")
	if v, found := os.LookupEnv("CONTAINER_SSHKEY"); found && len(identity) == 0 {
		log.Debugf("using $CONTAINER_SSHKEY: %s", v)
		return v
	}

	return identity
}

// NOTE: code inspired by Podman's client: https://github.com/containers/podman/blob/main/pkg/bindings/connection.go#L177
func sshClient(hostURL *url.URL) (*http.Client, error) {
	identity := getSSHKey()
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
