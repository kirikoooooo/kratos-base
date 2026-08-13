MAIN := ./app/agent-runtime/cmd
BIN_DIR := bin
# Prefer GVM's selected Go binary when GOROOT is set. Shell command lookup can
# otherwise resolve a stale Go toolchain that is incompatible with GOROOT.
GO ?= $(if $(GOROOT),$(GOROOT)/bin/go,go)
# Isolate build cache by the active Go version so GVM/toolchain switches do
# not reuse incompatible compiled standard-library objects.
GO_VERSION := $(shell $(GO) env GOVERSION | tr -cd '[:alnum:]._-')
GO_CACHE_DIR ?= /private/tmp/kratos-go-cache-$(GO_VERSION)
GO_RUN = GOCACHE=$(GO_CACHE_DIR) $(GO)

# Windows 与 macOS/Linux 分别选择 protoc 与二进制后缀
ifeq ($(OS),Windows_NT)
	PROTOC ?= C:\Users\lixiongfei\AppData\Local\Microsoft\WinGet\Packages\Google.Protobuf_Microsoft.Winget.Source_8wekyb3d8bbwe\bin\protoc.exe
	BIN_EXT := .exe
else
	PROTOC ?= protoc
	BIN_EXT :=
endif

BIN := $(BIN_DIR)/kratos-demo$(BIN_EXT)
BIN_MAC_ARM64 := $(BIN_DIR)/kratos-demo-darwin-arm64
BIN_MAC_AMD64 := $(BIN_DIR)/kratos-demo-darwin-amd64
BIN_WIN := $(BIN_DIR)/kratos-demo.exe

DIST_WIN := dist/AgentCli-win64
DIST_MAC := dist/AgentCli-mac-arm64
RELEASE_NAME := AgentCli

.PHONY: config proto wire build build-bin build-mac build-win release-win release-mac cli cli-bin run run-bin dashboard memory

config:
	$(PROTOC) --proto_path=. --go_out=paths=source_relative:. internal/conf/conf.proto

proto:
	$(PROTOC) --proto_path=. --proto_path=third_party --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. --go-http_out=paths=source_relative:. api/task/v1/task.proto api/memory/v1/memory.proto

wire:
	cd app/agent-runtime/cmd && wire

# 编译全部 Go 包（不含可执行文件产物）
build:
	$(GO_RUN) build ./...

# 当前平台原生二进制（mac/Linux: bin/kratos-demo，Windows: bin/kratos-demo.exe）
build-bin:
	@mkdir -p $(BIN_DIR)
	$(GO_RUN) build -o $(BIN) $(MAIN)
	@echo "built $(BIN)"

# macOS 可执行文件（Apple Silicon + Intel）
build-mac:
	@mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=arm64 $(GO_RUN) build -o $(BIN_MAC_ARM64) $(MAIN)
	GOOS=darwin GOARCH=amd64 $(GO_RUN) build -o $(BIN_MAC_AMD64) $(MAIN)
	@echo "built $(BIN_MAC_ARM64)"
	@echo "built $(BIN_MAC_AMD64)"

# Windows 可执行文件（可在 macOS 上交叉编译）
build-win:
	@mkdir -p $(BIN_DIR)
	GOOS=windows GOARCH=amd64 $(GO_RUN) build -o $(BIN_WIN) $(MAIN)
	@echo "built $(BIN_WIN)"

# 可分发给 Windows 用户的目录：exe + configs + 说明
release-win: build-win
	@mkdir -p $(DIST_WIN)/configs
	cp $(BIN_WIN) $(DIST_WIN)/$(RELEASE_NAME).exe
	@echo "app/agent-runtime/configs/ is generated on the first run"
	@echo "release ready: $(DIST_WIN)/"
	@echo "  $(RELEASE_NAME).exe -cli"

release-mac: build-bin
	@mkdir -p $(DIST_MAC)/configs
	cp $(BIN) $(DIST_MAC)/$(RELEASE_NAME)
	@echo "app/agent-runtime/configs/ is generated on the first run"
	@echo "release ready: $(DIST_MAC)/"
	@echo "  ./$(RELEASE_NAME) -cli"

cli:
	$(GO_RUN) run $(MAIN) -conf ./app/agent-runtime/configs -cli $(ARGS)

# 使用已编译的原生二进制启动 CLI（先 make build-bin）
cli-bin: build-bin
	$(BIN) -cli

run:
	$(GO_RUN) run $(MAIN) -conf ./app/agent-runtime/configs

run-bin: build-bin
	$(BIN)

# Dashboard is a control-plane BFF. It requires agent-runtime gRPC at :9000.
dashboard:
	$(GO_RUN) run ./app/dashboard/cmd

# Memory exposes its read-side prompt-context gRPC API at :9002 by default.
memory:
	$(GO_RUN) run ./app/memory/cmd
