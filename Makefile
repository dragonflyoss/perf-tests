all: help

# Build file-server image.
docker-build-file-server:
	@echo "Begin to use docker build file-server image."
	docker build -t file-server:latest -f ./tools/file-server/Dockerfile .
.PHONY: docker-build-file-server

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
	@echo "make lint                           run code lint"
	@echo "make markdownlint                   run markdown lint"
	@echo "make clean                          clean"
