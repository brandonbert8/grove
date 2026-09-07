.PHONY: build test vet fmt lint run-hello grove-version tidy

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

run-hello:
	go run ./examples/hello

grove-version:
	go run ./cmd/grove version
