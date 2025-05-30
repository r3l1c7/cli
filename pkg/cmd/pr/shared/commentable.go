package shared

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/surveyext"
	"github.com/spf13/cobra"
)

var errNoUserComments = errors.New("no comments found for current user")
var errDeleteNotConfirmed = errors.New("deletion not confirmed")

type InputType int

const (
	InputTypeEditor InputType = iota
	InputTypeInline
	InputTypeWeb
)

type Commentable interface {
	Link() string
	Identifier() string
	CurrentUserComments() []api.Comment
}

type CommentableOptions struct {
	IO                        *iostreams.IOStreams
	HttpClient                func() (*http.Client, error)
	RetrieveCommentable       func() (Commentable, ghrepo.Interface, error)
	EditSurvey                func(string) (string, error)
	InteractiveEditSurvey     func(string) (string, error)
	ConfirmSubmitSurvey       func() (bool, error)
	ConfirmCreateIfNoneSurvey func() (bool, error)
	ConfirmDeleteLastComment  func(string) (bool, error)
	OpenInBrowser             func(string) error
	Interactive               bool
	InputType                 InputType
	Body                      string
	EditLast                  bool
	DeleteLast                bool
	DeleteLastConfirmed       bool
	CreateIfNone              bool
	Quiet                     bool
	Host                      string
}

func CommentablePreRun(cmd *cobra.Command, opts *CommentableOptions) error {
	inputFlags := 0
	if cmd.Flags().Changed("body") {
		opts.InputType = InputTypeInline
		inputFlags++
	}
	if cmd.Flags().Changed("body-file") {
		opts.InputType = InputTypeInline
		inputFlags++
	}
	if web, _ := cmd.Flags().GetBool("web"); web {
		opts.InputType = InputTypeWeb
		inputFlags++
	}
	if editor, _ := cmd.Flags().GetBool("editor"); editor {
		opts.InputType = InputTypeEditor
		inputFlags++
	}

	if opts.CreateIfNone && !opts.EditLast {
		return cmdutil.FlagErrorf("`--create-if-none` can only be used with `--edit-last`")
	}

	if opts.DeleteLastConfirmed && !opts.DeleteLast {
		return cmdutil.FlagErrorf("`--yes` should only be used with `--delete-last`")
	}

	if opts.DeleteLast {
		if inputFlags > 0 {
			return cmdutil.FlagErrorf("should not provide comment body when using `--delete-last`")
		}
		if opts.IO.CanPrompt() || opts.DeleteLastConfirmed {
			opts.Interactive = opts.IO.CanPrompt()
			return nil
		}
		return cmdutil.FlagErrorf("should provide `--yes` to confirm deletion in non-interactive mode")
	}

	if inputFlags == 0 {
		if !opts.IO.CanPrompt() {
			return cmdutil.FlagErrorf("flags required when not running interactively")
		}
		opts.Interactive = true
	} else if inputFlags > 1 {
		return cmdutil.FlagErrorf("specify only one of `--body`, `--body-file`, `--editor`, or `--web`")
	}

	return nil
}

func CommentableRun(opts *CommentableOptions) error {
	commentable, repo, err := opts.RetrieveCommentable()
	if err != nil {
		return err
	}
	opts.Host = repo.RepoHost()
	if opts.DeleteLast {
		return deleteComment(commentable, opts)
	}

	// Create new comment, bail before complexities of updating the last comment
	if !opts.EditLast {
		return createComment(commentable, opts)
	}

	// Update the last comment, handling success or unexpected errors accordingly
	err = updateComment(commentable, opts)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errNoUserComments) {
		return err
	}

	// Determine whether to create new comment, prompt user if interactive and missing option
	if !opts.CreateIfNone && opts.Interactive {
		opts.CreateIfNone, err = opts.ConfirmCreateIfNoneSurvey()
		if err != nil {
			return err
		}
	}
	if !opts.CreateIfNone {
		return errNoUserComments
	}

	// Create new comment because updating the last comment failed due to no user comments
	if opts.Interactive {
		fmt.Fprintln(opts.IO.ErrOut, "No comments found. Creating a new comment.")
	}

	return createComment(commentable, opts)
}

