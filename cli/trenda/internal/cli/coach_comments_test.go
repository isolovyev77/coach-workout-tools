// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// These pin the behaviour of the comment commands against a server that answers
// the way the live Trenda API was observed to: reads come back inside get-by-id,
// asking for several workouts at once contaminates each thread with the others'
// comments, add answers with an empty body, and a wrong comment id comes back as
// CommentNotFound rather than a 404.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeComment struct {
	ID        int    `json:"id"`
	AuthorID  int    `json:"author_id"`
	Author    string `json:"author_name"`
	Role      string `json:"author_role"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// commentAPI replays Trenda's comment endpoints, including the multi-id defect:
// when more than one id is asked for, every workout is handed every comment.
func commentAPI(t *testing.T, threads map[int][]fakeComment) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var posted []map[string]any
	nextID := 9000

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		posted = append(posted, map[string]any{"path": r.URL.Path, "body": body})
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case commentsReadPath:
			raw, _ := body["ids"].([]any)
			var ids []int
			for _, v := range raw {
				if f, ok := v.(float64); ok {
					ids = append(ids, int(f))
				}
			}
			var union []fakeComment
			if len(ids) > 1 {
				for _, id := range ids {
					union = append(union, threads[id]...)
				}
			}
			models := []map[string]any{}
			for _, id := range ids {
				thread, ok := threads[id]
				if !ok {
					continue
				}
				if len(ids) > 1 {
					thread = union
				}
				models = append(models, map[string]any{"id": id, "comments": thread})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": models})

		case commentAddPath:
			id := int(body["workout_id"].(float64))
			nextID++
			threads[id] = append(threads[id], fakeComment{
				ID: nextID, AuthorID: 48, Author: "Coach", Role: "Coach",
				Text: body["text"].(string), CreatedAt: "2026-08-13T09:00:00+00:00",
			})
			_, _ = w.Write([]byte(`{}`))

		case commentEditPath:
			target := int(body["comment_id"].(float64))
			for wid, thread := range threads {
				for i := range thread {
					if thread[i].ID == target {
						thread[i].Text = body["text"].(string)
						threads[wid] = thread
						_, _ = w.Write([]byte(`{}`))
						return
					}
				}
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"CommentNotFound","message":"Requested comment not found"}`))

		case commentDeletePath:
			target := int(body["comment_id"].(float64))
			for wid, thread := range threads {
				for i := range thread {
					if thread[i].ID == target {
						threads[wid] = append(thread[:i], thread[i+1:]...)
						_, _ = w.Write([]byte(`{}`))
						return
					}
				}
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"CommentNotFound","message":"Requested comment not found"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &posted
}

func commentFixture() map[int][]fakeComment {
	return map[int][]fakeComment{
		66196: {{ID: 2695, AuthorID: 48, Author: "Coach", Role: "Coach",
			Text: "<p>Как самочувствие?</p>", CreatedAt: "2026-08-12T22:52:00+00:00"}},
		66197: {{ID: 2700, AuthorID: 320, Author: "Client", Role: "Client",
			Text: "<p>Всё хорошо</p>", CreatedAt: "2026-08-12T23:10:00+00:00"}},
	}
}

func TestCommentsReadsOneThread(t *testing.T) {
	srv, posted := commentAPI(t, commentFixture())

	out, err := runTrenda(t, srv.URL, "--agent", "coach", "comments", "--workout-id", "66196")
	if err != nil {
		t.Fatalf("comments failed: %v\n%s", err, out)
	}

	var env struct {
		Data struct {
			Comments []struct {
				ID        int    `json:"id"`
				Text      string `json:"text"`
				TextPlain string `json:"text_plain"`
			} `json:"comments"`
		} `json:"data"`
	}
	if jErr := json.Unmarshal([]byte(out), &env); jErr != nil {
		t.Fatalf("not the expected envelope: %v\n%s", jErr, out)
	}
	if len(env.Data.Comments) != 1 {
		t.Fatalf("got %d comments, want the one on this workout", len(env.Data.Comments))
	}
	if env.Data.Comments[0].ID != 2695 {
		t.Errorf("comment id = %d, want 2695", env.Data.Comments[0].ID)
	}
	// The id is what edit/delete need, and the plain text is what a coach reads.
	if env.Data.Comments[0].TextPlain != "Как самочувствие?" {
		t.Errorf("text_plain = %q, want the text without markup", env.Data.Comments[0].TextPlain)
	}
	if !strings.Contains(env.Data.Comments[0].Text, "<p>") {
		t.Errorf("the API field must survive untouched, got %q", env.Data.Comments[0].Text)
	}

	// Reading must never batch ids: the server contaminates threads when it can.
	for _, call := range *posted {
		if call["path"] != commentsReadPath {
			continue
		}
		ids, _ := call["body"].(map[string]any)["ids"].([]any)
		if len(ids) != 1 {
			t.Errorf("read asked for %d ids, want exactly 1", len(ids))
		}
	}
}

