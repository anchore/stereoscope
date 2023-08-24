FROM ubuntu:20.04

# install make
RUN apt-get update && \
    apt-get install -y \
        make \
        curl

# install crane
RUN curl -o /tmp/ggcr.tar.gz -LsS https://github.com/google/go-containerregistry/releases/download/v0.16.1/go-containerregistry_Linux_x86_64.tar.gz && \
    tar -C /usr/local/bin -xzf /tmp/ggcr.tar.gz && \
    rm /tmp/ggcr.tar.gz
