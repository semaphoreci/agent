#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "executor": "dockercompose",

    "compose": {
      "containers": [
        {
          "name": "main",
          "image": "renderedtext/hello"
        }
      ],

      "image_pull_credentials": [
        {
          "env_vars": [
            { "name": "DOCKER_CREDENTIAL_TYPE", "value": "#{Base64.encode64("DockerHub")}" },
            { "name": "DOCKERHUB_USERNAME", "value": "#{Base64.encode64("lasagna")}" },
            { "name": "DOCKERHUB_PASSWORD", "value": "#{Base64.encode64("spaghetti")}" }
          ]
        }
      ]
    },

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "echo Hello World" }
    ],

    "epilogue_always_commands": [],

    "callbacks": {
      "finished": "https://httpbin.org/status/200",
      "teardown_finished": "https://httpbin.org/status/200"
    }
  }
JSON

wait_for_job_to_finish

assert_job_log <<-LOG
  {"event":"job_started",  "timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Setting up image pull credentials"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Setting up credentials for DockerHub"}
  {"event":"cmd_output",   "timestamp":"*", "output":"docker login --username $DOCKERHUB_USERNAME --password $DOCKERHUB_PASSWORD"}
  {"event":"cmd_output",   "timestamp":"*", "output":"WARNING! Using --password via the CLI is insecure. Use --password-stdin.\\nError response from daemon: Get https://registry-1.docker.io/v2/: unauthorized: incorrect username or password\\n"}

  {"event":"cmd_finished", "timestamp":"*", "directive":"Setting up image pull credentials", "event":"cmd_finished","exit_code":1,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG