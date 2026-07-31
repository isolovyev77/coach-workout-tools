// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"

	"btwb-pp-cli/internal/btwbhtml"
)

func planTracks() []btwbhtml.Track {
	return []btwbhtml.Track{
		{ID: 101, Name: "Coach Personal Track"},
		{ID: 202, Name: "Example Gym WODs"},
	}
}

// A member can read a gym's programming but not write to it. When they aim at a
// track btwb did not offer, the error has to say why, otherwise the failure
// looks like a bug in this CLI rather than a missing permission.
func TestResolvePlanTrackExplainsMissingAdminRights(t *testing.T) {
	only := []btwbhtml.Track{{ID: 101, Name: "Coach Personal Track"}}
	_, err := resolvePlanTrack(only, 202, "")
	if err == nil {
		t.Fatal("expected an error for a track that is not plannable")
	}
	msg := err.Error()
	if !strings.Contains(msg, "admin rights") {
		t.Errorf("error = %q, want it to name the missing permission", msg)
	}
	if !strings.Contains(msg, "101") {
		t.Errorf("error = %q, want it to list what is available", msg)
	}
}

func TestResolvePlanTrackByIDAndName(t *testing.T) {
	tracks := planTracks()

	if id, err := resolvePlanTrack(tracks, 202, ""); err != nil || id != 202 {
		t.Errorf("by id = (%d, %v)", id, err)
	}
	if id, err := resolvePlanTrack(tracks, 0, "example"); err != nil || id != 202 {
		t.Errorf("by name = (%d, %v), want a case-insensitive match", id, err)
	}
}

// Silently picking one of several matches would write the workout to a track
// the user did not mean.
func TestResolvePlanTrackRefusesAmbiguity(t *testing.T) {
	tracks := []btwbhtml.Track{
		{ID: 1, Name: "Example WODs"},
		{ID: 2, Name: "Example Gymnastics"},
	}
	if _, err := resolvePlanTrack(tracks, 0, "example"); err == nil {
		t.Fatal("expected an error when the name matches more than one track")
	} else if !strings.Contains(err.Error(), "--track-id") {
		t.Errorf("error = %q, want it to point at the unambiguous flag", err)
	}

	// With no hint at all and several tracks, guessing is equally wrong.
	if _, err := resolvePlanTrack(tracks, 0, ""); err == nil {
		t.Error("expected an error when no track was chosen and several exist")
	}
}

// The single-track case is the common one (no admin rights), and asking the
// user to name their only option is pointless friction.
func TestResolvePlanTrackDefaultsWhenThereIsOnlyOne(t *testing.T) {
	only := []btwbhtml.Track{{ID: 101, Name: "Coach Personal Track"}}
	id, err := resolvePlanTrack(only, 0, "")
	if err != nil || id != 101 {
		t.Errorf("resolve = (%d, %v)", id, err)
	}
}

// An empty list means the page did not render the select, which in practice
// means the session is no longer valid. That is exit code 4, not a usage error.
func TestResolvePlanTrackTreatsAnEmptyListAsAnAuthProblem(t *testing.T) {
	_, err := resolvePlanTrack(nil, 0, "")
	if err == nil {
		t.Fatal("expected an error for an empty track list")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %q, want it to suggest the session expired", err)
	}
}
