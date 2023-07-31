#!/bin/ruby

Encoding.default_external = Encoding::UTF_8

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
          "image": "ruby:2.6"
        }
      ]
    },

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "echo 特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。" }
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
  {"event":"cmd_started",  "timestamp":"*", "directive":"echo 特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。"}
  {"event":"cmd_output",   "timestamp":"*", "output":"特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo 特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。特定の伝説に拠る物語の由来については諸説存在し。","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG
