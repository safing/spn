#!/bin/bash

cd "$( dirname "${BASH_SOURCE[0]}" )"

if [[ ! -f "../../../${1}" ]]; then
  echo "please compile ${1}.go in main directory"
  exit 1
fi

bin_path="$(realpath ../../../${1})"
data_path="$(realpath .)/data/me"
if [[ ! -d "$data_path" ]]; then
  mkdir "$data_path"
fi

leftover=$(sudo docker ps -a | grep safing-simpletest-me | cut -d" " -f1)
if [[ $leftover != "" ]]; then
  sudo docker stop $leftover
  sudo docker rm $leftover
fi

sudo docker run -ti --name safing-simpletest-me --network safing-simpletest-network -v $bin_path:/opt/port17 -v $data_path:/opt/data ubuntu /opt/port17 -db /opt/data/db -name me $2 $3 $4 $5 $6 $7 $8 $9

# sudo docker rm safing-simpletest-me
