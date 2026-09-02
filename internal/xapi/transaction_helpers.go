package xapi

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// ---- math ports ------------------------------------------------------------- //

// cubic is a cubic Bezier easing curve defined by four control values.
type cubic struct{ c []float64 }

func newCubic(c []float64) cubic { return cubic{c: c} }

// value evaluates the curve at time t, mirroring the reference get_value.
func (cu cubic) value(t float64) float64 {
	c := cu.c
	if t <= 0.0 {
		g := 0.0
		if c[0] > 0.0 {
			g = c[1] / c[0]
		} else if c[1] == 0.0 && c[2] > 0.0 {
			g = c[3] / c[2]
		}
		return g * t
	}
	if t >= 1.0 {
		g := 0.0
		if c[2] < 1.0 {
			g = (c[3] - 1.0) / (c[2] - 1.0)
		} else if c[2] == 1.0 && c[0] < 1.0 {
			g = (c[1] - 1.0) / (c[0] - 1.0)
		}
		return 1.0 + g*(t-1.0)
	}
	start, end, mid := 0.0, 1.0, 0.0
	for start < end {
		mid = (start + end) / 2
		xEst := cubicCalc(c[0], c[2], mid)
		if abs(t-xEst) < 0.00001 {
			return cubicCalc(c[1], c[3], mid)
		}
		if xEst < t {
			start = mid
		} else {
			end = mid
		}
	}
	return cubicCalc(c[1], c[3], mid)
}

func cubicCalc(a, b, m float64) float64 {
	return 3.0*a*(1-m)*(1-m)*m + 3.0*b*(1-m)*m*m + m*m*m
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// interpolate blends two equal-length vectors by factor f.
func interpolate(from, to []float64, f float64) []float64 {
	out := make([]float64, len(from))
	for i := range from {
		out[i] = from[i]*(1-f) + to[i]*f
	}
	return out
}

// rotationMatrix returns [cos, -sin, sin, cos] for the given degrees.
func rotationMatrix(degrees float64) []float64 {
	rad := degrees * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	return []float64{cos, -sin, sin, cos}
}

// floatToHex ports the reference float-to-hex used for the matrix values.
func floatToHex(x float64) string {
	var result []string
	quotient := int(x)
	fraction := x - float64(quotient)

	for quotient > 0 {
		quotient = int(x / 16)
		remainder := int(x - float64(quotient)*16)
		if remainder > 9 {
			result = append([]string{string(rune(remainder + 55))}, result...)
		} else {
			result = append([]string{strconv.Itoa(remainder)}, result...)
		}
		x = float64(quotient)
	}
	if fraction == 0 {
		return strings.Join(result, "")
	}
	result = append(result, ".")
	for fraction > 0 {
		fraction *= 16
		integer := int(fraction)
		fraction -= float64(integer)
		if integer > 9 {
			result = append(result, string(rune(integer+55)))
		} else {
			result = append(result, strconv.Itoa(integer))
		}
	}
	return strings.Join(result, "")
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// ---- HTML traversal --------------------------------------------------------- //

// metaContent returns the content attribute of <meta name="wantName">.
func metaContent(doc *html.Node, wantName string) string {
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "meta" {
			if attr(n, "name") == wantName {
				found = attr(n, "content")
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

// attr returns the value of the named attribute, or "".
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// elementChildren returns the element (non-text) children of n.
func elementChildren(n *html.Node) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			out = append(out, c)
		}
	}
	return out
}

// animFrames returns the loading-x-anim elements in document order.
func animFrames(doc *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.HasPrefix(attr(n, "id"), "loading-x-anim") {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

// frameRow selects the animation frame row keyed by the verification bytes and
// parses its SVG path into integer groups.
func frameRow(doc *html.Node, keyBytes []byte, rowIdx int) ([]int, error) {
	frames := animFrames(doc)
	if len(frames) < 4 {
		return nil, fmt.Errorf("tx: fewer than 4 loading-x-anim frames")
	}
	frame := frames[int(keyBytes[5])%4]
	svgKids := elementChildren(frame)
	if len(svgKids) == 0 {
		return nil, fmt.Errorf("tx: empty animation frame")
	}
	pathKids := elementChildren(svgKids[0])
	if len(pathKids) < 2 {
		return nil, fmt.Errorf("tx: animation frame missing path")
	}
	d := attr(pathKids[1], "d")
	if len(d) < 9 {
		return nil, fmt.Errorf("tx: animation path too short")
	}

	var arr [][]int
	for seg := range strings.SplitSeq(d[9:], "C") {
		cleaned := strings.TrimSpace(digitsRe.ReplaceAllString(seg, " "))
		if cleaned == "" {
			arr = append(arr, nil)
			continue
		}
		var row []int
		for tok := range strings.FieldsSeq(cleaned) {
			row = append(row, atoiSafe(tok))
		}
		arr = append(arr, row)
	}

	rowValue := int(keyBytes[rowIdx]) % 16
	if rowValue >= len(arr) {
		return nil, fmt.Errorf("tx: row index %d out of range %d", rowValue, len(arr))
	}
	return arr[rowValue], nil
}
