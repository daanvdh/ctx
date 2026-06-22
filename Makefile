BINARY=ctx
MCP_BINARY=ctx-mcp
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) .

build-mcp:
	go build -o $(BUILD_DIR)/$(MCP_BINARY) ./cmd/ctx-mcp

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -rf $(BUILD_DIR)
