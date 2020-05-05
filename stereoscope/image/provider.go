package image

type Provider interface {
	Provide() (*Image, error)
}
