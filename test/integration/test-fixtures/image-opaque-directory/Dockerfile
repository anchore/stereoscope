FROM --platform=linux/amd64 rockylinux:9.2.20230513-minimal@sha256:6ff3d41b1fea114dfe6f3b8cf0517a0806f9410404df7e931c32b65f7e76d1d8

RUN curl -sLO https://corretto.aws/downloads/latest/amazon-corretto-11-x64-linux-jdk.rpm
RUN rpm -i amazon-corretto-11-x64-linux-jdk.rpm

# Regression: https://github.com/anchore/syft/issues/264
