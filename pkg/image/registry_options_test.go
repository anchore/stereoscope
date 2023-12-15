package image

import (
	"crypto/x509"
	"os"
	"testing"

	"github.com/docker/go-connections/tlsconfig"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryOptions_AuthenticationOptions(t *testing.T) {
	tests := []struct {
		name                   string
		registry               string
		input                  RegistryOptions
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
			name:     "docker.io can be matched as an authority",
			registry: "docker.io",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "docker.io",
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
			name:     "registry-1.docker.io can be matched as an authority alias for docker.io",
			registry: "registry-1.docker.io",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "docker.io",
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
			name:     "index.docker.io can be matched as an authority alias for docker.io",
			registry: "index.docker.io",
			input: RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "index.docker.io",
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
			name:     "mtls setup",
			registry: "localhost:5000",
			input: RegistryOptions{
				InsecureSkipTLSVerify: true,
				CAFileOrDir:           "ca.crt",
				Credentials: []RegistryCredentials{
					{
						Authority:  "localhost:5000",
						Username:   "username",
						Password:   "tOpsYKrets",
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
				CAFile:             "", // note: we load this into the pool ourselves from in the tlsConfig
				CertFile:           "client.crt",
				KeyFile:            "client.key",
				InsecureSkipVerify: true,
			},
		},
		{
			name:     "always attempt mtls, but use basic auth with specific authority",
			registry: "localhost:5000",
			input: RegistryOptions{
				InsecureSkipTLSVerify: true,
				CAFileOrDir:           "ca.crt",
				Credentials: []RegistryCredentials{
					{
						Authority:  "",
						ClientCert: "client.crt",
						ClientKey:  "client.key",
					},
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
			wantTlsOptions: &tlsconfig.Options{
				CAFile:             "", // note: we load this into the pool ourselves from in the tlsConfig
				CertFile:           "client.crt",
				KeyFile:            "client.key",
				InsecureSkipVerify: true,
			},
		},
		{
			name:     "use tls options from most specific authority",
			registry: "localhost:5000",
			input: RegistryOptions{
				InsecureSkipTLSVerify: true,
				CAFileOrDir:           "ca.crt",
				Credentials: []RegistryCredentials{
					{
						Authority:  "",
						ClientCert: "bad-client.crt", // should be overridden
						ClientKey:  "bad-client.key", // should be overridden
					},
					{
						Authority:  "localhost:5000",
						Username:   "username",
						Password:   "tOpsYKrets",
						ClientCert: "client.crt",
						ClientKey:  "client.key",
					},
					// duplicate is ignored (match the best first candidate)
					{
						Authority:  "localhost:5000",
						Username:   "dup-username",
						Password:   "dup-tOpsYKrets",
						ClientCert: "dup-client.crt",
						ClientKey:  "dup-client.key",
					},
				},
			},
			authenticatorAssertion: basicAuth(authn.Basic{
				Username: "username",
				Password: "tOpsYKrets",
			}),
			wantTlsOptions: &tlsconfig.Options{
				CAFile:             "", // note: we load this into the pool ourselves from in the tlsConfig
				CertFile:           "client.crt",
				KeyFile:            "client.key",
				InsecureSkipVerify: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualAuth := test.input.Authenticator(test.registry)
			tlsOptions := test.input.tlsOptions(test.registry)
			assert.Equal(t, test.wantTlsOptions, tlsOptions)
			test.authenticatorAssertion(t, actualAuth)
		})
	}
}

func TestRegistryOptions_selectMostSpecificCredentials(t *testing.T) {
	unspecifiedMTLS1 := RegistryCredentials{
		Authority:  "",
		ClientCert: "all-client1.crt",
		ClientKey:  "all-client1.key",
	}
	unspecifiedMTLS2 := RegistryCredentials{
		Authority:  "",
		ClientCert: "all-client2.crt",
		ClientKey:  "all-client2.key",
	}

	otherBasicAuth1 := RegistryCredentials{
		Authority: "other:5000",
		Username:  "user1",
		Password:  "pass1",
	}

	localhostBasicAuth1 := RegistryCredentials{
		Authority: "localhost:5000",
		Username:  "user1",
		Password:  "pass1",
	}

	localhostBasicAuth2 := RegistryCredentials{
		Authority: "localhost:5000",
		Username:  "user2",
		Password:  "pass2",
	}

	dockerBasicAuth := RegistryCredentials{
		Authority: "docker.io",
		Username:  "user1",
		Password:  "pass1",
	}

	tests := []struct {
		name        string
		credentials []RegistryCredentials
		registry    string
		want        []credentialSelection
	}{
		{
			name: "no credentials",
			want: nil,
		},
		{
			name:     "one matching credential",
			registry: "localhost:5000",
			credentials: []RegistryCredentials{
				localhostBasicAuth1,
			},
			want: []credentialSelection{
				{
					credentials: localhostBasicAuth1,
					index:       0,
				},
			},
		},
		{
			name:     "two matching credentials",
			registry: "localhost:5000",
			credentials: []RegistryCredentials{
				localhostBasicAuth1,
				localhostBasicAuth2,
			},
			want: []credentialSelection{
				{
					credentials: localhostBasicAuth1,
					index:       0,
				},
				{
					credentials: localhostBasicAuth2,
					index:       1,
				},
			},
		},
		{
			name:     "two matching credentials -- order preserved",
			registry: "localhost:5000",
			credentials: []RegistryCredentials{
				localhostBasicAuth2,
				localhostBasicAuth1,
			},
			want: []credentialSelection{
				{
					credentials: localhostBasicAuth2,
					index:       0,
				},
				{
					credentials: localhostBasicAuth1,
					index:       1,
				},
			},
		},
		{
			name:     "no matching credentials",
			registry: "localhost:5000",
			credentials: []RegistryCredentials{
				otherBasicAuth1,
			},
			want: nil,
		},
		{
			name:     "no matching credentials, docker requested",
			registry: "docker.io",
			credentials: []RegistryCredentials{
				otherBasicAuth1,
			},
			want: nil,
		},
		{
			name:     "docker requested by one alias, specified by another",
			registry: "index.docker.io",
			credentials: []RegistryCredentials{
				dockerBasicAuth,
			},
			want: []credentialSelection{
				{
					credentials: dockerBasicAuth,
					index:       0,
				},
			},
		},
		{
			name:     "match unspecified authority and sort last (order preserved)",
			registry: "localhost:5000",
			credentials: []RegistryCredentials{
				unspecifiedMTLS1,
				unspecifiedMTLS2,
				localhostBasicAuth1,
				localhostBasicAuth2,
			},
			want: []credentialSelection{
				{
					credentials: localhostBasicAuth1,
					index:       2,
				},
				{
					credentials: localhostBasicAuth2,
					index:       3,
				},
				{
					credentials: unspecifiedMTLS1,
					index:       0,
				},
				{
					credentials: unspecifiedMTLS2,
					index:       1,
				},
			},
		},
		{
			name:     "match unspecified authority and sort last (stable order)",
			registry: "localhost:5000",
			credentials: []RegistryCredentials{
				unspecifiedMTLS2,
				unspecifiedMTLS1,
				localhostBasicAuth2,
				localhostBasicAuth1,
			},
			want: []credentialSelection{
				{
					credentials: localhostBasicAuth2,
					index:       2,
				},
				{
					credentials: localhostBasicAuth1,
					index:       3,
				},
				{
					credentials: unspecifiedMTLS2,
					index:       0,
				},
				{
					credentials: unspecifiedMTLS1,
					index:       1,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RegistryOptions{
				Credentials: tt.credentials,
			}
			assert.Equal(t, tt.want, r.selectMostSpecificCredentials(tt.registry))
		})
	}
}

func TestRegistryOptions_TLSConfig_rootCAs(t *testing.T) {
	certFile := "test-fixtures/certs/server.crt"
	systemCerts, err := tlsconfig.SystemCertPool()
	require.NoError(t, err)

	certPool := systemCerts.Clone()

	pem, err := os.ReadFile(certFile)
	require.NoError(t, err)
	certPool.AppendCertsFromPEM(pem)

	tests := []struct {
		name            string
		registryOptions RegistryOptions
		want            *x509.CertPool
		wantErr         assert.ErrorAssertionFunc
	}{
		{
			name: "add single root cert",
			registryOptions: RegistryOptions{
				CAFileOrDir: certFile,
			},
			want: certPool,
		},
		{
			name: "add root certs from dir",
			registryOptions: RegistryOptions{
				CAFileOrDir: "test-fixtures/certs",
			},
			want: certPool,
		},
		{
			name: "skip TLS verify does not load certs", // just like the stdlib
			registryOptions: RegistryOptions{
				InsecureSkipTLSVerify: true,
				CAFileOrDir:           certFile,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.registryOptions.TLSConfig("dont-care")
			require.NoError(t, err)
			assert.True(t, tt.want.Equal(got.RootCAs))
		})
	}
}
