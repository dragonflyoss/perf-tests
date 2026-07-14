all: help

# Build file-server image.
docker-build-file-server:
	@echo "Begin to use docker build file-server image."
	docker buildx build --platform linux/amd64,linux/arm64 -t file-server:latest -f ./tools/file-server/Dockerfile .
.PHONY: docker-build-file-server

# Build proxy-bench image.
docker-build-proxy-bench:
	@echo "Begin to use docker build proxy-bench image."
	docker buildx build --platform linux/amd64,linux/arm64 -t proxy-bench:latest -f ./tools/proxy-bench/Dockerfile .
.PHONY: docker-build-proxy-bench

# Run code lint
lint: markdownlint
	@echo "Begin to golangci-lint."
	@golangci-lint run
.PHONY: lint

# Run markdown lint
markdownlint:
	@echo "Begin to markdownlint."
	@./hack/markdownlint.sh
.PHONY: markdownlint

# Clear compiled files
clean:
	@go clean
	@rm -rf bin .go .cache
.PHONY: clean

help: 
	@echo "make docker-build-file-server       build file-server image"
	@echo "make docker-build-proxy-bench       build proxy-bench image"
	@echo "make lint                           run code lint"
	@echo "make markdownlint                   run markdown lint"
	@echo "make clean                          clean"
