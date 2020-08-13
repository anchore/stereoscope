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
  mkdir -p path/branch/one
  mkdir -p path/branch/two
  echo "first file" > path/branch/one/file-1.txt
  echo "second file" > path/branch/two/file-2.txt
  echo "third file" > path/file-3.txt

  # permissions
  chmod -R 755 path
  chmod -R 700 path/branch/one/
  chmod 664 path/file-3.txt

  # tar + owner
  # note: sort by name is important for test file header entry ordering
  tar --sort=name --owner=1337 --group=5432 -cvf "/scratch/${FIXTURE_NAME}" path/

popd
EOF
