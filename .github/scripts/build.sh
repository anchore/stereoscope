#!/usr/bin/env bash
set -uo pipefail

# Based on https://gist.github.com/eduncan911/68775dba9d3c028181e4 and https://gist.github.com/makeworld-the-better-one/e1bb127979ae4195f43aaa3ad46b1097
# but improved to use the `go` command so it never goes out of date.

type setopt >/dev/null 2>&1

contains() {
    # Source: https://stackoverflow.com/a/8063398/7361270
    [[ $1 =~ (^|[[:space:]])$2($|[[:space:]]) ]]
}

mkdir -p snapshot
rm -f snapshot/*

OUTPUT=snapshot/stereoscope-example
FAILURES=""

# You can set your own flags on the command line
FLAGS=${FLAGS:-"-ldflags=\"-s -w\""}

# A list of OSes and architectures to not build for, space-separated
# It can be set from the command line when the script is called.
NOT_ALLOWED_OS=${NOT_ALLOWED_OS:-"js android ios solaris illumos aix dragonfly plan9"}
NOT_ALLOWED_ARCH=${NOT_ALLOWED_ARCH:-"riscv64 mips mips64 mips64le ppc64 ppc64le s390x wasm"}


# Get all targets
while IFS= read -r target; do
    GOOS=${target%/*}
    GOARCH=${target#*/}
    BIN_FILENAME="${OUTPUT}-${GOOS}-${GOARCH}"

    if contains "$NOT_ALLOWED_OS" "$GOOS" ; then
        continue
    fi

    if contains "$NOT_ALLOWED_ARCH" "$GOARCH" ; then
        continue
    fi

    # Check for arm and set arm version
    if [[ $GOARCH == "arm" ]]; then
        # Set what arm versions each platform supports
        if [[ $GOOS == "darwin" ]]; then
            arms="7"
        elif [[ $GOOS == "windows" ]]; then
             # This is a guess, it's not clear what Windows supports from the docs
             # But I was able to build all these on my machine
            arms="5 6 7"
        elif [[ $GOOS == *"bsd"  ]]; then
            arms="6 7"
        else
            # Linux goes here
            arms="5 6 7"
        fi

        # Now do the arm build
        for GOARM in $arms; do
            BIN_FILENAME="${OUTPUT}-${GOOS}-${GOARCH}${GOARM}"
            if [[ "${GOOS}" == "windows" ]]; then BIN_FILENAME="${BIN_FILENAME}.exe"; fi
            CMD="GOARM=${GOARM} GOOS=${GOOS} GOARCH=${GOARCH} go build $FLAGS -o ${BIN_FILENAME} $@"
            echo "${CMD}"
            eval "${CMD}" || FAILURES="${FAILURES} ${GOOS}/${GOARCH}${GOARM}"
        done
    else
        # Build non-arm here
        if [[ "${GOOS}" == "windows" ]]; then BIN_FILENAME="${BIN_FILENAME}.exe"; fi
        CMD="GOOS=${GOOS} GOARCH=${GOARCH} go build $FLAGS -o ${BIN_FILENAME} $@"
        echo "${CMD}"
        eval "${CMD}" || FAILURES="${FAILURES} ${GOOS}/${GOARCH}"
    fi
done <<< "$(go tool dist list)"

if [[ "${FAILURES}" != "" ]]; then
    echo ""
    echo "build failed for: ${FAILURES}"
    exit 1
fi