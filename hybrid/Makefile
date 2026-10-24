.PHONY: test vet exercise gen

test:
	go test ./...

vet:
	go vet ./...

# The lean_import mission's exercise (servicekit#53): one server, three
# transports, the same JSON.
exercise:
	go run ./cmd/demo -check

# Regenerate demo/gen from demo/pb (needs buf; uses remote plugins).
gen:
	cd demo && buf generate
