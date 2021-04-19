#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

#
# Not every Docker image has pre-installed bash. For example the Alpine docker
# image.
#
# This test verifies the behaviour of the Agent in case bash is not part of the
# standard PATH. In these scenarios, we expect to see a warning displayed to the
# customer.
#

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "executor": "dockercompose",

    "compose": {
      "containers": [
        {
          "name": "main",
          "image": "alpine"
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
    }
  }
JSON

wait_for_job_to_finish

assert_job_log <<-LOG
  {"event":"job_started",  "timestamp":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Pulling docker images..."}
  *** LONG_OUTPUT ***
  {"event":"cmd_finished", "timestamp":"*", "directive":"Pulling docker images...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Starting the docker image..."}
  {"event":"cmd_output",   "timestamp":"*", "output":"Starting a new bash session.\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Failed to start the docker image\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"*"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Starting the docker image...","event":"cmd_finished","exit_code":1,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
