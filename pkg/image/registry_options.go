package image

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/anchore/stereoscope/internal/log"
)

// RegistryOptions for the OCI registry provider and containerd provider.
// If no specific Credential is found in the RegistryCredentials, will check
// for Keychain, and barring that will use Default Keychain (unless DisableDefaultKeychain is set).
type RegistryOptions struct {
	InsecureSkipTLSVerify bool
	InsecureUseHTTP       bool
	Credentials           []RegistryCredentials
	Keychain              authn.Keychain
	CAFileOrDir           string

	// DisableBlobResume turns off resuming a registry blob download from where it stopped when the
	// connection drops partway through, so the layer fails outright instead. Resuming is on by
	// default, and applies to the OCI registry provider only. It also turns off what comes with it:
	// a read that receives nothing for two minutes is treated as a drop, and the first resume of a
	// pull pings and authenticates against the registry again.
	//
	// The idle clock is the part worth knowing, since it is the one piece that changes a pull which
	// previously only ran slowly: a registry that pauses longer than that mid-blob is reopened rather
	// than waited on. One that then ignores the range costs a re-download of what had already
	// arrived, and one that refuses it outright (a 416, say) fails the layer where waiting would have
	// finished it. Two minutes is picked to sit well above a pull-through cache filling from upstream.
	//
	// Note that this is a Go API only: syft, grype and anchorectl would each need to plumb a config
	// key through before an operator meeting such a registry could reach it.
	DisableBlobResume bool

	// DisableDefaultKeychain stops the OCI registry provider from falling back to
	// authn.DefaultKeychain when neither Credentials nor Keychain yields an authenticator for the
	// registry, and has it connect anonymously instead. The default keychain reads the docker and
	// podman config files of the current user (found through $HOME, $DOCKER_CONFIG, $XDG_CONFIG_HOME,
	// $XDG_RUNTIME_DIR and $REGISTRY_AUTH_FILE) and runs any credential helper they name, which a
	// caller that must only ever present the credentials it was handed cannot allow.
	DisableDefaultKeychain bool

	// RetryAttempts is how many times in total the OCI registry provider tries a registry request
	// that fails in a way go-containerregistry considers transient (a dropped connection, or a
	// status such as 429 or 503). 1 means a request is never retried. Zero or less keeps
	// go-containerregistry's default of 3.
	//
	// This covers a request up to the point its response headers arrive. A blob that drops
	// mid-stream is resumed on a budget of its own, which DisableBlobResume turns off.
	RetryAttempts int

	// RetryBackoff is the wait before the first retry of a registry request. Each later wait is
	// three times the one before it. Zero or less keeps go-containerregistry's default of one second.
	//
	// The waits are not capped, so they grow quickly with RetryAttempts: at the default backoff, 5
	// attempts wait 40 seconds in total and 10 attempts wait over two and a half hours. Raise
	// RetryAttempts together with a shorter RetryBackoff, and bound the whole call with the context.
	RetryBackoff time.Duration

	// DialTimeout bounds establishing a connection to the registry. It is applied on top of the
	// dialer inherited from http.DefaultTransport, which has a timeout of its own (30 seconds unless
	// the process changed it), so it can shorten that but not extend it. Zero or less leaves the
	// inherited behavior alone.
	DialTimeout time.Duration

	// TLSHandshakeTimeout bounds the TLS handshake with the registry. Zero or less keeps the value
	// inherited from http.DefaultTransport (10 seconds unless the process changed it).
	TLSHandshakeTimeout time.Duration

	// ResponseHeaderTimeout bounds the wait for a registry to start answering once a request has been
	// sent. It does not limit how long a response body takes to read, so it is safe for large blobs.
	// Zero or less keeps the value inherited from http.DefaultTransport, which by default is no limit,
	// leaving only the context to end a request the registry never answers.
	//
	// Running into it counts as a transient failure, so the request is retried as RetryAttempts
	// allows. A resumed blob request is held to the shorter of this and the two minutes the resume
	// allows for a response.
	ResponseHeaderTimeout time.Duration
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

// TLSConfig selects the tls.Config object for handling TLS authentication with a registry.
func (r RegistryOptions) TLSConfig(registry string) (*tls.Config, error) {
	if r.InsecureSkipTLSVerify {
		log.Debugf("TLS verification is disabled for registry %q", registry)
	}

	tlsOptions := r.tlsOptions(registry)

	if tlsOptions == nil {
		tlsOptions = &tlsconfig.Options{
			InsecureSkipVerify: r.InsecureSkipTLSVerify,
		}
	}

	// note: tlsOptions allows for CAFile, however, this doesn't allow us to provide possibly multiple CA certs
	// to the underlying root pool. In order to support this we need to do the work to load the certs ourselves.
	tlsConfig, err := tlsconfig.Client(*tlsOptions)
	if err != nil {
		return nil, fmt.Errorf("unable to configure TLS client config: %w", err)
	}

	if !r.InsecureSkipTLSVerify && r.CAFileOrDir != "" {
		fi, err := os.Stat(r.CAFileOrDir)
		if err != nil {
			return nil, fmt.Errorf("unable to stat %q: %w", r.CAFileOrDir, err)
		}
		// load all the files in the directory as CA certs
		rootCAs := tlsConfig.RootCAs
		if rootCAs == nil {
			rootCAs, err = x509.SystemCertPool()
			if err != nil {
				log.Warnf("unable to load system cert pool: %w", err)
				rootCAs = x509.NewCertPool()
			}
		}

		var files []string
		if fi.IsDir() {
			// glob all *.crt, *.pem, and *.cert files within the dir itself. io/fs patterns are
			// slash-separated and relative, so a host path (absolute, or with windows separators)
			// never matches against os.DirFS(".")
			matches, err := doublestar.Glob(os.DirFS(r.CAFileOrDir), "*.{crt,pem,cert}")
			if err != nil {
				return nil, fmt.Errorf("unable to find certs in %q: %w", r.CAFileOrDir, err)
			}
			for _, m := range matches {
				files = append(files, filepath.Join(r.CAFileOrDir, m))
			}
		} else {
			files = []string{r.CAFileOrDir}
		}

		for _, certFile := range files {
			log.Tracef("loading CA certificate from %q", certFile)
			pem, err := os.ReadFile(certFile)
			if err != nil {
				return nil, fmt.Errorf("could not read CA certificate %q: %w", certFile, err)
			}
			if !rootCAs.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("failed to append certificates from PEM file: %q", certFile)
			}
		}

		tlsConfig.RootCAs = rootCAs
	}

	return tlsConfig, nil
}

// tlsOptions selects the tlsconfig.Options object for handling TLS authentication with a registry. Note: this will
// not consider the CAFileOrDir option, as that is handled by TLSConfig.
func (r RegistryOptions) tlsOptions(registry string) *tlsconfig.Options {
	var options *tlsconfig.Options
	for _, selection := range r.selectMostSpecificCredentials(registry) {
		c := selection.credentials
		if c.ClientCert != "" || c.ClientKey != "" {
			options = &tlsconfig.Options{
				InsecureSkipVerify: r.InsecureSkipTLSVerify,
				CertFile:           c.ClientCert,
				KeyFile:            c.ClientKey,
			}
		}

		if options != nil {
			log.Tracef("using custom TLS credentials from config index %d", selection.index+1)
			break
		}
	}

	if r.InsecureSkipTLSVerify && options == nil {
		options = &tlsconfig.Options{
			InsecureSkipVerify: r.InsecureSkipTLSVerify,
		}
	}

	return options
}
