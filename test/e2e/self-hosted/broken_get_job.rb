#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

$JOB_ID = "bad-job-id"

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "echo 'hello'" }
    ],

    "epilogue_always_commands": [],

    "callbacks": {
      "finished": "#{finished_callback_url}",
      "teardown_finished": "#{teardown_callback_url}"
    },
    "logger": #{$LOGGER}
  }
JSON

wait_for_job_to_get_stuck
