//go:build !race

package hgpak_test

// budgetFactor is 1 without the race detector: the assertions are the spec's
// own numbers. See race_test.go for why the instrumented build gets slack.
const budgetFactor = 1
