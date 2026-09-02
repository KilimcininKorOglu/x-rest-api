//go:build live

package xapi

import (
	"fmt"
	"testing"
	"time"
)

// TestLiveListWrite exercises the full list-management lifecycle and deletes the
// list it creates, so the account is left unchanged. Run with:
//
//	go test -tags live -run TestLiveListWrite -count=1 -v ./internal/xapi/
func TestLiveListWrite(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)
	name := fmt.Sprintf("smoke %d", time.Now().Unix())

	id, err := c.CreateList(name, "created by a smoke test", true)
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	t.Logf("CreateList: id=%s", id)
	// Always clean up, even if a later step fails.
	defer func() {
		if e := c.DeleteList(id); e != nil {
			t.Errorf("cleanup DeleteList: %v", e)
		} else {
			t.Log("DeleteList: OK (cleaned up)")
		}
	}()

	if info, e := c.ListInfo(id); e != nil {
		t.Errorf("ListInfo: %v", e)
	} else {
		t.Logf("ListInfo: name=%q private=%v members=%d", info.Name, info.Private, info.MemberCount)
	}

	newName := name + " upd"
	pub := false
	if e := c.UpdateList(id, &newName, nil, &pub); e != nil {
		t.Errorf("UpdateList: %v", e)
	} else {
		t.Log("UpdateList: OK")
	}

	if e := c.ListAddMember(id, "12"); e != nil { // jack
		t.Errorf("ListAddMember: %v", e)
	} else {
		t.Log("ListAddMember: OK")
		if e := c.ListRemoveMember(id, "12"); e != nil {
			t.Errorf("ListRemoveMember: %v", e)
		} else {
			t.Log("ListRemoveMember: OK")
		}
	}

	if e := c.MuteList(id); e != nil {
		t.Errorf("MuteList: %v", e)
	} else {
		t.Log("MuteList: OK")
		if e := c.UnmuteList(id); e != nil {
			t.Errorf("UnmuteList: %v", e)
		} else {
			t.Log("UnmuteList: OK")
		}
	}
}
