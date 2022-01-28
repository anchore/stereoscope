FROM quay.io/podman/stable

EXPOSE 22

RUN yum -y install openssh openssh-server openssh-clients && \
    yum -y clean all

ADD setup.sh /setup.sh
ADD setup.service /etc/systemd/system/setup.service
RUN systemctl enable sshd.service podman.socket setup.service

CMD [ "/sbin/init" ]