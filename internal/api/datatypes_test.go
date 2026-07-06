package api

import (
	"strings"
	"testing"
)

// TestCatalogIsReadOnly pins the read-only contract: no data type in the
// embedded catalog may advertise a mutating operation, and every operation it
// does advertise must be one of the known read-only ops. This is the immutable
// guard for the "description-behavior mismatch" finding — write ops such as
// create/update/batchDelete must never reappear in datatypes.json. (The package
// init also panics on a write op, so a regression fails the whole test binary at
// load; this test additionally names the offending type and op.)
func TestCatalogIsReadOnly(t *testing.T) {
	// Operations that mutate server state. None may appear in the catalog.
	writeOps := []string{"create", "update", "patch", "replace", "delete", "batchDelete", "batchCreate", "batchUpdate"}
	isWrite := make(map[string]bool, len(writeOps))
	for _, op := range writeOps {
		isWrite[op] = true
	}

	for _, dt := range DataTypes() {
		for _, op := range dt.Operations {
			if isWrite[op] {
				t.Errorf("%s advertises mutating operation %q; tool is read-only", dt.EndpointName, op)
			}
			if !readOnlyOps[op] {
				t.Errorf("%s advertises unknown operation %q (not in the read-only allowlist %v)",
					dt.EndpointName, op, sortedKeys(readOnlyOps))
			}
		}
	}
}

func sortedKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ",")
}

// apiGetOnlyTypes are the catalog types readable by NEITHER typed command
// (`data list` needs the list op; `rollup daily` needs dailyRollUp) — only the
// `api get` escape hatch reaches them. Each entry is a documented exception
// (SKILL.md / AGENTS.md name it); the CLI's unsupported-type hints point these
// at `api get` instead of at a typed command that would also fail.
var apiGetOnlyTypes = map[string]bool{
	"daily-heart-rate-zones": true, // reconcile-only upstream: no list, no dailyRollUp
}

// TestEveryTypeReachableByTypedCommandOrDocumentedException guards catalog
// reachability: every type must be readable by `data list` or `rollup daily`,
// or be explicitly recorded (and therefore documented) in apiGetOnlyTypes.
// Without this, a catalog addition no documented command can read ships
// silently — the caller only finds out from a circular error hint. The second
// loop keeps the exception list honest: an entry that gains a typed op must be
// removed so docs and hints follow.
func TestEveryTypeReachableByTypedCommandOrDocumentedException(t *testing.T) {
	for _, dt := range DataTypes() {
		typed := dt.Supports("list") || dt.Supports("dailyRollUp")
		if !typed && !apiGetOnlyTypes[dt.EndpointName] {
			t.Errorf("%s (ops %v) is readable by no typed command and is not a documented api-get-only exception; "+
				"add the op it supports, or record it in apiGetOnlyTypes AND document it in SKILL.md/AGENTS.md",
				dt.EndpointName, dt.Operations)
		}
		if typed && apiGetOnlyTypes[dt.EndpointName] {
			t.Errorf("%s is listed in apiGetOnlyTypes but supports a typed command (ops %v); remove the stale exception",
				dt.EndpointName, dt.Operations)
		}
	}
}

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
