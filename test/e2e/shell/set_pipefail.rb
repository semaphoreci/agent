#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

#
# Running the following set of commands caused the Agent to freeze up.
#
#   sleep infinity &
#   set -eo pipefail
#   cat non_existant | sort
#
# These are regressions tests that verify that this is no longer a problem.
#

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "sleep infinity &" },
      { "directive": "set -eo pipefail" },
      { "directive": "cat non_existant | sort" }
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
  {"event":"cmd_started",  "timestamp":"*", "directive":"sleep infinity &"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"sleep infinity &","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"set -eo pipefail"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"set -eo pipefail","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"cat non_existant | sort"}
  {"event":"cmd_output",   "timestamp":"*", "output":"cat: non_existant: No such file or directory\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"cat non_existant | sort","exit_code":1,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}
  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
