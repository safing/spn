#!/bin/bash

cd "$( dirname "${BASH_SOURCE[0]}" )"

realpath() {
    path=`eval echo "$1"`
    folder=$(dirname "$path")
    echo $(cd "$folder"; pwd)/$(basename "$path"); 
}

if [[ ! -f "../../${1}" ]]; then
  echo "please compile ${1}.go in main directory"
  exit 1
fi

bin_path="$(realpath ../../${1})"
data_path="$(realpath ./data/me)"
if [[ ! -d "$data_path" ]]; then
  mkdir "$data_path"
fi

leftover=$(docker ps -a | grep spn-simpletest-me | cut -d" " -f1)
if [[ $leftover != "" ]]; then
  docker stop $leftover
  docker rm $leftover
fi

shift # remove first arg

docker run -ti --name spn-simpletest-me --network spn-simpletest-network -v $bin_path:/opt/spn:ro -v $data_path/me:/opt/data --entrypoint /opt/spn toolset.safing.network/dev --data /opt/data -name me -log trace $2 $3 $4 $5 $6 $7 $8 $9

# docker rm spn-simpletest-me
