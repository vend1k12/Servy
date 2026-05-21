package safepath

import "testing"

func TestAllowlistedShellCanResolveThroughSystemSymlink(t *testing.T) {
	if _, err := LookPath("/bin/sh"); err != nil {
		t.Fatalf("/bin/sh should be executable through safe path verification: %v", err)
	}
}
