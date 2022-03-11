all: help

# Run markdown lint
markdownlint:
	@echo "Begin to markdownlint."
	@./hack/markdownlint.sh
.PHONY: markdownlint

clean:
	@go clean
	@rm -rf bin .go .cache
.PHONY: clean

help: 
	@echo "make markdownlint                   run markdown lint"
	@echo "make clean                          clean"
