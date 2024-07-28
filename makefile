### Examples

.PHONEY: run_event
run_event:
	go run ./cmd/cli/ \
		--parse-json \
		event \
		--file "C:\repos\go\go-sam-lambda-hello-world\events\event.json"

.PHONEY: run_api
run_api:
	go run ./cmd/cli/ \
		--parse-json \
		api \
		--template "C:\repos\go\go-sam-lambda-hello-world\template.yaml"