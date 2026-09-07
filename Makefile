.PHONY: build fmt fmt-check vet test testacc check docs fmt-examples release-check snapshot

build:
	go build -trimpath -o bin/terraform-provider-cloady .

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }

vet:
	go vet ./...

test:
	go test -race ./...

testacc:
	TF_ACC=1 go test -race -count=1 -timeout=10m ./internal/provider

docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest generate --provider-name cloady

fmt-examples:
	terraform fmt -check -recursive examples

check: fmt-check vet test build

release-check:
	goreleaser check

snapshot:
	goreleaser release --snapshot --clean --skip=publish,sign
