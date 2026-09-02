package xapi

// Runtime generator for the x-client-transaction-id header. x.com
// enforces this header only on hardened ops (search here). It is derived from the
// home page's twitter-site-verification key plus the loading-x-anim SVG frames,
// so it needs no headless browser. The animation key is computed once; the id is
// then keyed to each request's method+path.

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"golang.org/x/net/html"
)

const (
	defaultKeyword       = "obfiowerehiring"
	additionalRandom     = 3
	onDemandFileURL      = "https://abs.twimg.com/responsive-web/client-web/ondemand.s.%sa.js"
	txEpochOffsetSeconds = 1682924400
	txTotalTime          = 4096
)

var (
	onDemandFileRe = regexp.MustCompile(`,(\d+):["']ondemand\.s["']`)
	indicesRe      = regexp.MustCompile(`\(\w\[(\d{1,2})\],\s*16\)`)
	digitsRe       = regexp.MustCompile(`[^\d]+`)
)

// txGen builds and caches the animation key, then generates per-request ids.
type txGen struct {
	session   tls_client.HttpClient
	userAgent string
	cookie    string

	once           sync.Once
	initErr        error
	keyBytes       []byte
	animationKey   string
	rowIndex       int
	keyByteIndices []int
}

func newTxGen(session tls_client.HttpClient, userAgent, cookie string) *txGen {
	return &txGen{session: session, userAgent: userAgent, cookie: cookie}
}

// TransactionID returns the header value for method+path, building the animation
// key on first use. An empty method defaults to GET.
func (g *txGen) TransactionID(method, path string) (string, error) {
	g.once.Do(g.build)
	if g.initErr != nil {
		return "", g.initErr
	}
	if method == "" {
		method = "GET"
	}
	return g.generate(method, path), nil
}

// build fetches the home page and ondemand.s file and derives the animation key.
func (g *txGen) build() {
	home, err := g.fetch("https://x.com/home")
	if err != nil {
		g.initErr = fmt.Errorf("tx: fetch home: %w", err)
		return
	}
	doc, err := html.Parse(strings.NewReader(home))
	if err != nil {
		g.initErr = fmt.Errorf("tx: parse home: %w", err)
		return
	}
	key := metaContent(doc, "twitter-site-verification")
	if key == "" {
		g.initErr = fmt.Errorf("tx: twitter-site-verification key not found")
		return
	}
	g.keyBytes, err = base64.StdEncoding.DecodeString(key)
	if err != nil {
		g.initErr = fmt.Errorf("tx: decode key: %w", err)
		return
	}

	ondemand, err := g.fetchOndemand(home)
	if err != nil {
		g.initErr = err
		return
	}
	if err := g.parseIndices(ondemand); err != nil {
		g.initErr = err
		return
	}
	frameRow, err := frameRow(doc, g.keyBytes, g.rowIndex)
	if err != nil {
		g.initErr = err
		return
	}
	g.animationKey = g.animate(frameRow)
}

