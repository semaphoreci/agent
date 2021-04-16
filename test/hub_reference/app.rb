# rubocop:disable all

require "sinatra"

set :bind, "0.0.0.0"

$registered = false

get "/is_alive" do
  "yes"
end

get "/private/is_registered" do
  $registered ? "yes" : "no"
end

post "/register" do
  $registered = true
end

post "/hearthbeat" do
  $hearthbeat = true
end
