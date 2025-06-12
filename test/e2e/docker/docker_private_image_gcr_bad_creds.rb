#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

start_job <<-JSON
  {
    "job_id": "#{$JOB_ID}",

    "executor": "dockercompose",

    "compose": {
      "containers": [
        {
          "name": "main",
          "image": "#{ENV['GCR_IMAGE']}"
        }
      ],

      "image_pull_credentials": [
        {
          "env_vars": [
            { "name": "DOCKER_CREDENTIAL_TYPE", "value": "#{Base64.strict_encode64("GCR")}" },
            { "name": "GCR_HOSTNAME", "value": "#{Base64.strict_encode64(ENV['GCR_HOSTNAME'])}" }
          ],
          "files": [
            { "path": "/tmp/gcr/keyfile.json", "content": "#{ENV['GCR_KEYFILE_BAD']}", "mode": "0755" }
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
  {"event":"job_started","timestamp":"*"}
  {"directive":"Setting up image pull credentials","event":"cmd_started","timestamp":"*"}
  {"event":"cmd_output","output":"Setting up credentials for GCR\\n","timestamp":"*"}
  {"event":"cmd_output","output":"cat /tmp/gcr/keyfile.json | docker login -u _json_key --password-stdin https://$GCR_HOSTNAME\\n","timestamp":"*"}
  {"event":"cmd_output","output":"Error response from daemon: Get \\"https://us-east4-docker.pkg.dev/v2/\\": unauthorized: authentication failed\\n","timestamp":"*"}
  {"event":"cmd_output","output":"\\n","timestamp":"*"}
  {"directive":"Setting up image pull credentials","event":"cmd_finished","exit_code":1,"finished_at":"*","started_at":"*","timestamp":"*"}
  {"event":"job_finished","result":"failed","timestamp":"*"}
LOG
