package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCanonicalMarket_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	vol := 50000.0
	note := "CFTC regulated binary contract"

	m := CanonicalMarket{
		ID:                  "KALSHI:KXFED-MARCH2026",
		Venue:               VenueKalshi,
		Title:               "Fed rate cut March 2026",
		NormalizedTitle:     "fed rate cut march 2026",
		Description:         "Will the Fed cut rates in March 2026?",
		Outcomes:            []Outcome{{Label: "Yes", Price: 0.65}, {Label: "No", Price: 0.35}},
		ResolutionTime:      &now,
		ResolutionTimeUTC:   &now,
		YesPrice:            0.65,
		NoPrice:             0.35,
		Spread:              0.02,
		Liquidity:           125000.0,
		Volume24h:           &vol,
		Status:              StatusOpen,
		ContractType:        ContractBinary,
		SettlementMechanism: SettlementCFTC,
		SettlementNote:      &note,
		RulesHash:           "abc123",
		DataStalenessFlag:   false,
		IngestedAt:          now,
		RawPayload:          json.RawMessage(`{"ticker":"KXFED-MARCH2026"}`),
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CanonicalMarket
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != m.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, m.ID)
	}
	if decoded.Venue != m.Venue {
		t.Errorf("Venue = %q, want %q", decoded.Venue, m.Venue)
	}
	if decoded.YesPrice != m.YesPrice {
		t.Errorf("YesPrice = %f, want %f", decoded.YesPrice, m.YesPrice)
	}
	if len(decoded.Outcomes) != 2 {
		t.Errorf("Outcomes len = %d, want 2", len(decoded.Outcomes))
	}
	if decoded.Status != StatusOpen {
		t.Errorf("Status = %q, want %q", decoded.Status, StatusOpen)
	}
	if decoded.SettlementMechanism != SettlementCFTC {
		t.Errorf("SettlementMechanism = %q, want %q", decoded.SettlementMechanism, SettlementCFTC)
	}
}

func TestCanonicalMarket_OptionalFieldsNil(t *testing.T) {
	m := CanonicalMarket{
		ID:                  "POLYMARKET:0xabc",
		Venue:               VenuePolymarket,
		Title:               "Test market",
		NormalizedTitle:     "test market",
		Outcomes:            []Outcome{{Label: "Yes", Price: 0.50}},
		YesPrice:            0.50,
		NoPrice:             0.50,
		Spread:              0.01,
		Liquidity:           1000.0,
		Status:              StatusOpen,
		ContractType:        ContractBinary,
		SettlementMechanism: SettlementOptimisticOracle,
		RulesHash:           "def456",
		IngestedAt:          time.Now(),
		RawPayload:          json.RawMessage(`{}`),
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CanonicalMarket
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ResolutionTime != nil {
		t.Error("ResolutionTime should be nil")
	}
	if decoded.Volume24h != nil {
		t.Error("Volume24h should be nil")
	}
	if decoded.SettlementNote != nil {
		t.Error("SettlementNote should be nil")
	}
}
