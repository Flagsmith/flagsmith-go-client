module github.com/Flagsmith/flagsmith-go-client/v5/sdk-spec

go 1.25.0

require github.com/Flagsmith/flagsmith-go-client/v5 v5.3.0

require (
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/go-resty/resty/v2 v2.17.2 // indirect
	github.com/itlightning/dateparse v0.2.1 // indirect
	github.com/ohler55/ojg v1.28.1 // indirect
	golang.org/x/net v0.58.0 // indirect
)

// Build against the SDK source in this repository, not a published release.
replace github.com/Flagsmith/flagsmith-go-client/v5 => ../
