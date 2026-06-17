package api

import "testing"

func TestDataTypesCatalog(t *testing.T) {
	all := DataTypes()
	if len(all) != 31 {
		t.Fatalf("catalog has %d types, want 31", len(all))
	}
	// Sorted by endpoint name.
	for i := 1; i < len(all); i++ {
		if all[i-1].EndpointName > all[i].EndpointName {
			t.Errorf("catalog not sorted: %q before %q", all[i-1].EndpointName, all[i].EndpointName)
		}
	}
}

func TestLookupDataTypeForms(t *testing.T) {
	for _, name := range []string{"heart-rate", "heart_rate", "Heart Rate", "HEART-RATE"} {
		dt, ok := LookupDataType(name)
		if !ok {
			t.Fatalf("LookupDataType(%q) not found", name)
		}
		if dt.EndpointName != "heart-rate" {
			t.Errorf("LookupDataType(%q).EndpointName = %q, want heart-rate", name, dt.EndpointName)
		}
	}
	if _, ok := LookupDataType("not-a-type"); ok {
		t.Error("LookupDataType(not-a-type) = found, want missing")
	}
}

func TestDataTypeScopeAndOps(t *testing.T) {
	hr, _ := LookupDataType("heart-rate")
	if !hr.Supports("list") {
		t.Error("heart-rate should support list")
	}
	if want := "https://www.googleapis.com/auth/googlehealth.health_metrics_and_measurements.readonly"; hr.ReadScope() != want {
		t.Errorf("ReadScope = %q, want %q", hr.ReadScope(), want)
	}

	// A rollup/reconcile-only type must report list unsupported.
	tc, _ := LookupDataType("total-calories")
	if tc.Supports("list") {
		t.Error("total-calories should NOT support list")
	}
}
