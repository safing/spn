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

if [[ ! -f "../../cmds/hub/hub" ]]; then
  echo "please build the hub cmd using cmds/hub/build"
  exit 1
fi

bin_path="$(realpath ../../cmds/hub/hub)"
data_path="$(realpath ./data)"
if [[ ! -d "$data_path" ]]; then
  mkdir "$data_path"
fi
shared_path="$(realpath ./data/shared)"
if [[ ! -d "$shared_path" ]]; then
  mkdir "$shared_path"
fi

docker network ls | grep spn-simpletest-network >/dev/null 2>&1
if [[ $? -ne 0 ]]; then
  docker network create spn-simpletest-network --subnet 6.0.0.0/24
fi

for (( i = 1; i <= 3; i++ )); do
  docker run -d \
  --name spn-simpletest-node${i} \
  --network spn-simpletest-network \
  -v $bin_path:/opt/spn:ro \
  -v $data_path/node${i}:/opt/data \
  -v $shared_path:/opt/shared \
  --entrypoint /opt/spn \
  toolset.safing.network/dev \
  --data /opt/data \
  --bootstrap-file /opt/shared/bootstrap.dsd \
  --log debug

  if [[ $i -eq 1 ]]; then
    echo "giving first hub time to start and create bootstrap file"
    sleep 5
  fi
done

docker ps -a | grep spn-simpletest-
