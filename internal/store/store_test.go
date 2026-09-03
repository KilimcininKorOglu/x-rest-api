package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSettingsRoundtrip(t *testing.T) {
	s := openTemp(t)
	if got, _ := s.GetSetting(SettingProxy, "def"); got != "def" {
		t.Errorf("missing key should return default, got %q", got)
	}
	if err := s.SetSetting(SettingEnableWrites, "true"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if !s.GetSettingBool(SettingEnableWrites, false) {
		t.Errorf("enable_writes should be true")
	}
}

func TestPerOpLocking(t *testing.T) {
	s := openTemp(t)
	id1, err := s.CreateAccount("acc1", "at1", "ct1", true)
	if err != nil {
		t.Fatalf("create acc1: %v", err)
	}
	if _, err := s.CreateAccount("acc2", "at2", "ct2", true); err != nil {
		t.Fatalf("create acc2: %v", err)
	}

	// Lock acc1 for SearchTimeline only.
	if err := s.LockAccountOp(id1, "SearchTimeline", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("lock: %v", err)
	}

	// SearchTimeline skips acc1; UserTweets still sees both.
	if got, _ := s.ListAvailableAccountsForOp("SearchTimeline"); len(got) != 1 || got[0].Label != "acc2" {
		t.Errorf("SearchTimeline available = %v, want [acc2]", got)
	}
	if got, _ := s.ListAvailableAccountsForOp("UserTweets"); len(got) != 2 {
		t.Errorf("UserTweets available = %d, want 2", len(got))
	}

	// An expired lock frees the account for the op again.
	if err := s.LockAccountOp(id1, "SearchTimeline", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("relock: %v", err)
	}
	if got, _ := s.ListAvailableAccountsForOp("SearchTimeline"); len(got) != 2 {
		t.Errorf("after expiry SearchTimeline available = %d, want 2", len(got))
	}

	// Deleting an account removes its locks.
	if err := s.LockAccountOp(id1, "SearchTimeline", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("relock2: %v", err)
	}
	if err := s.DeleteAccount(id1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if locks, _ := s.ActiveLocksByAccount(); len(locks[id1]) != 0 {
		t.Errorf("deleted account still has locks: %v", locks[id1])
	}
}

func TestAPIKeyLookup(t *testing.T) {
	s := openTemp(t)
	if _, err := s.CreateAPIKey("k1", "secret123", true, nil); err != nil {
		t.Fatalf("create key: %v", err)
	}
	k, err := s.GetAPIKeyByKey("secret123")
	if err != nil || !k.CanWrite || !k.Enabled {
		t.Fatalf("lookup = %+v, %v", k, err)
	}
}

func TestAdminAndSession(t *testing.T) {
	s := openTemp(t)
	n, _ := s.CountAdmins()
	if n != 0 {
		t.Fatalf("fresh db should have 0 admins, got %d", n)
	}
	id, err := s.CreateAdmin("root", "hash")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := s.CreateSession("sess1", id, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if got, ok := s.SessionAdminID("sess1"); !ok || got != id {
		t.Errorf("session lookup failed: %d, %v", got, ok)
	}
	// Expired session is rejected.
	_ = s.CreateSession("sess2", id, time.Now().Add(-time.Hour))
	if _, ok := s.SessionAdminID("sess2"); ok {
		t.Errorf("expired session should be rejected")
	}
}

func TestLogInsertAndList(t *testing.T) {
	s := openTemp(t)
	up := 200
	err := s.InsertLog(RequestLog{
		Method: "GET", Path: "/v1/users/naval", Status: 200,
		DurationMS: 12, UpstreamStatus: &up, RemoteIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("insert log: %v", err)
	}
	logs, err := s.ListLogs(LogFilter{Limit: 10})
	if err != nil || len(logs) != 1 || logs[0].Path != "/v1/users/naval" {
		t.Fatalf("list logs = %+v, %v", logs, err)
	}
}

func TestDailyUsageCounter(t *testing.T) {
	s := openTemp(t)
	id, err := s.CreateAccount("d1", "a", "c", true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for range 3 {
		if err := s.MarkAccountUsed(id); err != nil {
			t.Fatalf("mark: %v", err)
		}
	}
	a, err := s.GetAccount(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if a.DailyCount != 3 {
		t.Errorf("DailyCount = %d, want 3", a.DailyCount)
	}
	if a.DailyDate != DailyStamp(time.Now()) {
		t.Errorf("DailyDate = %q, want today", a.DailyDate)
	}
}
