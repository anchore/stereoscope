FROM centos:7

RUN curl -sLO https://corretto.aws/downloads/latest/amazon-corretto-11-x64-linux-jdk.rpm
RUN rpm -i amazon-corretto-11-x64-linux-jdk.rpm

# Regression: https://github.com/anchore/syft/issues/264
