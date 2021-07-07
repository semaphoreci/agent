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
          "image": "#{ENV['DOCKER_REGISTRY_IMAGE']}"
        }
      ],

      "image_pull_credentials": [
        {
          "env_vars": [
            { "name": "DOCKER_CREDENTIAL_TYPE", "value": "#{Base64.encode64("GenericDocker")}" },
            { "name": "DOCKER_URL", "value": "#{Base64.encode64(ENV['DOCKER_URL'])}" },
            { "name": "DOCKER_USERNAME", "value": "#{Base64.encode64("lasagna")}" },
            { "name": "DOCKER_PASSWORD", "value": "#{Base64.encode64("spaghetti")}" }
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
      "finished": "#{finished_callback_url}",
      "teardown_finished": "#{teardown_callback_url}"
    },
    "logger": #{$LOGGER}
  }
JSON

wait_for_job_to_finish

assert_job_log <<-LOG
{"event":"job_started",  "timestamp":"*"}

{"event":"cmd_started",  "timestamp":"*", "directive":"Setting up image pull credentials"}
{"event":"cmd_output",   "timestamp":"*", "output":"Setting up credentials for Docker\\n"}
{"event":"cmd_output",   "timestamp":"*", "output":"docker login -u \\"$DOCKER_USERNAME\\" -p \\"$DOCKER_PASSWORD\\" $DOCKER_URL\\n"}
{"event":"cmd_output",   "timestamp":"*", "output":"WARNING! Using --password via the CLI is insecure. Use --password-stdin.\\n"}
{"event":"cmd_output",   "output":"Error response from daemon: login attempt to https://#{ENV['DOCKER_URL']}/v2/ failed with status: 401 Unauthorized\\n","timestamp":"*"}
{"event":"cmd_output",   "timestamp":"*", "output":"\\n"}
{"event":"cmd_finished", "timestamp":"*", "directive":"Setting up image pull credentials", "event":"cmd_finished","exit_code":1,"finished_at":"*","started_at":"*","timestamp":"*"}
{"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
