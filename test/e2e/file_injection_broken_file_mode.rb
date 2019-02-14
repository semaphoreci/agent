#!/bin/ruby
# rubocop:disable all

require_relative '../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "env_vars": [],

    "files": [
      { "path": "test.txt", "content": "#{`echo "hello" | base64`}", "mode": "obviously broken" }
    ],

    "commands": [
      { "directive": "cat test.txt" },
      { "directive": "cat /a/b/c" },
      { "directive": "stat -c '%a' /tmp/a" }
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
  {"event":"cmd_output",   "timestamp":"*", "output":"Injecting test.txt with file mode obviously broken"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Failed to set file mode to obviously broken"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":1,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=failed"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=failed","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
