VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

BIN := addch
DIST := dist

.PHONY: all build test vet lint clean dist install

all: build

build:
	go build $(LDFLAGS) -o $(BIN) ./cmd/addch

test:
	go test ./...

vet:
	go vet ./...

lint: vet

# Build binaries for all supported platforms into ./dist
dist:
	mkdir -p $(DIST)
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/addch-linux-amd64 ./cmd/addch
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/addch-linux-arm64 ./cmd/addch
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/addch-darwin-amd64 ./cmd/addch
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/addch-darwin-arm64 ./cmd/addch
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/addch-windows-amd64.exe ./cmd/addch
	GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/addch-windows-arm64.exe ./cmd/addch

install:
	go install $(LDFLAGS) ./cmd/addch

clean:
	rm -f $(BIN)
	rm -rf $(DIST)
