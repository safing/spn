#!/bin/bash

cd "$( dirname "${BASH_SOURCE[0]}" )"

realpath() {
    path=`eval echo "$1"`
    folder=$(dirname "$path")
    echo $(cd "$folder"; pwd)/$(basename "$path"); 
}

leftovers=$(docker ps -a | grep spn-test-simple | cut -d" " -f1)
if [[ $leftovers != "" ]]; then
  docker stop $leftovers
  docker rm $leftovers
fi

if [[ ! -f "../../cmds/hub/hub" ]]; then
  echo "please build the hub cmd using cmds/hub/build"
  exit 1
fi

SPN_TEST_BIN="$(realpath ../../cmds/hub/hub)"
SPN_TEST_DATA_DIR="$(realpath ./data)"
if [[ ! -d "$SPN_TEST_DATA_DIR" ]]; then
  mkdir "$SPN_TEST_DATA_DIR"
fi
SPN_TEST_SHARED_DATA_DIR="$(realpath ./data/shared)"
if [[ ! -d "$SPN_TEST_SHARED_DATA_DIR" ]]; then
  mkdir "$SPN_TEST_SHARED_DATA_DIR"
fi

# Export variables
export SPN_TEST_BIN
export SPN_TEST_DATA_DIR
export SPN_TEST_SHARED_DATA_DIR

# Run!
docker-compose -p spn-test-simple up --remove-orphans
