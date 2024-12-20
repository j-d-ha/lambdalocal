
### General targets
.PHONEY: lint
lint: ## run golangci-lint
	@echo "Running golangci-lint"
	@golangci-lint run

.PHONEY: test
test: ## run tests
	@echo "Running tests"
	@go test -v ./... -count=1

.PHONEY: lambdalocal-help
lambdalocal-help: ## show lambdalocal help
	go run . --help

### Examples

.PHONEY: run_event
run_event:
	go run . \
		--parse-json \
		--verbose \
		event \
		--file "./example/api-gateway-basic/event.json"

.PHONEY: run_api
run_api:
	go run . \
		--parse-json \
		api \
		--template "./example/api-gateway-basic/template.yaml"