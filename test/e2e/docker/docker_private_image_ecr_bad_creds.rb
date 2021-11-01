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
          "image": "#{ENV['AWS_IMAGE']}"
        }
      ],

      "image_pull_credentials": [
        {
          "env_vars": [
            { "name": "DOCKER_CREDENTIAL_TYPE", "value": "#{Base64.strict_encode64("AWS_ECR")}" },
            { "name": "AWS_REGION", "value": "#{Base64.strict_encode64(ENV['AWS_REGION'])}" },
            { "name": "AWS_ACCESS_KEY_ID", "value": "#{Base64.strict_encode64("AAABBBCCCDDDEEEFFF")}" },
            { "name": "AWS_SECRET_ACCESS_KEY", "value": "#{Base64.strict_encode64('abcdefghijklmnop')}" }
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
  {"event":"cmd_output",   "timestamp":"*", "output":"Setting up credentials for ECR\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"$(aws ecr get-login --no-include-email --region $AWS_REGION)\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"An error occurred (UnrecognizedClientException) when calling the GetAuthorizationToken operation: The security token included in the request is invalid.\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Setting up image pull credentials", "event":"cmd_finished","exit_code":1,"finished_at":"*","started_at":"*","timestamp":"*"}
  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
