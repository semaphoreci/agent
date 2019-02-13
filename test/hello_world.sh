#!/bin/bash

set -euo pipefail
IFS=$'\n\t'

JOB_ID=$(uuidgen)

echo "Sending Job Request"

curl -X POST -k "https://0.0.0.0:8000/jobs/$JOB_ID" --data @- <<JSON
{
  "id": "$JOB_ID",

  "env_vars": [
    { "name": "A" },
    { "value": "aGVsbG8K" }
  ],

  "files": [
    { "path": "/tmp/test.txt", "mode": "0644", "content": "aGVsbG8K" }
  ],

  "commands": [
    { "directive": "echo Hello World" }
  ],

  "epilogue_commands": [],

  "callbacks": {
    "started": "https://httpbin.org/status/200",
    "finished": "https://httpbin.org/status/200",
    "teardown_finished": "https://httpbin.org/status/200"
  }
}
JSON

echo "Waiting for job to finish"
sleep 2

curl -k "https://0.0.0.0:8000/jobs/$JOB_ID/log"
