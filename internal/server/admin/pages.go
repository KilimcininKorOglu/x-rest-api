package admin

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"x-rest-api/internal/store"
)

const logsPageSize = 50

// labelMaps returns id->label for accounts and id->name for keys, for log views.
func (h *Handler) labelMaps() (map[int64]string, map[int64]string) {
	accLabels := map[int64]string{}
	if accs, err := h.st.ListAccounts(); err == nil {
		for _, a := range accs {
			accLabels[a.ID] = a.Label
		}
	}
	keyNames := map[int64]string{}
	if keys, err := h.st.ListAPIKeys(); err == nil {
		for _, k := range keys {
			keyNames[k.ID] = k.Name
		}
	}
	return accLabels, keyNames
}

// ---- dashboard -------------------------------------------------------------- //

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	accs, _ := h.st.ListAccounts()
	avail, _ := h.st.ListAvailableAccounts()
	keys, _ := h.st.ListAPIKeys()
	reqs, _ := h.st.CountLogs()
	recent, _ := h.st.ListLogs(store.LogFilter{Limit: 10})
	accLabels, _ := h.labelMaps()
	h.render(w, r, "dashboard", map[string]any{
		"Title":  "Dashboard",
		"Active": "dashboard",
		"Stats": map[string]int{
			"Accounts": len(accs), "Available": len(avail), "Keys": len(keys), "Requests": reqs,
		},
		"Recent":        recent,
		"AccountLabels": accLabels,
	})
}

// ---- accounts --------------------------------------------------------------- //

func (h *Handler) accountsPage(w http.ResponseWriter, r *http.Request) {
	accs, _ := h.st.ListAccounts()
	locks, _ := h.st.ActiveLocksByAccount()
	h.render(w, r, "accounts", map[string]any{
		"Title": "Accounts", "Active": "accounts", "Accounts": accs, "Locks": locks,
	})
}

func (h *Handler) accountCreate(w http.ResponseWriter, r *http.Request) {
	label := r.FormValue("label")
	at := r.FormValue("auth_token")
	ct0 := r.FormValue("ct0")
	if label == "" || at == "" || ct0 == "" {
		setFlash(w, r, "err", "label, auth_token and ct0 are required")
		http.Redirect(w, r, "/admin/accounts", http.StatusFound)
		return
	}
	if _, err := h.st.CreateAccount(label, at, ct0, r.FormValue("enabled") != ""); err != nil {
		setFlash(w, r, "err", err.Error())
	} else {
		setFlash(w, r, "ok", "account added")
	}
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

func (h *Handler) accountToggle(w http.ResponseWriter, r *http.Request) {
	a, err := h.st.GetAccount(pathID(r))
	if err == nil {
		err = h.st.UpdateAccount(a.ID, a.Label, a.AuthToken, a.CT0, !a.Enabled)
	}
	flashErr(w, r, err, "account updated")
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

func (h *Handler) accountDelete(w http.ResponseWriter, r *http.Request) {
	flashErr(w, r, h.st.DeleteAccount(pathID(r)), "account deleted")
	http.Redirect(w, r, "/admin/accounts", http.StatusFound)
}

// ---- API keys --------------------------------------------------------------- //

func (h *Handler) keysPage(w http.ResponseWriter, r *http.Request) {
	keys, _ := h.st.ListAPIKeys()
	accs, _ := h.st.ListAccounts()
	accLabels, _ := h.labelMaps()
	data := map[string]any{
		"Title": "API Keys", "Active": "keys",
		"Keys": keys, "Accounts": accs, "AccountLabels": accLabels,
	}
	if c, err := r.Cookie("newkey"); err == nil && c.Value != "" {
		data["NewKey"] = c.Value
		// #nosec G124 -- Secure is set at runtime via secureCookie(r); HttpOnly and SameSite are always on.
		http.SetCookie(w, &http.Cookie{Name: "newkey", Value: "", Path: "/admin", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureCookie(r)})
	}
	h.render(w, r, "keys", data)
}

func (h *Handler) keyCreate(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		setFlash(w, r, "err", "name is required")
		http.Redirect(w, r, "/admin/keys", http.StatusFound)
		return
	}
	key := newToken()
	if _, err := h.st.CreateAPIKey(name, key, r.FormValue("can_write") != "", boundAccount(r)); err != nil {
		setFlash(w, r, "err", err.Error())
		http.Redirect(w, r, "/admin/keys", http.StatusFound)
		return
	}
	// #nosec G124 -- Secure is set at runtime via secureCookie(r); HttpOnly and SameSite are always on.
	http.SetCookie(w, &http.Cookie{Name: "newkey", Value: key, Path: "/admin", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureCookie(r)})
	http.Redirect(w, r, "/admin/keys", http.StatusFound)
}

func (h *Handler) keyToggle(w http.ResponseWriter, r *http.Request) {
	keys, _ := h.st.ListAPIKeys()
	id := pathID(r)
	for _, k := range keys {
		if k.ID == id {
			flashErr(w, r, h.st.UpdateAPIKey(k.ID, k.Name, !k.Enabled, k.CanWrite, k.BoundAccountID), "key updated")
			break
		}
	}
	http.Redirect(w, r, "/admin/keys", http.StatusFound)
}

func (h *Handler) keyDelete(w http.ResponseWriter, r *http.Request) {
	flashErr(w, r, h.st.DeleteAPIKey(pathID(r)), "key deleted")
	http.Redirect(w, r, "/admin/keys", http.StatusFound)
}

// ---- logs ------------------------------------------------------------------- //

func (h *Handler) logsPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "logs", h.logData(r))
}

