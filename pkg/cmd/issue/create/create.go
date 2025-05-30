package create

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	HttpClient       func() (*http.Client, error)
	Config           func() (gh.Config, error)
	IO               *iostreams.IOStreams
	BaseRepo         func() (ghrepo.Interface, error)
	Browser          browser.Browser
	Prompter         prShared.Prompt
	Detector         fd.Detector
	TitledEditSurvey func(string, string) (string, string, error)

	RootDirOverride string

	HasRepoOverride bool
	EditorMode      bool
	WebMode         bool
	RecoverFile     string

	Title       string
	Body        string
	Interactive bool

	Assignees []string
	Labels    []string
	Projects  []string
	Milestone string
	Template  string
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Browser:    f.Browser,
		Prompter:   f.Prompter,

		TitledEditSurvey: prShared.TitledEditSurvey(&prShared.UserEditor{Config: f.Config, IO: f.IOStreams}),
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new issue",
		Long: heredoc.Docf(`
			Create an issue on GitHub.

			Adding an issue to projects requires authorization with the %[1]sproject%[1]s scope.
			To authorize, run %[1]sgh auth refresh -s project%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			$ gh issue create --title "I found a bug" --body "Nothing works"
			$ gh issue create --label "bug,help wanted"
			$ gh issue create --label bug --label "help wanted"
			$ gh issue create --assignee monalisa,hubot
			$ gh issue create --assignee "@me"
			$ gh issue create --project "Roadmap"
			$ gh issue create --template "Bug Report"
		`),
		Args:    cmdutil.NoArgsQuoteReminder,
		Aliases: []string{"new"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.HasRepoOverride = cmd.Flags().Changed("repo")

			var err error
			opts.EditorMode, err = prShared.InitEditorMode(f, opts.EditorMode, opts.WebMode, opts.IO.CanPrompt())
			if err != nil {
				return err
			}

			titleProvided := cmd.Flags().Changed("title")
			bodyProvided := cmd.Flags().Changed("body")
			if bodyFile != "" {
				b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
				if err != nil {
					return err
				}
				opts.Body = string(b)
				bodyProvided = true
			}

			if !opts.IO.CanPrompt() && opts.RecoverFile != "" {
				return cmdutil.FlagErrorf("`--recover` only supported when running interactively")
			}

			if opts.Template != "" && bodyProvided {
				return errors.New("`--template` is not supported when using `--body` or `--body-file`")
			}

			opts.Interactive = !opts.EditorMode && !(titleProvided && bodyProvided)

			if opts.Interactive && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("must provide `--title` and `--body` when not running interactively")
			}

			if runF != nil {
				return runF(opts)
			}
			return createRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Supply a title. Will prompt for one otherwise.")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Supply a body. Will prompt for one otherwise.")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().BoolVarP(&opts.EditorMode, "editor", "e", false, "Skip prompts and open the text editor to write the title and body in. The first line is the title and the remaining text is the body.")
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the browser to create an issue")
	cmd.Flags().StringSliceVarP(&opts.Assignees, "assignee", "a", nil, "Assign people by their `login`. Use \"@me\" to self-assign.")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Add labels by `name`")
	cmd.Flags().StringSliceVarP(&opts.Projects, "project", "p", nil, "Add the issue to projects by `title`")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Add the issue to a milestone by `name`")
	cmd.Flags().StringVar(&opts.RecoverFile, "recover", "", "Recover input from a failed run of create")
	cmd.Flags().StringVarP(&opts.Template, "template", "T", "", "Template `name` to use as starting body text")

	return cmd
}

