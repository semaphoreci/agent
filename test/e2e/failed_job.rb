#!/bin/ruby
# rubocop:disable all

require_relative '../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "executor": "shell",

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "false" }
    ],

    "epilogue_commands": [],

    "callbacks": {
      "started": "https://httpbin.org/status/200",
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
  {"event":"cmd_started",  "timestamp":"*", "directive":"false"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"false","exit_code":1,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=failed"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=failed","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
