#!/bin/ruby
# rubocop:disable all

# A job stopped while its commands run leaves nothing behind in the cluster.

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
      { "directive": "sleep infinity" },
      { "directive": "echo 'here'" }
    ],

    "epilogue_always_commands": [],

    "callbacks": {
      "finished": "#{finished_callback_url}",
      "teardown_finished": "#{teardown_callback_url}"
    },
    "logger": #{$LOGGER}
  }
JSON

wait_for_command_to_start("sleep infinity")
assert_job_resources_exist

stop_job
wait_for_job_to_finish

assert_job_result("stopped")
assert_no_job_resources_left
