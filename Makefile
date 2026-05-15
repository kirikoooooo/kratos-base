PROTOC=C:\Users\lixiongfei\AppData\Local\Microsoft\WinGet\Packages\Google.Protobuf_Microsoft.Winget.Source_8wekyb3d8bbwe\bin\protoc.exe

.PHONY: config proto wire build

config:
	$(PROTOC) --proto_path=. --go_out=paths=source_relative:. internal/conf/conf.proto

proto:
	$(PROTOC) --proto_path=. --proto_path=third_party --go_out=paths=source_relative:. --go-grpc_out=paths=source_relative:. --go-http_out=paths=source_relative:. api/task/v1/task.proto

wire:
	cd cmd/kratos-demo && wire

build:
	go build ./...
