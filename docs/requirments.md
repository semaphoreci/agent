# Requirments

This doc lists and explains the reasons for deprecating our current Job Runner
in favor of Semaphore Agents.

Topics:

- Agents in enterprise installations
- Running compose style CI and executing commands without Net::SSH
- Log collection and Live log for Agents (exploration of the resumable log collection to increase stability)
- DevOps overhead with Ruby
- Running Agents inside vs. outside of KVM images
- Open source code base for Agents and how to handle proprietery KVM managment
- Installation of Agents and security
- Support for multiple & extendable Agent backends
  - Kubernetes backend
  - NoVM backend
  - KVM backend
  - Docker backend
  - SSH backend
  - Docker Compose backend
  - Docker swarm backend
  - iOS
  - Windows
