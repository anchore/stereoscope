#!/usr/bin/env bash

/usr/bin/echo "setting up ssh material..."

test -f /root/.ssh/id_ed25519 || (/usr/bin/echo -e 'y\n' | /usr/bin/ssh-keygen -t ed25519 -f /root/.ssh/id_ed25519 -N '')
test -f /root/.ssh/id_ed25519.pub || (/usr/bin/echo -e 'y\n' | ssh-keygen -y -t ed25519 -f ~/.ssh/id_ed25519 > ~/.ssh/id_ed25519.pub)
test -f /root/.ssh/authorized_keys || (/usr/bin/echo -e 'y\n' | /usr/bin/cp /root/.ssh/id_ed25519.pub /root/.ssh/authorized_keys)

chown -R root:root /root/.ssh
chmod 777 /root/.ssh/id_ed25519
chmod 777 /root/.ssh/id_ed25519.pub

/usr/bin/echo "ssh material setup!"
