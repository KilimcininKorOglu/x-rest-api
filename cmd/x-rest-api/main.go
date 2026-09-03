// Command x-rest-api serves x.com's private GraphQL surfaces as a REST API and an
// htmx admin panel on one port. Only PORT (and optionally DB_PATH) come from the
// environment; every other setting lives in SQLite and is managed from /admin.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"x-rest-api/internal/config"
	"x-rest-api/internal/server"
	"x-rest-api/internal/server/admin"
	"x-rest-api/internal/store"
	"x-rest-api/internal/xapi"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("x-rest-api: %v", err)
	}
}

func run() error {
	cfg := config.Load()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	sess, err := newSession(st)
	if err != nil {
		return err
	}

	// Load persisted queryId overrides so a restart keeps the last refresh.
	if ids, err := st.AllQueryIDs(); err == nil && len(ids) > 0 {
		sess.SetQueryIDs(ids)
	}

	refresh := refreshQueryIDs(st, sess)
	adminH := admin.New(st, refresh)
	srv := server.NewServer(st, sess)
	srv.SetRefresh(refresh) // auto-refresh queryIds when x.com reports code 336
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Routes(adminH.Router()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go retentionLoop(ctx, st)

	go func() {
		log.Printf("x-rest-api listening on %s (admin: /admin, api: /v1)", cfg.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	cancel()
	log.Println("x-rest-api shutting down")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	return httpSrv.Shutdown(shutCtx)
}

// refreshQueryIDs returns a closure that pulls live queryIds from the x.com
// bundle, persists them, and applies them to the running session.
func refreshQueryIDs(st *store.Store, sess *xapi.Session) func() (int, error) {
	return func() (int, error) {
		ids, feats, err := sess.FetchManifest()
		if err != nil {
			return 0, err
		}
		if err := st.UpsertQueryIDs(ids); err != nil {
			return 0, err
		}
		merged, err := st.AllQueryIDs()
		if err != nil {
			return 0, err
		}
		sess.SetQueryIDs(merged)
		// Feature-flag names are applied in-memory only; ops.json holds the base
		// values, and the next refresh repopulates the discovered flags.
		sess.SetFeatureSwitches(feats)
		return len(ids), nil
	}
}

// newSession builds the shared upstream transport from the stored settings.
func newSession(st *store.Store) (*xapi.Session, error) {
	ua, _ := st.GetSetting(store.SettingUserAgent, "")
	proxy, _ := st.GetSetting(store.SettingProxy, "")
	txID, _ := st.GetSetting(store.SettingTxID, "")
	profile, _ := st.GetSetting(store.SettingClientProfile, "")
	return xapi.NewSession(ua, proxy, txID, profile)
}

// retentionLoop periodically prunes old request logs per the retention setting.
func retentionLoop(ctx context.Context, st *store.Store) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := st.DeleteLogsOlderThan(st.GetSettingInt(store.SettingLogRetention, 0)); err != nil {
				log.Printf("retention: %v", err)
			}
			if err := st.PurgeExpiredAccountLocks(); err != nil {
				log.Printf("retention: purge locks: %v", err)
			}
		}
	}
}
