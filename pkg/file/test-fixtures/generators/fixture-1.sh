#!/usr/bin/env bash
set -uex

FIXTURE_TAR_PATH=$1

TEMP_DIR=$(mktemp -d -t stereoscope-fixture-XXXXXXXXXX)
trap 'rm -rf $TEMP_DIR' EXIT

pushd "$TEMP_DIR"

  # content
  mkdir -p path/branch/one
  mkdir -p path/branch/two
  echo "first file" > path/branch/one/file-1.txt
  echo "second file" > path/branch/two/file-2.txt
  echo "third file" > path/file-3.txt

  # permissions
  chmod -R 700 path/branch/one/
  chmod -R 755 path/branch/two/

  # tar + owner
  tar --owner=1337 --group=5432 -cvf "$FIXTURE_TAR_PATH" path/

popd