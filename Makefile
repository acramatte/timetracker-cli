BINARY := timetracker
BUILD_DIR := bin
STATICCHECK_CHECKS := all,-ST1000,-ST1003,-ST1020

.PHONY: fmt fmt-check vet lint test acceptance build check clean

fmt:
	gofmt -l -w .

fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		printf 'gofmt needed on:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi

vet:
	go vet ./...

lint: vet
	go tool staticcheck -checks '$(STATICCHECK_CHECKS)' ./...

test:
	go test ./...

acceptance:
	go test -tags acceptance ./cmd/timetracker -run 'Test(CLIHelp|PomodoroCommandCompletesAtDeadline)' -count=1

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/$(BINARY)

check: fmt-check lint test build

clean:
	rm -rf $(BUILD_DIR)
