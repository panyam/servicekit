.PHONY: test test-race vet cover cover-html cover-func testall install-hooks

REPORT_DIR := test-reports

test:
	go test ./...

# Run tests with race detector (5 iterations to catch intermittent races).
test-race:
	go test -race -count=5 -timeout 300s ./...

vet:
	go vet ./...

cover: ## Run tests with coverage summary
	go test -cover ./... -count=1 -timeout 60s

cover-html: ## Run tests with coverage and generate HTML report
	@mkdir -p $(REPORT_DIR)
	go test -coverprofile=$(REPORT_DIR)/coverage.out ./... -count=1 -timeout 60s
	go tool cover -html=$(REPORT_DIR)/coverage.out -o $(REPORT_DIR)/coverage.html
	@echo "Coverage report: $(REPORT_DIR)/coverage.html"

cover-func: ## Show per-function coverage sorted by lowest (top 30)
	@mkdir -p $(REPORT_DIR)
	go test -coverprofile=$(REPORT_DIR)/coverage.out ./... -count=1 -timeout 60s
	go tool cover -func=$(REPORT_DIR)/coverage.out | sort -k3 -n | head -30

testall: vet cover-html test-race ## Run full test suite: vet + coverage + race detection
	@echo ""
	@echo "=== servicekit testall complete ==="
	@echo "Coverage report: $(REPORT_DIR)/coverage.html"

install-hooks:
	@if [ -f .git ]; then \
		HOOKS_DIR=$$(sed 's/gitdir: //' .git | xargs dirname | xargs dirname)/hooks; \
	else \
		HOOKS_DIR=.git/hooks; \
	fi; \
	cp scripts/pre-push "$$HOOKS_DIR/pre-push" && \
	chmod +x "$$HOOKS_DIR/pre-push" && \
	echo "Pre-push hook installed to $$HOOKS_DIR/pre-push"
