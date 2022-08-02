FROM ubuntu:20.04
RUN  apt-get update && apt-get install curl -y && \
     curl -sLO https://github.com/sylabs/singularity/releases/download/v3.10.0/singularity-ce_3.10.0-focal_amd64.deb && \
     apt-get install -f ./singularity-ce_3.10.0-focal_amd64.deb -y
