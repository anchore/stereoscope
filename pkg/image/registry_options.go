package image

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
