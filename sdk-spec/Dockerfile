# Runs this SDK, built from source, under the Flagsmith SDK behavioural spec.
# docker build -f sdk-spec/Dockerfile --build-context harness=<sdk-spec>/harness/go .
FROM docker.io/library/golang:1.25-alpine AS build

WORKDIR /sdk
COPY go.mod go.sum ./
COPY sdk-spec/go.mod sdk-spec/go.sum ./sdk-spec/
RUN cd sdk-spec && go mod download

COPY . .
COPY --from=harness . /harness
RUN go run /harness/build.go -o /out/harness sdk-spec/steps.go

FROM scratch
COPY --from=build /out/harness /harness
ENTRYPOINT ["/harness"]
