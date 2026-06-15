package main

import (
	"os"
	"path/filepath"
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

// TestScript runs every .txt file under testdata/script/ against the live binary.
// Each script runs in an isolated $WORK directory with HOME=$WORK/home — a clean
// slate equivalent to a fresh machine install.
func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			home := filepath.Join(env.WorkDir, "home")
			if err := os.MkdirAll(home, 0755); err != nil {
				return err
			}
			env.Setenv("HOME", home)
			return nil
		},
	})
}
