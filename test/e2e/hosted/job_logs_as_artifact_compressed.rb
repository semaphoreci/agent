#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

# Here, we use the SEMAPHORE_JOB_ID as the job ID for this test.
$JOB_ID = ENV["SEMAPHORE_JOB_ID"]

# Additionally, we pass the artifact related environment variables
# to the job, so that it can upload the job logs as an artifact after the job is done.
start_job <<-JSON
  {
    "job_id": "#{$JOB_ID}",
    "executor": "shell",
    "env_vars": [
      { "name": "SEMAPHORE_JOB_ID", "value": "#{Base64.strict_encode64(ENV["SEMAPHORE_JOB_ID"])}" },
      { "name": "SEMAPHORE_ORGANIZATION_URL", "value": "#{Base64.strict_encode64(ENV["SEMAPHORE_ORGANIZATION_URL"])}" },
      { "name": "SEMAPHORE_ARTIFACT_TOKEN", "value": "#{Base64.strict_encode64(ENV["SEMAPHORE_ARTIFACT_TOKEN"])}" },
      { "name": "SEMAPHORE_AGENT_UPLOAD_JOB_LOGS", "value": "#{Base64.strict_encode64("always")}" }
    ],
    "files": [],
    "commands": [
      { "directive": "for i in $(seq 1 5000); do echo \\\"${i} $(LC_ALL=C tr -dc 'A-Za-z0-9!#$%&()*+,-./:;<=>?@^_{|}~' </dev/urandom | head -c 256)\\\"; done" }
    ],
    "epilogue_always_commands": [],
    "callbacks": {
      "finished": "#{finished_callback_url}",
      "teardown_finished": "#{teardown_callback_url}"
    },
    "logger": {
      "method": "pull"
    }
  }
JSON

wait_for_job_to_finish

assert_job_log <<-LOG
  {"event":"job_started",  "timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_AGENT_UPLOAD_JOB_LOGS\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_ARTIFACT_TOKEN\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_ID\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_ORGANIZATION_URL\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Injecting Files"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"for i in $(seq 1 5000); do echo \\\"${i} $(LC_ALL=C tr -dc 'A-Za-z0-9!#$%&()*+,-./:;<=>?@^_{|}~' </dev/urandom | head -c 256)\\\"; done"}
  *** LONG_OUTPUT ***
  {"event":"cmd_finished", "timestamp":"*", "directive":"for i in $(seq 1 5000); do echo \\\"${i} $(LC_ALL=C tr -dc 'A-Za-z0-9!#$%&()*+,-./:;<=>?@^_{|}~' </dev/urandom | head -c 256)\\\"; done","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG

assert_artifact_is_compressed
