run:
	go run *.go run $(JOB)

serve:
	go run *.go serve

test:
	go test -short -v ./...

build:
	rm -rf build
	go build -o build/agent main.go

docker.build: build
	-docker stop agent
	-docker rm agent
	-docker rmi agent
	docker build -t agent -f Dockerfile.test .

docker.run: docker.build
	-docker stop agent
	docker run --net=host --name agent -tdi agent
	sleep 2

e2e: docker.run
	bash test/$(NAME).sh

