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
    "executor": "dockercompose",
    "compose": {
      "containers": [
        {
          "name": "main",
          "image": "ruby:2.6"
        }
      ]
    },
    "env_vars": [
      { "name": "SEMAPHORE_JOB_ID", "value": "#{Base64.strict_encode64(ENV["SEMAPHORE_JOB_ID"])}" },
      { "name": "SEMAPHORE_ORGANIZATION_URL", "value": "#{Base64.strict_encode64(ENV["SEMAPHORE_ORGANIZATION_URL"])}" },
      { "name": "SEMAPHORE_ARTIFACT_TOKEN", "value": "#{Base64.strict_encode64(ENV["SEMAPHORE_ARTIFACT_TOKEN"])}" },
      { "name": "SEMAPHORE_AGENT_UPLOAD_JOB_LOGS", "value": "#{Base64.strict_encode64("always")}" }
    ],
    "files": [],
    "commands": [
      { "directive": "for i in {1..10}; do echo \\\"[$i] this is some output, just for testing purposes\\\" && sleep 1; done" }
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
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_AGENT_UPLOAD_JOB_LOGS\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_ARTIFACT_TOKEN\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_ID\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_ORGANIZATION_URL\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Injecting Files"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Injecting Files","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"for i in {1..10}; do echo \\"[$i] this is some output, just for testing purposes\\" && sleep 1; done"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[1] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[2] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[3] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[4] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[5] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[6] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[7] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[8] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[9] this is some output, just for testing purposes\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"[10] this is some output, just for testing purposes\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"for i in {1..10}; do echo \\"[$i] this is some output, just for testing purposes\\" && sleep 1; done","exit_code":0,"finished_at":"*","started_at":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Exporting environment variables"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Exporting SEMAPHORE_JOB_RESULT\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Exporting environment variables","exit_code":0,"started_at":"*","finished_at":"*"}

  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG

assert_artifact_is_available