# rubocop:disable all

require "sinatra"
require "json"

$stdout.sync = true

set :bind, "0.0.0.0"

$registered = false
$jobs = []
$job_states = {}
$finished = {}
$teardown = {}
$logs = ""

before do
  begin
    request.body.rewind

    @json_body = JSON.parse(request.body.read)
  rescue StandardError => e
  end
end

#
# The official API that is used by the agent to
# connect to Semaphore 2.0
#

post "/api/v1/self_hosted_agents/register" do
  $registered = true

  {
    "access_token" => "dsjfaklsd123412341",
  }.to_json
end

post "/api/v1/self_hosted_agents/hearthbeat" do
  $hearthbeat = true
end

post "/api/v1/self_hosted_agents/acquire" do
  if $jobs.size > 0
    job = $jobs.shift
    puts JSON.parse(job)["id"]
    $job_states[JSON.parse(job)["id"]] = "started"
    job
  else
    status 404
  end
end

get "/jobs/:id/status" do
  $job_states[params["id"]]
end

post "/jobs/:id/callbacks/finished" do
  $job_states[params["id"]] = "finished"
end

post "/jobs/:id/callbacks/teardown" do
  $teardown[params["id"]] = true
end

post "/jobs/:id/logs" do
  request.body.rewind
  events = request.body.read

  $logs += events

  puts "incomming"
  puts events

  status 200
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
  $logs
end

post "/private/schedule_job" do
  puts "Scheduled job #{@json_body["id"]}"

  $jobs << @json_body
end

post "/private/schedule_stop/:id" do
  puts "CCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
  puts "Scheduled stop #{params["id"]}"

  $job_states[params["id"]] = "stopping"
end
