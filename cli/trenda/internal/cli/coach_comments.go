// Copyright 2026 Coach Workout Tools Contributors. Licensed under Apache-2.0. See LICENSE.
//
// Comments on a workout: the thread a coach and a client hold under a training
// day. Trenda exposes three write routes for them and no read route at all -
// a workout's comments ride along inside get-by-id. That call takes a list of
// ids, but asking for more than one is a trap: the server then hands every
// workout in the answer the union of all their comments, so a comment written
// under Monday shows up under Tuesday as well. Verified against the live API.
// Every read here therefore asks for exactly one workout.
//
// Text is stored as HTML, because the web editor is a rich-text field. A coach
// typing on a command line should not have to know that, so plain text is
// wrapped and escaped on the way in and stripped on the way out, with --html
// for anyone who does want to send markup.

package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

const (
	commentsReadPath  = "/api/v1/coach/workout/get-by-id"
	commentAddPath    = "/api/v1/coach/workout/add-comment"
	commentEditPath   = "/api/v1/coach/workout/edit-comment"
	commentDeletePath = "/api/v1/coach/workout/delete-comment"
)

type workoutComment struct {
	ID                   int             `json:"id"`
	WorkoutExerciseID    *string         `json:"workout_exercise_id"`
	WorkoutExerciseTitle *string         `json:"workout_exercise_title"`
	AuthorID             int             `json:"author_id"`
	AuthorRole           string          `json:"author_role"`
	AuthorName           string          `json:"author_name"`
	Text                 string          `json:"text"`
	CreatedAt            string          `json:"created_at"`
	MediaFileIDs         json.RawMessage `json:"media_file_ids"`
}

// addCommentCommands attaches the comment verbs to the generated `coach` group.
// They live in this file rather than in the generated ones so a regeneration
// does not silently drop them.
func addCommentCommands(coach *cobra.Command, flags *rootFlags) {
	coach.AddCommand(newCoachCommentsCmd(flags))
	coach.AddCommand(newCoachAddCommentCmd(flags))
	coach.AddCommand(newCoachEditCommentCmd(flags))
	coach.AddCommand(newCoachDeleteCommentCmd(flags))
}

// fetchComments reads the comment thread of one workout. Single id by design -
// see the note at the top of this file.
func fetchComments(c interface {
	Post(path string, body any) (json.RawMessage, int, error)
}, workoutID int) ([]workoutComment, error) {
	data, _, err := c.Post(commentsReadPath, map[string]any{"ids": []int{workoutID}})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Models []struct {
			ID       int              `json:"id"`
			Comments []workoutComment `json:"comments"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("reading comments of workout %d: %w", workoutID, err)
	}
	if len(payload.Models) == 0 {
		return nil, notFoundErr(fmt.Errorf("workout %d not found, or it belongs to another coach", workoutID))
	}
	return payload.Models[0].Comments, nil
}

var (
	blockTagRe = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>|</h[1-6]>`)
	anyTagRe   = regexp.MustCompile(`<[^>]*>`)
	manyNLRe   = regexp.MustCompile(`\n{3,}`)
)

