BINARY := tdl
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE := $(shell git log -1 --format=%cd --date=format:%Y-%m-%d 2>/dev/null || echo unknown)
LDFLAGS := -s -w \
	-X github.com/iyear/tdl/pkg/consts.Version=$(VERSION) \
	-X github.com/iyear/tdl/pkg/consts.Commit=$(COMMIT) \
	-X github.com/iyear/tdl/pkg/consts.CommitDate=$(DATE)
BIN_DIR := bin

.PHONY: build build-windows build-linux build-darwin build-all clean release packaging

# build for the current host platform
build:
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-$(VERSION)-$$(go env GOOS)-$$(go env GOARCH) .

# cross-compile for Windows x64
build-windows:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-$(VERSION)-windows-amd64.exe .

# cross-compile for Linux x64
build-linux:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-$(VERSION)-linux-amd64 .

# cross-compile for macOS (both arches)
build-darwin:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-$(VERSION)-darwin-amd64 .
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-$(VERSION)-darwin-arm64 .

# build all common platforms
build-all: build-windows build-linux build-darwin build

clean:
	rm -rf $(BIN_DIR)

# official goreleaser targets (require goreleaser installed)
release:
	goreleaser build --rm-dist --single-target --snapshot
	@echo "go to '.tdl/dist' directory to see the package!"

packaging:
	goreleaser release --skip-publish --auto-snapshot --rm-dist
	@echo "go to '.tdl/dist' directory to see the packages!"
