/*
Package buildinfo carries what this binary was built from.

It is a package of its own so that the -ldflags path points at program metadata
rather than at some format or feature package that happens to be convenient;
otherwise "what version am I" becomes a question you ask the pak reader.
*/
package buildinfo

// Version and Commit are injected at build time via -ldflags -X (R1.2). The
// defaults are what an unadorned `go build` or `go test` produces.
var (
	Version = "dev"
	Commit  = "unknown"
)

// UserAgent is the value sent to the GitHub API (R5.1). It carries the program
// name and version and nothing that identifies the machine or the user.
func UserAgent() string { return "nmsbonker/" + Version }
