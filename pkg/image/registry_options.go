package image

import (
	"github.com/docker/go-connections/tlsconfig"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/anchore/stereoscope/internal/log"
)

// RegistryOptions for the OCI registry provider.
// If no specific Credential is found in the RegistryCredentials, will check
// for Keychain, and barring that will use Default Keychain.
type RegistryOptions struct {
	InsecureSkipTLSVerify bool
	InsecureUseHTTP       bool
	Credentials           []RegistryCredentials
	Keychain              authn.Keychain
	Platform              string
	CAFileOrDir           string
}

// Authenticator selects the credentials used to authenticate with a registry. Returns an authn.Authenticator
// object capable for handling high level credentials for the registry.
func (r RegistryOptions) Authenticator(registry string) authn.Authenticator {
	var authenticator authn.Authenticator
	for idx, credentials := range r.Credentials {
		if !credentials.canBeUsedWithRegistry(registry) {
			continue
		}

		authenticator = credentials.authenticator()

		if authenticator != nil {
			log.Tracef("using registry credentials from config index %d", idx)
			break
		}
	}

	return authenticator
}

// TLSOptions selects the tlsconfig.Options object for handling TLS authentication with a registry.
func (r RegistryOptions) TLSOptions(registry string, insecureSkipTLSVerify bool) *tlsconfig.Options {
	var options *tlsconfig.Options
	for idx, credentials := range r.Credentials {
		if !credentials.canBeUsedWithRegistry(registry) {
			continue
		}

		options = credentials.tlsOptions(insecureSkipTLSVerify)

		if options != nil {
			log.Tracef("using custom TLS options from config index %d", idx)
			break
		}
	}

	return options
}
