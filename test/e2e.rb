# rubocop:disable all

Encoding.default_external = Encoding::UTF_8

require 'tempfile'
require 'json'
require 'timeout'
require 'base64'

require_relative "./e2e_support/api_mode"
require_relative "./e2e_support/listener_mode"

$JOB_ID = `uuidgen`.strip

# based on secret passed to the running server
$TOKEN = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.gLEycyHdyRRzUpauBxdDFmxT5KoOApFO5MHuvWPgFtY"

$AGENT_PORT_IN_TESTS = 30000

$strategy = nil

case ENV["TEST_MODE"]
when "api"    then $strategy = ApiMode.new
when "listen" then $strategy = ListenerMode.new
else raise "Testing Mode not set"
end

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
