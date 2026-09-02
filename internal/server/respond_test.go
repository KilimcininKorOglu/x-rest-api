package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"x-rest-api/internal/xapi"
)

func TestWriteCSV(t *testing.T) {
	rec := httptest.NewRecorder()
	data := []xapi.XUser{
		{RestID: "1", ScreenName: "a", Name: "Alice", FollowersCount: 3},
		{RestID: "2", ScreenName: "b", Name: "Bob"},
	}
	if !writeCSV(rec, data, "CUR") {
		t.Fatal("writeCSV returned false for a slice of structs")
	}
	if ct := rec.Header().Get("content-type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type = %q", ct)
	}
	if rec.Header().Get("X-Next-Cursor") != "CUR" {
		t.Errorf("missing X-Next-Cursor header")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "rest_id,screen_name,name") {
		t.Errorf("header row missing json tags: %q", body)
	}
	if !strings.Contains(body, "1,a,Alice") {
		t.Errorf("row missing: %q", body)
	}
}

func TestWriteCSVRejectsNonSlice(t *testing.T) {
	rec := httptest.NewRecorder()
	if writeCSV(rec, map[string]any{"x": 1}, "") {
		t.Error("writeCSV should return false for a non-slice payload")
	}
}
