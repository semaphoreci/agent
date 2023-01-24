#!/bin/ruby
# rubocop:disable all

$AGENT_CONFIG = {
  "endpoint" => "localhost:4567",
  "token" => "321h1l2jkh1jk42341",
  "no-https" => true,
  "shutdown-hook-path" => "",
  "disconnect-after-job" => false,
  "env-vars" => [],
  "files" => [],
  "fail-on-missing-files" => false,
  "kubernetes-executor" => true,
  "kubernetes-image-pull-policy" => "IfNotPresent"
}

require_relative '../../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",
    "executor": "dockercompose",
    "compose": {
      "containers": [
        {
          "name": "main",
          "image": "ruby:3-slim"
        }
      ]
    },
    "env_vars": [],
    "files": [],
    "commands": [
      { "directive": "echo Hello World" }
    ],
    "epilogue_always_commands": [
      { "directive": "echo Hello Epilogue $SEMAPHORE_JOB_RESULT" }
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

  {"event":"cmd_started",  "timestamp":"*", "directive":"Starting shell session..."}
  *** LONG_OUTPUT ***
  {"event":"cmd_finished", "timestamp":"*", "directive":"Starting shell session...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Injecting Files"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo Hello World"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Hello World\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo Hello World","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo Hello Epilogue $SEMAPHORE_JOB_RESULT"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Hello Epilogue passed\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo Hello Epilogue $SEMAPHORE_JOB_RESULT","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG
