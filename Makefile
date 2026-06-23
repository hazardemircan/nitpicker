BINARY  := ai-code-review
BIN_DIR := task/bin

.PHONY: build build-linux build-darwin build-windows tidy clean

build: build-linux build-darwin build-windows

build-linux:
	GOOS=linux   GOARCH=amd64 go build -o $(BIN_DIR)/linux-amd64/$(BINARY)          .

build-darwin:
	GOOS=darwin  GOARCH=amd64 go build -o $(BIN_DIR)/darwin-amd64/$(BINARY)         .

build-windows:
	GOOS=windows GOARCH=amd64 go build -o $(BIN_DIR)/windows-amd64/$(BINARY).exe    .

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
