.PHONY: build-tool
build-tool:
	go build

.PHONY: generate
generate:
	./json-schema-generator -r ./testPkgs/crd -o ./testdata/schema

.PHONY: test
test: generate
	go test -v ./...
