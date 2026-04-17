BINARY := gh-action-lint

.PHONY: build fmt test clean

build:
	go build -o $(BINARY) .

fmt:
	gofmt -w .

test:
	go test ./...

clean:
	rm -f $(BINARY)
