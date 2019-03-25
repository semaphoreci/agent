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
          "image": "ruby:2.6"
        }
      ]
    },

    "env_vars": [
      { "name": "A", "value": "#{`echo "hello" | base64`}" },
      { "name": "B", "value": "#{`echo "how are you?" | base64`}" },
      { "name": "C", "value": "#{`echo "quotes ' quotes" | base64`}" },
      { "name": "D", "value": "#{`echo '$PATH:/etc/a' | base64`}" }
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
      "started": "https://httpbin.org/status/200",
      "finished": "https://httpbin.org/status/200",
      "teardown_finished": "https://httpbin.org/status/200"
    }
  }
JSON

wait_for_job_to_finish

assert_job_log <<-LOG
  {"event":"job_started",  "timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Pulling docker images..."}
  {"event":"cmd_output",   "timestamp":"*", "output":"*"}
  {"event":"cmd_output",   "timestamp":"*", "output":"*"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Pulling docker images...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

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

  {"event":"cmd_started",  "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=passed"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=passed","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG
