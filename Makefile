BINARY=ctx
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) .

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -rf $(BUILD_DIR)
