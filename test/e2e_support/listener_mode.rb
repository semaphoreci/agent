# rubocop:disable all

class ListenerMode

  HUB_ENDPOINT = "http://localhost:4567"

  def boot_up_agent
    system "docker stop $(docker ps -q)"
    system "docker rm $(docker ps -qa)"
    system "docker-compose -f test/e2e_support/docker-compose-listen.yml build"
    system "docker-compose -f test/e2e_support/docker-compose-listen.yml up -d"

    wait_for_agent_to_register_in_the_hub
  end

  def start_job(request)
    File.write("/tmp/j1", request.to_json)

    system "curl -X POST -H 'Content-Type: application/json' -d @/tmp/j1 #{HUB_ENDPOINT}/private/schedule_job"
  end

  def wait_for_command_to_start(cmd)
    sleep 3
    false
  end

  def wait_for_job_to_finish
    puts "Waiting for job to finish "

    loop do
      print "."

      response = `curl -s --fail -X GET -k "#{HUB_ENDPOINT}/private/jobs/#{$JOB_ID}/is_finished"`

      if response == "yes"
        return
      else
        sleep 1
      end
    end

    sleep 5

    puts
  end

  def stop_job
    output = `curl -H --fail -X POST -k "#{HUB_ENDPOINT}/private/schedule_stop/#{$JOB_ID}"`

    abort "Failed to stob job: #{output}" if $?.exitstatus != 0
  end

  def assert_job_log(expected_log)
    puts "========================="
    puts "Asserting Job Logs"

    sleep 3

    actual_log = `curl -s #{HUB_ENDPOINT}/private/jobs/#{$JOB_ID}/logs`

    puts "-----------------------------------"
    puts actual_log
    puts "-----------------------------------"

    abort "Failed to fetch logs: #{actual_log}" if $?.exitstatus != 0

    actual_log   = actual_log.split("\n").map(&:strip).reject(&:empty?)
    expected_log = expected_log.split("\n").map(&:strip).reject(&:empty?)

    index_in_actual_logs = 0
    index_in_expected_logs = 0

    while index_in_actual_logs < actual_log.length && index_in_expected_logs < expected_log.length
      begin
        puts "Comparing log lines Actual=#{index_in_actual_logs} Expected=#{index_in_expected_logs}"

        expected_log_line = expected_log[index_in_expected_logs]
        actual_log_line   = actual_log[index_in_actual_logs]

        puts "  actual:   #{actual_log_line}"
        puts "  expected: #{expected_log_line}"

        actual_log_line_json = Hash[JSON.parse(actual_log_line).sort]

        if expected_log_line =~ /\*\*\* LONG_OUTPUT \*\*\*/
          if actual_log_line_json["event"] == "cmd_output"
            # if we have a *** LONG_OUTPUT *** marker

            # we go to next actual log line
            # but we stay on the same expected log line

            index_in_actual_logs += 1

            next
          else
            # end of the LONG_OUTPUT marker, we increase the expected log line
            # and in the next iteration we will compare regularly again
            index_in_expected_logs += 1

            next
          end
        else
          # if there is no marker, we compare the JSONs
          # we ignore the timestamps because they change every time

          expected_log_line_json = Hash[JSON.parse(expected_log_line).sort]

          if expected_log_line_json.keys != actual_log_line_json.keys
            abort "(fail) JSON keys are different."
          end

          expected_log_line_json.keys.each do |key|
            # Special case when we want to ignore only the output
            # ignore expected entries with '*'

            next if expected_log_line_json[key] == "*"

            if expected_log_line_json[key] != actual_log_line_json[key]
              abort "(fail) Values for '#{key}' are not equal."
            end
          end

          index_in_actual_logs += 1
          index_in_expected_logs += 1
        end

        puts "success"
      rescue
        puts ""
        puts "Line Number: Actual=#{index_in_actual_logs} Expected=#{index_in_expected_logs}"
        puts "Expected: '#{expected_log_line}'"
        puts "Actual:   '#{actual_log_line}'"

        abort "(fail) Failed to parse log line"
      end
    end

    if index_in_actual_logs != actual_log.length
      abort "(fail) There are unchecked log lines from the actual log"
    end

    if index_in_expected_logs != expected_log.length
      abort "(fail) There are unchecked log lines from the expected log"
    end
  end

  def finished_callback_url
    "http://hub:4567/jobs/#{$JOB_ID}/callbacks/finished"
  end

  def teardown_callback_url
    "http://hub:4567/jobs/#{$JOB_ID}/callbacks/finished"
  end

  private

  def wait_for_agent_to_register_in_the_hub
    puts "Waiting for agent to register in the hub "

    loop do
      print "."

      response = `curl -s --fail -X GET -k "#{HUB_ENDPOINT}/private/is_registered"`

      if response == "yes"
        break
      else
        sleep 1
      end
    end

    puts
  end

end