func createComment(commentable Commentable, opts *CommentableOptions) error {
	switch opts.InputType {
	case InputTypeWeb:
		openURL := commentable.Link() + "#issuecomment-new"
		if opts.IO.IsStdoutTTY() && !opts.Quiet {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.OpenInBrowser(openURL)
	case InputTypeEditor:
		var body string
		var err error
		if opts.Interactive {
			body, err = opts.InteractiveEditSurvey("")
		} else {
			body, err = opts.EditSurvey("")
		}
		if err != nil {
			return err
		}
		opts.Body = body
	}

	if opts.Interactive {
		cont, err := opts.ConfirmSubmitSurvey()
		if err != nil {
			return err
		}
		if !cont {
			return errors.New("Discarding...")
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	params := api.CommentCreateInput{Body: opts.Body, SubjectId: commentable.Identifier()}
	url, err := api.CommentCreate(apiClient, opts.Host, params)
	if err != nil {
		return err
	}

	if !opts.Quiet {
		fmt.Fprintln(opts.IO.Out, url)
	}

	return nil
}

func updateComment(commentable Commentable, opts *CommentableOptions) error {
	comments := commentable.CurrentUserComments()
	if len(comments) == 0 {
		return errNoUserComments
	}

	lastComment := &comments[len(comments)-1]

	switch opts.InputType {
	case InputTypeWeb:
		openURL := lastComment.Link()
		if opts.IO.IsStdoutTTY() && !opts.Quiet {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.OpenInBrowser(openURL)
	case InputTypeEditor:
		var body string
		var err error
		initialValue := lastComment.Content()
		if opts.Interactive {
			body, err = opts.InteractiveEditSurvey(initialValue)
		} else {
			body, err = opts.EditSurvey(initialValue)
		}
		if err != nil {
			return err
		}
		opts.Body = body
	}

	if opts.Interactive {
		cont, err := opts.ConfirmSubmitSurvey()
		if err != nil {
			return err
		}
		if !cont {
			return errors.New("Discarding...")
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	params := api.CommentUpdateInput{Body: opts.Body, CommentId: lastComment.Identifier()}
	url, err := api.CommentUpdate(apiClient, opts.Host, params)
	if err != nil {
		return err
	}

	if !opts.Quiet {
		fmt.Fprintln(opts.IO.Out, url)
	}

	return nil
}

func deleteComment(commentable Commentable, opts *CommentableOptions) error {
	comments := commentable.CurrentUserComments()
	if len(comments) == 0 {
		return errNoUserComments
	}

	lastComment := comments[len(comments)-1]

	cs := opts.IO.ColorScheme()

	if opts.Interactive && !opts.DeleteLastConfirmed {
		// This is not an ideal way of truncating a random string that may
		// contain emojis or other kind of wide chars.
		truncated := lastComment.Body
		if len(lastComment.Body) > 40 {
			truncated = lastComment.Body[:40] + "..."
		}

		fmt.Fprintf(opts.IO.Out, "%s Deleted comments cannot be recovered.\n", cs.WarningIcon())
		ok, err := opts.ConfirmDeleteLastComment(truncated)
		if err != nil {
			return err
		}
		if !ok {
			return errDeleteNotConfirmed
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	params := api.CommentDeleteInput{CommentId: lastComment.Identifier()}
	deletionErr := api.CommentDelete(apiClient, opts.Host, params)
	if deletionErr != nil {
		return deletionErr
	}

	if !opts.Quiet {
		fmt.Fprintln(opts.IO.ErrOut, "Comment deleted")
	}

	return nil
}

func CommentableConfirmSubmitSurvey(p Prompt) func() (bool, error) {
	return func() (bool, error) {
		return p.Confirm("Submit?", true)
	}
}

func CommentableInteractiveEditSurvey(cf func() (gh.Config, error), io *iostreams.IOStreams) func(string) (string, error) {
	return func(initialValue string) (string, error) {
		editorCommand, err := cmdutil.DetermineEditor(cf)
		if err != nil {
			return "", err
		}
		cs := io.ColorScheme()
		fmt.Fprintf(io.Out, "- %s to draft your comment in %s... ", cs.Bold("Press Enter"), cs.Bold(surveyext.EditorName(editorCommand)))
		_ = waitForEnter(io.In)
		return surveyext.Edit(editorCommand, "*.md", initialValue, io.In, io.Out, io.ErrOut)
	}
}

func CommentableInteractiveCreateIfNoneSurvey(p Prompt) func() (bool, error) {
	return func() (bool, error) {
		return p.Confirm("No comments found. Create one?", true)
	}
}

func CommentableEditSurvey(cf func() (gh.Config, error), io *iostreams.IOStreams) func(string) (string, error) {
	return func(initialValue string) (string, error) {
		editorCommand, err := cmdutil.DetermineEditor(cf)
		if err != nil {
			return "", err
		}
		return surveyext.Edit(editorCommand, "*.md", initialValue, io.In, io.Out, io.ErrOut)
	}
}

func CommentableConfirmDeleteLastComment(p Prompt) func(string) (bool, error) {
	return func(body string) (bool, error) {
		return p.Confirm(fmt.Sprintf("Delete the comment: %q?", body), true)
	}
}

func waitForEnter(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	return scanner.Err()
}
