run:
	go run *.go run $(JOB)

serve:
	go run *.go serve

test:
	go test -short -v ./...
