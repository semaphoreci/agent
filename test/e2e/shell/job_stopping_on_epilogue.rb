#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "echo 'here'" }
    ],

    "epilogue_always_commands": [
      { "directive": "sleep infinity" }
    ],

    "callbacks": {
      "finished": "#{finished_callback_url}",
      "teardown_finished": "#{teardown_callback_url}"
    },
    "logger": #{$LOGGER}
  }
JSON

wait_for_command_to_start("sleep infinity")

sleep 1

stop_job

wait_for_job_to_finish

assert_job_log <<-LOG
  {"event":"job_started",  "timestamp":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Injecting Files"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"echo 'here'"}
  {"event":"cmd_output",   "timestamp":"*", "output":"here\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo 'here'","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=passed"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=passed","exit_code":0,"started_at":"*","finished_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"sleep infinity"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"sleep infinity","exit_code":1,"finished_at":"*","started_at":"*"}
  {"event":"job_finished", "timestamp":"*", "result":"stopped"}
LOG
