package docker

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anchore/stereoscope/internal/log"
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

func processConfig(overrideContext, dir string, cfg *configfile.ConfigFile) (string, error) {
	dockerContext := resolveContextName(overrideContext, cfg)

	log.Debugf("current docker context: %s", dockerContext)

	return endpointFromContext(dir, dockerContext)
}

func resolveContextName(contextOverride string, config *configfile.ConfigFile) string {
	if contextOverride != "" {
		return contextOverride
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

func endpointFromContext(dir, ctxName string) (string, error) {
	st := store.New(dir, store.Config{})

	meta, err := st.GetMetadata(ctxName)
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
