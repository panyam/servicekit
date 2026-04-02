.PHONY: test test-race vet install-hooks

test:
	go test ./...

# Run tests with race detector (5 iterations to catch intermittent races).
test-race:
	go test -race -count=5 -timeout 300s ./...

vet:
	go vet ./...

install-hooks:
	@if [ -f .git ]; then \
		HOOKS_DIR=$$(sed 's/gitdir: //' .git | xargs dirname | xargs dirname)/hooks; \
	else \
		HOOKS_DIR=.git/hooks; \
	fi; \
	cp scripts/pre-push "$$HOOKS_DIR/pre-push" && \
	chmod +x "$$HOOKS_DIR/pre-push" && \
	echo "Pre-push hook installed to $$HOOKS_DIR/pre-push"
