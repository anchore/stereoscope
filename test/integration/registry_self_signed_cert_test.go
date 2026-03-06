package integration

import (
	"os/exec"
	"testing"

	"github.com/anchore/stereoscope/internal/testutil"
)

func TestRegistrySelfSignedCert(t *testing.T) {
	fixturesPath := testutil.GetFixturePath(t, "registry")

	runMakeTarget := func(targets ...string) func(*testing.T) {
		return func(t *testing.T) {
			t.Logf("Running make targets %s", targets)

			cmd := exec.Command("make", targets...)
			cmd.Dir = fixturesPath
			runAndShow(t, cmd)
		}
	}

	tests := []struct {
		name    string
		setup   func(*testing.T)
		run     func(*testing.T)
		cleanup func(*testing.T)
	}{
		{
			name:    "go case",
			setup:   runMakeTarget(),
			run:     runMakeTarget("run"),
			cleanup: runMakeTarget("stop"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				tt.cleanup(t)
			})
			tt.setup(t)
			tt.run(t)
		})
	}
}
