# Simple Makefile for Ditto

DOCKER_IMAGE = phathdt379/ditto
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.1.0")

.PHONY: help build test docker-build docker-push tag release clean

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

build: ## Build Go binary
	go build -o bin/ditto .

test: ## Run tests
	go test -v ./...

docker-build: ## Build Docker image
	docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

docker-push: ## Push Docker image
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest

tag: ## Create and push git tag
	@echo "Fetching latest tags from origin..."
	@git fetch origin --tags --force || true
	@echo "Current version: $(VERSION)"
	@echo "Recent tags:"
	@git tag -l | tail -5 | sort -V
	@echo ""
	@read -p "Enter new version (e.g., v1.0.1): " NEW_VERSION; \
	if [ -z "$$NEW_VERSION" ]; then \
		echo "Version cannot be empty"; \
		exit 1; \
	fi; \
	if git tag -l | grep -q "^$$NEW_VERSION$$"; then \
		echo "Tag $$NEW_VERSION already exists locally. Deleting..."; \
		git tag -d $$NEW_VERSION; \
	fi; \
	echo "Creating tag $$NEW_VERSION..."; \
	git tag -a $$NEW_VERSION -m "Release $$NEW_VERSION"; \
	git push origin $$NEW_VERSION; \
	echo "Tag $$NEW_VERSION created and pushed successfully!"

release: test docker-build tag ## Full release: test, build, tag
	@echo "Release completed!"

clean: ## Clean build artifacts
	rm -rf bin/
	docker image prune -f

run: ## Run locally
	go run main.go

logs: ## Show container logs
	docker logs -f ditto-1

version: ## Show current version and recent tags
	@git fetch origin --tags --force || true
	@echo "Current version: $(VERSION)"
	@echo "Recent tags:"
	@git tag -l | tail -5 | sort -V