// The defect this guards against is the server's, not ours: asking for two
// workouts hands both threads to both. A batched read would silently quote a
// client comment from the wrong day.
func TestCommentsNeverBatchesWorkouts(t *testing.T) {
	srv, _ := commentAPI(t, commentFixture())

	out, err := runTrenda(t, srv.URL, "--agent", "coach", "comments", "--workout-id", "66197")
	if err != nil {
		t.Fatalf("comments failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "Как самочувствие") {
		t.Errorf("thread of another workout leaked into the answer:\n%s", out)
	}
}

func TestAddCommentEscapesAndReportsNewID(t *testing.T) {
	threads := commentFixture()
	srv, posted := commentAPI(t, threads)

	out, err := runTrenda(t, srv.URL, "--agent", "--yes", "coach", "add-comment",
		"--workout-id", "66196", "--text", "5 < 6 & сложно\nвторая строка")
	if err != nil {
		t.Fatalf("add-comment failed: %v\n%s", err, out)
	}

	var sent string
	for _, call := range *posted {
		if call["path"] == commentAddPath {
			sent, _ = call["body"].(map[string]any)["text"].(string)
		}
	}
	if !strings.Contains(sent, "&lt;") || !strings.Contains(sent, "&amp;") {
		t.Errorf("special characters were not escaped, sent %q", sent)
	}
	if !strings.Contains(sent, "<br>") {
		t.Errorf("line break was lost, sent %q", sent)
	}

	// The API answers an add with {}, so the id has to be recovered by re-reading.
	var env struct {
		Data struct {
			Comment struct {
				ID int `json:"id"`
			} `json:"comment"`
		} `json:"data"`
	}
	if jErr := json.Unmarshal([]byte(out), &env); jErr != nil {
		t.Fatalf("not the expected envelope: %v\n%s", jErr, out)
	}
	if env.Data.Comment.ID == 0 {
		t.Errorf("no id for the new comment; edit and delete would be unreachable:\n%s", out)
	}
}

func TestAddCommentHTMLFlagSendsMarkupUntouched(t *testing.T) {
	srv, posted := commentAPI(t, commentFixture())

	if out, err := runTrenda(t, srv.URL, "--agent", "--yes", "coach", "add-comment",
		"--workout-id", "66196", "--html", "--text", "<p>Жирно: <b>да</b></p>"); err != nil {
		t.Fatalf("add-comment --html failed: %v\n%s", err, out)
	}
	for _, call := range *posted {
		if call["path"] != commentAddPath {
			continue
		}
		sent, _ := call["body"].(map[string]any)["text"].(string)
		if sent != "<p>Жирно: <b>да</b></p>" {
			t.Errorf("markup was altered: %q", sent)
		}
	}
}

// Writing into a thread the client reads must not happen by accident.
func TestWritesRefuseWithoutConfirmation(t *testing.T) {
	srv, posted := commentAPI(t, commentFixture())

	cases := [][]string{
		{"coach", "add-comment", "--workout-id", "66196", "--text", "привет"},
		{"coach", "edit-comment", "--comment-id", "2695", "--text", "привет"},
		{"coach", "delete-comment", "--comment-id", "2695"},
	}
	for _, args := range cases {
		out, err := runTrenda(t, srv.URL, append([]string{"--json", "--no-input"}, args...)...)
		if err == nil {
			t.Errorf("%s wrote without --yes:\n%s", strings.Join(args, " "), out)
		}
		if code := ExitCode(err); code != 2 {
			t.Errorf("%s: exit code = %d, want 2 (usage)", strings.Join(args, " "), code)
		}
	}
	for _, call := range *posted {
		if call["path"] == commentAddPath || call["path"] == commentEditPath || call["path"] == commentDeletePath {
			t.Fatalf("a write reached the API without confirmation: %v", call["path"])
		}
	}
}

// --agent bundles --yes together with --json and --no-input. If the comment
// writes honoured that inherited flag, the confirmation would be waived for
// precisely the caller it exists to stop, and an agent could post to a client's
// thread by default. The yes has to be said about this command.
func TestAgentModeAloneDoesNotAuthoriseAWrite(t *testing.T) {
	srv, posted := commentAPI(t, commentFixture())

	cases := [][]string{
		{"coach", "add-comment", "--workout-id", "66196", "--text", "привет"},
		{"coach", "edit-comment", "--comment-id", "2695", "--text", "привет"},
		{"coach", "delete-comment", "--comment-id", "2695"},
	}
	for _, args := range cases {
		out, err := runTrenda(t, srv.URL, append([]string{"--agent"}, args...)...)
		if err == nil {
			t.Errorf("%s wrote under --agent alone:\n%s", strings.Join(args, " "), out)
		}
	}
	for _, call := range *posted {
		switch call["path"] {
		case commentAddPath, commentEditPath, commentDeletePath:
			t.Fatalf("--agent alone reached %v", call["path"])
		}
	}

	// The same commands with an explicit --yes must still go through.
	if out, err := runTrenda(t, srv.URL, "--agent", "--yes",
		"coach", "add-comment", "--workout-id", "66196", "--text", "привет"); err != nil {
		t.Fatalf("explicit --yes was refused: %v\n%s", err, out)
	}
}

func TestEditCommentReplacesText(t *testing.T) {
	threads := commentFixture()
	srv, _ := commentAPI(t, threads)

	if out, err := runTrenda(t, srv.URL, "--agent", "--yes", "coach", "edit-comment",
		"--comment-id", "2695", "--workout-id", "66196", "--text", "Как восстановление?"); err != nil {
		t.Fatalf("edit-comment failed: %v\n%s", err, out)
	}
	if got := threads[66196][0].Text; !strings.Contains(got, "Как восстановление?") {
		t.Errorf("stored text = %q, want the replacement", got)
	}
}

func TestDeleteCommentRemovesIt(t *testing.T) {
	threads := commentFixture()
	srv, _ := commentAPI(t, threads)

	if out, err := runTrenda(t, srv.URL, "--agent", "--yes", "coach", "delete-comment",
		"--comment-id", "2695", "--workout-id", "66196"); err != nil {
		t.Fatalf("delete-comment failed: %v\n%s", err, out)
	}
	if len(threads[66196]) != 0 {
		t.Errorf("comment survived deletion: %+v", threads[66196])
	}
}

// A wrong id is the common mistake, and the API reports it with its own code
// rather than a 404. It must not surface as a generic upstream error.
func TestUnknownCommentIDIsNotFound(t *testing.T) {
	srv, _ := commentAPI(t, commentFixture())

	_, err := runTrenda(t, srv.URL, "--agent", "--yes", "coach", "delete-comment", "--comment-id", "424242")
	if err == nil {
		t.Fatal("deleting an unknown comment succeeded")
	}
	if code := ExitCode(err); code != 3 {
		t.Errorf("exit code = %d, want 3 (not found)", code)
	}
	if !strings.Contains(err.Error(), "coach comments") {
		t.Errorf("error should say where ids come from, got: %v", err)
	}
}

func TestCommentTextRoundTrip(t *testing.T) {
	cases := []struct{ in, want string }{
		{"простой текст", "простой текст"},
		{"две\nстроки", "две\nстроки"},
		{"5 < 6 & 7 > 6", "5 < 6 & 7 > 6"},
	}
	for _, c := range cases {
		if got := commentPlainText(commentHTML(c.in)); got != c.want {
			t.Errorf("round trip of %q = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEmptyTextIsRefusedBeforeAnyRequest(t *testing.T) {
	srv, posted := commentAPI(t, commentFixture())

	_, err := runTrenda(t, srv.URL, "--agent", "--yes", "coach", "add-comment", "--workout-id", "66196", "--text", "   ")
	if err == nil {
		t.Fatal("an empty comment was accepted")
	}
	if code := ExitCode(err); code != 2 {
		t.Errorf("exit code = %d, want 2 (usage)", code)
	}
	for _, call := range *posted {
		if call["path"] == commentAddPath {
			t.Fatal("an empty comment reached the API")
		}
	}
}