func createRun(opts *CreateOptions) (err error) {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return
	}

	// TODO projectsV1Deprecation
	// Remove this section as we should no longer need to detect
	if opts.Detector == nil {
		cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
		opts.Detector = fd.NewDetector(cachedClient, baseRepo.RepoHost())
	}

	projectsV1Support := opts.Detector.ProjectsV1()

	isTerminal := opts.IO.IsStdoutTTY()

	var milestones []string
	if opts.Milestone != "" {
		milestones = []string{opts.Milestone}
	}

	meReplacer := prShared.NewMeReplacer(apiClient, baseRepo.RepoHost())
	assignees, err := meReplacer.ReplaceSlice(opts.Assignees)
	if err != nil {
		return err
	}

	tb := prShared.IssueMetadataState{
		Type:          prShared.IssueMetadata,
		Assignees:     assignees,
		Labels:        opts.Labels,
		ProjectTitles: opts.Projects,
		Milestones:    milestones,
		Title:         opts.Title,
		Body:          opts.Body,
	}

	if opts.RecoverFile != "" {
		err = prShared.FillFromJSON(opts.IO, opts.RecoverFile, &tb)
		if err != nil {
			err = fmt.Errorf("failed to recover input: %w", err)
			return
		}
	}

	tpl := prShared.NewTemplateManager(httpClient, baseRepo, opts.Prompter, opts.RootDirOverride, !opts.HasRepoOverride, false)

	if opts.WebMode {
		var openURL string
		if opts.Title != "" || opts.Body != "" || tb.HasMetadata() {
			openURL, err = generatePreviewURL(apiClient, baseRepo, tb, projectsV1Support)
			if err != nil {
				return
			}
			if !prShared.ValidURL(openURL) {
				err = fmt.Errorf("cannot open in browser: maximum URL length exceeded")
				return
			}
		} else if ok, _ := tpl.HasTemplates(); ok {
			openURL = ghrepo.GenerateRepoURL(baseRepo, "issues/new/choose")
		} else {
			openURL = ghrepo.GenerateRepoURL(baseRepo, "issues/new")
		}
		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	}

	if isTerminal {
		fmt.Fprintf(opts.IO.ErrOut, "\nCreating issue in %s\n\n", ghrepo.FullName(baseRepo))
	}

	repo, err := api.GitHubRepo(apiClient, baseRepo)
	if err != nil {
		return
	}
	if !repo.HasIssuesEnabled {
		err = fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(baseRepo))
		return
	}

	action := prShared.SubmitAction
	templateNameForSubmit := ""
	var openURL string

	if opts.Interactive {
		defer prShared.PreserveInput(opts.IO, &tb, &err)()

		if opts.Title == "" {
			err = prShared.TitleSurvey(opts.Prompter, opts.IO, &tb)
			if err != nil {
				return
			}
		}

		if opts.Body == "" {
			templateContent := ""

			if opts.RecoverFile == "" {
				var template prShared.Template

				if opts.Template != "" {
					template, err = tpl.Select(opts.Template)
					if err != nil {
						return
					}
				} else {
					template, err = tpl.Choose()
					if err != nil {
						return
					}
				}

				if template != nil {
					templateContent = string(template.Body())
					templateNameForSubmit = template.NameForSubmit()
				} else {
					templateContent = string(tpl.LegacyBody())
				}
			}

			err = prShared.BodySurvey(opts.Prompter, &tb, templateContent)
			if err != nil {
				return
			}
		}

		openURL, err = generatePreviewURL(apiClient, baseRepo, tb, projectsV1Support)
		if err != nil {
			return
		}

		allowPreview := !tb.HasMetadata() && prShared.ValidURL(openURL)
		action, err = prShared.ConfirmIssueSubmission(opts.Prompter, allowPreview, repo.ViewerCanTriage())
		if err != nil {
			err = fmt.Errorf("unable to confirm: %w", err)
			return
		}

		if action == prShared.MetadataAction {
			fetcher := &prShared.MetadataFetcher{
				IO:        opts.IO,
				APIClient: apiClient,
				Repo:      baseRepo,
				State:     &tb,
			}
			err = prShared.MetadataSurvey(opts.Prompter, opts.IO, baseRepo, fetcher, &tb, projectsV1Support)
			if err != nil {
				return
			}

			action, err = prShared.ConfirmIssueSubmission(opts.Prompter, !tb.HasMetadata(), false)
			if err != nil {
				return
			}
		}

		if action == prShared.CancelAction {
			fmt.Fprintln(opts.IO.ErrOut, "Discarding.")
			err = cmdutil.CancelError
			return
		}
	} else {
		if opts.EditorMode {
			if opts.Template != "" {
				var template prShared.Template
				template, err = tpl.Select(opts.Template)
				if err != nil {
					return
				}
				if tb.Title == "" {
					tb.Title = template.Title()
				}
				templateNameForSubmit = template.NameForSubmit()
				tb.Body = string(template.Body())
			}

			tb.Title, tb.Body, err = opts.TitledEditSurvey(tb.Title, tb.Body)
			if err != nil {
				return
			}
		}
		if tb.Title == "" {
			err = fmt.Errorf("title can't be blank")
			return
		}
	}

	if action == prShared.PreviewAction {
		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	} else if action == prShared.SubmitAction {
		params := map[string]interface{}{
			"title": tb.Title,
			"body":  tb.Body,
		}
		if templateNameForSubmit != "" {
			params["issueTemplate"] = templateNameForSubmit
		}

		err = prShared.AddMetadataToIssueParams(apiClient, baseRepo, params, &tb, projectsV1Support)
		if err != nil {
			return
		}

		var newIssue *api.Issue
		newIssue, err = api.IssueCreate(apiClient, repo, params)
		if err != nil {
			return
		}

		fmt.Fprintln(opts.IO.Out, newIssue.URL)
	} else {
		panic("Unreachable state")
	}

	return
}

func generatePreviewURL(apiClient *api.Client, baseRepo ghrepo.Interface, tb prShared.IssueMetadataState, projectsV1Support gh.ProjectsV1Support) (string, error) {
	openURL := ghrepo.GenerateRepoURL(baseRepo, "issues/new")
	return prShared.WithPrAndIssueQueryParams(apiClient, baseRepo, openURL, tb, projectsV1Support)
}
