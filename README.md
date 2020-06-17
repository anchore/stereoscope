# Stereoscope (TDB Name)

**This is a Prototype! Using this in production will end in tragedy**

A library for working with container image contents, layer file trees, and squashed file trees.

## Getting Started

See `examples/basic.go`

```bash
docker image save centos:8 -o centos.tar
go run examples/basic.go tarball://centos.tar
```

## Overview

This library provides the means to:
- parse and read images from multiple sources (currently docker V2 schema images read from the docker daemon and from an archive on disk)
- build a file tree representing each layer blob (note: not the layer contents)
- create a squashed file tree representation for each layer (note: note the squashed contents)
- search one or more file trees for selected paths
- catalog file metadata in all layers
- query the underlying image tar for content (file content within a layer)

## Names...

- Stereoscope: A stereoscope is a device that takes multiple flat, two-dimensional images to create an impression of a single three-dimensional image. This library takes a container image and provides the user with usable file tree manipulation and a content API (i.e. depth and detail, like a stereoscope).
- Cartographer: A map maker. This library essentially maps out an image into file trees from various perspectives.
