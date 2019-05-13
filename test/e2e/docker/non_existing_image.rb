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
          "image": "semaphaaaaaaphoreci/ruuuby:2.6"
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
  {"event":"cmd_started",  "timestamp":"*", "directive":"Pulling docker images..."}
  {"event":"cmd_output",   "timestamp":"*", "output":"Pulling main ... \\r\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Pulling main ... error\\r\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"ERROR: for main  pull access denied for semaphaaaaaaphoreci/ruuuby, repository does not exist or may require 'docker login'\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"pull access denied for semaphaaaaaaphoreci/ruuuby, repository does not exist or may require 'docker login'\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Pulling docker images...","event":"cmd_finished","exit_code":1,"finished_at":"*","started_at":"*","timestamp":"*"}
  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
