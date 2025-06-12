#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

start_job <<-JSON
  {
    "job_id": "#{$JOB_ID}",

    "executor": "dockercompose",

    "compose": {
      "containers": [
        {
          "name": "main",
          "image": "registry.semaphoreci.com/ruby:2.6"
        }
      ]
    },

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "false" }
    ],

    "epilogue_always_commands": [
      { "directive": "echo Hello Epilogue $SEMAPHORE_JOB_RESULT" }
    ],

    "epilogue_on_pass_commands": [
      { "directive": "echo Hello On Pass Epilogue $SEMAPHORE_JOB_RESULT" }
    ],

    "epilogue_on_fail_commands": [
      { "directive": "echo Hello On Fail Epilogue $SEMAPHORE_JOB_RESULT" }
    ],

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

  {"event":"cmd_started",  "timestamp":"*", "directive":"Pulling docker images..."}
  *** LONG_OUTPUT ***
  {"event":"cmd_finished", "timestamp":"*", "directive":"Pulling docker images...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Starting the docker image..."}
  {"event":"cmd_output",   "timestamp":"*", "output":"Starting a new bash session.\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Starting the docker image...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Injecting Files"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"false"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"false","exit_code":1,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo Hello Epilogue $SEMAPHORE_JOB_RESULT"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Hello Epilogue failed\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo Hello Epilogue $SEMAPHORE_JOB_RESULT","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo Hello On Fail Epilogue $SEMAPHORE_JOB_RESULT"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Hello On Fail Epilogue failed\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo Hello On Fail Epilogue $SEMAPHORE_JOB_RESULT","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
