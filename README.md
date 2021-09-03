# Semaphore 2.0 Agent

Base agent responsibilities:

- Run jobs
- Provide output logs
- Run a server
- Inject env variables
- Inject files
- Run epilogue commands
- Set up an SSH jump-point (only available in hosted environment)

Docker Based CI/CD:

- Run commands in docker container
- Start multiple docker containers, connect them via DNS
- Checkout source code, run tests
- Inject files and environments variables
- Pull private docker images
- Store and restore files from cache (only available in hosted environment for now)
- Build docker in docker
- Upload docker images from docker
- Set up an SSH jump-point (only available in hosted version)

## Usage

This agent is intended to be used in two environments: hosted or self hosted.

### Hosted environment

In the hosted environment, the agent runs inside Semaphore's infrastructure, starting an HTTP server that receives jobs in HTTP requests. The [`agent serve`](#agent-serve-flags) command is used to run the agent in that scenario.

### Self hosted environment

In the self hosted environment, you control where you run it and no HTTP server is started; that way, all communication happens from the agent to Semaphore and no HTTP endpoint is required to exist inside your infrastructure.

The [`agent start`](#agent-start-flags) command is used to run the agent in that scenario.

## Commands

### `agent start [flags]`

Starts the agent in a self hosted environment. The agent will register itself with Semaphore and periodically sync with Semaphore. No HTTP server is started and exposed. Read more about it [in the docs](https://docs.semaphoreci.com/ci-cd-environment/self-hosted-agents-overview).

### `agent serve [flags]`

Starts the agent as an HTTP server that receives requests in HTTP requests. Intended only to run jobs in Semaphore's own infrastructure. If you are looking for the command to run the agent in a self hosted environment, check [`agent start`](#agent-start-params).

Flags:

```txt
 --auth-token-secret  Auth token for accessing the server (required)
 --port               Set a custom port (default 8000)
 --host               Set the bind address to a specific IP (default 0.0.0.0)
 --tls-cert-path      Path to TLS Certificate (default `pwd/server.crt`)
 --tls-key-path       Path to TLS Private key (default `pwd/server.key`)
 --statsd-host        The host where to send StatsD metrics.
 --statsd-port        The port where to send StatsD metrics.
 --statsd-namespace   The prefix to be added to every StatsD metric.
```

Start with defaults:

```
agent serve --auth-token-secret 'myJwtToken'
```

When a job starts, the public SSH keys sent with the Job Request are injected into the `~/.ssh/authorized_keys`.

After that, a jump point for accessing the job is set up. For shell based
executors this is a simple `bash --login`. For docker compose based executors,
this is a more complex script that waits for the container to start up and
executes `docker-compose exec <name> bash --login`.

To SSH into an agent, use:

```bash
ssh -t -p <port> <ip> bash /tmp/ssh_jump_point
```

If configured, the Agent can publish the following StatsD metrics:

- compose_ci.docker_pull.duration, tagged with: [image name]
- compose_ci.docker_pull.success.rate, tagged with: [image name]
- compose_ci.docker_pull.failure.rate, tagged with: [image name]

To configure Statsd publishing provide the following command-line flags to the Agent:
- stats_host: The host where to send StatsD metrics.
- stats_port: The port where to send StatsD metrics.
- statsd-namespace:  The prefix to be added to every StatsD metric.

Example Usage:

```
agent serve --statsd-host "192.1.1.9" --statsd-port 8192 --statsd-namespace "agent.prod"
```

If StatsD flags are not provided, the Agent will not publish any StatsD metric.

### `agent run [path]`

Runs a single job. Useful for debugging or agent development. It takes the path to the job request YAML file as an argument

### `agent version`

Prints out the agent version
