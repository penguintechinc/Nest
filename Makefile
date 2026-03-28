# Project Template Makefile
# This Makefile provides common development tasks for multi-language projects

.PHONY: help setup install-deps dev dev-down dev-logs dev-restart \
	db-init db-migrate db-reset db-seed build build-api build-manager build-web \
	docker-build docker-push docker-build-api docker-build-manager docker-build-web \
	test test-api test-manager test-integration \
	lint lint-go lint-python fmt \
	monitoring-deploy monitoring-undeploy monitoring-status \
	deploy-dev deploy-prod \
	clean clean-docker clean-all \
	logs shell-api shell-manager health

# Default target
.DEFAULT_GOAL := help

# Variables
PROJECT_NAME := project-template
VERSION := $(shell cat .version 2>/dev/null || echo "development")
DOCKER_REGISTRY := ghcr.io
DOCKER_ORG := penguintechinc
GO_VERSION := 1.23.5
PYTHON_VERSION := 3.12
NODE_VERSION := 18

# Colors for output
RED := \033[31m
GREEN := \033[32m
YELLOW := \033[33m
BLUE := \033[34m
RESET := \033[0m

# Help target
help: ## Show this help message
	@echo "$(BLUE)$(PROJECT_NAME) Development Commands$(RESET)"
	@echo ""
	@echo "$(GREEN)Setup Commands:$(RESET)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && /Setup/ {printf "  $(YELLOW)%-20s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(GREEN)Development Commands:$(RESET)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && /Development/ {printf "  $(YELLOW)%-20s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(GREEN)Testing Commands:$(RESET)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && /Testing/ {printf "  $(YELLOW)%-20s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(GREEN)Build Commands:$(RESET)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && /Build/ {printf "  $(YELLOW)%-20s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(GREEN)Docker Commands:$(RESET)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && /Docker/ {printf "  $(YELLOW)%-20s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(GREEN)Other Commands:$(RESET)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && !/Setup|Development|Testing|Build|Docker/ {printf "  $(YELLOW)%-20s$(RESET) %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Setup Commands
setup: ## Setup - Install all dependencies and initialize the project
	@echo "$(BLUE)Setting up $(PROJECT_NAME)...$(RESET)"
	@$(MAKE) setup-env
	@$(MAKE) setup-go
	@$(MAKE) setup-python
	@$(MAKE) setup-node
	@$(MAKE) setup-git-hooks
	@echo "$(GREEN)Setup complete!$(RESET)"

install-deps: ## Setup - Install Go, Python, and Node.js dependencies
	@echo "$(BLUE)Installing all dependencies...$(RESET)"
	@$(MAKE) setup-go
	@$(MAKE) setup-python
	@$(MAKE) setup-node
	@echo "$(GREEN)All dependencies installed!$(RESET)"

setup-env: ## Setup - Create environment file from template
	@if [ ! -f .env ]; then \
		echo "$(YELLOW)Creating .env from .env.example...$(RESET)"; \
		cp .env.example .env; \
		echo "$(YELLOW)Please edit .env with your configuration$(RESET)"; \
	fi

setup-go: ## Setup - Install Go dependencies and tools
	@echo "$(BLUE)Setting up Go dependencies...$(RESET)"
	@go version || (echo "$(RED)Go $(GO_VERSION) not installed$(RESET)" && exit 1)
	@go mod download
	@go mod tidy
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/air-verse/air@latest

setup-python: ## Setup - Install Python dependencies and tools
	@echo "$(BLUE)Setting up Python dependencies...$(RESET)"
	@python3 --version || (echo "$(RED)Python $(PYTHON_VERSION) not installed$(RESET)" && exit 1)
	@pip install --upgrade pip
	@pip install -r requirements.txt
	@pip install black isort flake8 mypy pytest pytest-cov

setup-node: ## Setup - Install Node.js dependencies and tools
	@echo "$(BLUE)Setting up Node.js dependencies...$(RESET)"
	@node --version || (echo "$(RED)Node.js $(NODE_VERSION) not installed$(RESET)" && exit 1)
	@npm install
	@cd web && npm install

setup-git-hooks: ## Setup - Install Git pre-commit hooks
	@echo "$(BLUE)Installing Git hooks...$(RESET)"
	@cp scripts/git-hooks/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@cp scripts/git-hooks/commit-msg .git/hooks/commit-msg
	@chmod +x .git/hooks/commit-msg

# Development Commands
dev: ## Development - Start development environment
	@echo "$(BLUE)Starting development environment...$(RESET)"
	@docker-compose up -d postgres redis
	@sleep 5
	@$(MAKE) dev-services

dev-services: ## Development - Start all services for development
	@echo "$(BLUE)Starting development services...$(RESET)"
	@trap 'docker-compose down' INT; \
	concurrently --names "API,Web-Python,Web-Node" --prefix name --kill-others \
		"$(MAKE) dev-api" \
		"$(MAKE) dev-web-python" \
		"$(MAKE) dev-web-node"

dev-api: ## Development - Start Go API in development mode
	@echo "$(BLUE)Starting Go API...$(RESET)"
	@cd apps/api && air

dev-web-python: ## Development - Start Python web app in development mode
	@echo "$(BLUE)Starting Python web app...$(RESET)"
	@cd apps/web && python3 app.py

dev-web-node: ## Development - Start Node.js web app in development mode
	@echo "$(BLUE)Starting Node.js web app...$(RESET)"
	@cd web && npm run dev

dev-db: ## Development - Start only database services
	@docker-compose up -d postgres redis

dev-monitoring: ## Development - Start monitoring services
	@docker-compose up -d prometheus grafana

dev-full: ## Development - Start full development stack
	@docker-compose up -d

dev-down: ## Development - Stop development environment
	@echo "$(BLUE)Stopping development environment...$(RESET)"
	@docker-compose down

dev-logs: ## Development - View development logs
	@docker-compose logs -f

dev-restart: ## Development - Restart development services
	@echo "$(BLUE)Restarting development services...$(RESET)"
	@docker-compose restart

# Testing Commands
test: ## Testing - Run all tests
	@echo "$(BLUE)Running all tests...$(RESET)"
	@$(MAKE) test-api
	@$(MAKE) test-manager
	@$(MAKE) test-integration
	@echo "$(GREEN)All tests completed!$(RESET)"

test-api: ## Testing - Run Go API tests
	@echo "$(BLUE)Running Go API tests...$(RESET)"
	@cd apps/api && go test -v -race -coverprofile=coverage.out ./...

test-manager: ## Testing - Run Python Manager tests
	@echo "$(BLUE)Running Python Manager tests...$(RESET)"
	@cd apps/manager && pytest -v --cov=. --cov-report=xml --cov-report=html

test-integration: ## Testing - Run integration tests
	@echo "$(BLUE)Running integration tests...$(RESET)"
	@docker-compose -f docker-compose.test.yml up --build --abort-on-container-exit
	@docker-compose -f docker-compose.test.yml down

test-coverage: ## Testing - Generate coverage reports
	@$(MAKE) test
	@echo "$(GREEN)Coverage reports generated:$(RESET)"
	@echo "  Go: coverage-go.out"
	@echo "  Python: coverage-python.xml, htmlcov-python/"
	@echo "  Node.js: coverage/"

# Build Commands
build: ## Build - Build all applications
	@echo "$(BLUE)Building all applications...$(RESET)"
	@$(MAKE) build-api
	@$(MAKE) build-manager
	@$(MAKE) build-web
	@echo "$(GREEN)All builds completed!$(RESET)"

build-api: ## Build - Build Go API service
	@echo "$(BLUE)Building Go API service...$(RESET)"
	@mkdir -p bin
	@cd apps/api && go build -ldflags "-X main.version=$(VERSION)" -o ../../bin/api .

build-manager: ## Build - Build Python Manager service
	@echo "$(BLUE)Building Python Manager service...$(RESET)"
	@cd apps/manager && python3 -m py_compile .

build-web: ## Build - Build React frontend
	@echo "$(BLUE)Building React frontend...$(RESET)"
	@cd web && npm run build

build-production: ## Build - Build for production with optimizations
	@echo "$(BLUE)Building for production...$(RESET)"
	@CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags "-w -s -X main.version=$(VERSION)" -o bin/api ./apps/api
	@cd web && npm run build

# Docker Commands
docker-build: ## Docker - Build all Docker images
	@echo "$(BLUE)Building all Docker images...$(RESET)"
	@$(MAKE) docker-build-api
	@$(MAKE) docker-build-manager
	@$(MAKE) docker-build-web

docker-build-api: ## Docker - Build API Docker image
	@echo "$(BLUE)Building API Docker image...$(RESET)"
	@docker build -t $(DOCKER_REGISTRY)/$(DOCKER_ORG)/$(PROJECT_NAME)-api:$(VERSION) -f apps/api/Dockerfile .

docker-build-manager: ## Docker - Build Manager Docker image
	@echo "$(BLUE)Building Manager Docker image...$(RESET)"
	@docker build -t $(DOCKER_REGISTRY)/$(DOCKER_ORG)/$(PROJECT_NAME)-manager:$(VERSION) -f apps/manager/Dockerfile .

docker-build-web: ## Docker - Build Web Docker image
	@echo "$(BLUE)Building Web Docker image...$(RESET)"
	@docker build -t $(DOCKER_REGISTRY)/$(DOCKER_ORG)/$(PROJECT_NAME)-web:$(VERSION) -f web/Dockerfile web/

docker-push: ## Docker - Push images to registry
	@echo "$(BLUE)Pushing Docker images to registry...$(RESET)"
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_ORG)/$(PROJECT_NAME)-api:$(VERSION)
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_ORG)/$(PROJECT_NAME)-manager:$(VERSION)
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_ORG)/$(PROJECT_NAME)-web:$(VERSION)

docker-run: ## Docker - Run application with Docker Compose
	@docker-compose up --build

docker-clean: ## Docker - Clean up Docker resources
	@echo "$(BLUE)Cleaning up Docker resources...$(RESET)"
	@docker-compose down -v
	@docker system prune -f

# Code Quality Commands
lint: ## Code Quality - Run linting for all languages
	@echo "$(BLUE)=== Linting ===$(RESET)"
	@if command -v flake8 >/dev/null 2>&1; then echo "$(YELLOW)-- flake8 --$(RESET)"; python3 -m flake8 . --max-line-length=120 --exclude=.git,__pycache__,venv,node_modules --ignore=E501 || true; fi
	@if command -v black >/dev/null 2>&1; then echo "$(YELLOW)-- black --$(RESET)"; black --check . --exclude '/(\.git|venv|__pycache__|node_modules)/' || true; fi
	@if command -v isort >/dev/null 2>&1; then echo "$(YELLOW)-- isort --$(RESET)"; isort --check-only . --skip=.git --skip=__pycache__ --skip=venv --skip=node_modules || true; fi
	@if command -v mypy >/dev/null 2>&1; then echo "$(YELLOW)-- mypy --$(RESET)"; python3 -m mypy . --ignore-missing-imports || true; fi
	@if command -v golangci-lint >/dev/null 2>&1; then echo "$(YELLOW)-- golangci-lint --$(RESET)"; find . -name "go.mod" -not -path "*/.git/*" -not -path "*/vendor/*" | xargs -I{} dirname {} | xargs -I{} sh -c 'cd {} && golangci-lint run || true'; fi
	@if command -v hadolint >/dev/null 2>&1; then echo "$(YELLOW)-- hadolint --$(RESET)"; find . -name "Dockerfile*" -not -path "*/.git/*" | xargs hadolint || true; fi
	@if command -v shellcheck >/dev/null 2>&1; then echo "$(YELLOW)-- shellcheck --$(RESET)"; find . -name "*.sh" -not -path "*/.git/*" | xargs shellcheck || true; fi

lint-go: ## Code Quality - Run Go linting
	@echo "$(BLUE)Linting Go code...$(RESET)"
	@golangci-lint run

lint-python: ## Code Quality - Run Python linting
	@echo "$(BLUE)Linting Python code...$(RESET)"
	@flake8 .
	@mypy . --ignore-missing-imports

lint-node: ## Code Quality - Run Node.js linting
	@echo "$(BLUE)Linting Node.js code...$(RESET)"
	@npm run lint
	@cd web && npm run lint

fmt: ## Code Quality - Format all code for all languages
	@echo "$(BLUE)Formatting code...$(RESET)"
	@$(MAKE) format-go
	@$(MAKE) format-python
	@$(MAKE) format-node

format-go: ## Code Quality - Format Go code
	@echo "$(BLUE)Formatting Go code...$(RESET)"
	@go fmt ./...
	@goimports -w .

format-python: ## Code Quality - Format Python code
	@echo "$(BLUE)Formatting Python code...$(RESET)"
	@black .
	@isort .

format-node: ## Code Quality - Format Node.js code
	@echo "$(BLUE)Formatting Node.js code...$(RESET)"
	@npm run format
	@cd web && npm run format

# Database Commands
db-init: ## Database - Initialize database
	@echo "$(BLUE)Initializing database...$(RESET)"
	@docker-compose up -d postgres
	@sleep 5
	@$(MAKE) db-migrate
	@echo "$(GREEN)Database initialization complete!$(RESET)"

db-migrate: ## Database - Run database migrations
	@echo "$(BLUE)Running database migrations...$(RESET)"
	@go run scripts/migrate.go

db-seed: ## Database - Seed database with test data
	@echo "$(BLUE)Seeding database...$(RESET)"
	@go run scripts/seed.go

db-reset: ## Database - Reset database (WARNING: destroys data)
	@echo "$(RED)WARNING: This will destroy all data!$(RESET)"
	@read -p "Are you sure? (y/N): " confirm && [ "$$confirm" = "y" ]
	@docker-compose down -v
	@docker-compose up -d postgres redis
	@sleep 5
	@$(MAKE) db-migrate
	@$(MAKE) db-seed

db-backup: ## Database - Create database backup
	@echo "$(BLUE)Creating database backup...$(RESET)"
	@mkdir -p backups
	@docker-compose exec postgres pg_dump -U postgres project_template > backups/backup-$(shell date +%Y%m%d-%H%M%S).sql

db-restore: ## Database - Restore database from backup (requires BACKUP_FILE)
	@echo "$(BLUE)Restoring database from $(BACKUP_FILE)...$(RESET)"
	@docker-compose exec -T postgres psql -U postgres project_template < $(BACKUP_FILE)

# License Commands
license-validate: ## License - Validate license configuration
	@echo "$(BLUE)Validating license configuration...$(RESET)"
	@go run scripts/license-validate.go

license-test: ## License - Test license server integration
	@echo "$(BLUE)Testing license server integration...$(RESET)"
	@curl -f $${LICENSE_SERVER_URL:-https://license.penguintech.io}/api/v2/validate \
		-H "Authorization: Bearer $${LICENSE_KEY}" \
		-H "Content-Type: application/json" \
		-d '{"product": "'$${PRODUCT_NAME:-project-template}'"}'

# Version Management Commands
version-update: ## Version - Update version (patch by default)
	@./scripts/version/update-version.sh

version-update-minor: ## Version - Update minor version
	@./scripts/version/update-version.sh minor

version-update-major: ## Version - Update major version
	@./scripts/version/update-version.sh major

version-show: ## Version - Show current version
	@echo "Current version: $(VERSION)"

# Monitoring Commands
monitoring-deploy: ## Monitoring - Deploy monitoring stack to Kubernetes
	@echo "$(BLUE)Deploying monitoring stack to Kubernetes...$(RESET)"
	@kubectl apply -f infrastructure/monitoring/prometheus/
	@kubectl apply -f infrastructure/monitoring/grafana/
	@echo "$(GREEN)Monitoring stack deployed!$(RESET)"

monitoring-undeploy: ## Monitoring - Remove monitoring stack from Kubernetes
	@echo "$(BLUE)Removing monitoring stack from Kubernetes...$(RESET)"
	@kubectl delete -f infrastructure/monitoring/grafana/
	@kubectl delete -f infrastructure/monitoring/prometheus/
	@echo "$(GREEN)Monitoring stack removed!$(RESET)"

monitoring-status: ## Monitoring - Check monitoring stack status
	@echo "$(BLUE)Checking monitoring stack status...$(RESET)"
	@kubectl get deployments -n monitoring || echo "$(YELLOW)Monitoring namespace not found$(RESET)"
	@kubectl get services -n monitoring || echo "$(YELLOW)Monitoring services not found$(RESET)"

# Deployment Commands
deploy-dev: ## Deploy - Deploy to development Kubernetes cluster
	@echo "$(BLUE)Deploying to development cluster...$(RESET)"
	@$(MAKE) docker-build
	@$(MAKE) docker-push
	@kubectl apply -f infrastructure/k8s/dev/
	@echo "$(GREEN)Deployment to development complete!$(RESET)"

deploy-prod: ## Deploy - Deploy to production Kubernetes cluster
	@echo "$(BLUE)Deploying to production cluster...$(RESET)"
	@$(MAKE) docker-build
	@$(MAKE) docker-push
	@kubectl apply -f infrastructure/k8s/prod/
	@echo "$(GREEN)Deployment to production complete!$(RESET)"

deploy-staging: ## Deploy - Deploy to staging environment
	@echo "$(BLUE)Deploying to staging...$(RESET)"
	@$(MAKE) docker-build
	@$(MAKE) docker-push
	# Add staging deployment commands here

deploy-production: ## Deploy - Deploy to production environment
	@echo "$(BLUE)Deploying to production...$(RESET)"
	@$(MAKE) docker-build
	@$(MAKE) docker-push
	# Add production deployment commands here

# Health Check Commands
health: ## Health - Check service health
	@echo "$(BLUE)Checking service health...$(RESET)"
	@curl -f http://localhost:8080/health || echo "$(RED)API health check failed$(RESET)"
	@curl -f http://localhost:8000/health || echo "$(RED)Python web health check failed$(RESET)"
	@curl -f http://localhost:3000/health || echo "$(RED)Node web health check failed$(RESET)"

shell-api: ## Utilities - Open shell in API container
	@echo "$(BLUE)Opening shell in API container...$(RESET)"
	@docker-compose exec api /bin/sh

shell-manager: ## Utilities - Open shell in Manager container
	@echo "$(BLUE)Opening shell in Manager container...$(RESET)"
	@docker-compose exec manager /bin/sh

logs: ## Utilities - Show service logs
	@docker-compose logs -f

logs-api: ## Logs - Show API logs
	@docker-compose logs -f api

logs-web: ## Logs - Show web logs
	@docker-compose logs -f web-python web-node

logs-db: ## Logs - Show database logs
	@docker-compose logs -f postgres redis

# Cleanup Commands
clean: ## Clean - Clean build artifacts and caches
	@echo "$(BLUE)Cleaning build artifacts...$(RESET)"
	@rm -rf bin/
	@rm -rf dist/
	@rm -rf node_modules/
	@rm -rf web/node_modules/
	@rm -rf web/dist/
	@rm -rf __pycache__/
	@rm -rf .pytest_cache/
	@rm -rf htmlcov-python/
	@rm -rf coverage-*.out
	@rm -rf coverage-*.xml
	@go clean -cache -modcache

clean-docker: ## Clean - Clean Docker resources
	@$(MAKE) docker-clean

clean-all: ## Clean - Clean everything (build artifacts, Docker, etc.)
	@$(MAKE) clean
	@$(MAKE) clean-docker

# Security Commands
security-scan: ## Security - Run security scans
	@echo "$(BLUE)Running security scans...$(RESET)"
	@go list -json -m all | nancy sleuth
	@safety check --json

audit: ## Security - Run security audit
	@echo "$(BLUE)Running security audit...$(RESET)"
	@npm audit
	@cd web && npm audit
	@$(MAKE) security-scan

# Monitoring Commands
metrics: ## Monitoring - Show application metrics
	@echo "$(BLUE)Application metrics:$(RESET)"
	@curl -s http://localhost:8080/metrics | grep -E '^# (HELP|TYPE)' | head -20

monitor: ## Monitoring - Open monitoring dashboard
	@echo "$(BLUE)Opening monitoring dashboard...$(RESET)"
	@open http://localhost:3001  # Grafana

# Documentation Commands
docs-serve: ## Documentation - Serve documentation locally
	@echo "$(BLUE)Serving documentation...$(RESET)"
	@cd docs && python3 -m http.server 8080

docs-build: ## Documentation - Build documentation
	@echo "$(BLUE)Building documentation...$(RESET)"
	@echo "Documentation build not implemented yet"

# Git Commands
git-hooks-install: ## Git - Install Git hooks
	@$(MAKE) setup-git-hooks

git-hooks-test: ## Git - Test Git hooks
	@echo "$(BLUE)Testing Git hooks...$(RESET)"
	@.git/hooks/pre-commit
	@echo "$(GREEN)Git hooks test completed$(RESET)"

# Info Commands
info: ## Info - Show project information
	@echo "$(BLUE)Project Information:$(RESET)"
	@echo "Name: $(PROJECT_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Go Version: $(GO_VERSION)"
	@echo "Python Version: $(PYTHON_VERSION)"
	@echo "Node Version: $(NODE_VERSION)"
	@echo ""
	@echo "$(BLUE)Service URLs:$(RESET)"
	@echo "API: http://localhost:8080"
	@echo "Python Web: http://localhost:8000"
	@echo "Node Web: http://localhost:3000"
	@echo "Prometheus: http://localhost:9090"
	@echo "Grafana: http://localhost:3001"

env: ## Info - Show environment variables
	@echo "$(BLUE)Environment Variables:$(RESET)"
	@env | grep -E "^(LICENSE_|POSTGRES_|REDIS_|NODE_|GIN_|PY4WEB_)" | sort

# Standard Test Targets
test-unit: ## Testing - Run unit tests
	@$(MAKE) test

test-integration: ## Testing - Run integration tests
	@echo "$(YELLOW)No integration tests defined$(RESET)"

test-e2e: ## Testing - Run end-to-end tests
	@echo "$(YELLOW)No e2e tests defined$(RESET)"

test-functional: ## Testing - Run functional tests (APIs, pages, tabs, modals, buttons)
	@echo "$(YELLOW)No functional tests defined$(RESET)"

test-security: ## Testing - Run security tests
	@echo "$(BLUE)=== Security Scans ===$(RESET)"
	@if command -v bandit >/dev/null 2>&1; then echo "$(YELLOW)-- bandit --$(RESET)"; bandit -r . -x ./tests,./venv,./.git --quiet || true; fi
	@if command -v pip-audit >/dev/null 2>&1; then echo "$(YELLOW)-- pip-audit --$(RESET)"; find . -name "requirements.txt" -not -path "*/.git/*" -not -path "*/venv/*" | xargs -I{} pip-audit -r {} 2>/dev/null || true; fi
	@if command -v gosec >/dev/null 2>&1; then echo "$(YELLOW)-- gosec --$(RESET)"; find . -name "go.mod" -not -path "*/.git/*" -not -path "*/vendor/*" | xargs -I{} dirname {} | xargs -I{} sh -c 'cd {} && gosec ./... || true'; fi
	@if command -v govulncheck >/dev/null 2>&1; then echo "$(YELLOW)-- govulncheck --$(RESET)"; find . -name "go.mod" -not -path "*/.git/*" -not -path "*/vendor/*" | xargs -I{} dirname {} | xargs -I{} sh -c 'cd {} && govulncheck ./... || true'; fi
	@find . -name "package.json" -not -path "*/.git/*" -not -path "*/node_modules/*" -maxdepth 3 | xargs -I{} dirname {} | xargs -I{} sh -c 'cd {} && npm audit 2>/dev/null || true'
	@if command -v gitleaks >/dev/null 2>&1; then echo "$(YELLOW)-- gitleaks --$(RESET)"; gitleaks detect --source . --no-git 2>/dev/null || true; fi

smoke-test: ## Testing - Run smoke tests (build, run, health checks)
	@$(MAKE) lint
	@$(MAKE) build
	@echo "$(GREEN)Smoke tests completed$(RESET)"

seed-mock-data: ## Testing - Seed database with mock data
	@echo "$(YELLOW)No mock data seeding defined$(RESET)"

pre-commit: ## Testing - Run pre-commit checks (linting, security, build, tests)
	@echo "$(BLUE)=== Running pre-commit checks ===$(RESET)"
	@$(MAKE) lint
	@$(MAKE) test-security
	@$(MAKE) test
	@echo "$(GREEN)Pre-commit checks complete$(RESET)"