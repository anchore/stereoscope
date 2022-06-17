package credhelpers

import (
	"errors"
	"testing"

	"github.com/GoogleCloudPlatform/docker-credential-gcr/config"
	"github.com/GoogleCloudPlatform/docker-credential-gcr/store"
	"github.com/stretchr/testify/assert"
)

func Test_NewGCRHelper_Fails_UnableToLoadConfig(t *testing.T) {
	//GIVEN
	loadConfig = func() (config.UserConfig, error) {
		return nil, errors.New("failed to find file")
	}

	//WHEN
	helper, err := NewGCRHelper("https://gcr.io/google-containers")

	//THEN
	assert.Nil(t, helper)
	assert.Error(t, err)
}

func Test_NewGCRHelper_Fails_UnableToLoadStore(t *testing.T) {
	//GIVEN
	loadConfig = func() (config.UserConfig, error) {
		return nil, nil
	}
	getDefaultGCRCredStore = func() (store.GCRCredStore, error) {
		return nil, errors.New("failed to load store")
	}

	//WHEN
	helper, err := NewGCRHelper("https://gcr.io/google-containers")

	//THEN
	assert.Nil(t, helper)
	assert.Error(t, err)
}

func Test_NewGCRHelper(t *testing.T) {
	//GIVEN
	loadConfig = func() (config.UserConfig, error) {
		return nil, nil
	}
	getDefaultGCRCredStore = func() (store.GCRCredStore, error) {
		return nil, nil
	}

	//WHEN
	helper, err := NewGCRHelper("https://gcr.io/google-containers")

	//THEN
	assert.NotNil(t, helper)
	assert.NoError(t, err)
}

func Test_GetRegistryCredentials(t *testing.T) {
	//GIVEN
	username, token, authority := "username", "token", "https://gcr.io/google-containers"
	mInternalHelper := new(mockInternalHelper)
	mInternalHelper.On("Get").Return(username, token, nil)
	helper := GCRHelper{
		helper:    mInternalHelper,
		authority: authority,
	}

	//WHEN
	creds, err := helper.GetRegistryCredentials()

	//THEN
	assert.NoError(t, err)
	assert.Equal(t, username, creds.Username)
	assert.Equal(t, token, creds.Token)
	assert.Equal(t, authority, creds.Authority)
}

func Test_GetRegistryCredentials_Fails(t *testing.T) {
	//GIVEN
	authority := "https://gcr.io/google-containers"
	mInternalHelper := new(mockInternalHelper)
	mInternalHelper.On("Get").Return("", "", errors.New("fails"))
	helper := GCRHelper{
		helper:    mInternalHelper,
		authority: authority,
	}

	//WHEN
	creds, err := helper.GetRegistryCredentials()

	//THEN
	assert.Error(t, err)
	assert.Nil(t, creds)
}
