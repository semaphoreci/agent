#!/bin/ruby
# rubocop:disable all

require_relative '../../e2e'

aws_account_id = ENV['AWS_ACCOUNT_ID']

start_job <<-JSON
  {
    "job_id": "#{$JOB_ID}",

    "executor": "dockercompose",

    "compose": {
      "containers": [
        {
          "name": "main",
          "image": "#{ENV['AWS_IMAGE']}"
        }
      ],

      "image_pull_credentials": [
        {
          "env_vars": [
            { "name": "DOCKER_CREDENTIAL_TYPE", "value": "#{Base64.strict_encode64("AWS_ECR")}" },
            { "name": "AWS_REGION", "value": "#{Base64.strict_encode64(ENV['AWS_REGION'])}" },
            { "name": "AWS_ACCESS_KEY_ID", "value": "#{Base64.strict_encode64(ENV['AWS_ACCESS_KEY_ID'])}" },
            { "name": "AWS_SECRET_ACCESS_KEY", "value": "#{Base64.strict_encode64(ENV['AWS_SECRET_ACCESS_KEY'])}" }
          ]
        }
      ],
      "host_setup_commands": [
        { "directive": "curl 'https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip' -o 'awscliv2.zip'" },
        { "directive": "unzip awscliv2.zip" },
        { "directive": "./aws/install" }
      ]
    },

    "env_vars": [],

    "files": [],

    "commands": [
      { "directive": "echo Hello World" }
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

  {"event":"cmd_started",  "timestamp":"*", "directive":"Setting up image pull credentials"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Setting up credentials for ECR\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin #{aws_account_id}.dkr.ecr.$AWS_REGION.amazonaws.com\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"WARNING! Your credentials are stored unencrypted in /root/.docker/config.json.\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Configure a credential helper to remove this warning. See\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"https://docs.docker.com/engine/reference/commandline/login/#credential-stores\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"Login Succeeded\\n"}
  {"event":"cmd_output",   "timestamp":"*", "output":"\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Setting up image pull credentials", "exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}
  {"event":"cmd_started",  "timestamp":"*", "directive":"Pulling docker images..."}
  *** LONG_OUTPUT ***
  {"event":"cmd_finished", "timestamp":"*", "directive":"Pulling docker images...", "exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

  {"event":"cmd_started",  "timestamp":"*", "directive":"Starting the docker image..."}
  {"event":"cmd_output",   "timestamp":"*", "output":"Starting a new bash session.\\n"}
  {"event":"cmd_finished", "timestamp":"*", "directive":"Starting the docker image...","event":"cmd_finished","exit_code":0,"finished_at":"*","started_at":"*","timestamp":"*"}

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
  {"event":"job_finished", "timestamp":"*", "result":"passed"}
LOG
