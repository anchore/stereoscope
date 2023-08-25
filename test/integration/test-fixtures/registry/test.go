package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anchore/stereoscope"
	"github.com/anchore/stereoscope/pkg/image"
)

func main() {
	img, err := stereoscope.GetImage(
		context.Background(),
		"registry:registry.null:5000/busybox:latest",
		stereoscope.WithRegistryOptions(image.RegistryOptions{
			InsecureSkipTLSVerify: false,
			InsecureUseHTTP:       false,
			Credentials: []image.RegistryCredentials{
				{
					Authority:  "registry.null:5000",
					Username:   "testuser",
					Password:   "testpass",
					CAFile:     "/certs/server.crt",
					ClientCert: "/certs/client.crt",
					ClientKey:  "/certs/client.key",
				},
			},
		}),
	)
	if err != nil {
		panic("could not get image: " + err.Error())
	}
	if img == nil {
		panic("image is nil")
	}

	if len(img.Layers) == 0 {
		panic("image has no layers")
	}

	b, err := json.MarshalIndent(img.Metadata, "", "\t")
	if err != nil {
		panic(fmt.Sprintf("could not marshal image metadata: %+v", err))
	}
	if _, err := os.Stdout.Write(b); err != nil {
		panic(fmt.Sprintf("could not write image metadata: %+v", err))
	}
}
