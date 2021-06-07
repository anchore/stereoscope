#!/usr/bin/env bash
set -eux -o pipefail

# use this script to capture only the beginning of files for use a MIME type detection testing

input=$1
name=$(basename $input)

# all you need to mimetype detection usually is within the first sector of reading
head -c 512 $input > $name