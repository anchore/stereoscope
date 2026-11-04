# image-windows-nanoserver-sac2016

A real Windows image, pulled rather than built: `reference` pins it by digest, and
`TestImageHardLinks_WindowsLinkIntoLowerLayer` reads it straight from the registry for platform
`windows/amd64`. Its two layers are foreign layers served by Microsoft, which the registry source
fetches like any other layer. The test runs in the Linux integration job and on a Windows host.

Its update layer holds 3,816 hardlink headers that name a file only the base layer carries (WinSxS
names for System32 files). Docker BuildKit, buildah and kaniko builds on Linux don't emit that
shape (they copy the file up instead); Windows layer import (hcsshim's `AddLink`) resolves such a
link against the parent layers. It is the shape behind the syft
regression where these names lost their contents, MIME types and digests.
