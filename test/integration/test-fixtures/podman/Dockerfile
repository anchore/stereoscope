FROM quay.io/podman/stable:v4.6.1@sha256:6a070625cff8ed7b3c0a77534e50c2efbaf333929f20094883911acb071b4cd8

EXPOSE 22

RUN yum -y install openssh openssh-server openssh-clients && \
    yum -y clean all

ADD setup.sh /setup.sh
ADD setup.service /etc/systemd/system/setup.service
RUN systemctl enable sshd.service podman.socket setup.service

CMD [ "/sbin/init" ]
