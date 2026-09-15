# `make check` is the gate: a change is not done until it passes.

.PHONY: check

check:
	@echo '==> gofmt'
	@test -z "$$(gofmt -l .)" || { echo 'gofmt: these files need formatting:'; gofmt -l .; exit 1; }
	@echo '==> go vet'
	go vet ./...
	@echo '==> go test -race'
	go test -race ./...
	@echo '==> rulecheck'
	go run ./tools/rulecheck
