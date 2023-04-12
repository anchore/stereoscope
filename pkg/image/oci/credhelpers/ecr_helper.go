package credhelpers

import (
	ecr "github.com/awslabs/amazon-ecr-credential-helper/ecr-login"

	"github.com/anchore/stereoscope/pkg/image"
)

type ECRHelper struct {
	authority string
	helper    internalHelper
}

func NewECRHelper(authority string) ECRHelper {
	return ECRHelper{
		authority: authority,
		helper:    ecr.NewECRHelper(),
	}
}

func (e *ECRHelper) GetECRCredentials() (*image.RegistryCredentials, error) {
	username, password, err := e.helper.Get(e.authority)
	if err != nil {
		return nil, err
	}
	return &image.RegistryCredentials{
		Authority: e.authority,
		Username:  username,
		Password:  password,
	}, nil
}
