package flagsmith

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// getUserAgent returns the User-Agent header value in the format "flagsmith-go-sdk/<version>".
// If the version cannot be determined (e.g., during development), it returns "flagsmith-go-sdk/unknown".
func getUserAgent() string {
	const sdkName = "flagsmith-go-sdk"
	const unknownVersion = "unknown"
	const modulePrefix = "github.com/Flagsmith/flagsmith-go-client"

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return fmt.Sprintf("%s/%s", sdkName, unknownVersion)
	}

	// Check if SDK module path matches (supports any major version: v4, v5, etc.)
	isSDKModule := func(path string) bool {
		return path == modulePrefix || strings.HasPrefix(path, modulePrefix+"/")
	}

	// If this is the main module (running tests or examples from within the SDK repo),
	// use the main module version
	if isSDKModule(info.Main.Path) {
		version := info.Main.Version
		if version != "" && version != "(devel)" {
			return fmt.Sprintf("%s/%s", sdkName, version)
		}
	}

	// Otherwise, look for the SDK in the dependencies (when used as a library)
	for _, dep := range info.Deps {
		if isSDKModule(dep.Path) {
			version := dep.Version
			if version != "" && version != "(devel)" {
				return fmt.Sprintf("%s/%s", sdkName, version)
			}
			break
		}
	}

	return fmt.Sprintf("%s/%s", sdkName, unknownVersion)
}
