# rubocop:disable all

Encoding.default_external = Encoding::UTF_8

require 'tempfile'
require 'json'
require 'yaml'
require 'timeout'
require 'base64'

require_relative "./e2e_support/api_mode"
require_relative "./e2e_support/listener_mode"

$JOB_ID = `uuidgen`.strip
$LOGGER = ""

$strategy = nil

case ENV["TEST_MODE"]
when "api" then
  $strategy = ApiMode.new
  $LOGGER = '{ "method": "pull" }'
when "listen" then
  $strategy = ListenerMode.new

  $LOGGER = <<-JSON
  {
    "method": "push",
    "url": "http://localhost:4567/api/v1/logs/#{$JOB_ID}",
    "token": "jwtToken"
  }
  JSON

  if !$AGENT_CONFIG
    $AGENT_CONFIG = {
      "endpoint" => "localhost:4567",
      "token" => "321h1l2jkh1jk42341",
      "no-https" => true,
      "shutdown-hook-path" => "",
      "disconnect-after-job" => false,
      "env-vars" => [],
      "files" => [],
      "fail-on-missing-files" => false
    }
  end
else
  raise "Testing Mode not set"
end

$strategy.boot_up_agent

def start_job(request)
  $strategy.start_job(request)
end

def stop_job
  $strategy.stop_job()
end

def wait_for_command_to_start(cmd)
  $strategy.wait_for_command_to_start(cmd)
end

def wait_for_job_to_finish
  $strategy.wait_for_job_to_finish()
end

def assert_job_log(expected_log)
  $strategy.assert_job_log(expected_log)
end

def finished_callback_url
  $strategy.finished_callback_url
end

def teardown_callback_url
  $strategy.teardown_callback_url
end

def shutdown_agent
  $strategy.shutdown_agent
end

def wait_for_agent_to_shutdown
  $strategy.wait_for_agent_to_shutdown
end

def assert_artifact_is_available
  system "artifact pull job agent/job_logs.txt"
  if $?.exitstatus == 0
    abort "agent/job_logs.txt does not exist"
  else
    echo "agent/job_logs.txt exists!"
  end
end

def bad_callback_url
  "https://httpbin.org/status/500"
end
