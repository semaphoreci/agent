# rubocop:disable all

require "sinatra"
require "json"

set :bind, "0.0.0.0"

$registered = false
$jobs = []
$stop_requests = {}
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

post "/register" do
  $registered = true
end

post "/hearthbeat" do
  $hearthbeat = true
end

post "/acquire" do
  if $jobs.size > 0
    $jobs.shift
  else
    status 404
  end
end

post "/jobs/:id/callbacks/finished" do
  $finished[params["id"]] = true
end

post "/jobs/:id/callbacks/teardown" do
  $teardown[params["id"]] = true
end

post "/stream" do
  request.body.rewind
  events = request.body.read

  $logs += events

  puts $logs

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
  puts "Scheduled stop #{params["id"]}"

  $stop_requests[params["id"]] = true
end

get "/private/jobs/:id/is_finished" do
  if $finished[params["id"]]
    "yes"
  else
    "no"
  end
end
