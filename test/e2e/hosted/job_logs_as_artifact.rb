#!/bin/ruby
# rubocop:disable all

$LOGGER = '{ "method": "pull", "max_size_in_bytes": 100 }'

require_relative '../../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",
    "executor": "shell",
    "env_vars": [],
    "files": [],
    "commands": [
      { "directive": "for i in {1..10}; do echo \"[$i] this is some output, just for testing purposes\"; done" },
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

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Injecting Files"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"for i in {1..10}; do echo \"[$i] this is some output, just for testing purposes\"; done"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[1] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[2] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[3] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[4] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[5] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[6] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[7] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[8] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[9] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[10] this is some output, just for testing purposes\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"for i in {1..10}; do echo \"[$i] this is some output, just for testing purposes\"; done","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG

assert_artifact_is_available