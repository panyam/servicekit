.PHONY: test vet exercise gen

test:
	go test ./...

vet:
	go vet ./...

# The lean_import mission's exercise (servicekit#53): one server, four
# transports, the same JSON, over cleartext and over TLS (servicekit#61).
exercise:
	go run ./cmd/demo -check
	go run ./cmd/demo -check -tls

# Regenerate demo/gen from demo/pb (needs buf; uses remote plugins).
gen:
	cd demo && buf generate
