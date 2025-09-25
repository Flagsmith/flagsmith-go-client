.EXPORT_ALL_VARIABLES:

EVALUATION_CONTEXT_SCHEMA_URL ?= https://raw.githubusercontent.com/Flagsmith/flagsmith/main/sdk/evaluation-context.json

EVALUATION_RESULT_SCHEMA_URL ?= https://raw.githubusercontent.com/Flagsmith/flagsmith/main/sdk/evaluation-result.json


.PHONY: generate-evaluation-context
generate-evaluation-context:
	curl ${EVALUATION_CONTEXT_SCHEMA_URL} | npx quicktype  \
		--src-lang schema \
		--lang go \
		--package evalcontext \
		--omit-empty \
		--just-types-and-package \
		--top-level EvaluationContext \
		-o evalcontext/evaluationcontext.go

.PHONY: generate-evaluation-result
generate-evaluation-result:
	curl ${EVALUATION_RESULT_SCHEMA_URL} | npx quicktype  \
		--src-lang schema \
		--lang go \
		--package evalcontext\
		--omit-empty \
		--just-types-and-package \
		--top-level EvaluationResult \
		-o evalcontext/evaluationresult.go
