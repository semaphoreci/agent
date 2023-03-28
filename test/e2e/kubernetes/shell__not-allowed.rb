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
  "kubernetes-executor" => true
}

require_relative '../../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",
    "executor": "shell",
    "env_vars": [],
    "files": [],
    "commands": [
      { "directive": "echo hello" }
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

  {"event":"cmd_started",  "timestamp":"*", "directive":"Constructing pod spec..."}
  {"event":"cmd_output",   "timestamp":"*", "output":"Kubernetes executor requires containers to be specified."}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Constructing pod spec...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