// fetch performs a browser-like GET and returns the body as text.
func (g *txGen) fetch(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header = http.Header{
		"accept":            {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		"accept-language":   {"en-US,en;q=0.9"},
		"user-agent":        {g.userAgent},
		"cookie":            {g.cookie},
		http.HeaderOrderKey: {"accept", "accept-language", "user-agent", "cookie"},
	}
	resp, err := g.session.Do(req)
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

// fetchOndemand resolves the ondemand.s file URL from the home HTML and fetches it.
func (g *txGen) fetchOndemand(home string) (string, error) {
	m := onDemandFileRe.FindStringSubmatch(home)
	if m == nil {
		return "", fmt.Errorf("tx: ondemand.s index not found")
	}
	hashRe := regexp.MustCompile(`,` + m[1] + `:"([0-9a-f]+)"`)
	hm := hashRe.FindStringSubmatch(home)
	if hm == nil {
		return "", fmt.Errorf("tx: ondemand.s hash not found")
	}
	return g.fetch(fmt.Sprintf(onDemandFileURL, hm[1]))
}

// parseIndices extracts the row index and key-byte indices from the ondemand file.
func (g *txGen) parseIndices(ondemand string) error {
	matches := indicesRe.FindAllStringSubmatch(ondemand, -1)
	if len(matches) < 2 {
		return fmt.Errorf("tx: could not get KEY_BYTE indices")
	}
	nums := make([]int, 0, len(matches))
	for _, m := range matches {
		nums = append(nums, atoiSafe(m[1]))
	}
	g.rowIndex = nums[0]
	g.keyByteIndices = nums[1:]
	return nil
}

// generate builds the transaction id for one request.
func (g *txGen) generate(method, path string) string {
	timeNow := int((time.Now().UnixMilli() - txEpochOffsetSeconds*1000) / 1000)
	timeBytes := []byte{
		byte(timeNow & 0xFF), byte((timeNow >> 8) & 0xFF),
		byte((timeNow >> 16) & 0xFF), byte((timeNow >> 24) & 0xFF),
	}
	payload := fmt.Sprintf("%s!%s!%d%s%s", method, path, timeNow, defaultKeyword, g.animationKey)
	sum := sha256.Sum256([]byte(payload))

	buf := make([]byte, 0, len(g.keyBytes)+4+16+1)
	buf = append(buf, g.keyBytes...)
	buf = append(buf, timeBytes...)
	buf = append(buf, sum[:16]...)
	buf = append(buf, additionalRandom)

	randNum := byte(rand.Intn(256))
	out := make([]byte, 0, len(buf)+1)
	out = append(out, randNum)
	for _, b := range buf {
		out = append(out, b^randNum)
	}
	return strings.TrimRight(base64.StdEncoding.EncodeToString(out), "=")
}

// animate turns one frame row and the derived target time into the animation key.
func (g *txGen) animate(frames []int) string {
	fromColor := []float64{float64(frames[0]), float64(frames[1]), float64(frames[2]), 1.0}
	toColor := []float64{float64(frames[3]), float64(frames[4]), float64(frames[5]), 1.0}
	fromRotation := []float64{0.0}
	toRotation := []float64{solve(float64(frames[6]), 60.0, 360.0, true)}

	rest := frames[7:]
	curves := make([]float64, len(rest))
	for i, item := range rest {
		curves[i] = solve(float64(item), isOdd(i), 1.0, false)
	}

	frameTime := 1
	for _, idx := range g.keyByteIndices {
		frameTime *= int(g.keyBytes[idx]) % 16
	}
	frameTime = int(math.Round(float64(frameTime)/10)) * 10
	targetTime := float64(frameTime) / txTotalTime

	val := newCubic(curves).value(targetTime)
	color := clampColor(interpolate(fromColor, toColor, val))
	rotation := interpolate(fromRotation, toRotation, val)
	matrix := rotationMatrix(rotation[0])

	var sb strings.Builder
	for i := 0; i < len(color)-1; i++ {
		fmt.Fprintf(&sb, "%x", int(math.RoundToEven(color[i])))
	}
	for _, v := range matrix {
		rounded := roundEven2(v)
		if rounded < 0 {
			rounded = -rounded
		}
		hexVal := floatToHex(rounded)
		switch {
		case strings.HasPrefix(hexVal, "."):
			sb.WriteString(strings.ToLower("0" + hexVal))
		case hexVal != "":
			sb.WriteString(hexVal)
		default:
			sb.WriteString("0")
		}
	}
	sb.WriteString("00")
	return strings.NewReplacer(".", "", "-", "").Replace(sb.String())
}

// solve maps a byte value into [min,max], flooring or rounding to 2 decimals.
func solve(value, minVal, maxVal float64, rounding bool) float64 {
	result := value*(maxVal-minVal)/255 + minVal
	if rounding {
		return math.Floor(result)
	}
	return roundEven2(result)
}

func isOdd(n int) float64 {
	if n%2 != 0 {
		return -1.0
	}
	return 0.0
}

// roundEven2 rounds to 2 decimals with banker's rounding, matching Python round().
func roundEven2(x float64) float64 {
	return math.RoundToEven(x*100) / 100
}

func clampColor(in []float64) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = math.Max(0, math.Min(255, v))
	}
	return out
}
