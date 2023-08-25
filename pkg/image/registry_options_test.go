package image

import (
	"testing"

	"github.com/docker/go-connections/tlsconfig"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/stretchr/testify/assert"
)

func TestRegistryOptions_Authenticator(t *testing.T) {
	tests := []struct {
		name                   string
		registry               string
		input                  RegistryOptions
		tlsInsecureSkipVerify  bool
		authenticatorAssertion func(t *testing.T, actual authn.Authenticator)
		wantTlsOptions         *tlsconfig.Options
	}{
		{
			name:     "basicAuth credentials match registry",
			registry: "localhost:5000",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
						Username:  "username",
						Password:  "tOpsYKrets",
					},
				},
			},
			authenticatorAssertion: basicAuth(authn.Basic{
				Username: "username",
				Password: "tOpsYKrets",
			}),
		},
		{
			name:     "basicAuth credentials don't match registry",
			registry: "localhost:5000",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost",
						Username:  "username",
						Password:  "tOpsYKrets",
					},
				},
			},
			authenticatorAssertion: nilAuthenticator(),
		},
		{
			name:     "authority with missing credentials",
			registry: "localhost:5000",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
					},
				},
			},
			authenticatorAssertion: nilAuthenticator(),
		},
		{
			name:     "empty struct",
			registry: "localhost:5000",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{},
				},
			},
			authenticatorAssertion: nilAuthenticator(),
		},
		{
			name:     "empty credentials slice",
			registry: "localhost:5000",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{},
			},
			authenticatorAssertion: nilAuthenticator(),
		},
		{
			name:     "given multiple matches, select first match",
			registry: "localhost:5000",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
						Username:  "username",
						Password:  "tOpsYKrets",
					},
					{
						Authority: "localhost:5000",
						Username:  "SOMETHING ELSE",
						Password:  "BLERG",
					},
				},
			},
			authenticatorAssertion: basicAuth(authn.Basic{
				Username: "username",
				Password: "tOpsYKrets",
			}),
		},
		{
			name:     "basic auth without authority",
			registry: "localhost:5000",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Username: "username",
						Password: "tOpsYKrets",
					},
				},
			},
			authenticatorAssertion: basicAuth(authn.Basic{
				Username: "username",
				Password: "tOpsYKrets",
			}),
		},
		{
			name:     "bearer token credentials match registry",
			registry: "localhost:5000",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
						Token:     "JRR",
					},
				},
			},
			authenticatorAssertion: bearerToken(authn.Bearer{
				Token: "JRR",
			}),
		},
		{
			name:     "bearer token credentials don't match registry",
			registry: "localhost:5000",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost",
						Token:     "JRR",
					},
				},
			},
			authenticatorAssertion: nilAuthenticator(),
		},
		{
			name:     "bearer token without authority",
			registry: "localhost:5000",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Token: "JRR",
					},
				},
			},
			authenticatorAssertion: bearerToken(authn.Bearer{
				Token: "JRR",
			}),
		},
		{
			name:                  "mtls setup",
			registry:              "localhost:5000",
			tlsInsecureSkipVerify: true,
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority:  "localhost:5000",
						Username:   "username",
						Password:   "tOpsYKrets",
						CAFile:     "ca.crt",
						ClientCert: "client.crt",
						ClientKey:  "client.key",
					},
				},
			},
			authenticatorAssertion: basicAuth(authn.Basic{
				Username: "username",
				Password: "tOpsYKrets",
			}),
			wantTlsOptions: &tlsconfig.Options{
				CAFile:             "ca.crt",
				CertFile:           "client.crt",
				KeyFile:            "client.key",
				InsecureSkipVerify: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualAuth, tlsOptions := test.input.Authenticator(test.registry, test.tlsInsecureSkipVerify)
			assert.Equal(t, test.wantTlsOptions, tlsOptions)
			test.authenticatorAssertion(t, actualAuth)
		})
	}
}
