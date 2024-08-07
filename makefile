### Examples

.PHONEY: run_event
run_event:
	go run . \
		--parse-json \
		event \
		--file "./example/api-gateway-basic/event.json"

.PHONEY: run_api
run_api:
	go run . \
		--parse-json \
		api \
		--template "./example/api-gateway-basic/template.yaml"