MAIN := ./cmd/kratos-demo
BIN_DIR := bin

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

.PHONY: config proto wire build build-bin build-mac build-win cli cli-bin run run-bin

config:
	$(PROTOC) --proto_path=. --go_out=paths=source_relative:. internal/conf/conf.proto

proto:
	$(PROTOC) --proto_path=. --proto_path=third_party --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. --go-http_out=paths=source_relative:. api/task/v1/task.proto

wire:
	cd cmd/kratos-demo && wire

# 编译全部 Go 包（不含可执行文件产物）
build:
	go build ./...

# 当前平台原生二进制（mac/Linux: bin/kratos-demo，Windows: bin/kratos-demo.exe）
build-bin:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) $(MAIN)
	@echo "built $(BIN)"

# macOS 可执行文件（Apple Silicon + Intel）
build-mac:
	@mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=arm64 go build -o $(BIN_MAC_ARM64) $(MAIN)
	GOOS=darwin GOARCH=amd64 go build -o $(BIN_MAC_AMD64) $(MAIN)
	@echo "built $(BIN_MAC_ARM64)"
	@echo "built $(BIN_MAC_AMD64)"

# Windows 可执行文件（可在 macOS 上交叉编译）
build-win:
	@mkdir -p $(BIN_DIR)
	GOOS=windows GOARCH=amd64 go build -o $(BIN_WIN) $(MAIN)
	@echo "built $(BIN_WIN)"

cli:
	go run $(MAIN) -conf ./configs -cli

# 使用已编译的原生二进制启动 CLI（先 make build-bin）
cli-bin: build-bin
	$(BIN) -conf ./configs -cli

run:
	go run $(MAIN) -conf ./configs

run-bin: build-bin
	$(BIN) -conf ./configs
