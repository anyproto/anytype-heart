#!/bin/bash

set -v
set -e

function stop_and_cleanup() {
  EXIT_CODE=$?
  docker-compose stop -t 1
  docker-compose logs --no-color --tail="all" > /tmp/docker_nodes.log 2>&1
  docker-compose run --rm test python ./utils/send_results.py --build-id=${CIRCLE_BUILD_NUM} --build-url=${CIRCLE_BUILD_URL} --git-branch=${CIRCLE_BRANCH} --exit-code=${EXIT_CODE}
  docker-compose down
}

function start_test() {
  docker-compose run --rm test
}

trap stop_and_cleanup SIGINT SIGTERM EXIT

docker-compose pull test

TAG="latest"
BASE_TAG="master"
if [ "${CIRCLE_SHA1}" != "" ]; then
  TAG=${CIRCLE_SHA1}
fi

export TAG
export BASE_TAG

start_test
