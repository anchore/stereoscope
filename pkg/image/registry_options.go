package image

import (
	"sort"

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

type credentialSelection struct {
	credentials RegistryCredentials
	index       int
}

func (r RegistryOptions) selectMostSpecificCredentials(registry string) []credentialSelection {
	var selection []credentialSelection
	for idx, credentials := range r.Credentials {
		if !credentials.canBeUsedWithRegistry(registry) {
			continue
		}

		selection = append(selection, credentialSelection{
			credentials: credentials,
			index:       idx,
		})
	}

	sort.Slice(selection, func(i, j int) bool {
		iHasAuthority := selection[i].credentials.hasAuthoritySpecified()
		jHasAuthority := selection[j].credentials.hasAuthoritySpecified()
		if iHasAuthority && jHasAuthority {
			return selection[i].index < selection[j].index
		}
		if iHasAuthority && !jHasAuthority {
			return true
		}

		if jHasAuthority && !iHasAuthority {
			return false
		}

		return false
	})

	return selection
}

// Authenticator selects the credentials used to authenticate with a registry. Returns an authn.Authenticator
// object capable for handling high level credentials for the registry.
func (r RegistryOptions) Authenticator(registry string) authn.Authenticator {
	var authenticator authn.Authenticator
	for _, selection := range r.selectMostSpecificCredentials(registry) {
		authenticator = selection.credentials.authenticator()

		if authenticator != nil {
			log.Tracef("using registry credentials from config index %d", selection.index+1)
			break
		}
	}

	return authenticator
}

// TLSOptions selects the tlsconfig.Options object for handling TLS authentication with a registry.
func (r RegistryOptions) TLSOptions(registry string, insecureSkipTLSVerify bool) *tlsconfig.Options {
	var options *tlsconfig.Options
	for _, selection := range r.selectMostSpecificCredentials(registry) {
		c := selection.credentials
		if c.ClientCert != "" || c.ClientKey != "" {
			options = &tlsconfig.Options{
				InsecureSkipVerify: insecureSkipTLSVerify,
				CertFile:           c.ClientCert,
				KeyFile:            c.ClientKey,
			}
		}

		if options != nil {
			log.Tracef("using custom TLS credentials from config index %d", selection.index+1)
			break
		}
	}

	if insecureSkipTLSVerify && options == nil {
		options = &tlsconfig.Options{
			InsecureSkipVerify: insecureSkipTLSVerify,
		}
	}

	return options
}
