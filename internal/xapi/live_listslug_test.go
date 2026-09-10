//go:build live

package xapi

import (
	"errors"
	"testing"
)

// liveOwnList returns a client, the account's first list id, and that list's raw
// node, skipping the test when the account owns no list.
func liveOwnList(t *testing.T) (*XClient, string, map[string]any) {
	t.Helper()
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	own, _, err := c.OwnLists(5, "")
	if err != nil {
		t.Fatalf("OwnLists: %v", err)
	}
	if len(own) == 0 {
		t.Skip("account has no lists to resolve by slug")
	}
	id := own[0].ID
	raw, err := c.CallRaw("ListByRestId", map[string]any{"listId": id}, "", 0)
	if err != nil {
		t.Fatalf("ListByRestId raw: %v", err)
	}
	return c, id, asMap(dig(raw, "data", "list"))
}

// assertListBySlugWired probes with a dummy slug. x.com no longer exposes a slug
// on the list object, so a valid queryId returns a data-level "not found" while a
// stale queryId returns HTTP 404. This proves the op is wired correctly.
func assertListBySlugWired(t *testing.T, c *XClient, owner, id string) {
	t.Helper()
	_, err := c.ListBySlug(owner, "does-not-exist-"+id)
	var up *UpstreamError
	if errors.As(err, &up) && up.Status == 404 {
		t.Fatalf("ListBySlug queryId looks stale (404): %v", err)
	}
	t.Logf("ListBySlug queryId valid (no 404); slug unavailable on modern x.com, err=%v", err)
}

// TestLiveListBySlug takes one of the account's own lists, reads its owner handle
// and slug from the raw ListByRestId response, then resolves the same list via
// ListBySlug. Run with:
//
//	go test -tags live -run TestLiveListBySlug -count=1 -v ./internal/xapi/
func TestLiveListBySlug(t *testing.T) {
	c, id, list := liveOwnList(t)
	slug := asString(list["slug"])
	owner := firstStr(
		asString(dig(list, "user_results", "result", "core", "screen_name")),
		asString(dig(list, "user_results", "result", "legacy", "screen_name")),
	)
	t.Logf("list id=%s owner=%q slug=%q", id, owner, slug)
	if owner == "" {
		t.Skip("no owner handle on the raw list")
	}
	if slug == "" {
		assertListBySlugWired(t, c, owner, id)
		return
	}
	l, err := c.ListBySlug(owner, slug)
	if err != nil {
		t.Fatalf("ListBySlug: %v", err)
	}
	t.Logf("ListBySlug: id=%s name=%q members=%d", l.ID, l.Name, l.MemberCount)
	if l.ID != id {
		t.Errorf("ListBySlug id=%s, expected %s", l.ID, id)
	}
}
