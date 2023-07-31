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
    "job_id": "#{$JOB_ID}",

    "executor": "dockercompose",

    "compose": {
      "containers": [
        {
          "name": "main",
          "image": "ruby:3-slim",
          "env_vars": [
            { "name": "FOO", "value": "#{`echo "bar" | base64 | tr -d '\n'`}" }
          ]
        }
      ]
    },

    "env_vars": [
      { "name": "A", "value": "#{`echo "hello" | base64 | tr -d '\n'`}" },
      { "name": "B", "value": "#{`echo "how are you?" | base64 | tr -d '\n'`}" },
      { "name": "C", "value": "#{`echo "quotes ' quotes" | base64 | tr -d '\n'`}" },
      { "name": "D", "value": "#{`echo '$PATH:/etc/a' | base64 | tr -d '\n'`}" }
    ],

    "files": [],

    "commands": [
      { "directive": "echo $A" },
      { "directive": "echo $B" },
      { "directive": "echo $C" },
      { "directive": "echo $D" },
      { "directive": "echo $FOO" }
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

  {"event":"cmd_started",  "timestamp":"*", "directive":"Creating Kubernetes resources for job..."}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Creating Kubernetes resources for job...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Starting shell session..."}
  *** LONG_OUTPUT ***
  {"event":"cmd_finished", "timestamp":"*", "directive":"Starting shell session...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

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

  {"event":"cmd_started",  "timestamp":"*", "directive":"echo $FOO"}
  {"event":"cmd_output",   "timestamp":"*", "output":"bar\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"echo $FOO","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG
