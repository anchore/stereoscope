package image

import (
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRegistryOptions_Authenticator_Basic(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		input    *RegistryOptions
		expected *authn.Basic
	}{
		{
			name:     "require user pass success",
			registry: "localhost:5000",
			input: &RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
						Username:  "username",
						Password:  "tOpsYKrets",
					},
				},
			},
			expected: &authn.Basic{
				Username: "username",
				Password: "tOpsYKrets",
			},
		},
		{
			name:     "mismatched registry",
			registry: "localhost:5000",
			input: &RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost",
						Username:  "username",
						Password:  "tOpsYKrets",
					},
				},
			},
			expected: nil,
		},
		{
			name:     "no password",
			registry: "localhost:5000",
			input: &RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
						Username:  "username",
					},
				},
			},
			expected: nil,
		},
		{
			name:     "no username",
			registry: "localhost:5000",
			input: &RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
						Password:  "awesome-sauce",
					},
				},
			},
			expected: nil,
		},
		{
			name:     "no username or password",
			registry: "localhost:5000",
			input: &RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
					},
				},
			},
			expected: nil,
		},
		{
			name:     "empty struct",
			registry: "localhost:5000",
			input: &RegistryOptions{
				Credentials: []RegistryCredentials{
					{},
				},
			},
			expected: nil,
		},
		{
			name:     "multiple matches",
			registry: "localhost:5000",
			input: &RegistryOptions{
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
			expected: &authn.Basic{
				Username: "username",
				Password: "tOpsYKrets",
			},
		},
		{
			name:     "basic auth over bearer",
			registry: "localhost:5000",
			input: &RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
						Username:  "username",
						Password:  "tOpsYKrets",
					},
					{
						Authority: "localhost:5000",
						Token:     "BLERG",
					},
				},
			},
			expected: &authn.Basic{
				Username: "username",
				Password: "tOpsYKrets",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualAuth := test.input.Authenticator(test.registry)
			if test.expected == nil && actualAuth == nil {
				// pass
				return
			}
			actual, ok := actualAuth.(*authn.Basic)
			if !ok {
				t.Fatalf("unable to get basic auth obj: %+v", actualAuth)
			}
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestRegistryOptions_Authenticator_Bearer(t *testing.T) {
	tests := []struct {
		name     string
		registry string
		input    *RegistryOptions
		expected *authn.Bearer
	}{
		{
			name:     "require bearer token success",
			registry: "localhost:5000",
			input: &RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
						Token:     "JRR",
					},
				},
			},
			expected: &authn.Bearer{
				Token: "JRR",
			},
		},
		{
			name:     "missing bearer token",
			registry: "localhost:5000",
			input: &RegistryOptions{
				Credentials: []RegistryCredentials{
					{
						Authority: "localhost:5000",
					},
				},
			},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualAuth := test.input.Authenticator(test.registry)
			if test.expected == nil && actualAuth == nil {
				// pass
				return
			}
			actual, ok := actualAuth.(*authn.Bearer)
			if !ok {
				t.Fatalf("unable to get bearer obj: %+v", actualAuth)
			}
			assert.Equal(t, test.expected, actual)
		})
	}
}
