#!/bin/bash

# Environment variables:
# - SEMAPHORE_ORG_NAME
# - AGENT_REGISTRATION_TOKEN
# - AGENT_DESIRED_COUNT
# - AGENT_MAX_COUNT

CWD=$(pwd)

if [[ -z "${SEMAPHORE_ORG_NAME}" ]]; then
  echo "SEMAPHORE_ORG_NAME is not set. Exiting..."
  exit 1
fi

if [[ -z "${AGENT_REGISTRATION_TOKEN}" ]]; then
  echo "AGENT_REGISTRATION_TOKEN is not set. Exiting..."
  exit 1
fi

if [[ -z "${AGENT_DESIRED_COUNT}" ]]; then
  AGENT_DESIRED_COUNT=5
  echo "AGENT_DESIRED_COUNT is not set. Using default value of $AGENT_DESIRED_COUNT"
fi

if [[ -z "${AGENT_MAX_COUNT}" ]]; then
  AGENT_MAX_COUNT=5
  echo "AGENT_MAX_COUNT is not set. Using default value of $AGENT_MAX_COUNT"
fi

function start_agents() {
  count=$1
  created=0
  for i in $(seq 1 $count); do
    container_id=$(docker run --rm -d -it \
      -v $CWD:/app \
      --env SEMAPHORE_ORG_NAME=${SEMAPHORE_ORG_NAME} \
      --env AGENT_REGISTRATION_TOKEN=${AGENT_REGISTRATION_TOKEN} \
      empty-ubuntu-self-hosted-agent /app/build/agent start --endpoint $SEMAPHORE_ORG_NAME.semaphoreci.com --token $AGENT_REGISTRATION_TOKEN --disconnect-after-job)
    if [ $? -eq 0 ]; then
      echo "Created container $container_id"
      created=$(($created + 1))
    else
      echo "Command to start agent returned $?"
    fi
  done

  echo "${created}"
}

function cleanup_agents() {
  echo "Stopping agents..."
  docker ps --format '{"id": "{{ .ID }}", "image": "{{ .Image }}"}' | grep "agent" | jq '.id' | xargs docker stop
}

trap cleanup_agents SIGINT SIGTERM

echo "Starting agent manager..."
while sleep 5; do
  running_agents=$(docker ps | grep agent | wc -l)
  if [ $running_agents -lt $AGENT_DESIRED_COUNT ]; then
    agents_to_create=$(($AGENT_DESIRED_COUNT - $running_agents))
    echo "Starting up $agents_to_create agents..."
    started_agents=$(start_agents $agents_to_create)
    echo "$started_agents new agents created."
  else
    echo "Desired number of agents already in place."
  fi
done