// commentPlainText renders stored comment HTML as the text a coach would read
// aloud. It is a presentation helper, not a sanitiser.
func commentPlainText(s string) string {
	s = blockTagRe.ReplaceAllString(s, "\n")
	s = anyTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = manyNLRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// commentHTML turns what a coach typed into what the API stores. Escaping first
// means a workout note containing "<" or "&" survives the round trip intact.
func commentHTML(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	escaped := html.EscapeString(strings.TrimSpace(s))
	return "<p>" + strings.ReplaceAll(escaped, "\n", "<br>") + "</p>"
}

// commentBody resolves the text of a comment from the flags that can carry it.
func commentBody(text, textFile string, asHTML bool) (string, error) {
	switch {
	case text != "" && textFile != "":
		return "", usageErr(fmt.Errorf("pass --text or --text-file, not both"))
	case textFile != "":
		raw, err := os.ReadFile(textFile)
		if err != nil {
			return "", usageErr(fmt.Errorf("reading --text-file: %w", err))
		}
		text = string(raw)
	}
	if strings.TrimSpace(text) == "" {
		return "", usageErr(fmt.Errorf("comment text is empty: pass --text or --text-file"))
	}
	if asHTML {
		return strings.TrimSpace(text), nil
	}
	return commentHTML(text), nil
}

// confirmComment gates every write behind a yes that was typed for this command.
//
// assumeOK comes from a --yes declared on the command itself, not from the
// persistent one: --agent turns the persistent --yes on as part of its bundle of
// defaults, which would have made the confirmation here a formality for exactly
// the callers it exists to stop. An agent must say --yes about this write.
func confirmComment(cmd *cobra.Command, flags *rootFlags, assumeOK bool, prompt string) (bool, error) {
	if assumeOK {
		return true, nil
	}
	if flags.noInput {
		return false, usageErr(fmt.Errorf(
			"refusing to write to a client-visible thread without confirmation: pass --yes"))
	}
	if !isTerminal(cmd.OutOrStdout()) && !flags.asJSON {
		return false, usageErr(fmt.Errorf(
			"stdin is not interactive: pass --yes to write"))
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", prompt)
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y"), nil
}

// commentEnvelope prints the {action,resource,path,status,...} envelope the rest
// of this CLI emits, so a caller can parse every command the same way.
func commentEnvelope(cmd *cobra.Command, flags *rootFlags, path string, status int, data any) error {
	envelope := map[string]any{
		"action":   "post",
		"resource": "coach",
		"path":     path,
		"status":   status,
		"success":  status >= 200 && status < 300,
	}
	if data != nil {
		envelope["data"] = data
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return printOutput(cmd.OutOrStdout(), json.RawMessage(body), true)
}

func wantsJSON(cmd *cobra.Command, flags *rootFlags) bool {
	return flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !flags.quiet && !flags.plain && !flags.csv)
}

// printComments writes the thread for a human: newest last, the way it reads on
// screen, with the id each write command needs.
func printComments(cmd *cobra.Command, workoutID int, comments []workoutComment) {
	w := cmd.OutOrStdout()
	if len(comments) == 0 {
		fmt.Fprintf(w, "No comments on workout %d\n", workoutID)
		return
	}
	fmt.Fprintf(w, "Workout %d, %d comment(s)\n", workoutID, len(comments))
	for _, c := range comments {
		created := c.CreatedAt
		if len(created) >= 16 {
			created = strings.Replace(created[:16], "T", " ", 1)
		}
		fmt.Fprintf(w, "\n  #%d  %s (%s)  %s\n", c.ID, c.AuthorName, c.AuthorRole, created)
		if c.WorkoutExerciseTitle != nil && *c.WorkoutExerciseTitle != "" {
			fmt.Fprintf(w, "  on: %s\n", *c.WorkoutExerciseTitle)
		}
		for _, line := range strings.Split(commentPlainText(c.Text), "\n") {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}
}

func commentsWithPlain(comments []workoutComment) []map[string]any {
	out := make([]map[string]any, 0, len(comments))
	for _, c := range comments {
		raw, err := json.Marshal(c)
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		// The API field stays untouched; the rendered form is added beside it so
		// an agent does not have to strip HTML to quote a client back to them.
		m["text_plain"] = commentPlainText(c.Text)
		out = append(out, m)
	}
	return out
}

func newCoachCommentsCmd(flags *rootFlags) *cobra.Command {
	var workoutID int

	cmd := &cobra.Command{
		Use:   "comments",
		Short: "Комментарии под тренировкой: кто написал, когда и что",
		Long: `Читает переписку под одной тренировкой.

У API нет отдельного маршрута для комментариев - они приходят вместе с
тренировкой, и только когда её запрашивают поодиночке. Команда сама
запрашивает одну тренировку, поэтому чужие комментарии в ответ не попадут.

Идентификатор комментария из этого списка нужен для edit-comment и
delete-comment: узнать его больше неоткуда.`,
		Example: "  trenda-pp-cli coach comments --workout-id 66196",
		Annotations: map[string]string{
			"pp:endpoint": "coach.comments", "pp:method": "POST", "pp:path": commentsReadPath,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if workoutID == 0 && !flags.dryRun {
				return usageErr(fmt.Errorf("required flag \"workout-id\" not set"))
			}
			if dryRunOK(flags) {
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			comments, err := fetchComments(c, workoutID)
			if err != nil {
				if ExitCode(err) != 0 {
					return err
				}
				return classifyAPIError(err, flags)
			}
			if wantsJSON(cmd, flags) {
				if flags.quiet {
					return nil
				}
				return commentEnvelope(cmd, flags, commentsReadPath, 200, map[string]any{
					"workout_id": workoutID,
					"comments":   commentsWithPlain(comments),
				})
			}
			printComments(cmd, workoutID, comments)
			return nil
		},
	}
	cmd.Flags().IntVar(&workoutID, "workout-id", 0, "Идентификатор тренировки")
	return cmd
}

func newCoachAddCommentCmd(flags *rootFlags) *cobra.Command {
	var workoutID int
	var text, textFile string
	var asHTML bool
	var assumeOK bool

	cmd := &cobra.Command{
		Use:   "add-comment",
		Short: "Написать комментарий под тренировкой клиента",
		Long: `Добавляет комментарий под тренировку. Клиент его увидит.

Текст передаётся как обычный текст: переносы строк сохраняются, символы
вроде < и & экранируются. Свою разметку можно послать с --html.

API на добавление отвечает пустым телом, поэтому команда перечитывает ветку
и показывает созданный комментарий вместе с его идентификатором - он нужен,
чтобы потом отредактировать или удалить написанное.`,
		Example: `  trenda-pp-cli coach add-comment --workout-id 66196 --text "Как самочувствие?"`,
		Annotations: map[string]string{
			"pp:endpoint": "coach.add-comment", "pp:method": "POST", "pp:path": commentAddPath,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if workoutID == 0 && !flags.dryRun {
				return usageErr(fmt.Errorf("required flag \"workout-id\" not set"))
			}
			if dryRunOK(flags) {
				return nil
			}
			body, err := commentBody(text, textFile, asHTML)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			before, err := fetchComments(c, workoutID)
			if err != nil {
				if ExitCode(err) != 0 {
					return err
				}
				return classifyAPIError(err, flags)
			}
			known := make(map[int]bool, len(before))
			for _, prev := range before {
				known[prev.ID] = true
			}

			ok, err := confirmComment(cmd, flags, assumeOK,
				fmt.Sprintf("Post this comment to workout %d, where the client can read it?\n  %s\n",
					workoutID, commentPlainText(body)))
			if err != nil {
				return err
			}
			if !ok {
				return writeNoop(flags, "declined", "not posted")
			}

			_, status, err := c.Post(commentAddPath, map[string]any{
				"workout_id": workoutID,
				"text":       body,
			})
			if err != nil {
				return classifyAPIError(err, flags)
			}

			created := newestUnknown(c, workoutID, known)
			if wantsJSON(cmd, flags) {
				if flags.quiet {
					return nil
				}
				payload := map[string]any{"workout_id": workoutID}
				if created != nil {
					payload["comment"] = commentsWithPlain([]workoutComment{*created})[0]
				}
				return commentEnvelope(cmd, flags, commentAddPath, status, payload)
			}
			if created != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Posted comment #%d on workout %d\n", created.ID, workoutID)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Posted a comment on workout %d\n", workoutID)
			return nil
		},
	}
	cmd.Flags().IntVar(&workoutID, "workout-id", 0, "Идентификатор тренировки")
	cmd.Flags().StringVar(&text, "text", "", "Текст комментария")
	cmd.Flags().StringVar(&textFile, "text-file", "", "Файл с текстом комментария")
	cmd.Flags().BoolVar(&asHTML, "html", false, "Отправить текст как HTML, без экранирования")
	cmd.Flags().BoolVar(&assumeOK, "yes", false, "Подтвердить запись в ветку, которую видит клиент")
	return cmd
}

// newestUnknown re-reads the thread and returns the comment that was not there
// before the write. A failure to re-read is not a failure of the write, so the
// caller gets nil rather than an error.
func newestUnknown(c interface {
	Post(path string, body any) (json.RawMessage, int, error)
}, workoutID int, known map[int]bool) *workoutComment {
	after, err := fetchComments(c, workoutID)
	if err != nil {
		return nil
	}
	var newest *workoutComment
	for i := range after {
		if known[after[i].ID] {
			continue
		}
		if newest == nil || after[i].ID > newest.ID {
			newest = &after[i]
		}
	}
	return newest
}

func newCoachEditCommentCmd(flags *rootFlags) *cobra.Command {
	var commentID, workoutID int
	var text, textFile string
	var asHTML bool
	var assumeOK bool

	cmd := &cobra.Command{
		Use:   "edit-comment",
		Short: "Переписать свой комментарий",
		Long: `Заменяет текст комментария целиком.

Идентификатор берётся из 'coach comments --workout-id N'. Если передать ещё
и --workout-id, команда покажет прежний текст перед заменой - иначе показать
его неоткуда, потому что читать комментарий по одному идентификатору API не
умеет.`,
		Example: `  trenda-pp-cli coach edit-comment --comment-id 2695 --text "Как самочувствие?"`,
		Annotations: map[string]string{
			"pp:endpoint": "coach.edit-comment", "pp:method": "POST", "pp:path": commentEditPath,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if commentID == 0 && !flags.dryRun {
				return usageErr(fmt.Errorf("required flag \"comment-id\" not set"))
			}
			if dryRunOK(flags) {
				return nil
			}
			body, err := commentBody(text, textFile, asHTML)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			prompt := fmt.Sprintf("Replace comment #%d with:\n  %s\n", commentID, commentPlainText(body))
			if workoutID != 0 {
				if current := commentByID(c, workoutID, commentID); current != nil {
					prompt = fmt.Sprintf("Replace comment #%d\n  was: %s\n  now: %s\n",
						commentID, commentPlainText(current.Text), commentPlainText(body))
				}
			}
			ok, err := confirmComment(cmd, flags, assumeOK, prompt)
			if err != nil {
				return err
			}
			if !ok {
				return writeNoop(flags, "declined", "not edited")
			}

			_, status, err := c.Post(commentEditPath, map[string]any{
				"comment_id": commentID,
				"text":       body,
			})
			if err != nil {
				return commentWriteError(err, commentID, flags)
			}
			if wantsJSON(cmd, flags) {
				if flags.quiet {
					return nil
				}
				return commentEnvelope(cmd, flags, commentEditPath, status, map[string]any{
					"comment_id": commentID,
					"text":       body,
					"text_plain": commentPlainText(body),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Edited comment #%d\n", commentID)
			return nil
		},
	}
	cmd.Flags().IntVar(&commentID, "comment-id", 0, "Идентификатор комментария")
	cmd.Flags().IntVar(&workoutID, "workout-id", 0, "Тренировка комментария: нужна, чтобы показать прежний текст")
	cmd.Flags().StringVar(&text, "text", "", "Новый текст комментария")
	cmd.Flags().StringVar(&textFile, "text-file", "", "Файл с новым текстом")
	cmd.Flags().BoolVar(&asHTML, "html", false, "Отправить текст как HTML, без экранирования")
	cmd.Flags().BoolVar(&assumeOK, "yes", false, "Подтвердить запись в ветку, которую видит клиент")
	return cmd
}

func newCoachDeleteCommentCmd(flags *rootFlags) *cobra.Command {
	var commentID, workoutID int
	var assumeOK bool

	cmd := &cobra.Command{
		Use:   "delete-comment",
		Short: "Удалить комментарий",
		Long: `Удаляет комментарий безвозвратно.

Идентификатор берётся из 'coach comments --workout-id N'. С --workout-id
команда сначала покажет, что именно удаляет.`,
		Example: "  trenda-pp-cli coach delete-comment --comment-id 2695 --workout-id 66196",
		Annotations: map[string]string{
			"pp:endpoint": "coach.delete-comment", "pp:method": "POST", "pp:path": commentDeletePath,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if commentID == 0 && !flags.dryRun {
				return usageErr(fmt.Errorf("required flag \"comment-id\" not set"))
			}
			if dryRunOK(flags) {
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			prompt := fmt.Sprintf("Delete comment #%d for good?", commentID)
			if workoutID != 0 {
				if current := commentByID(c, workoutID, commentID); current != nil {
					prompt = fmt.Sprintf("Delete comment #%d by %s for good?\n  %s\n",
						commentID, current.AuthorName, commentPlainText(current.Text))
				}
			}
			ok, err := confirmComment(cmd, flags, assumeOK, prompt)
			if err != nil {
				return err
			}
			if !ok {
				return writeNoop(flags, "declined", "not deleted")
			}

			_, status, err := c.Post(commentDeletePath, map[string]any{"comment_id": commentID})
			if err != nil {
				return commentWriteError(err, commentID, flags)
			}
			if wantsJSON(cmd, flags) {
				if flags.quiet {
					return nil
				}
				return commentEnvelope(cmd, flags, commentDeletePath, status, map[string]any{
					"comment_id": commentID,
					"deleted":    true,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted comment #%d\n", commentID)
			return nil
		},
	}
	cmd.Flags().IntVar(&commentID, "comment-id", 0, "Идентификатор комментария")
	cmd.Flags().IntVar(&workoutID, "workout-id", 0, "Тренировка комментария: нужна, чтобы показать удаляемый текст")
	cmd.Flags().BoolVar(&assumeOK, "yes", false, "Подтвердить запись в ветку, которую видит клиент")
	return cmd
}

// commentByID looks one comment up inside its workout's thread. Used only to
// show a human what they are about to overwrite or delete, so a lookup failure
// costs the preview, not the command.
func commentByID(c interface {
	Post(path string, body any) (json.RawMessage, int, error)
}, workoutID, commentID int) *workoutComment {
	comments, err := fetchComments(c, workoutID)
	if err != nil {
		return nil
	}
	for i := range comments {
		if comments[i].ID == commentID {
			return &comments[i]
		}
	}
	return nil
}

// commentWriteError keeps the API's own "no such comment" answer from being
// reported as a generic upstream failure: a wrong id is the common mistake here,
// and it deserves the not-found exit code and a pointer at where ids come from.
func commentWriteError(err error, commentID int, flags *rootFlags) error {
	if strings.Contains(err.Error(), "CommentNotFound") {
		return notFoundErr(fmt.Errorf("comment %d not found, or it belongs to another coach"+
			"\nhint: list ids with 'trenda-pp-cli coach comments --workout-id <id>'", commentID))
	}
	return classifyAPIError(err, flags)
}
