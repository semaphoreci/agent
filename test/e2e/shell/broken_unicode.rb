#!/bin/ruby

Encoding.default_external = Encoding::UTF_8

# rubocop:disable all

require_relative '../../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "env_vars": [],
    "files": [],

    "commands": [
      { "directive": "echo | awk '{ printf(\\\"%c%c%c%c%c\\\", 150, 150, 150, 150, 150) }'"}
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

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Injecting Files"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive": "echo | awk '{ printf(\\\"%c%c%c%c%c\\\", 150, 150, 150, 150, 150) }'"}
  {"event":"cmd_output",   "timestamp":"*", "output":"\ufffd\ufffd\ufffd\ufffd\ufffd"}
  {"event":"cmd_finished", "timestamp":"*", "directive": "echo | awk '{ printf(\\\"%c%c%c%c%c\\\", 150, 150, 150, 150, 150) }'","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=passed"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=passed","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG
