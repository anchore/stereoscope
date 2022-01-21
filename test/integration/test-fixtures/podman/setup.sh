#!/usr/bin/env bash

/usr/bin/echo "setting up ssh material..."

test -f /root/.ssh/id_rsa || (/usr/bin/echo -e 'y\n' | /usr/bin/ssh-keygen -t rsa -f /root/.ssh/id_rsa -N '')
test -f /root/.ssh/id_rsa.pub || (/usr/bin/echo -e 'y\n' | ssh-keygen -y -t rsa -f ~/.ssh/id_rsa > ~/.ssh/id_rsa.pub)
test -f /root/.ssh/authorized_keys || (/usr/bin/echo -e 'y\n' | /usr/bin/cp /root/.ssh/id_rsa.pub /root/.ssh/authorized_keys)

chown -R root:root /root/.ssh

/usr/bin/echo "ssh material setup!"
