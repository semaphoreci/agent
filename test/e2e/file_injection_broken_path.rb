#!/bin/ruby
# rubocop:disable all

require_relative '../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "env_vars": [],

    "files": [
      { "path": "C://%  // test.txt", "content": "#{`echo "hello" | base64`}", "mode": "0644" },
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
  {"event":"cmd_output",   "timestamp":"*", "output":"Injecting test.txt with file mode 0644"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Injecting /a/b/c with file mode 0644"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Injecting /tmp/a with file mode +x"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"cat test.txt"}
  {"event":"cmd_output",   "timestamp":"*", "output":"hello"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"cat test.txt","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"cat /a/b/c"}
  {"event":"cmd_output",   "timestamp":"*", "output":"hello"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"cat /a/b/c","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"stat -c '%a' /tmp/a"}
  {"event":"cmd_output",   "timestamp":"*", "output":"755"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"stat -c '%a' /tmp/a","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=passed"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"export SEMAPHORE_JOB_RESULT=passed","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG
