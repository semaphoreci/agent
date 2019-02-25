# rubocop:disable all

require 'tempfile'
require 'json'

$JOB_ID = `uuidgen`.strip

# based on secret passed to the running server
$TOKEN = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.gLEycyHdyRRzUpauBxdDFmxT5KoOApFO5MHuvWPgFtY"

def start_job(request)
  r = Tempfile.new
  r.write(request)
  r.close

  puts "============================"
  puts "Sending job request to Agent"

  output = `curl -H "Authorization: Bearer #{$TOKEN}" --fail -X POST -k "https://0.0.0.0:8000/jobs" --data @#{r.path}`

  abort "Failed to send: #{output}" if $?.exitstatus != 0
end

def stop_job
  puts "============================"
  puts "Stopping job..."

  output = `curl -H "Authorization: Bearer #{$TOKEN}" --fail -X POST -k "https://0.0.0.0:8000/jobs/terminate"`

  abort "Failed to stob job: #{output}" if $?.exitstatus != 0
end

def wait_for_job_to_finish
  puts "========================="
  puts "Waiting for job to finish"

  sleep 2
end

def assert_job_log(expected_log)
  puts "========================="
  puts "Asserting Job Logs"

  actual_log = `curl -H "Authorization: Bearer #{$TOKEN}" -k "https://0.0.0.0:8000/jobs/#{$JOB_ID}/log"`

  abort "Failed to fetch logs: #{actual_log}" if $?.exitstatus != 0

  actual_log   = actual_log.split("\n").map(&:strip).reject(&:empty?)
  expected_log = expected_log.split("\n").map(&:strip).reject(&:empty?)

  expected_log.zip(actual_log).each.with_index do |pair, index|
    begin
      puts "Comparing log lines #{index}"

      expected_log_line = pair[0]
      actual_log_line   = pair[1]

      expected_log_line_json = Hash[JSON.parse(expected_log_line).sort]
      actual_log_line_json   = Hash[JSON.parse(actual_log_line).sort]

      puts "  expected: #{expected_log_line_json.to_json}"
      puts "  actual:   #{actual_log_line_json.to_json}"

      if expected_log_line_json.keys != actual_log_line_json.keys
        abort "(fail) JSON keys are different."
      end

      expected_log_line_json.keys.each do |key|
        # ignore expected entries with '*'
        next if expected_log_line_json[key] == "*"

        if expected_log_line_json[key] != actual_log_line_json[key]
          abort "(fail) Values for '#{key}' are not equal."
        end
      end

      puts "success"
    rescue
      puts ""
      puts "Line Number: #{index}"
      puts "Expected: '#{expected_log_line}'"
      puts "Actual:   '#{actual_log_line}'"

      abort "(fail) Failed to parse log line"
    end
  end
end
