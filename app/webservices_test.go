package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadTLSCertFallback(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.pem")

	if _, err := loadTLSCert(missing, missing, ProdEnv); err == nil {
		t.Error("env prod without certFile must fail instead of using the embedded certificate")
	}
	if _, err := loadTLSCert(missing, missing, DevEnv); err != nil {
		t.Errorf("env dev should fall back to the embedded certificate: %v", err)
	}
}

func TestServerErrorRestartsApp(t *testing.T) {
	app, _ := newTestApp(t, map[string]RelayConfig{"r": {GPIO: 4}}, nil)
	app.signals = make(chan os.Signal)
	app.HandleOSSignals()

	app.serverErr <- errors.New("listener died")

	select {
	case <-app.Restart():
	case <-time.After(5 * time.Second):
		t.Fatal("a web server error did not restart the App")
	}
	if len(app.Handover()) != 1 {
		t.Errorf("restart after a server error handed over %d relays, want 1", len(app.Handover()))
	}
}
