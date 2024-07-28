### Examples

.PHONEY: run_event
run_event:
	go run ./cmd/cli/ \
		--parse-json \
		event \
		--file "./example/api-gateway-basic/events/event.json"

.PHONEY: run_api
run_api:
	go run ./cmd/cli/ \
		--parse-json \
		api \
		--template "./example/api-gateway-basic/template.yaml"