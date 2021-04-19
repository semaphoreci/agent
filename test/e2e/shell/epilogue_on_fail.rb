#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "false" }
    ],

    "epilogue_always_commands": [
      { "directive": "echo Hello Epilogue" }
    ],

    "epilogue_on_pass_commands": [
      { "directive": "echo Hello On Pass Epilogue" }
    ],

    "epilogue_on_fail_commands": [
      { "directive": "echo Hello On Fail Epilogue" }
    ],

    "callbacks": {
      "finished": "#{finished_callback_url}",
      "teardown_finished": "#{teardown_callback_url}"
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

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo Hello Epilogue"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Hello Epilogue\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo Hello Epilogue","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo Hello On Fail Epilogue"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Hello On Fail Epilogue\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo Hello On Fail Epilogue","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
