package protocol

import (
	"encoding/json"
	"testing"
)

func TestNewStatusChangedTrace(t *testing.T) {
	payload := NewStatusChangedTrace("registered", "normalized", "normalize")

	if payload.Step != StepStatusChanged {
		t.Fatalf("expected step %q, got %q", StepStatusChanged, payload.Step)
	}
	if payload.Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
	if payload.DataSnapshot["from_status"] != "registered" {
		t.Fatalf("unexpected from_status: %v", payload.DataSnapshot["from_status"])
	}
	if payload.DataSnapshot["to_status"] != "normalized" {
		t.Fatalf("unexpected to_status: %v", payload.DataSnapshot["to_status"])
	}
	if payload.DataSnapshot["event"] != "normalize" {
		t.Fatalf("unexpected event: %v", payload.DataSnapshot["event"])
	}
}

func TestMarshalTracePayload(t *testing.T) {
	payload := NewStatusChangedTrace("registered", "normalized", "normalize")

	data, err := MarshalTracePayload(payload)
	if err != nil {
		t.Fatalf("marshal trace payload: %v", err)
	}

	var got TracePayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal trace payload: %v", err)
	}
	if got.Step != StepStatusChanged {
		t.Fatalf("expected step %q, got %q", StepStatusChanged, got.Step)
	}
	if got.Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be serialized")
	}
}
