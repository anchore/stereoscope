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
}

// Authenticator returns an object capable of authenticating against the given registry. If no credentials match the
// given registry, or there is partial information configured, then nil is returned.
func (r RegistryOptions) Authenticator(registry string, insecureSkipTLSVerify bool) (authn.Authenticator, *tlsconfig.Options) {
	for idx, credentials := range r.Credentials {
		if !credentials.canBeUsedWithRegistry(registry) {
			continue
		}

		authenticator := credentials.authenticator()
		if authenticator == nil {
			continue
		}

		log.Debugf("using registry credentials from config index %d", idx)

		var options *tlsconfig.Options
		if insecureSkipTLSVerify || credentials.CAFile != "" || credentials.ClientCert != "" || credentials.ClientKey != "" {
			options = &tlsconfig.Options{
				InsecureSkipVerify: insecureSkipTLSVerify,
				CAFile:             credentials.CAFile,
				CertFile:           credentials.ClientCert,
				KeyFile:            credentials.ClientKey,
			}
		}

		return authenticator, options
	}

	return nil, nil
}
