//go:build !windows

package mbin

// platformName names this platform in messages about release assets.
const platformName = "Linux"

// platformAssets are the Linux binary and library per flavor.
//
//nolint:gochecknoglobals // a fixed table, read-only
var platformAssets = map[string][2]string{
	FlavorDotnet10:      {"MBINCompiler-linux-dotnet10", "libMBIN-linux-dotnet10.so"},
	FlavorSelfContained: {"MBINCompiler-linux", "libMBIN-linux.so"},
}
