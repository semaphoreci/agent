#!/bin/ruby
# rubocop:disable all

# A job whose commands exit with 130 is reported as stopped by the agent itself,
# without a stop request. It still leaves nothing behind in the cluster.

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
  "kubernetes-default-image" => "ruby:3-slim"
}

require_relative '../../e2e'
require_relative '../../e2e_support/kubernetes'

start_job <<-JSON
  {
    "job_id": "#{$JOB_ID}",
    "executor": "shell",
    "env_vars": [],
    "files": [],
    "commands": [
      { "directive": "sleep 10" },
      { "directive": "echo 'exit 130' | sh" },
      { "directive": "echo 'should not run'" }
    ],

    "epilogue_always_commands": [],

    "callbacks": {
      "finished": "#{finished_callback_url}",
      "teardown_finished": "#{teardown_callback_url}"
    },
    "logger": #{$LOGGER}
  }
JSON

wait_for_command_to_start("sleep 10")
assert_job_resources_exist

wait_for_job_to_finish

assert_job_result("stopped")
assert_no_job_resources_left
