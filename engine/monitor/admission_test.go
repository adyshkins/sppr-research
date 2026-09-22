package monitor

import (
	"reflect"
	"testing"
)

func TestExternalBlockRequiresReasonAndPreservesState(t *testing.T) {
	m := makeMonitor(t)
	before := m.Snapshot()
	if _, e := m.Block(packet(0), ""); e == nil {
		t.Fatal("empty reason")
	}
	if !reflect.DeepEqual(before, m.Snapshot()) {
		t.Fatal("invalid block mutated")
	}
	r, e := m.Block(packet(0), "INFORMATION_REQUIRED")
	if e != nil || r.Computed || r.KPI != nil || r.Event != nil || m.Snapshot().NextDay != 1 {
		t.Fatal(r, e)
	}
}
func TestForkIndependentState(t *testing.T) {
	m := makeMonitor(t)
	fork := m.Fork()
	mustObserve(t, fork, packet(0))
	if m.Snapshot().NextDay != 0 || fork.Snapshot().NextDay != 1 {
		t.Fatal("fork shares state")
	}
}
