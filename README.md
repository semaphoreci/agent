# [WIP] Semaphore 2.0 Agents

Base agent responsibilities:

- [x] Run jobs
- [x] Provide output logs
- [x] Run a server
- [x] Inject env variables
- [x] Inject files
- [x] Run epilogue commands

Compose style CI milestone:

- [ ] Run commands in docker container
- [ ] Start multiple docker containers, connect them via DNS
- [ ] Checkout source code, run tests
- [ ] Inject files and environments variables
- [ ] Pull private docker images
- [ ] Store and restore files from cache
- [ ] Build docker in docker
- [ ] Upload docker images from docker
- [ ] Run code on host, expose docker containers
- [ ] Use Kubernetes as backend


## Agent

### Usage:

```agent [command] [flag]```

Commands:
```
  version   Print Agent version
  serve     Start server
  run       Runs a single job
```
Flags:
```
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
