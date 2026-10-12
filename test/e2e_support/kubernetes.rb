# rubocop:disable all

# Helpers for the Kubernetes executor e2e tests, which need to check
# what the agent created, and removed, in the cluster.

require 'json'
require 'timeout'

# kubectl runs in the agent container, which uses the same
# cluster configuration as the agent itself.
def kubectl
  "docker compose -f test/e2e_support/docker-compose-listen.yml exec -T agent kubectl"
end

def run_kubectl(args)
  output = `#{kubectl} #{args}`
  abort "(fail) kubectl #{args} failed: #{output}" if $?.exitstatus != 0
  output
end

# Every resource the agent creates for a job has this label.
def job_resources
  run_kubectl("get pods,secrets -n default -l semaphoreci.com/job-id=#{$JOB_ID} -o name")
    .split("\n").map(&:strip).reject(&:empty?)
end

def wait_for_job_pod_to_exist
  puts "Waiting for the job pod to exist"

  Timeout.timeout(120) do
    loop do
      break if job_resources.any? { |r| r.start_with?("pod/") }
      sleep 1
    end
  end

  puts "Job resources: #{job_resources}"
end

def assert_job_resources_exist
  resources = job_resources
  puts "Job resources: #{resources}"

  abort "(fail) the job pod does not exist" unless resources.include?("pod/semaphore-job-#{$JOB_ID}")
  abort "(fail) the job secret does not exist" unless resources.include?("secret/semaphore-job-#{$JOB_ID}-secret")
end

# A deleted pod is still listed while it terminates,
# so we give it some time to go away.
def assert_no_job_resources_left
  puts "Checking that no job resources are left"

  begin
    Timeout.timeout(120) do
      loop do
        resources = job_resources
        break if resources.empty?

        puts "Still there: #{resources}"
        sleep 2
      end
    end
  rescue Timeout::Error
    abort "(fail) job resources were left behind: #{job_resources}"
  end

  puts "success: no job resources left"
end

def assert_job_result(expected)
  puts "Waiting for job_finished event"

  result = nil
  Timeout.timeout(60) do
    loop do
      logs = `curl -s --fail #{ListenerMode::HUB_ENDPOINT}/private/jobs/#{$JOB_ID}/logs`
      event = logs.split("\n").map { |l| JSON.parse(l) rescue nil }.compact.find { |e| e["event"] == "job_finished" }

      if event
        result = event["result"]
        break
      end

      sleep 1
    end
  end

  abort "(fail) expected job result '#{expected}', got '#{result}'" if result != expected
  puts "success: job result is #{result}"
end
