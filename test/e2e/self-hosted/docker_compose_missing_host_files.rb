#!/bin/ruby
# rubocop:disable all

File.write("/tmp/agent/file1.txt", "Hello from file1.txt")
File.write("/tmp/agent/file2.txt", "Hello from file2.txt")

$AGENT_CONFIG = {
  "endpoint" => "hub:4567",
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
  "fail-on-missing-files" => false
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
          "image": "ruby:2.6"
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

  {"event":"cmd_started",  "timestamp":"*", "directive":"cat /tmp/agent/file1.txt"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Hello from file1.txt"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"cat /tmp/agent/file1.txt","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"cat /tmp/agent/file2.txt"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Hello from file2.txt"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"cat /tmp/agent/file2.txt","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"cat /tmp/agent/notfound.txt"}
  {"event":"cmd_output",   "timestamp":"*", "output":"cat: /tmp/agent/notfound.txt: No such file or directory\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"cat /tmp/agent/notfound.txt","exit_code":1,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
