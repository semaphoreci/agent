.PHONY: e2e
AGENT_PORT_IN_TESTS=30000
AGENT_SSH_PORT_IN_TESTS=2222

SECURITY_TOOLBOX_BRANCH ?= master
SECURITY_TOOLBOX_TMP_DIR ?= /tmp/security-toolbox

AGENT_VERSION=dev
ifneq ($(SEMAPHORE_GIT_TAG_NAME),)
	AGENT_VERSION=$(SEMAPHORE_GIT_TAG_NAME)
endif

check.prepare:
	rm -rf $(SECURITY_TOOLBOX_TMP_DIR)
	git clone git@github.com:renderedtext/security-toolbox.git $(SECURITY_TOOLBOX_TMP_DIR) && (cd $(SECURITY_TOOLBOX_TMP_DIR) && git checkout $(SECURITY_TOOLBOX_BRANCH) && cd -)

check.static: check.prepare
	docker run -it -v $$(pwd):/app \
		-v $(SECURITY_TOOLBOX_TMP_DIR):$(SECURITY_TOOLBOX_TMP_DIR) \
		registry.semaphoreci.com/ruby:2.7 \
		bash -c 'cd /app && $(SECURITY_TOOLBOX_TMP_DIR)/code --language go -d'

check.deps: check.prepare
	docker run -it -v $$(pwd):/app \
		-v $(SECURITY_TOOLBOX_TMP_DIR):$(SECURITY_TOOLBOX_TMP_DIR) \
		registry.semaphoreci.com/ruby:2.7 \
		bash -c 'cd /app && $(SECURITY_TOOLBOX_TMP_DIR)/dependencies --language go -d'

check.docker: check.prepare
	docker run -it -v $$(pwd):/app \
		-v $(SECURITY_TOOLBOX_TMP_DIR):$(SECURITY_TOOLBOX_TMP_DIR) \
		-v /var/run/docker.sock:/var/run/docker.sock \
		registry.semaphoreci.com/ruby:2.7 \
		bash -c 'cd /app && $(SECURITY_TOOLBOX_TMP_DIR)/docker -d --image semaphoreci/agent:latest --skip-files Dockerfile.ecr,Dockerfile.test,Dockerfile.dev,test/hub_reference/Dockerfile,Dockerfile.empty_ubuntu'

lint:
	revive -formatter friendly -config lint.toml ./...

run:
	go run *.go run $(JOB)
.PHONY: run

serve:
	go run *.go serve --auth-token-secret 'TzRVcspTmxhM9fUkdi1T/0kVXNETCi8UdZ8dLM8va4E'
.PHONY: serve

test:
	gotestsum --format short-verbose --junitfile junit-report.xml --packages="./..." -- -p 1
.PHONY: test

test.bench:
	go test -benchmem -run=^$$ -bench . ./... -count=10

build:
	rm -rf build
	env GOOS=linux GOARCH=386 go build -o build/agent main.go
.PHONY: build

build.windows:
	rm -rf build
	env GOOS=windows GOARCH=amd64 go build -o build/agent.exe main.go

e2e: build
	ruby test/e2e/$(TEST).rb

e2e.listen.mode.logs:
	docker-compose -f test/e2e_support/docker-compose-listen.yml logs -f

#
# An ubuntu environment that has the ./build/agent CLI mounted.
# This environment is ideal for testing self-hosted agents without the fear
# that some runaway command will mess up your dev environment.
#
empty.ubuntu.machine:
	docker run --rm -v $(PWD):/app -ti empty-ubuntu-self-hosted-agent /bin/bash

empty.ubuntu.machine.build:
	docker build -f Dockerfile.empty_ubuntu -t empty-ubuntu-self-hosted-agent .

ecr.test.build:
	docker build -f Dockerfile.ecr -t agent-testing .

ecr.test.push:
	aws ecr get-login-password --region $(AWS_REGION) | docker login --username AWS --password-stdin $(AWS_ECR_REGISTRY)
	docker tag agent-testing:latest $(AWS_ECR_REGISTRY)/agent-testing:latest
	docker push $(AWS_ECR_REGISTRY)/agent-testing:latest

# Helpful when developing and testing locally
docker.build.dev:
	docker build -f Dockerfile.dev -t semaphoreci/agent:dev .

#
# Docker Release
#
docker.build:
	env GOOS=linux GOARCH=amd64 go build -ldflags='-s -w -X "main.VERSION=$(AGENT_VERSION)"' -o build/agent main.go
	docker build -f Dockerfile.self_hosted -t semaphoreci/agent:latest .

docker.push:
	docker tag semaphoreci/agent:latest semaphoreci/agent:$(AGENT_VERSION)
	docker push semaphoreci/agent:$(AGENT_VERSION)
	docker push semaphoreci/agent:latest

release.major:
	git fetch --tags
	latest=$$(git tag | sort --version-sort | tail -n 1); new=$$(echo $$latest | cut -c 2- | awk -F '.' '{ print "v" $$1+1 ".0.0" }');          echo $$new; git tag $$new; git push origin $$new

release.minor:
	git fetch --tags
	latest=$$(git tag | sort --version-sort | tail -n 1); new=$$(echo $$latest | cut -c 2- | awk -F '.' '{ print "v" $$1 "." $$2 + 1 ".0" }');  echo $$new; git tag $$new; git push origin $$new

release.patch:
	git fetch --tags
	latest=$$(git tag | sort --version-sort | tail -n 1); new=$$(echo $$latest | cut -c 2- | awk -F '.' '{ print "v" $$1 "." $$2 "." $$3+1 }'); echo $$new; git tag $$new; git push origin $$new
