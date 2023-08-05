package docker

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/context/store"
	"github.com/docker/docker/pkg/homedir"
)

const (
	defaultContextName = "default"
	dockerEndpoint     = "docker"

	envOverrideContext = "DOCKER_CONTEXT"
)

var (
	configFileDir  = filepath.Join(homedir.Get(), ".docker")
	contextsDir    = filepath.Join(configFileDir, "contexts")
	configFileName = filepath.Join(configFileDir, "config.json")
)

func resolveContextName(config *configfile.ConfigFile) string {
	if ctxName := os.Getenv(envOverrideContext); ctxName != "" {
		return ctxName
	}
	if config != nil && config.CurrentContext != "" {
		return config.CurrentContext
	}
	return defaultContextName
}

func loadConfig(filename string) (*configfile.ConfigFile, error) {
	cfg := configfile.New(filename)

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	err = cfg.LoadFromReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("cant parse docker config: %w", err)
	}

	return cfg, err
}

func endpointFromContext(ctx string) (string, error) {
	st := store.New(config.ContextStoreDir(), store.Config{})

	meta, err := st.GetMetadata(ctx)
	if err != nil {
		return "", fmt.Errorf("cant get docker config metadata: %w", err)
	}

	// retrieving endpoint
	ep, ok := meta.Endpoints[dockerEndpoint].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("cant get docker endpoint from metadata: %w", err)
	}

	host, ok := ep["Host"].(string)
	if !ok {
		return "", fmt.Errorf("cant get docker host from metadata: %w", err)
	}

	return host, nil
}
