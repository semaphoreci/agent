$AGENT_CONFIG = {
  "endpoint" => "hub:4567",
  "token" => "321h1l2jkh1jk42341",
  "no-https" => true,
  "shutdown-hook-path" => "",
  "disconnect-after-job" => false,
  "disconnect-after-idle-timeout" => 30,
  "env-vars" => [],
  "files" => [],
  "fail-on-missing-files" => false
}

require_relative '../../e2e'

start_job <<-JSON
  {
    "id": "#{$JOB_ID}",
    "env_vars": [],
    "files": [],
    "commands": [
      { "directive": "sleep 5" }
    ],

    "epilogue_always_commands": [],

    "callbacks": {
      "finished": "#{finished_callback_url}",
      "teardown_finished": "#{teardown_callback_url}"
    },
    "logger": #{$LOGGER}
  }
JSON

wait_for_agent_to_shutdown
