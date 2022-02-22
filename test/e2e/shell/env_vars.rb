#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "env_vars": [
      { "name": "A", "value": "#{`echo "hello" | base64`.strip}" },
      { "name": "B", "value": "#{`echo "how are you?" | base64`.strip}" },
      { "name": "C", "value": "#{`echo "quotes ' quotes" | base64`.strip}" },
      { "name": "D", "value": "#{`echo '$PATH:/etc/a' | base64`.strip}" }
    ],

    "files": [],

    "commands": [
      { "directive": "echo $A" },
      { "directive": "echo $B" },
      { "directive": "echo $C" },
      { "directive": "echo $D" }
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
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting A\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting B\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting C\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting D\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Injecting Files"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo $A"}
  {"event":"cmd_output",   "timestamp":"*", "output":"hello\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo $A","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo $B"}
  {"event":"cmd_output",   "timestamp":"*", "output":"how are you?\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo $B","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo $C"}
  {"event":"cmd_output",   "timestamp":"*", "output":"quotes ' quotes\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo $C","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo $D"}
  {"event":"cmd_output",   "timestamp":"*", "output":"$PATH:/etc/a\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo $D","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}
  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG
