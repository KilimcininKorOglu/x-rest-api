package xapi

import (
	"encoding/json"
	"strings"
)

// X Jobs. All three ops are GraphQL GET with no feature flags. A job result is
// an { rest_id, result: { core, company_profile_results, user_results } } object;
// parseJob handles both a search item and the single jobData detail.

// JobSearchFilter holds the structured search parameters for JobSearch. Empty
// fields are omitted from the request.
type JobSearchFilter struct {
	Keyword        string
	LocationID     string
	Location       string
	LocationType   string
	SeniorityLevel string
	CompanyName    string
	EmploymentType string
	Industry       string
}

// searchParams renders the filter into x.com's searchParams object, dropping
// empty fields.
func (f JobSearchFilter) searchParams() map[string]any {
	sp := map[string]any{}
	put := func(k, v string) {
		if v != "" {
			sp[k] = v
		}
	}
	put("keyword", f.Keyword)
	put("job_location_id", f.LocationID)
	put("job_location", f.Location)
	put("job_location_type", f.LocationType)
	put("seniority_level", f.SeniorityLevel)
	put("company_name", f.CompanyName)
	put("employment_type", f.EmploymentType)
	put("industry", f.Industry)
	return sp
}

// JobSearch searches X Jobs and returns the matches plus a next cursor.
func (c *XClient) JobSearch(f JobSearchFilter, count int, cursor string) ([]Job, string, error) {
	vars := map[string]any{"searchParams": f.searchParams()}
	if count > 0 {
		vars["count"] = count
	}
	if cursor != "" {
		vars["cursor"] = cursor
	}
	payload, err := c.call("JobSearchQueryScreenJobsQuery", vars)
	if err != nil {
		return nil, "", err
	}
	var jobs []Job
	for _, it := range asSlice(dig(payload, "data", "job_search", "items_results")) {
		if j := parseJob(asMap(it)); j != nil {
			jobs = append(jobs, *j)
		}
	}
	return jobs, asString(dig(payload, "data", "job_search", "slice_info", "next_cursor")), nil
}

// JobDetails returns one job by id.
func (c *XClient) JobDetails(id string) (*Job, error) {
	payload, err := c.call("JobScreenQuery", map[string]any{"jobId": id})
	if err != nil {
		return nil, err
	}
	return parseJob(asMap(dig(payload, "data", "jobData"))), nil
}

// JobLocations returns location suggestions for a query.
func (c *XClient) JobLocations(query string) ([]JobLocation, error) {
	payload, err := c.call("LocationSelectorQuery", map[string]any{"query": query})
	if err != nil {
		return nil, err
	}
	var locs []JobLocation
	for _, it := range asSlice(dig(payload, "data", "location_type_ahead")) {
		m := asMap(it)
		if id := asString(m["location_id"]); id != "" {
			locs = append(locs, JobLocation{ID: id, Name: asString(m["display_name"])})
		}
	}
	return locs, nil
}

// parseJob builds a Job from a raw job result, or nil when it carries no core.
func parseJob(job map[string]any) *Job {
	core := asMap(dig(job, "result", "core"))
	if core == nil {
		return nil
	}
	redirect := asString(core["redirect_url"])
	if redirect == "" {
		redirect = asString(core["external_url"])
	}
	return &Job{
		ID:                 asString(job["rest_id"]),
		Title:              asString(core["title"]),
		Description:        jobDescription(core["job_description"]),
		Location:           asString(core["location"]),
		JobFunction:        asString(core["job_function"]),
		FormattedSalary:    asString(core["formatted_salary"]),
		SalaryMin:          asInt(core["salary_min"]),
		SalaryMax:          asInt(core["salary_max"]),
		SalaryInterval:     asInt(core["salary_interval"]),
		SalaryCurrencyCode: asString(core["salary_currency_code"]),
		JobPageURL:         asString(core["job_page_url"]),
		RedirectURL:        redirect,
		IsFeatured:         asBool(core["featured"]),
		Company:            parseJobCompany(asMap(dig(job, "result", "company_profile_results", "result"))),
		User:               parseJobUser(asMap(dig(job, "result", "user_results", "result"))),
	}
}

func parseJobCompany(company map[string]any) *JobCompany {
	if company == nil {
		return nil
	}
	return &JobCompany{
		ID:   asString(company["rest_id"]),
		Name: asString(dig(company, "core", "name")),
		Logo: asString(dig(company, "logo", "normal_url")),
	}
}

func parseJobUser(user map[string]any) *JobUser {
	if user == nil {
		return nil
	}
	return &JobUser{
		ID:           asString(user["rest_id"]),
		UserName:     asString(dig(user, "core", "screen_name")),
		FullName:     asString(dig(user, "core", "name")),
		ProfileImage: asString(dig(user, "avatar", "image_url")),
		IsVerified:   asBool(dig(user, "verification", "verified")),
		VerifiedType: asString(dig(user, "verification", "verified_type")),
	}
}

// jobDescription unwraps a job description: x.com stores it either as plain text
// or as a JSON document whose blocks[].text lines join with newlines.
func jobDescription(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	var doc map[string]any
	if json.Unmarshal([]byte(s), &doc) == nil {
		if blocks := asSlice(doc["blocks"]); blocks != nil {
			var parts []string
			for _, b := range blocks {
				parts = append(parts, asString(asMap(b)["text"]))
			}
			return strings.Join(parts, "\n")
		}
	}
	return s
}
