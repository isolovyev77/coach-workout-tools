// Copyright 2026 isolovyev. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// movementFixture rewrites the captured air-squat page to claim a given slug,
// so a mismatch can be staged without inventing a payload shape.
func movementFixture(t *testing.T, slug string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "capjson", "testdata", "movement-air-squat.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	tile := doc["tiles"].([]any)[0].(map[string]any)
	tile["slug"] = slug
	tile["acf"].(map[string]any)["slug"] = slug
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// cmsReplay serves whatever the handler decides, at the CMS path shape.
func cmsReplay(t *testing.T, handler http.HandlerFunc) *rootFlags {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	// fetchCMS builds its URL from cmsBase, so the test points that at the
	// replay server for the duration of the test.
	old := cmsBase
	cmsBase = srv.URL + "/tiles?full_path="
	t.Cleanup(func() { cmsBase = old })
	return &rootFlags{timeout: 5 * time.Second}
}

// A cached CDN handing back a neighbouring movement is the one failure a coach
// could not spot by reading the output: the cues would be confident, complete
// and for the wrong lift. The command must refuse rather than print them.
func TestFetchMovementRefusesAMismatchedSlug(t *testing.T) {
	var calls int32
	flags := cmsReplay(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write(movementFixture(t, "deadlift"))
	})

	_, err := fetchMovement(flags, "push-jerk")
	if err == nil {
		t.Fatal("a movement served under the wrong slug was accepted")
	}
	if !strings.Contains(err.Error(), "deadlift") || !strings.Contains(err.Error(), "push-jerk") {
		t.Errorf("error = %q, want both the requested and the returned slug named", err)
	}
	if code := ExitCode(err); code == 0 {
		t.Error("exit code = 0; a mismatch must not look like success")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("made %d requests, want 2 (one retry before giving up)", got)
	}
}

// A one-off blip should cost a retry, not an error.
func TestFetchMovementRetriesPastATransientMismatch(t *testing.T) {
	var calls int32
	flags := cmsReplay(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Write(movementFixture(t, "deadlift"))
			return
		}
		w.Write(movementFixture(t, "push-jerk"))
	})

	mv, err := fetchMovement(flags, "push-jerk")
	if err != nil {
		t.Fatalf("retry did not recover: %v", err)
	}
	if mv.Slug != "push-jerk" {
		t.Errorf("slug = %q, want push-jerk", mv.Slug)
	}
}

// The happy path must stay a single request: this guard is not an excuse to
// double the load on CrossFit's CMS.
func TestFetchMovementMakesOneRequestWhenTheSlugMatches(t *testing.T) {
	var calls int32
	flags := cmsReplay(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write(movementFixture(t, "air-squat"))
	})

	if _, err := fetchMovement(flags, "air-squat"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("made %d requests for a matching slug, want 1", got)
	}
}

// An unknown movement is the everyday mistake and must stay a plain not-found
// pointing at the catalogue, not the mismatch error.
func TestFetchMovementUnknownSlug(t *testing.T) {
	flags := cmsReplay(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"not found"}`)
	})

	_, err := fetchMovement(flags, "push-jerkk")
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	if code := ExitCode(err); code != 3 {
		t.Errorf("exit code = %d, want 3 (not found)", code)
	}
	if !strings.Contains(err.Error(), "cap movements") {
		t.Errorf("error = %q, want it to point at the catalogue", err)
	}
}
