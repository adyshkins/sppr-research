package scenario

import "testing"

func TestGenerateActions(t *testing.T) {
	input := validInput()

	actions, err := GenerateActions(input)
	if err != nil {
		t.Fatalf("generate actions: %v", err)
	}
	if len(actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(actions))
	}

	for _, action := range actions {
		if action.Code == "" {
			t.Fatalf("expected action code")
		}
		if action.Description == "" {
			t.Fatalf("expected action description")
		}
		if action.ActionType == "" {
			t.Fatalf("expected action type")
		}
		if action.Metadata["product"] != input.Product {
			t.Fatalf("unexpected product metadata: %v", action.Metadata["product"])
		}
		if action.Metadata["cause_code"] != input.CauseCode {
			t.Fatalf("unexpected cause_code metadata: %v", action.Metadata["cause_code"])
		}
		if action.Metadata["event_id"] != input.EventID {
			t.Fatalf("unexpected event_id metadata: %v", action.Metadata["event_id"])
		}
	}
}
