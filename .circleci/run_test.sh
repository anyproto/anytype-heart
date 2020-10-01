#!/bin/bash

set -v
set -e

function stop_and_cleanup() {
  EXIT_CODE=$?
  docker-compose run --rm test python ./utils/send_results.py --build-id=${CIRCLE_BUILD_NUM} --build-url=${CIRCLE_BUILD_URL} --git-branch=${CIRCLE_BRANCH} --exit-code=${EXIT_CODE}
  docker-compose down
}

function start_test() {
  docker-compose run --rm test
}

trap stop_and_cleanup SIGINT SIGTERM EXIT

docker-compose pull test

TAG="latest"
if [ "${CIRCLE_SHA1}" != "" ]; then
  TAG=${CIRCLE_SHA1}
fi

export TAG

start_test
