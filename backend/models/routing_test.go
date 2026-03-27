package models

import (
	"encoding/json"
	"testing"
)

func TestSimulatedOnlyTrue_MarshalJSON(t *testing.T) {
	s := simulatedOnlyTrue{}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	if string(data) != "true" {
		t.Errorf("MarshalJSON = %s, want true", data)
	}
}

func TestSimulatedOnlyTrue_IsSimulated(t *testing.T) {
	s := simulatedOnlyTrue{}
	if !s.IsSimulated() {
		t.Error("IsSimulated() = false, want true")
	}
}

func TestRoutingDecision_SimulatedOnlyAlwaysTrue(t *testing.T) {
	rd := RoutingDecision{
		DecisionID: "test-001",
		SelectedVenue: VenueKalshi,
	}

	data, err := json.Marshal(rd)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if string(raw["simulated_only"]) != "true" {
		t.Errorf("simulated_only = %s, want true", raw["simulated_only"])
	}
}
