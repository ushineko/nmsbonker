//go:build race

package hgpak_test

/*
budgetFactor loosens AC6's timing assertions under the race detector.

The budgets in the spec (cold index under 5 s, warm load under 200 ms) are about
the binary a user runs. Race-instrumented code is several times slower --
measured here at roughly 8x on the warm path, which is dominated by building a
195,000-entry map -- so asserting the shipped budget under -race would fail a
build that is well inside it. `make test` runs with -race, so the alternative is
either a test that fails on every machine with the game installed or no
assertion at all.
*/
const budgetFactor = 10
