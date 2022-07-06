package credhelpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockInternalHelper struct {
	mock.Mock
}

func (m *mockInternalHelper) Get(_ string) (string, string, error) {
	args := m.Called()
	return args.Get(0).(string), args.Get(1).(string), args.Error(2)
}

func Test_GetECRCredentials(t *testing.T) {
	//GIVEN
	username, password, authority := "username", "password", "https://gallery.ecr.aws/"
	mInternalHelper := new(mockInternalHelper)
	mInternalHelper.On("Get").Return(username, password, nil)
	helper := ECRHelper{
		helper:    mInternalHelper,
		authority: authority,
	}

	//WHEN
	creds, err := helper.GetECRCredentials()

	//THEN
	assert.NoError(t, err)
	assert.Equal(t, username, creds.Username)
	assert.Equal(t, password, creds.Password)
	assert.Equal(t, authority, creds.Authority)
}

func Test_GetECRCredentials_Fails(t *testing.T) {
	//GIVEN
	helper := NewECRHelper("")

	//WHEN
	creds, err := helper.GetECRCredentials()

	//THEN
	assert.Nil(t, creds)
	assert.Error(t, err)
}
