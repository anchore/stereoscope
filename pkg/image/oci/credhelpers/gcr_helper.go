package credhelpers

import (
	"fmt"

	"github.com/GoogleCloudPlatform/docker-credential-gcr/config"
	"github.com/GoogleCloudPlatform/docker-credential-gcr/credhelper"
	"github.com/GoogleCloudPlatform/docker-credential-gcr/store"
	"github.com/anchore/stereoscope/pkg/image"
)

type GCRHelper struct {
	authority string
	helper    internalHelper
}

var loadConfig = config.LoadUserConfig
var getDefaultGCRCredStore = store.DefaultGCRCredStore

func NewGCRHelper(authority string) (*GCRHelper, error) {
	userConfig, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to load user config: %w", err)
	}
	credStore, err := getDefaultGCRCredStore()
	if err != nil {
		return nil, fmt.Errorf("unable to read load default cred store: %w", err)
	}
	helper := credhelper.NewGCRCredentialHelper(credStore, userConfig)

	return &GCRHelper{
		authority: authority,
		helper:    helper,
	}, nil
}

func (g *GCRHelper) GetRegistryCredentials() (*image.RegistryCredentials, error) {
	username, token, err := g.helper.Get(g.authority)
	if err != nil {
		return nil, fmt.Errorf("unable to load credentials: %w", err)
	}

	return &image.RegistryCredentials{
		Authority: g.authority,
		Username:  username,
		Token:     token,
	}, nil
}
