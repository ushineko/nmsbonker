// Package cli assembles the nmsbonker command tree (spec 001 R8).
package cli

import "github.com/spf13/cobra"

// Root builds the command tree.
func Root() *cobra.Command { return &cobra.Command{Use: "nmsbonker"} }

// UsageError marks a failure caused by how the command was invoked, which main
// turns into exit code 2 (R8.2).
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }
