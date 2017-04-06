#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace

function fun::step {
  local OPTS=""
  if [ "$1" = "-n" ]; then
    shift
    OPTS+="-n"
  fi
  GREEN="$1"
  shift
  if [ -t 2 ] ; then
    echo -e ${OPTS} "\x1B[97m* \x1B[92m${GREEN}\x1B[39m $*" >&2
  else
    echo ${OPTS} "* ${GREEN} $*" >&2
  fi
}

function fun::build_go {
  fun::step "Building virtlet go"
  rm "${VIRTLET_DIR}/_output" -Rf
  "${VIRTLET_DIR}/build/cmd.sh" build
  "${VIRTLET_DIR}/build/cmd.sh" copy
}

function fun::build_container {
  fun::step "Building virtlet container mirantis/virtlet"
  docker build -t mirantis/virtlet ${VIRTLET_DIR}
  # docker save mirantis/virtlet | docker exec -i kube-node-1 docker load  ### this is now done automatically by demo.sh
}

if [[ ${1:-} = "--help" || ${1:-} = "-h" ]]; then
  cat <<EOF >&2
Usage: ./update_virtlet.sh [--rebuild]

This script updates virtlet container with binaries from _output folder.
EOF
  exit 0
fi

#
# Main
#

if [[ ${1:-} = "--rebuild" ]]; then
  fun::build_go
fi

fun::build_container
