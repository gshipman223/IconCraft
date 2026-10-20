//go:build darwin

package native

import "testing"

func TestParseKeepAgentDisabled(t *testing.T) {
	out := "disabled services = {\n\t\"io.iconcraft.keepapplied\" => true\n}\n"
	if !parseKeepAgentDisabled(out) {
		t.Fatal("disabled keeper was not detected")
	}
	out = "disabled services = {\n\t\"io.iconcraft.keepapplied\" => false\n}\n"
	if parseKeepAgentDisabled(out) {
		t.Fatal("enabled keeper was reported disabled")
	}
	if parseKeepAgentDisabled("disabled services = {}\n") {
		t.Fatal("missing keeper was reported disabled")
	}
}
