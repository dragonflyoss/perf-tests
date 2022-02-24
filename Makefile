all: help

# Run markdown lint
markdownlint:
	@echo "Begin to markdownlint."
	@./hack/markdownlint.sh
.PHONY: markdownlint

help: 
	@echo "make markdownlint                   run markdown lint"
