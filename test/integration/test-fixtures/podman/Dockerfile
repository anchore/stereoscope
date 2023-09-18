FROM quay.io/podman/stable:v1.6.2@sha256:76d2c90d9af6a6e3d4649ac8f4f94222b540c1787d929855dcdb7a0f7d3f45b2

EXPOSE 22

RUN yum -y install openssh openssh-server openssh-clients && \
    yum -y clean all

ADD setup.sh /setup.sh
ADD setup.service /etc/systemd/system/setup.service
RUN systemctl enable sshd.service podman.socket setup.service

CMD [ "/sbin/init" ]
