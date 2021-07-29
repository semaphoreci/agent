# rubocop:disable all

require "sinatra"
require "json"

$stdout.sync = true

set :bind, "0.0.0.0"
set :logging, false

$registered = false
$jobs = []
$payloads = {}
$job_states = {}
$finished = {}
$teardown = {}
$logs = []

before do
  logger.level = 0

  begin
    request.body.rewind

    @json_request = JSON.parse(request.body.read)
  rescue StandardError => e
  end
end

#
# The official API that is used by the agent to
# connect to Semaphore 2.0
#

post "/api/v1/self_hosted_agents/register" do
  puts "[SYNC] Registration received"
  $registered = true

  {
    "access_token" => "dsjfaklsd123412341",
  }.to_json
end

post "/api/v1/self_hosted_agents/sync" do
  puts "[SYNC] Request #{@json_request.to_json}"

  response = case @json_request["state"]
            when "waiting-for-jobs"
              if $jobs.size > 0
                job = $jobs.shift

                {"action" => "run-job", "job_id" => job["id"]}
              else
                {"action" => "continue"}
              end
            when "running-job"
              job_id = @json_request["job_id"]
              if $job_states[job_id] == "stopping"
                {"action" => "stop-job"}
              else
                {"action" => "continue"}
              end
            when "stopping-job"
              {"action" => "continue"}
            when "finished-job"
              {"action" => "continue"}
            when "starting-job"
              {"action" => "continue"}
            when "failed-to-send-callback"
              job_id = @json_request["job_id"]
              $job_states[job_id] = "stuck"
              {"action" => "continue"}
            when "failed-to-fetch-job"
              job_id = @json_request["job_id"]
              puts "JOBID: #{job_id}"
              $job_states[job_id] = "stuck"
              {"action" => "continue"}
            when "failed-to-construct-job"
              $job_states[job_id] = "stuck"
              {"action" => "continue"}
            else
              raise "unknown state"
            end

  puts "[SYNC] Response #{response.to_json}"
  response.to_json
end

get "/api/v1/self_hosted_agents/jobs/:id" do
  job_id = params["id"]

  if job_id == "bad-job-id"
    halt 500, "error"
  else
    $payloads[params["id"]].to_json
  end
end

get "/api/v1/self_hosted_agents/jobs/:id/status" do
  $job_states[params["id"]]
end

post "/api/v1/logs/:id" do
  request.body.rewind
  events = request.body.read.split("\n")

  puts "Received #{events.length()} log events"
  $logs += events
  status 200
end

post "/jobs/:id/callbacks/finished" do
  puts "[CALLBACK] Finished job #{params["id"]}"
  $job_states[params["id"]] = "finished"
  status 202
end

post "/jobs/:id/callbacks/teardown" do
  $teardown[params["id"]] = true
  status 202
end

#
# Private APIs. Only needed to contoll the flow
# of e2e tests in the Agent.
#

get "/is_alive" do
  "yes"
end

get "/private/is_registered" do
  $registered ? "yes" : "no"
end

get "/private/jobs/:id/logs" do
  puts "Fetching logs"
  puts $logs.join("\n")
  $logs.join("\n")
end

post "/private/schedule_job" do
  job = JSON.parse(@json_request)
  puts "[PRIVATE] Scheduling job #{job["id"]}"

  puts "Scheduled job #{job["id"]}"

  $jobs << job
  $payloads[job["id"]] = job
  $job_states[job["id"]] = "running"
end

post "/private/schedule_stop/:id" do
  puts "Scheduled stop #{params["id"]}"

  $job_states[params["id"]] = "stopping"
end
