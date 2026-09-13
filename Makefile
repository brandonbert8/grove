.PHONY: build test vet fmt lint run-hello grove-version tidy build-cli

build:
	go build ./...

# build-cli stamps the release version into the binary.
# VERSION defaults to dev; releases pass VERSION=vX.Y.Z.
VERSION ?= dev
build-cli:
	go build -ldflags "-X github.com/brandonbert8/grove/packages/cli.versionOverride=$(VERSION)" -o grove ./cmd/grove

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
