.PHONY: test vet install-hooks

test:
	go test ./...

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
