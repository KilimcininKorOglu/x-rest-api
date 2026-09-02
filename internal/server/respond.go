package server

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"x-rest-api/internal/xapi"
)

// writeJSON encodes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("respond: encode: %v", err)
	}
}

// writeData wraps a successful payload as {"data": ...}.
func writeData(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// writeDataCursor wraps a payload as {"data": ...} plus a top-level "next_cursor"
// when one is present, so paginated reads can be stepped manually.
func writeDataCursor(w http.ResponseWriter, data any, cursor string) {
	body := map[string]any{"data": data}
	if cursor != "" {
		body["next_cursor"] = cursor
	}
	writeJSON(w, http.StatusOK, body)
}

// writeResult writes a read payload as JSON, or as CSV when ?format=csv is set and
// the payload is a list of records. CSV requests on a non-list payload (raw/single)
// get a 400 rather than a silent JSON fallback.
func writeResult(w http.ResponseWriter, r *http.Request, data any, cursor string) {
	if wantsRSS(r) {
		if !writeRSS(w, r, data) {
			writeError(w, http.StatusBadRequest, "rss is only available for tweet-list endpoints")
		}
		return
	}
	if strings.ToLower(r.URL.Query().Get("format")) != "csv" {
		writeDataCursor(w, data, cursor)
		return
	}
	if !writeCSV(w, data, cursor) {
		writeError(w, http.StatusBadRequest, "format=csv is only available for list results (drop raw=true and use a list endpoint)")
	}
}

// writeCSV renders a slice of flat structs as CSV, using json tags as headers and
// JSON-encoding any nested field. It returns false (writing nothing) when data is
// not a slice of structs, so the caller can report the error. The next cursor goes
// in the X-Next-Cursor header, because CSV has no envelope.
func writeCSV(w http.ResponseWriter, data any, cursor string) bool {
	rv := reflect.ValueOf(data)
	if rv.Kind() != reflect.Slice {
		return false
	}
	elem := rv.Type().Elem()
	if elem.Kind() != reflect.Struct {
		return false
	}
	cols, tags := csvColumns(elem)
	w.Header().Set("content-type", "text/csv; charset=utf-8")
	if cursor != "" {
		w.Header().Set("X-Next-Cursor", cursor)
	}
	cw := csv.NewWriter(w)
	_ = cw.Write(cols)
	for i := 0; i < rv.Len(); i++ {
		row := make([]string, len(tags))
		item := rv.Index(i)
		for j := range tags {
			row[j] = csvCell(item.Field(tags[j].index))
		}
		_ = cw.Write(row)
	}
	cw.Flush()
	return true
}

// csvField pairs a struct field's header name with its index.
type csvField struct {
	name  string
	index int
}

// csvColumns derives CSV headers from a struct's json tags, skipping tag "-".
func csvColumns(t reflect.Type) ([]string, []csvField) {
	var cols []string
	var fields []csvField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		cols = append(cols, name)
		fields = append(fields, csvField{name: name, index: i})
	}
	return cols, fields
}

// csvCell renders one field value: scalars directly, anything nested as JSON.
func csvCell(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	}
	if v.IsZero() {
		return ""
	}
	b, err := json.Marshal(v.Interface())
	if err != nil {
		return ""
	}
	return string(b)
}

// writeError wraps an error message as {"error": {"message": ...}}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": msg}})
}

// fail maps a client/upstream error to an HTTP status, writes it, and records the
// upstream status and message on the request for logging. Upstream non-2xx statuses
// are mirrored (so callers see rate limits); a missing transaction id is a 400;
// anything else is a 502.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	up, isUp := errors.AsType[*xapi.UpstreamError](err)
	_, isTx := errors.AsType[*xapi.TxRequiredError](err)
	if ri := getReqInfo(r); ri != nil {
		ri.errMsg = err.Error()
		if isUp {
			ri.upstreamStatus = &up.Status
		}
	}
	switch {
	case isTx:
		writeError(w, http.StatusBadRequest, err.Error())
	case isUp:
		writeError(w, up.Status, err.Error())
	default:
		writeError(w, http.StatusBadGateway, err.Error())
	}
}
