# stereoscope

A library for working with container image contents, layer file trees, and squashed file trees.

## Getting Started

See `examples/basic.go`

```bash
docker image save centos:8 -o centos.tar
go run examples/basic.go ./centos.tar
```

Note: To run tests you will need `skopeo` installed.

## Overview

This library provides the means to:
- parse and read images from multiple sources (currently docker V2 schema images read from the docker daemon and from an archive on disk)
- build a file tree representing each layer blob
- create a squashed file tree representation for each layer
- search one or more file trees for selected paths
- catalog file metadata in all layers
- query the underlying image tar for content (file content within a layer)
