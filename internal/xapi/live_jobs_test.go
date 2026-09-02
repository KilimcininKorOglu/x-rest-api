//go:build live

package xapi

import "testing"

// TestLiveJobs smoke-tests the X Jobs reads. Run with:
//
//	go test -tags live -run TestLiveJobs -count=1 -v ./internal/xapi/
func TestLiveJobs(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	jobs, cur, err := c.JobSearch(JobSearchFilter{Keyword: "engineer"}, 10, "")
	if err != nil {
		t.Fatalf("JobSearch: %v", err)
	}
	t.Logf("JobSearch: %d jobs, hasCursor=%v", len(jobs), cur != "")
	if len(jobs) > 0 {
		t.Logf("  first: id=%s title=%q company=%v", jobs[0].ID, jobs[0].Title, jobs[0].Company != nil)
		if d, e := c.JobDetails(jobs[0].ID); e != nil {
			t.Logf("  JobDetails: FAIL %v", e)
		} else if d != nil {
			t.Logf("  JobDetails: title=%q location=%q", d.Title, d.Location)
		}
	}

	locs, err := c.JobLocations("London")
	if err != nil {
		t.Fatalf("JobLocations: %v", err)
	}
	t.Logf("JobLocations: %d results", len(locs))
	if len(locs) > 0 {
		t.Logf("  first: id=%s name=%q", locs[0].ID, locs[0].Name)
	}
}
