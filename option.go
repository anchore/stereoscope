package stereoscope

import (
	"fmt"

	"github.com/anchore/stereoscope/pkg/image"
)

type Option func(*config) error

type config struct {
	Registry           image.RegistryOptions
	AdditionalMetadata []image.AdditionalMetadata
	Platform           *image.Platform
}

func applyOptions(cfg *config, options ...Option) error {
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(cfg); err != nil {
			return fmt.Errorf("unable to parse option: %w", err)
		}
	}
	return nil
}
