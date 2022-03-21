package docker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/context/docker"
	"github.com/docker/cli/cli/context/store"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/pkg/errors"
)

// much of this logic is copied from:
// - https://github.com/docker/cli/blob/6745f62a0b8c5c2a00306723da5fb835a7087ebd/cli/command/cli.go#L193-L237
// - https://github.com/docker/cli/blob/3304c49771ee27c87791e65064111b106551401b/cli/flags/common.go

func GetClient() (client.APIClient, error) {
	dockerTLSVerify := os.Getenv("DOCKER_TLS_VERIFY") != ""
	common := &flags.CommonOptions{
		TLS:       os.Getenv("DOCKER_TLS") != "",
		TLSVerify: dockerTLSVerify,
	}

	if dockerTLSVerify {
		dockerCertPath := os.Getenv("DOCKER_CERT_PATH")
		if dockerCertPath == "" {
			dockerCertPath = "~/.docker"
		}
		common.TLSOptions = &tlsconfig.Options{
			CAFile:   filepath.Join(dockerCertPath, flags.DefaultCaFile),
			CertFile: filepath.Join(dockerCertPath, flags.DefaultCertFile),
			KeyFile:  filepath.Join(dockerCertPath, flags.DefaultKeyFile),
		}
	}

	configFile := config.LoadDefaultConfigFile(io.Discard)
	contextStoreConfig := command.DefaultContextStoreConfig()
	baseContextStore := store.New(config.ContextStoreDir(), contextStoreConfig)
	contextStore := &command.ContextStoreWithDefault{
		Store: baseContextStore,
		Resolver: func() (*command.DefaultContext, error) {
			return command.ResolveDefaultContext(common, configFile, contextStoreConfig, io.Discard)
		},
	}
	currentContext, err := resolveContextName(common, configFile, contextStore)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve docker context: %w", err)
	}
	dockerEndpoint, err := resolveDockerEndpoint(contextStore, currentContext)
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve docker endpoint")
	}

	dockerClient, err := newAPIClientFromEndpoint(dockerEndpoint, configFile)
	if tlsconfig.IsErrEncryptedKey(err) {
		return nil, errors.New("docker client with TLS passphrase is unsupported")
	} else if err != nil {
		return nil, fmt.Errorf("failed create docker client: %w", err)
	}

	return dockerClient, nil
}

func resolveContextName(opts *flags.CommonOptions, config *configfile.ConfigFile, contextStore store.Reader) (string, error) {
	if opts.Context != "" && len(opts.Hosts) > 0 {
		return "", errors.New("cannot specify both docker 'host' and 'context'")
	}
	if opts.Context != "" {
		return opts.Context, nil
	}
	if len(opts.Hosts) > 0 {
		return command.DefaultContextName, nil
	}
	if _, present := os.LookupEnv("DOCKER_HOST"); present {
		return command.DefaultContextName, nil
	}
	if ctxName, ok := os.LookupEnv("DOCKER_CONTEXT"); ok {
		return ctxName, nil
	}
	if config != nil && config.CurrentContext != "" {
		_, err := contextStore.GetMetadata(config.CurrentContext)
		if store.IsErrContextDoesNotExist(err) {
			return "", errors.Errorf("current docker context %q cannot be found on the file system, please check your config file at %s", config.CurrentContext, config.Filename)
		}
		return config.CurrentContext, err
	}
	return command.DefaultContextName, nil
}

func resolveDockerEndpoint(s store.Reader, contextName string) (docker.Endpoint, error) {
	ctxMeta, err := s.GetMetadata(contextName)
	if err != nil {
		return docker.Endpoint{}, err
	}
	epMeta, err := docker.EndpointFromContext(ctxMeta)
	if err != nil {
		return docker.Endpoint{}, err
	}
	return docker.WithTLSData(s, contextName, epMeta)
}

func newAPIClientFromEndpoint(ep docker.Endpoint, configFile *configfile.ConfigFile) (client.APIClient, error) {
	clientOpts, err := ep.ClientOpts()
	if err != nil {
		return nil, err
	}
	customHeaders := make(map[string]string, len(configFile.HTTPHeaders))
	for k, v := range configFile.HTTPHeaders {
		customHeaders[k] = v
	}
	customHeaders["User-Agent"] = command.UserAgent()
	clientOpts = append(clientOpts, client.WithHTTPHeaders(customHeaders))
	return client.NewClientWithOpts(clientOpts...)
}
