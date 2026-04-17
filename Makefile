BINARY := gh-action-lint

.PHONY: default build fmt test clean

default: fmt test build

build:
	go build -o $(BINARY) .

fmt:
	gofmt -w .

test:
	go test -cover ./...

clean:
	rm -f $(BINARY)
