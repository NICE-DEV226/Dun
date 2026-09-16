package extension

import "testing"

func TestContractValues(t *testing.T) {
	if ABIVersion != 1 {
		t.Fatalf("ABI version = %d, want 1", ABIVersion)
	}
	if ABIMarkerExport != "dun_abi_version_0_1" {
		t.Fatalf("unexpected marker export %q", ABIMarkerExport)
	}
	if ProcessRequestExport != "dun_process_request" {
		t.Fatalf("unexpected process export %q", ProcessRequestExport)
	}
	if DecisionContinue.String() != "continue" || DecisionError.String() != "error" {
		t.Fatal("decision names are not stable")
	}
	if ErrnoOK != 0 || ErrnoInvalidCall != 1 {
		t.Fatal("errno values are not stable")
	}
	if !ValidateDecision(uint32(DecisionReject)) || ValidateDecision(99) {
		t.Fatal("decision validation is not stable")
	}
}
