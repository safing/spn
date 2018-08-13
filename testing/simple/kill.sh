#!/bin/bash

cd "$( dirname "${BASH_SOURCE[0]}" )"

sudo docker stop $(sudo docker ps -a | grep safing-simpletest | cut -d" " -f1)
sudo docker rm $(sudo docker ps -a | grep safing-simpletest | cut -d" " -f1)

sudo docker network ls | grep safing-simpletest-network >/dev/null 2>&1
if [[ $? -eq 0 ]]; then
  sudo docker network rm safing-simpletest-network
fi
