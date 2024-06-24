FROM --platform=linux/amd64 ubuntu:20.04
# force platform since https://github.com/sylabs/singularity/releases
# has no arm64 releases.
RUN  apt-get update && apt-get install curl -y && \
     curl -sLO https://github.com/sylabs/singularity/releases/download/v4.1.3/singularity-ce_4.1.3-focal_amd64.deb && \
     apt-get install -f ./singularity-ce_4.1.3-focal_amd64.deb -y
