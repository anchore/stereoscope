FROM ubuntu:jammy@sha256:b492494d8e0113c4ad3fe4528a4b5ff89faa5331f7d52c5c138196f69ce176a6

# install make
RUN apt-get update && \
    apt-get install -y \
        make \
        curl

# install crane
RUN curl -o /tmp/ggcr.tar.gz -LsS https://github.com/google/go-containerregistry/releases/download/v0.16.1/go-containerregistry_Linux_x86_64.tar.gz && \
    tar -C /usr/local/bin -xzf /tmp/ggcr.tar.gz && \
    rm /tmp/ggcr.tar.gz
