#!/bin/ruby
# rubocop:disable all

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
      { "directive": "sleep 10" },
      { "directive": "echo 'here'" }
    ],

    "epilogue_commands": [],

    "callbacks": {
      "started": "https://httpbin.org/status/200",
      "finished": "https://httpbin.org/status/200",
      "teardown_finished": "https://httpbin.org/status/200"
    }
  }
JSON

sleep 1

stop_job

sleep 1

assert_job_log <<-LOG
  {"event":"job_started",  "timestamp":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Injecting Files"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"sleep 10"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"sleep 10","exit_code":1,"finished_at":"*","started_at":"*"}
  {"event":"job_finished", "timestamp":"*", "result":"failed"}
LOG
