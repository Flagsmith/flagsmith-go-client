# Builds the SDK from source with the adapter for the Flagsmith SDK behavioural spec.
# Build from the repository root: docker build -f spec-adapter/Dockerfile .
FROM golang:1.25-alpine AS build

WORKDIR /sdk
COPY go.mod go.sum ./
COPY spec-adapter/go.mod spec-adapter/go.sum ./spec-adapter/
RUN cd spec-adapter && go mod download

COPY . .
RUN cd spec-adapter \
    && version=$(sed -n 's/.*"\.": *"\([^"]*\)".*/\1/p' ../.release-please-manifest.json) \
    && CGO_ENABLED=0 go build -buildvcs=false -ldflags "-X main.version=$version" -o /adapter .

FROM scratch
COPY --from=build /adapter /adapter

ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/adapter"]
