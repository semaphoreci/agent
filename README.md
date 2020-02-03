# Semaphore 2.0 Agents

Base agent responsibilities:

- [x] Run jobs
- [x] Provide output logs
- [x] Run a server
- [x] Inject env variables
- [x] Inject files
- [x] Run epilogue commands
- [x] Set up an SSH jump-point

Docker Based CI/CD:

- [x] Run commands in docker container
- [x] Start multiple docker containers, connect them via DNS
- [x] Checkout source code, run tests
- [x] Inject files and environments variables
- [x] Pull private docker images
- [x] Store and restore files from cache
- [x] Build docker in docker
- [x] Upload docker images from docker
- [x] Set up an SSH jump-point

## Usage

``` bash
agent [command] [flag]
```

Commands:

``` txt
  version   Print Agent version
  serve     Start server
  run       Runs a single job
```

Flags:

``` txt
 --auth-token-secret  Auth token for accessing the server (required)
 --port               Set a custom port (default 8000)
 --host               Set the bind address to a specific IP (default 0.0.0.0)
 --tls-cert-path      Path to TLS Certificate (default `pwd/server.crt`)
 --tls-key-path       Path to TLS Private key (default `pwd/server.key`)
```

Start with defaults:

```
agent serve --auth-token-secret 'myJwtToken'
```

## SSH jump-points

When a job starts, the public SSH keys sent with the Job Request are injected
into the '~/.ssh/authorized_keys'.

After that, a jump point for accessing the job is set up. For shell based
executors this is a simple `bash --login`. For docker compose based executors,
this is a more complex script that waits for the container to start up and
executes `docker-compose exec <name> bash --login`.

To SSH into an agent, use:

``` bash
ssh -t -p <port> <ip> bash /tmp/ssh_jump_point
```

