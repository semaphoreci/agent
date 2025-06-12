#!/bin/ruby
# rubocop:disable all

File.write("/tmp/agent/file1.txt", "Hello from file1.txt")
File.write("/tmp/agent/file2.txt", "Hello from file2.txt")

$AGENT_CONFIG = {
  "endpoint" => "localhost:4567",
  "token" => "321h1l2jkh1jk42341",
  "no-https" => true,
  "shutdown-hook-path" => "",
  "disconnect-after-job" => false,
  "env-vars" => [],
  "files" => [
    "/tmp/agent/file1.txt:/tmp/agent/file1.txt",
    "/tmp/agent/file2.txt:/tmp/agent/file2.txt",
    "/tmp/agent/notfound.txt:/tmp/agent/notfound.txt"
  ],
  "fail-on-missing-files" => true
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
          "image": "registry.semaphoreci.com/ruby:2.6"
        }
      ]
    },

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "cat /tmp/agent/file1.txt" },
      { "directive": "cat /tmp/agent/file2.txt" },
      { "directive": "cat /tmp/agent/notfound.txt" }
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
  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
