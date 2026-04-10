BINARY  := jisho
GOFLAGS := -trimpath -ldflags="-s -w"

.PHONY: build
build:
	go build $(GOFLAGS) -o $(BINARY) .

.PHONY: install
install:
	go install $(GOFLAGS) .

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: dist
dist:
	mkdir -p dist
	GOOS=linux  GOARCH=amd64 go build $(GOFLAGS) -o dist/$(BINARY)-linux-amd64 .
	GOOS=linux  GOARCH=arm64 go build $(GOFLAGS) -o dist/$(BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) -o dist/$(BINARY)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -o dist/$(BINARY)-windows-amd64.exe .

.PHONY: clean
clean:
	rm -f $(BINARY)
	rm -rf dist/
