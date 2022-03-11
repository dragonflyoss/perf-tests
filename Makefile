all: help

# Run markdown lint
markdownlint:
	@echo "Begin to markdownlint."
	@./hack/markdownlint.sh
.PHONY: markdownlint

# Run code lint
lint: markdownlint
	@echo "Begin to golangci-lint."
	@golangci-lint run
.PHONY: lint

# Clear compiled files
clean:
	@go clean
	@rm -rf bin .go .cache
.PHONY: clean

help: 
	@echo "make lint                           run code lint"
	@echo "make markdownlint                   run markdown lint"
	@echo "make clean                          clean"
