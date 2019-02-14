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
