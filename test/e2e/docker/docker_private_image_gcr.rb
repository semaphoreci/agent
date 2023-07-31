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
            { "path": "/tmp/gcr/keyfile.json", "content": "#{ENV['GCR_KEYFILE']}", "mode": "0755" }
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
  {"event":"cmd_output","output":"WARNING! Your password will be stored unencrypted in /root/.docker/config.json.\\n","timestamp":"*"}
  {"event":"cmd_output","output":"Configure a credential helper to remove this warning. See\\n","timestamp":"*"}
  {"event":"cmd_output","output":"https://docs.docker.com/engine/reference/commandline/login/#credentials-store\\n","timestamp":"*"}
  {"event":"cmd_output","output":"\\n","timestamp":"*"}
  {"event":"cmd_output","output":"Login Succeeded\\n","timestamp":"*"}
  {"event":"cmd_output","output":"\\n","timestamp":"*"}
  {"directive":"Setting up image pull credentials","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}
  {"directive":"Pulling docker images...","event":"cmd_started","timestamp":"*"}
  *** LONG_OUTPUT ***
  {"directive":"Pulling docker images...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Starting the docker image..."}
  {"event":"cmd_output",   "timestamp":"*", "output":"Starting a new bash session.\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Starting the docker image...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"directive":"Exporting environment variables","event":"cmd_started","timestamp":"*"}
  {"directive":"Exporting environment variables","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}
  {"directive":"Injecting Files","event":"cmd_started","timestamp":"*"}
  {"directive":"Injecting Files","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}
  {"directive":"echo Hello World","event":"cmd_started","timestamp":"*"}
  {"event":"cmd_output","output":"Hello World\\n","timestamp":"*"}
  {"directive":"echo Hello World","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}

  {"event":"job_finished","result":"passed","timestamp":"*"}
LOG
