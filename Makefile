.PHONY: build
build:
	docker build -t cert-manager-webhook-better-wapi:latest .

.PHONY: test
test:
	go test -v .

.PHONY: clean
clean:
	rm -f webhook

.PHONY: verify
verify:
	go fmt ./...
	go vet ./...