func (h *Handler) logsTable(w http.ResponseWriter, r *http.Request) {
	h.renderPartial(w, "logs", "logstable", h.logData(r))
}

func (h *Handler) logData(r *http.Request) map[string]any {
	path := r.URL.Query().Get("path")
	offset := atoi(r.URL.Query().Get("offset"))
	logs, _ := h.st.ListLogs(store.LogFilter{Path: path, Limit: logsPageSize + 1, Offset: offset})
	hasNext := len(logs) > logsPageSize
	if hasNext {
		logs = logs[:logsPageSize]
	}
	accLabels, keyNames := h.labelMaps()
	return map[string]any{
		"Title": "Logs", "Active": "logs",
		"Logs":          logs,
		"Filter":        map[string]any{"Path": path, "Offset": offset},
		"AccountLabels": accLabels,
		"KeyNames":      keyNames,
		"HasNext":       hasNext,
		"PrevOffset":    max(0, offset-logsPageSize),
		"NextOffset":    offset + logsPageSize,
	}
}

// ---- query IDs -------------------------------------------------------------- //

func (h *Handler) queryIDsPage(w http.ResponseWriter, r *http.Request) {
	ids, _ := h.st.ListQueryIDs()
	h.render(w, r, "query-ids", map[string]any{
		"Title": "Query IDs", "Active": "queryids", "QueryIDs": ids,
	})
}

func (h *Handler) queryIDsRefresh(w http.ResponseWriter, r *http.Request) {
	if h.refresh == nil {
		setFlash(w, r, "err", "query-id refresh is not available")
		http.Redirect(w, r, "/admin/query-ids", http.StatusFound)
		return
	}
	n, err := h.refresh()
	if err != nil {
		setFlash(w, r, "err", err.Error())
	} else {
		setFlash(w, r, "ok", "refreshed "+strconv.Itoa(n)+" query IDs from x.com")
	}
	http.Redirect(w, r, "/admin/query-ids", http.StatusFound)
}

// ---- settings --------------------------------------------------------------- //

func (h *Handler) settingsPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "settings", map[string]any{
		"Title": "Settings", "Active": "settings",
		"EnableWrites":   h.st.GetSettingBool(store.SettingEnableWrites, false),
		"PublicFallback": h.st.GetSettingBool(store.SettingPublicFallback, false),
		"LogRetention":   h.st.GetSettingInt(store.SettingLogRetention, 0),
		"DailyLimit":     h.st.GetSettingInt(store.SettingDailyRequestLimit, 0),
		"Proxy":          must(h.st.GetSetting(store.SettingProxy, "")),
		"UserAgent":      must(h.st.GetSetting(store.SettingUserAgent, "")),
		"ClientProfile":  must(h.st.GetSetting(store.SettingClientProfile, "")),
		"TxID":           must(h.st.GetSetting(store.SettingTxID, "")),
	})
}

func (h *Handler) settingsSave(w http.ResponseWriter, r *http.Request) {
	writes := "false"
	if r.FormValue("enable_writes") != "" {
		writes = "true"
	}
	_ = h.st.SetSetting(store.SettingEnableWrites, writes)
	pub := "false"
	if r.FormValue("enable_public_fallback") != "" {
		pub = "true"
	}
	_ = h.st.SetSetting(store.SettingPublicFallback, pub)
	_ = h.st.SetSetting(store.SettingLogRetention, strconv.Itoa(atoi(r.FormValue("log_retention_days"))))
	_ = h.st.SetSetting(store.SettingDailyRequestLimit, strconv.Itoa(atoi(r.FormValue("daily_request_limit"))))
	_ = h.st.SetSetting(store.SettingProxy, r.FormValue("proxy"))
	_ = h.st.SetSetting(store.SettingUserAgent, r.FormValue("user_agent"))
	_ = h.st.SetSetting(store.SettingClientProfile, r.FormValue("client_profile"))
	_ = h.st.SetSetting(store.SettingTxID, r.FormValue("tx_id"))
	setFlash(w, r, "ok", "settings saved (transport changes apply on restart)")
	http.Redirect(w, r, "/admin/settings", http.StatusFound)
}

// ---- helpers ---------------------------------------------------------------- //

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id
}

func boundAccount(r *http.Request) *int64 {
	v := r.FormValue("bound_account_id")
	if v == "" {
		return nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &id
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func must(v string, _ error) string { return v }

func flashErr(w http.ResponseWriter, r *http.Request, err error, okMsg string) {
	if err != nil {
		setFlash(w, r, "err", err.Error())
		return
	}
	setFlash(w, r, "ok", okMsg)
}
