run:
	go run *.go run $(JOB)
.PHONY: run

serve:
	go run *.go serve
.PHONY: serve

test:
	go test -short -v ./...
.PHONY: test

build:
	rm -rf build
	go build -o build/agent main.go
.PHONY: build

docker.build: build
	-docker stop agent
	-docker rm agent
	-docker rmi agent
	docker build -t agent -f Dockerfile.test .
.PHONY: docker.build

docker.run: docker.build
	-docker stop agent
	docker run --net=host --name agent -tdi agent
	sleep 2
.PHONY: docker.run

e2e: docker.run
	ruby test/$(NAME).rb
.PHONY: e2e
