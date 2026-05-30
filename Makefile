.PHONY: fmt-check tidy-check test race vet boundary integration smoke-distributed resilience smoke-consumer verify

fmt-check:
	test -z "$$(gofmt -l .)"

tidy-check:
	bash scripts/verify_tidy.sh

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

boundary:
	bash scripts/verify_boundary.sh

integration:
	bash scripts/verify_integration.sh

smoke-distributed:
	bash scripts/verify_distributed_smoke.sh

resilience:
	bash scripts/verify_distributed_resilience.sh

smoke-consumer:
	bash scripts/verify_go_template_smoke.sh

verify: fmt-check tidy-check test vet boundary
