#!/bin/ruby
# rubocop:disable all

# A job stopped while its pod is starting leaves nothing behind in the cluster.
# The pod spec decorator makes the pod unschedulable, so it stays pending
# until the job is stopped.

CONFIG_MAP = "e2e-unschedulable-pod"

$AGENT_CONFIG = {
  "endpoint" => "localhost:4567",
  "token" => "321h1l2jkh1jk42341",
  "no-https" => true,
  "shutdown-hook-path" => "",
  "disconnect-after-job" => false,
  "env-vars" => [],
  "files" => [],
  "fail-on-missing-files" => false,
  "kubernetes-executor" => true,
  "kubernetes-default-image" => "ruby:3-slim",
  "kubernetes-pod-spec" => CONFIG_MAP,
  "kubernetes-pod-start-timeout" => 600
}

require_relative '../../e2e'
require_relative '../../e2e_support/kubernetes'

# The pod spec is loaded when the job starts,
# so the config map only needs to exist by then.
# /tmp/agent is mounted in the agent container.
File.write("/tmp/agent/unschedulable-pod.yaml", <<-YAML)
apiVersion: v1
kind: ConfigMap
metadata:
  name: #{CONFIG_MAP}
  namespace: default
data:
  pod: |
    nodeSelector:
      semaphoreci.com/e2e-node-that-does-not-exist: "true"
YAML

run_kubectl("apply -f /tmp/agent/unschedulable-pod.yaml")
at_exit { system("#{kubectl} delete configmap #{CONFIG_MAP} -n default --ignore-not-found") }

start_job <<-JSON
  {
    "job_id": "#{$JOB_ID}",
    "executor": "shell",
    "env_vars": [],
    "files": [],
    "commands": [
      { "directive": "echo 'should not run'" }
    ],

    "epilogue_always_commands": [],

    "callbacks": {
      "finished": "#{finished_callback_url}",
      "teardown_finished": "#{teardown_callback_url}"
    },
    "logger": #{$LOGGER}
  }
JSON

wait_for_command_to_start("Starting shell session...")
wait_for_job_pod_to_exist
assert_job_resources_exist

phase = run_kubectl("get pod semaphore-job-#{$JOB_ID} -n default -o jsonpath='{.status.phase}'").strip
abort "(fail) expected the job pod to be pending, but it is '#{phase}'" if phase != "Pending"

stop_job
wait_for_job_to_finish

assert_job_result("stopped")
assert_no_job_resources_left
