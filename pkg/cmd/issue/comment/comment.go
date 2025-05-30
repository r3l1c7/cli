package comment

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdComment(f *cmdutil.Factory, runF func(*prShared.CommentableOptions) error) *cobra.Command {
	opts := &prShared.CommentableOptions{
		IO:                        f.IOStreams,
		HttpClient:                f.HttpClient,
		EditSurvey:                prShared.CommentableEditSurvey(f.Config, f.IOStreams),
		InteractiveEditSurvey:     prShared.CommentableInteractiveEditSurvey(f.Config, f.IOStreams),
		ConfirmSubmitSurvey:       prShared.CommentableConfirmSubmitSurvey(f.Prompter),
		ConfirmCreateIfNoneSurvey: prShared.CommentableInteractiveCreateIfNoneSurvey(f.Prompter),
		ConfirmDeleteLastComment:  prShared.CommentableConfirmDeleteLastComment(f.Prompter),
		OpenInBrowser:             f.Browser.Browse,
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "comment {<number> | <url>}",
		Short: "Add a comment to an issue",
		Long: heredoc.Doc(`
			Add a comment to a GitHub issue.

			Without the body text supplied through flags, the command will interactively
			prompt for the comment text.
		`),
		Example: heredoc.Doc(`
			$ gh issue comment 12 --body "Hi from GitHub CLI"
		`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RetrieveCommentable = func() (prShared.Commentable, ghrepo.Interface, error) {
				// TODO wm: more testing
				issueNumber, parsedBaseRepo, err := shared.ParseIssueFromArg(args[0])
				if err != nil {
					return nil, nil, err
				}

				// If the args provided the base repo then use that directly.
				var baseRepo ghrepo.Interface

				if parsedBaseRepo, present := parsedBaseRepo.Value(); present {
					baseRepo = parsedBaseRepo
				} else {
					// support `-R, --repo` override
					baseRepo, err = f.BaseRepo()
					if err != nil {
						return nil, nil, err
					}
				}

				httpClient, err := f.HttpClient()
				if err != nil {
					return nil, nil, err
				}

				fields := []string{"id", "url"}
				if opts.EditLast || opts.DeleteLast {
					fields = append(fields, "comments")
				}

				issue, err := issueShared.FindIssueOrPR(httpClient, baseRepo, issueNumber, fields)
				if err != nil {
					return nil, nil, err
				}

				return issue, baseRepo, nil
			}
			return prShared.CommentablePreRun(cmd, opts)
		},
		RunE: func(_ *cobra.Command, args []string) error {
			if bodyFile != "" {
				b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
				if err != nil {
					return err
				}
				opts.Body = string(b)
			}

			if runF != nil {
				return runF(opts)
			}
			return prShared.CommentableRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "The comment body `text`")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().BoolP("editor", "e", false, "Skip prompts and open the text editor to write the body in")
	cmd.Flags().BoolP("web", "w", false, "Open the web browser to write the comment")
	cmd.Flags().BoolVar(&opts.EditLast, "edit-last", false, "Edit the last comment of the current user")
	cmd.Flags().BoolVar(&opts.DeleteLast, "delete-last", false, "Delete the last comment of the current user")
	cmd.Flags().BoolVar(&opts.DeleteLastConfirmed, "yes", false, "Skip the delete confirmation prompt when --delete-last is provided")
	cmd.Flags().BoolVar(&opts.CreateIfNone, "create-if-none", false, "Create a new comment if no comments are found. Can be used only with --edit-last")

	return cmd
}
