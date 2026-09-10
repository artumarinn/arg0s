.PHONY: build test lint run clean

BIN_DIR := bin

build:
	go build -o $(BIN_DIR)/arg0s ./cmd/arg0s

test:
	go test ./...

lint:
	go vet ./...
	gofmt -l .

run: build
	./$(BIN_DIR)/arg0s

clean:
	rm -rf $(BIN_DIR)
