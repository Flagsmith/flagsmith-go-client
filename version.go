package flagsmith

import (
	"fmt"
	"runtime/debug"
)

// getUserAgent returns the User-Agent header value in the format "flagsmith-go-sdk/<version>".
// If the version cannot be determined (e.g., during development), it returns "flagsmith-go-sdk/unknown".
func getUserAgent() string {
	const sdkName = "flagsmith-go-sdk"
	const unknownVersion = "unknown"

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return fmt.Sprintf("%s/%s", sdkName, unknownVersion)
	}

	// Look for the main module version
	version := info.Main.Version

	// Handle cases where version is empty or "(devel)"
	if version == "" || version == "(devel)" {
		return fmt.Sprintf("%s/%s", sdkName, unknownVersion)
	}

	return fmt.Sprintf("%s/%s", sdkName, version)
}
