BINARY  := ai-code-review
BIN_DIR := task/bin

# Container image coordinates. Override IMAGE to push to your own registry, e.g.
#   make docker-push IMAGE=ghcr.io/youruser/nitpicker TAG=v1.0.0
IMAGE := ghcr.io/hazardemircan/nitpicker
TAG   := latest

.PHONY: build build-linux build-darwin build-windows tidy clean docker-build docker-push

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

# Build the container image locally.
docker-build:
	docker build -t $(IMAGE):$(TAG) .

# Build and push (logs into the registry first if needed). For multi-arch
# pushes the GitHub Actions workflow is preferred; this targets the host arch.
docker-push: docker-build
	docker push $(IMAGE):$(TAG)
