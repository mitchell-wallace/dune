package contract

import "testing"

func TestHostBinaryBuildCommandForPathUsesRequestedOutput(t *testing.T) {
	t.Parallel()

	got := HostBinaryBuildCommandForPath("/repo", "/tmp/rally-system", "1.2.3", "abc123")

	wantOutput := false
	for i := 0; i < len(got)-1; i++ {
		if got[i] == "-o" && got[i+1] == "/tmp/rally-system" {
			wantOutput = true
			break
		}
	}
	if !wantOutput {
		t.Fatalf("expected custom output path in build command, got %#v", got)
	}
}
