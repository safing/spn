#!/bin/bash

cd "$( dirname "${BASH_SOURCE[0]}" )"

realpath() {
    path=`eval echo "$1"`
    folder=$(dirname "$path")
    echo $(cd "$folder"; pwd)/$(basename "$path"); 
}

leftovers=$(docker ps -a | grep spn-simpletest | cut -d" " -f1)
if [[ $leftovers != "" ]]; then
  docker stop $leftovers
  docker rm $leftovers
fi

if [[ ! -f "../../spn" ]]; then
  echo "please compile main.go in main directory (output: spn)"
  exit 1
fi

bin_path="$(realpath ../../spn)"
data_path="$(realpath ./data)"
if [[ ! -d "$data_path" ]]; then
  mkdir "$data_path"
fi

docker network ls | grep spn-simpletest-network >/dev/null 2>&1
if [[ $? -ne 0 ]]; then
  docker network create spn-simpletest-network --subnet 6.0.0.0/24
fi

for (( i = 1; i <= 3; i++ )); do
  docker run -d --name spn-simpletest-node${i} --network spn-simpletest-network -v $bin_path:/opt/spn:ro -v $data_path/node${i}:/opt/data --entrypoint /opt/spn toolset.safing.network/dev --data /opt/data -name node${i} -log trace
done
