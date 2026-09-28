.PHONY: test vet fmt

test:
	@command -v node >/dev/null || { echo "Node.js is required for JavaScript regex parity tests"; exit 1; }
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w validation.go validation_test.go
