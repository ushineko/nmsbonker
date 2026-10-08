//go:build windows

package mbin

// platformName names this platform in messages about release assets.
const platformName = "Windows"

// platformAssets are the Windows binary and library per flavor (spec 020
// R3.1). The "self-contained" pair has no platform suffix: it is the one
// MBINCompiler has published for Windows since before the others existed.
// Despite the flavor's name it is framework-dependent on Windows, on .NET 8
// (found 2026-10-07, v7.04.1-pre3); the name is kept because it is a config
// value, and runtimeHint says what to install when it will not start.
//
//nolint:gochecknoglobals // a fixed table, read-only
var platformAssets = map[string][2]string{
	FlavorDotnet10:      {"MBINCompiler-dotnet10.exe", "libMBIN-dotnet10.dll"},
	FlavorSelfContained: {"MBINCompiler.exe", "libMBIN.dll"},
}
