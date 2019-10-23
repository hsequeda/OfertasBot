.DEFAULT_GOAL:=help
.PHONY: run install_deps help

# Compilation output
.ONESHELL:
SHELL := /bin/bash

run: ## Run the application
	# Export the token EnvVar.
	export TOKEN_BOT='931110470:AAHoPhcqzkgPeZjenglDOLy13GlK-vCV3EU'
	go run ./

install_deps: ## Install application deps.
	go mod vendor
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

