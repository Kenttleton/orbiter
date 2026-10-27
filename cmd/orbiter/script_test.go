package main

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain registers orbiter as a re-entrant command for testscript.
// When a script runs "exec orbiter", testscript re-runs this binary
// with os.Args[0]="orbiter", and Main dispatches to main() instead of tests.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"orbiter": main,
	})
}
