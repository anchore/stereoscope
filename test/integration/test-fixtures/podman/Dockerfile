FROM quay.io/podman/stable

EXPOSE 22

RUN yum -y install openssh openssh-server openssh-clients && \
    yum -y clean all

RUN systemctl enable sshd.service podman.socket

CMD [ "/sbin/init" ]