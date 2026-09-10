VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

BIN := addch
DIST := dist

.PHONY: all build test vet lint clean dist install

all: build

build:
	go build $(LDFLAGS) -o ./addch ./cmd/addch
	go build $(LDFLAGS) -o ./rmch ./cmd/rmch
	go build $(LDFLAGS) -o ./getch ./cmd/getch

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
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/rmch-linux-amd64 ./cmd/rmch
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/rmch-linux-arm64 ./cmd/rmch
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/rmch-darwin-amd64 ./cmd/rmch
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/rmch-darwin-arm64 ./cmd/rmch
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/rmch-windows-amd64.exe ./cmd/rmch
	GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/rmch-windows-arm64.exe ./cmd/rmch
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/getch-linux-amd64 ./cmd/getch
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/getch-linux-arm64 ./cmd/getch
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/getch-darwin-amd64 ./cmd/getch
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/getch-darwin-arm64 ./cmd/getch
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/getch-windows-amd64.exe ./cmd/getch
	GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/getch-windows-arm64.exe ./cmd/getch
	cd $(DIST) && sha256sum addch-* rmch-* getch-* > SHA256SUMS.txt

install:
	go install $(LDFLAGS) ./cmd/addch ./cmd/rmch ./cmd/getch

clean:
	rm -f $(BIN) ./rmch ./getch
	rm -rf $(DIST)
