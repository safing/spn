#!/bin/bash

cd "$( dirname "${BASH_SOURCE[0]}" )"

leftovers=$(sudo docker ps -a | grep safing-simpletest | cut -d" " -f1)
if [[ $leftovers != "" ]]; then
  sudo docker stop $leftovers
  sudo docker rm $leftovers
fi

if [[ ! -f "../../../port17node" ]]; then
  echo "please compile port17node.go in main directory"
  exit 1
fi

bin_path="$(realpath ../../../port17node)"
data_path="$(realpath .)/data"
if [[ ! -d "$data_path" ]]; then
  mkdir "$data_path"
fi

sudo docker network ls | grep safing-simpletest-network >/dev/null 2>&1
if [[ $? -ne 0 ]]; then
  sudo docker network create safing-simpletest-network --subnet 6.0.0.0/24
fi

for (( i = 1; i <= 3; i++ )); do
  sudo docker run -d --name safing-simpletest-node${i} --network safing-simpletest-network -v $bin_path:/opt/port17 -v $data_path/node${i}:/opt/data ubuntu /opt/port17 -db /opt/data/db -name node${i}
done
