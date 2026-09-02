package xapi

// Runtime query-id refresh. x.com rotates each op's queryId on web releases, so
// a static ops.json eventually 404s. FetchQueryIDs reads the client bundle and
// extracts the live queryId/operationName pairs, which the server persists and
// feeds back via SetQueryIDs. features/variables stay from ops.json.

import (
	"fmt"
	"io"
	"regexp"

	http "github.com/bogdanfinn/fhttp"
)

var (
	bundleURLRe = regexp.MustCompile(`https://abs\.twimg\.com/responsive-web/client-web[a-zA-Z-]*/(?:main|api)\.[0-9a-f]+\.js`)
	queryIDRe   = regexp.MustCompile(`queryId:"([^"]+)",operationName:"([^"]+)"`)
	// featureSwitches follows operationName in the same op-definition object; [^}]
	// keeps the match inside that object, and the flag list has no nested brackets.
	featSwitchRe = regexp.MustCompile(`operationName:"([^"]+)"[^}]*?featureSwitches:\[([^\]]*)\]`)
	quotedRe     = regexp.MustCompile(`"([^"]+)"`)
)

// fetchBundleJS fetches x.com's home page, finds the client bundle URLs, and
// returns each bundle's JS text. Shared by the queryId and feature-switch scans.
func (s *Session) fetchBundleJS() ([]string, error) {
	home, err := s.fetchText("https://x.com/home")
	if err != nil {
		return nil, fmt.Errorf("queryids: fetch home: %w", err)
	}
	bundles := dedupeStrings(bundleURLRe.FindAllString(home, -1))
	if len(bundles) == 0 {
		return nil, fmt.Errorf("queryids: no client bundle URL found in home page")
	}
	var out []string
	for _, url := range bundles {
		js, err := s.fetchText(url)
		if err != nil {
			continue // skip a bundle we cannot fetch; others may still work
		}
		out = append(out, js)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("queryids: could not fetch any of %d client bundle(s)", len(bundles))
	}
	return out, nil
}

// FetchQueryIDs fetches x.com's client bundle(s) and returns operationName ->
// queryId for every pair it finds. Coverage depends on the bundle; callers merge
// the result over the embedded ops.json rather than replacing it.
func (s *Session) FetchQueryIDs() (map[string]string, error) {
	js, err := s.fetchBundleJS()
	if err != nil {
		return nil, err
	}
	ids := scanQueryIDs(js)
	if len(ids) == 0 {
		return nil, fmt.Errorf("queryids: no queryId/operationName pairs found in bundle(s)")
	}
	return ids, nil
}

// FetchManifest fetches the bundle once and returns both the queryId overrides and
// the per-op feature-flag names, so a refresh does not download the bundle twice.
func (s *Session) FetchManifest() (map[string]string, map[string][]string, error) {
	js, err := s.fetchBundleJS()
	if err != nil {
		return nil, nil, err
	}
	ids := scanQueryIDs(js)
	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("queryids: no queryId/operationName pairs found in bundle(s)")
	}
	return ids, scanFeatureSwitches(js), nil
}

// scanQueryIDs extracts operationName -> queryId from the bundle texts.
func scanQueryIDs(bundles []string) map[string]string {
	ids := map[string]string{}
	for _, js := range bundles {
		for _, m := range queryIDRe.FindAllStringSubmatch(js, -1) {
			ids[m[2]] = m[1] // operationName -> queryId
		}
	}
	return ids
}

// scanFeatureSwitches extracts operationName -> [feature flag names] from the
// bundle texts, reading the featureSwitches list x.com attaches to each op.
func scanFeatureSwitches(bundles []string) map[string][]string {
	out := map[string][]string{}
	for _, js := range bundles {
		for _, m := range featSwitchRe.FindAllStringSubmatch(js, -1) {
			op := m[1]
			if _, seen := out[op]; seen {
				continue
			}
			var flags []string
			for _, q := range quotedRe.FindAllStringSubmatch(m[2], -1) {
				flags = append(flags, q[1])
			}
			if len(flags) > 0 {
				out[op] = flags
			}
		}
	}
	return out
}

// fetchText performs a browser-like GET and returns the body as text.
func (s *Session) fetchText(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header = http.Header{
		"accept":            {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		"accept-language":   {"en-US,en;q=0.9"},
		"user-agent":        {s.userAgent},
		http.HeaderOrderKey: {"accept", "accept-language", "user-agent"},
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
