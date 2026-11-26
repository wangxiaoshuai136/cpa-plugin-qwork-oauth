PLUGIN_ID := qwenwork

.PHONY: test build vet clean

test:
	go vet ./...
	go test ./...

build:
	CGO_ENABLED=1 go build -buildmode=c-shared -trimpath -ldflags "-s -w" -o bin/$(PLUGIN_ID).dll ./cmd/qwenwork-plugin

vet:
	go vet ./...
