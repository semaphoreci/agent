# rubocop:disable all

Encoding.default_external = Encoding::UTF_8

require 'tempfile'
require 'json'
require 'timeout'
require 'base64'

require_relative "./e2e_support/api_mode"
require_relative "./e2e_support/listener_mode"

$JOB_ID = `uuidgen`.strip

$strategy = nil

case ENV["TEST_MODE"]
when "api"    then $strategy = ApiMode.new
when "listen" then $strategy = ListenerMode.new
else raise "Testing Mode not set"
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
