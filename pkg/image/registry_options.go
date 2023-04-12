package image

import (
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
func (r RegistryOptions) Authenticator(registry string) authn.Authenticator {
	for idx, credentials := range r.Credentials {
		if !credentials.canBeUsedWithRegistry(registry) {
			continue
		}

		authenticator := credentials.authenticator()
		if authenticator == nil {
			continue
		}

		log.Debugf("using registry credentials from config index %d", idx)
		return authenticator
	}

	return nil
}
