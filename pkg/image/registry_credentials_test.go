package image

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/google/go-containerregistry/pkg/authn"
)

func TestRegistryCredentials_Authenticator(t *testing.T) {
	const exampleUsername = "some-example-username"
	const examplePassword = "some-example-password"
	const exampleToken = "some-example-token"

	tests := []struct {
		name                   string
		credentials            RegistryCredentials
		authenticatorAssertion func(t *testing.T, actual authn.Authenticator)
	}{
		{
			name: "basic auth",
			credentials: RegistryCredentials{
				Username: exampleUsername,
				Password: examplePassword,
			},
			authenticatorAssertion: basicAuth(authn.Basic{
				Username: exampleUsername,
				Password: examplePassword,
			}),
		},
		{
			name: "basic auth without username",
			credentials: RegistryCredentials{
				Username: "",
				Password: examplePassword,
			},
			authenticatorAssertion: nilAuthenticator(),
		},
		{
			name: "basic auth without password",
			credentials: RegistryCredentials{
				Username: exampleUsername,
				Password: "",
			},
			authenticatorAssertion: nilAuthenticator(),
		},
		{
			name: "bearer token",
			credentials: RegistryCredentials{
				Token: exampleToken,
			},
			authenticatorAssertion: bearerToken(authn.Bearer{
				Token: exampleToken,
			}),
		},
		{
			name: "basic auth preferred over bearer token",
			credentials: RegistryCredentials{
				Username: exampleUsername,
				Password: examplePassword,
				Token:    exampleToken,
			},
			authenticatorAssertion: basicAuth(authn.Basic{
				Username: exampleUsername,
				Password: examplePassword,
			}),
		},
		{
			name:                   "no values provided",
			credentials:            RegistryCredentials{},
			authenticatorAssertion: nilAuthenticator(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := test.credentials.authenticator()
			test.authenticatorAssertion(t, actual)
		})
	}
}

func basicAuth(expected authn.Basic) func(*testing.T, authn.Authenticator) {
	return func(t *testing.T, actual authn.Authenticator) {
		assertBasicAuth(t, expected, actual)
	}
}

func bearerToken(expected authn.Bearer) func(*testing.T, authn.Authenticator) {
	return func(t *testing.T, actual authn.Authenticator) {
		assertBearerToken(t, expected, actual)
	}
}

func nilAuthenticator() func(*testing.T, authn.Authenticator) {
	return func(t *testing.T, actual authn.Authenticator) {
		assert.Nil(t, actual)
	}
}

func assertBasicAuth(t *testing.T, expected authn.Basic, actual authn.Authenticator) {
	actualBasic, ok := actual.(*authn.Basic)
	if !ok {
		t.Fatalf("unable to get basicAuth object: %+v", actual)
	}

	assert.Equal(t, expected, *actualBasic)
}

func assertBearerToken(t *testing.T, expected authn.Bearer, actual authn.Authenticator) {
	actualBearer, ok := actual.(*authn.Bearer)
	if !ok {
		t.Fatalf("unable to get bearer object: %+v", actual)
	}

	assert.Equal(t, expected, *actualBearer)
}
