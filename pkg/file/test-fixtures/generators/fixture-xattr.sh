#!/usr/bin/env bash
set -ue

realpath() {
    [[ $1 = /* ]] && echo "$1" || echo "$PWD/${1#./}"
}

FIXTURE_TAR_PATH=$1
FIXTURE_NAME=$(basename $FIXTURE_TAR_PATH)
FIXTURE_DIR=$(realpath $(dirname $FIXTURE_TAR_PATH))

# note: since tar --sort is not an option on mac, and we want these generation scripts to be generally portable, we've
# elected to use docker to generate the tar
docker run --rm -i \
    -u $(id -u):$(id -g) \
    -v ${FIXTURE_DIR}:/scratch \
    -w /scratch \
        ubuntu:latest \
            /bin/bash -xs <<EOF
mkdir /tmp/stereoscope
pushd /tmp/stereoscope

  # content
  echo "first file" > file-1.txt
  setfattr -n user.comment -v "very cool" file-1.txt
  setfattr -n com.anchore.version -v "3.0" file-1.txt

  # tar + owner
  # note: sort by name is important for test file header entry ordering
  tar --xattrs --format=pax --sort=name --owner=1337 --group=5432 -cvf "/scratch/${FIXTURE_NAME}" file-1.txt

popd
EOF
