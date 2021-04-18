# rubocop:disable all

require "sinatra"
require "json"

set :bind, "0.0.0.0"

$registered = false
$jobs = []

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

post "/private/schedule_job" do
  puts "Scheduled job #{@json_body["id"]}"

  $jobs << @json_body
end
