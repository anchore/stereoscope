package podman

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"

	"github.com/docker/docker/pkg/homedir"
	"github.com/pelletier/go-toml"
)

// env vars
// XDG_CONFIG_HOME
// XDG_RUNTIME_DIR
// CONTAINER_SSHKEY

// {env vars} -> {config path} -> address

type containersConfig struct {
	Engine engine `toml:"engine"`
}

type engine struct {
	ActiveService       string                        `toml:"active_service"`
	ServiceDestinations map[string]serviceDestination `toml:"service_destinations"`
}

type serviceDestination struct {
	URI      string `toml:"uri"`
	Identity string `toml:"identity"`
}

func findUnixAddressFromFile(path string) string {
	cc, err := parseContainerConfig(path)
	if err != nil {
		return ""
	}

	if cc == nil {
		return ""
	}

	return findUnixAddress(*cc)
}

func findDestinationOfType(cc containersConfig, ty string) *serviceDestination {
	// always attempt what the config says is the current service
	if destination, ok := cc.Engine.ServiceDestinations[cc.Engine.ActiveService]; ok {
		if isScheme(destination.URI, ty) {
			return &destination
		}
	}

	// fallback to looking at all configured services if the active service is not found or is not unix
	for _, destination := range cc.Engine.ServiceDestinations {
		if destination.URI == "" {
			continue
		}
		if isScheme(destination.URI, ty) {
			return &destination
		}
	}
	return nil
}

func findSSHConnectionInfoFromFile(path string) (string, string) {
	cc, err := parseContainerConfig(path)
	if err != nil {
		return "", ""
	}

	if cc == nil {
		return "", ""
	}

	return findSSHConnectionInfo(*cc)
}
func findSSHConnectionInfo(cc containersConfig) (string, string) {
	dest := findDestinationOfType(cc, "ssh")
	if dest == nil {
		return "", ""
	}

	return dest.URI, dest.Identity
}

func findUnixAddress(cc containersConfig) string {
	dest := findDestinationOfType(cc, "unix")
	if dest == nil {
		return ""
	}
	return parseUnixAddress(dest.URI)
}

func parseUnixAddress(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}

	if u.Scheme == "unix" {
		return fmt.Sprintf("unix://%s", u.Path)
	}
	return ""
}

func isScheme(uri, scheme string) bool {
	u, err := url.Parse(uri)
	if err != nil {
		return false
	}

	return u.Scheme == scheme
}

func parseContainerConfig(path string) (*containersConfig, error) {
	configBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cc containersConfig
	if err := toml.Unmarshal(configBytes, &cc); err != nil {
		return nil, err
	}
	return &cc, nil
}

// func defaultUnixAddress(xdgRuntime string) string {
// 	return fmt.Sprintf("unix://%s/podman/podman.sock", xdgRuntime)
// }

var (
	// _configPath is the path to the containers/containers.conf
	// inside a given config directory.
	_configPath = "containers/containers.conf"
	// paths holds a list of config files, they are sorted from
	// the least to the most relevant, i.e. a config file in
	// the home directory has precedence over all other configs
	configPaths = []string{
		// holds the default containers config path
		"/usr/share/" + _configPath,
		// holds the default config path overridden by the root user
		"/etc/" + _configPath,
		// holds the containers config path overridden by the rootless user
		filepath.Join(homedir.Get(), "/.config/", _configPath),
	}
)

func getUnixSocketAddress(paths []string) (address string) {
	for _, p := range configPaths {
		a := findUnixAddressFromFile(p)
		if a != "" {
			address = a
		}
	}

	return
}

func getSSHAddress(paths []string) (address, identity string) {
	for _, p := range paths {
		a, id := findSSHConnectionInfoFromFile(p)
		if a != "" && id != "" {
			address = a
			identity = id
		}
	}

	return
}
