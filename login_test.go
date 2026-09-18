package miru_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	miru "github.com/takara-ai/miru-code"
)

func loginTestEnv(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	dir := t.TempDir()
	t.Setenv("MIRU_AUTH_BASE_URL", server.URL)
	t.Setenv("MIRU_CREDENTIALS_DIR", dir)
	for _, key := range []string{"TAKARA_API_KEY", "MIRU_SAGEMAKER_ENDPOINT_ARN", "MIRU_SAGEMAKER_ENDPOINT_NAME", "MIRU_SAGEMAKER_REGION", "AWS_PROFILE"} {
		t.Setenv(key, "")
	}
	return filepath.Join(dir, "credentials.json")
}

func TestDeviceLoginPersistsRefreshableCredentials(t *testing.T) {
	path := loginTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/device/code" {
			_, _ = w.Write([]byte(`{"device_code":"private-device","user_code":"ABCD","verification_uri":"https://example.test/login","verification_uri_complete":"https://example.test/login?code=ABCD","expires_in":600,"interval":0}`))
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("device_code") != "private-device" {
			t.Error("missing device code")
		}
		_, _ = w.Write([]byte(`{"access_token":"test-access","refresh_token":"test-refresh","expires_in":3600,"token_type":"Bearer","scope":"test"}`))
	})
	startCtx, cancelStart := context.WithCancel(context.Background())
	session, err := miru.StartDeviceLogin(startCtx)
	cancelStart()
	if err != nil {
		t.Fatal(err)
	}
	if session.UserCode != "ABCD" || session.VerificationURL != "https://example.test/login?code=ABCD" {
		t.Fatal("incorrect login display fields")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("credentials saved before approval")
	}
	if err := session.Finish(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored["kind"] != "device_code" || stored["refresh_token"] != "test-refresh" || stored["expires_at"] == "" || os.Getenv("TAKARA_API_KEY") != "test-access" {
		t.Fatal("credentials were not persisted and activated")
	}
}

func TestDeviceLoginCancellation(t *testing.T) {
	for _, stage := range []string{"start", "wait", "poll"} {
		t.Run(stage, func(t *testing.T) {
			path := loginTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if stage == "start" || r.URL.Path == "/oauth/token" {
					<-r.Context().Done()
					return
				}
				interval := 0
				if stage == "wait" {
					interval = 60
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "private-device", "user_code": "ABCD", "verification_uri": "https://example.test/login", "expires_in": 600, "interval": interval})
			})
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			session, err := miru.StartDeviceLogin(ctx)
			if err == nil {
				err = session.Finish(ctx)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected deadline cancellation, got %v", err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("cancelled login saved credentials")
			}
		})
	}
}
