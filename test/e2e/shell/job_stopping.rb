#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "watch -n 10000 ls" },
      { "directive": "echo 'here'" }
    ],

    "epilogue_always_commands": [],

    "callbacks": {
      "finished": "https://httpbin.org/status/200",
      "teardown_finished": "https://httpbin.org/status/200"
    }
  }
JSON

# In the above test we are running a blocking 'watch ls' command that will
# never finish on its own. This is perfect for testing out our job stopping
# procedures.
#
# Another alternative would be to use 'sleep infinity', however this is more
# troublesome to test when it comes to assert_has_no_running_process. Unrelated
# sleep commands might be running on the system that would cause a flakky test.
#
# Setting -n 100000 helps while observing the process with strace. It creates
# less syscalls than the default -n 2.

wait_for_command_to_start("watch -n 10000 ls")

sleep 20

stop_job

wait_for_job_to_finish

assert_job_log <<-LOG
  {"event":"job_started",  "timestamp":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Injecting Files"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"watch -n 10000 ls"}
  {"event":"job_finished", "timestamp":"*", "result":"stopped"}
LOG

sleep 5

assert_has_no_running_process("watch")
