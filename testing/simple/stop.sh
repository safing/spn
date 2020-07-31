#!/bin/bash

cd "$( dirname "${BASH_SOURCE[0]}" )"

docker stop $(docker ps -a | grep spn-simpletest | cut -d" " -f1)
docker rm $(docker ps -a | grep spn-simpletest | cut -d" " -f1)

docker network ls | grep spn-simpletest-network >/dev/null 2>&1
if [[ $? -eq 0 ]]; then
  docker network rm spn-simpletest-network
fi

rm -r data/shared
