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

def assert_artifact_is_compressed
  puts "Checking if artifact is available and compressed"

  # We give 20s for the artifact to appear here, to give the agent enough time
  # to realize the "archivator" has reached out for the logs, and can close the logger.
  Timeout.timeout(20) do
    loop do
      `artifact pull job agent/job_logs.txt.gz -f -d job_logs.gz && (gunzip -c job_logs.gz | tail -n1 | grep -q "Exporting SEMAPHORE_JOB_RESULT")`
      if $?.exitstatus == 0
        puts "sucess: agent/job_logs.txt.gz exists and is compressed!"
        break
      else
        print "."
        sleep 2
      end
    end
  end
end

def assert_artifact_is_available
  puts "Checking if artifact is available"

  # We give 20s for the artifact to appear here, to give the agent enough time
  # to realize the "archivator" has reached out for the logs, and can close the logger.
  Timeout.timeout(20) do
    loop do
      `artifact pull job agent/job_logs.txt`
      if $?.exitstatus == 0
        puts "sucess: agent/job_logs.txt exists!"
        break
      else
        print "."
        sleep 2
      end
    end
  end
end

def assert_artifact_is_not_available

  # We sleep here to make sure the agent has enough time to realize
  # the "archivator" has reached out for the logs, and can close the logger.
  puts "Waiting 20s to check if artifact exists..."
  sleep 20

  `artifact pull job agent/job_logs.txt`
  if $?.exitstatus == 0
    abort "agent/job_logs.txt artifact exists, but shouldn't!"
  else
    puts "sucess: agent/job_logs.txt does not exist"
  end
end

def bad_callback_url
  "https://httpbingo.org/status/500"
end
