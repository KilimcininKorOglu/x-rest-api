package xapi

import "testing"

// TestScanFeatureSwitches checks that per-op featureSwitches names are pulled from
// a bundle-shaped string, one entry per op, first occurrence wins.
func TestScanFeatureSwitches(t *testing.T) {
	js := `a={queryId:"q1",operationName:"SearchTimeline",operationType:"query",` +
		`metadata:{featureSwitches:["flag_a","flag_b"],fieldToggles:["x"]}},` +
		`b={queryId:"q2",operationName:"UserTweets",metadata:{featureSwitches:["flag_c"]}}`
	m := scanFeatureSwitches([]string{js})
	if got := m["SearchTimeline"]; len(got) != 2 || got[0] != "flag_a" || got[1] != "flag_b" {
		t.Errorf("SearchTimeline flags = %v", got)
	}
	if got := m["UserTweets"]; len(got) != 1 || got[0] != "flag_c" {
		t.Errorf("UserTweets flags = %v", got)
	}
}

// TestScanQueryIDs checks operationName -> queryId extraction.
func TestScanQueryIDs(t *testing.T) {
	js := `x={queryId:"ABC123",operationName:"SearchTimeline"},y={queryId:"DEF456",operationName:"UserTweets"}`
	m := scanQueryIDs([]string{js})
	if m["SearchTimeline"] != "ABC123" || m["UserTweets"] != "DEF456" {
		t.Errorf("ids = %v", m)
	}
}
