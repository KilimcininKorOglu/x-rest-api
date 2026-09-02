//go:build live

package xapi

import "testing"

// TestLiveDiscovery smoke-tests the Faz A user-discovery reads. Run with:
//
//	go test -tags live -run TestLiveDiscovery -count=1 -v ./internal/xapi/
func TestLiveDiscovery(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)
	rep := func(name string, n int, err error) {
		if err != nil {
			t.Logf("  FAIL %-14s %v", name, err)
			return
		}
		t.Logf("  ok   %-14s n=%d", name, n)
	}

	sug, _, e := c.Suggestions(false, 5, "")
	rep("Suggestions", len(sug), e)
	aff, _, e := c.Affiliates("jack", 5, "")
	rep("Affiliates", len(aff), e)
	own, _, e := c.OwnLists(10, "")
	rep("OwnLists", len(own), e)
	an, e := c.Analytics("", "", "Day", nil, false)
	rep("Analytics", len(an), e)
}
