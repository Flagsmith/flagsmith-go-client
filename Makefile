.EXPORT_ALL_VARIABLES:

EVALUATION_CONTEXT_SCHEMA_URL ?= https://raw.githubusercontent.com/Flagsmith/flagsmith/main/sdk/evaluation-context.json
SDK_BENCHMARKS := flagengine/sdk-benchmarks
SDK_BENCHMARKS_ENVIRONMENTS := $(abspath $(SDK_BENCHMARKS)/environments)


.PHONY: generate-evaluation-context
generate-evaluation-context:
	curl ${EVALUATION_CONTEXT_SCHEMA_URL} | npx quicktype  \
		--src-lang schema \
		--lang go \
		--package flagsmith \
		--omit-empty \
		--just-types-and-package \
		--top-level EvaluationContext \
		-o evaluationcontext.go

$(SDK_BENCHMARKS_ENVIRONMENTS):
	git submodule update --init -- $(SDK_BENCHMARKS)

.PHONY: benchmark
benchmark: | $(SDK_BENCHMARKS_ENVIRONMENTS)
	go test -bench=. ./benchmarks/
