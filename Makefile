.EXPORT_ALL_VARIABLES:

EVALUATION_CONTEXT_SCHEMA_URL ?= https://raw.githubusercontent.com/Flagsmith/flagsmith/main/sdk/evaluation-context.json


.PHONY: generate-evaluation-context
generate-evaluation-context:
	curl ${EVALUATION_CONTEXT_SCHEMA_URL} | npx quicktype  \
		--src-lang schema \
		--lang go \
		--package flagsmith \
		--omit-empty \
		--just-types-and-package \
		-o evaluationcontext.go
