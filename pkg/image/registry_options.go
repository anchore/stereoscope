package image

import (
	"github.com/anchore/stereoscope/internal/log"
	"github.com/google/go-containerregistry/pkg/authn"
)

// RegistryOptions for the OCI registry provider.
type RegistryOptions struct {
	InsecureSkipTLSVerify bool
	Credentials           []RegistryCredentials
}

// RegistryCredentials contains any information necessary to authenticate against an OCI-distribution-compliant
// registry (either with basic auth or bearer token). Note: only valid for the OCI registry provider.
type RegistryCredentials struct {
	Authority string
	Username  string
	Password  string
	Token     string
}

// Authenticator returns an object capable of authenticating against the given registry. If no credentials match the
// given registry, or there is partial information configured, then nil is returned.
func (r *RegistryOptions) Authenticator(registry string) authn.Authenticator {
	for idx, auth := range r.Credentials {
		if auth.Authority == registry {
			if auth.Username != "" && auth.Password != "" {
				log.Debugf("using registry credentials for %q (config idx=%d)", auth.Authority, idx)
				return &authn.Basic{
					Username: auth.Username,
					Password: auth.Password,
				}
			} else if auth.Token != "" {
				log.Debugf("using registry token for %q (config idx=%d)", auth.Authority, idx)
				return &authn.Bearer{
					Token: auth.Token,
				}
			}
		}
	}

	return nil
}
