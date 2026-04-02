.PHONY: test test-race vet install-hooks

test:
	go test ./...

# Run tests with race detector.
# Note: TestWebSocketReaderRace may fail until gocurrent fixes Reader.closedChan race (#4).
test-race:
	go test -race -count=1 -timeout 120s ./...

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
