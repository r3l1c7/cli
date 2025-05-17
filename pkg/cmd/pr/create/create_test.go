package create

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCreate(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := os.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name      string
		tty       bool
		stdin     string
		cli       string
		config    string
		wantsErr  bool
		wantsOpts CreateOptions
	}{
		{
			name:     "empty non-tty",
			tty:      false,
			cli:      "",
			wantsErr: true,
		},
		{
			name:     "only title non-tty",
			tty:      false,
			cli:      "--title mytitle",
			wantsErr: true,
		},
		{
			name:     "minimum non-tty",
			tty:      false,
			cli:      "--title mytitle --body ''",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				TitleProvided:       true,
				Body:                "",
				BodyProvided:        true,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "empty tty",
			tty:      true,
			cli:      "",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "",
				TitleProvided:       false,
				Body:                "",
				BodyProvided:        false,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "body from stdin",
			tty:      false,
			stdin:    "this is on standard input",
			cli:      "-t mytitle -F -",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				TitleProvided:       true,
				Body:                "this is on standard input",
				BodyProvided:        true,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "body from file",
			tty:      false,
			cli:      fmt.Sprintf("-t mytitle -F '%s'", tmpFile),
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				TitleProvided:       true,
				Body:                "a body from file",
				BodyProvided:        true,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "template from file name tty",
			tty:      true,
			cli:      "-t mytitle --template bug_fix.md",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				TitleProvided:       true,
				Body:                "",
				BodyProvided:        false,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
				Template:            "bug_fix.md",
			},
		},
		{
			name:     "template from file name non-tty",
			tty:      false,
			cli:      "-t mytitle --template bug_fix.md",
			wantsErr: true,
		},
		{
			name:     "template and body",
			tty:      false,
			cli:      `-t mytitle --template bug_fix.md --body "pr body"`,
			wantsErr: true,
		},
		{
			name:     "template and body file",
			tty:      false,
			cli:      "-t mytitle --template bug_fix.md --body-file body_file.md",
			wantsErr: true,
		},
		{
			name:     "fill-first",
			tty:      false,
			cli:      "--fill-first",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "",
				TitleProvided:       false,
				Body:                "",
				BodyProvided:        false,
				Autofill:            false,
				FillFirst:           true,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "fill and fill-first",
			tty:      false,
			cli:      "--fill --fill-first",
			wantsErr: true,
		},
		{
			name:     "dry-run and web",
			tty:      false,
			cli:      "--web --dry-run",
			wantsErr: true,
		},
		{
			name:     "editor by cli",
			tty:      true,
			cli:      "--editor",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "",
				Body:                "",
				RecoverFile:         "",
				WebMode:             false,
				EditorMode:          true,
				MaintainerCanModify: true,
			},
		},
		{
			name:     "editor by config",
			tty:      true,
			cli:      "",
			config:   "prefer_editor_prompt: enabled",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "",
				Body:                "",
				RecoverFile:         "",
				WebMode:             false,
				EditorMode:          true,
				MaintainerCanModify: true,
			},
		},
		{
			name:     "editor and web",
			tty:      true,
			cli:      "--editor --web",
			wantsErr: true,
		},
		{
			name:     "can use web even though editor is enabled by config",
			tty:      true,
			cli:      `--web --title mytitle --body "issue body"`,
			config:   "prefer_editor_prompt: enabled",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				Body:                "issue body",
				TitleProvided:       true,
				BodyProvided:        true,
				RecoverFile:         "",
				WebMode:             true,
				EditorMode:          false,
				MaintainerCanModify: true,
			},
		},
		{
			name:     "editor with non-tty",
			tty:      false,
			cli:      "--editor",
			wantsErr: true,
		},
		{
			name: "fill and base",
			cli:  "--fill --base trunk",
			wantsOpts: CreateOptions{
				Autofill:            true,
				BaseBranch:          "trunk",
				MaintainerCanModify: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, stdout, stderr := iostreams.Test()
			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			} else if tt.tty {
				ios.SetStdinTTY(true)
				ios.SetStdoutTTY(true)
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
				Config: func() (gh.Config, error) {
					if tt.config != "" {
						return config.NewFromString(tt.config), nil
					}
					return config.NewBlankConfig(), nil
				},
			}

			var opts *CreateOptions
			cmd := NewCmdCreate(f, func(o *CreateOptions) error {
				opts = o
				return nil
			})

			args, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetOut(stderr)
			cmd.SetErr(stderr)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())

			assert.Equal(t, tt.wantsOpts.Body, opts.Body)
			assert.Equal(t, tt.wantsOpts.BodyProvided, opts.BodyProvided)
			assert.Equal(t, tt.wantsOpts.Title, opts.Title)
			assert.Equal(t, tt.wantsOpts.TitleProvided, opts.TitleProvided)
			assert.Equal(t, tt.wantsOpts.Autofill, opts.Autofill)
			assert.Equal(t, tt.wantsOpts.FillFirst, opts.FillFirst)
			assert.Equal(t, tt.wantsOpts.WebMode, opts.WebMode)
			assert.Equal(t, tt.wantsOpts.RecoverFile, opts.RecoverFile)
			assert.Equal(t, tt.wantsOpts.IsDraft, opts.IsDraft)
			assert.Equal(t, tt.wantsOpts.MaintainerCanModify, opts.MaintainerCanModify)
			assert.Equal(t, tt.wantsOpts.BaseBranch, opts.BaseBranch)
			assert.Equal(t, tt.wantsOpts.HeadBranch, opts.HeadBranch)
			assert.Equal(t, tt.wantsOpts.Template, opts.Template)
		})
	}
}

func Test_createRun(t *testing.T) {
	tests := []struct {
		name               string
		setup              func(*CreateOptions, *testing.T) func()
		cmdStubs           func(*run.CommandStubber)
		promptStubs        func(*prompter.PrompterMock)
		httpStubs          func(*httpmock.Registry, *testing.T)
		expectedOutputs    []string
		expectedOut        string
		expectedErrOut     string
		expectedBrowse     string
		wantErr            string
		tty                bool
		customBranchConfig bool
	}{
		{
			name: "nontty web",
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.WebMode = true
				opts.HeadBranch = "feature"
				return func() {}
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
			},
			expectedBrowse: "https://github.com/OWNER/REPO/compare/master...feature?body=&expand=1",
		},
		{
			name: "nontty",
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{ "data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
						} } } }`,
						func(input map[string]interface{}) {
							assert.Equal(t, "REPOID", input["repositoryId"])
							assert.Equal(t, "my title", input["title"])
							assert.Equal(t, "my body", input["body"])
							assert.Equal(t, "master", input["baseRefName"])
							assert.Equal(t, "feature", input["headRefName"])
						}))
			},
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				opts.HeadBranch = "feature"
				return func() {}
			},
			expectedOut: "https://github.com/OWNER/REPO/pull/12\n",
		},
		{
			name: "dry-run-nontty-with-default-base",
			tty:  false,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				opts.HeadBranch = "feature"
				opts.DryRun = true
				return func() {}
			},
			expectedOutputs: []string{
				"Would have created a Pull Request with:",
				`title:	my title`,
				`draft:	false`,
				`base:	master`,
				`head:	feature`,
				`maintainerCanModify:	false`,
				`body:`,
				`my body`,
				``,
			},
			expectedErrOut: "",
		},
		{
			name: "dry-run-nontty-with-all-opts",
			tty:  false,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "TITLE"
				opts.Body = "BODY"
				opts.BaseBranch = "trunk"
				opts.HeadBranch = "feature"
				opts.Assignees = []string{"monalisa"}
				opts.Labels = []string{"bug", "todo"}
				opts.Projects = []string{"roadmap"}
				opts.Reviewers = []string{"hubot", "monalisa", "/core", "/robots"}
				opts.Milestone = "big one.oh"
				opts.DryRun = true
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
					httpmock.StringResponse(`
					{ "data": {
						"u000": { "login": "MonaLisa", "id": "MONAID" },
						"u001": { "login": "hubot", "id": "HUBOTID" },
						"repository": {
							"l000": { "name": "bug", "id": "BUGID" },
							"l001": { "name": "TODO", "id": "TODOID" }
						},
						"organization": {
							"t000": { "slug": "core", "id": "COREID" },
							"t001": { "slug": "robots", "id": "ROBOTID" }
						}
					} }
					`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryMilestoneList\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "milestones": {
						"nodes": [
							{ "title": "GA", "id": "GAID" },
							{ "title": "Big One.oh", "id": "BIGONEID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				mockRetrieveProjects(t, reg)
			},
			expectedOutputs: []string{
				"Would have created a Pull Request with:",
				`title:	TITLE`,
				`draft:	false`,
				`base:	trunk`,
				`head:	feature`,
				`labels:	bug, todo`,
				`reviewers:	hubot, monalisa, /core, /robots`,
				`assignees:	monalisa`,
				`milestones:	big one.oh`,
				`projects:	roadmap`,
				`maintainerCanModify:	false`,
				`body:`,
				`BODY`,
				``,
			},
			expectedErrOut: "",
		},
		{
			name: "dry-run-tty-with-default-base",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				opts.HeadBranch = "feature"
				opts.DryRun = true
				return func() {}
			},
			expectedOutputs: []string{
				`Would have created a Pull Request with:`,
				`Title: my title`,
				`Draft: false`,
				`Base: master`,
				`Head: feature`,
				`MaintainerCanModify: false`,
				`Body:`,
				``,
				`  my body                                                                     `,
				``,
				``,
			},
			expectedErrOut: heredoc.Doc(`

			Dry Running pull request for feature into master in OWNER/REPO

		`),
		},
		{
			name: "dry-run-tty-with-all-opts",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "TITLE"
				opts.Body = "BODY"
				opts.BaseBranch = "trunk"
				opts.HeadBranch = "feature"
				opts.Assignees = []string{"monalisa"}
				opts.Labels = []string{"bug", "todo"}
				opts.Projects = []string{"roadmap"}
				opts.Reviewers = []string{"hubot", "monalisa", "/core", "/robots"}
				opts.Milestone = "big one.oh"
				opts.DryRun = true
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
					httpmock.StringResponse(`
					{ "data": {
						"u000": { "login": "MonaLisa", "id": "MONAID" },
						"u001": { "login": "hubot", "id": "HUBOTID" },
						"repository": {
							"l000": { "name": "bug", "id": "BUGID" },
							"l001": { "name": "TODO", "id": "TODOID" }
						},
						"organization": {
							"t000": { "slug": "core", "id": "COREID" },
							"t001": { "slug": "robots", "id": "ROBOTID" }
						}
					} }
					`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryMilestoneList\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "milestones": {
						"nodes": [
							{ "title": "GA", "id": "GAID" },
							{ "title": "Big One.oh", "id": "BIGONEID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				mockRetrieveProjects(t, reg)
			},
			expectedOutputs: []string{
				`Would have created a Pull Request with:`,
				`Title: TITLE`,
				`Draft: false`,
				`Base: trunk`,
				`Head: feature`,
				`Labels: bug, todo`,
				`Reviewers: hubot, monalisa, /core, /robots`,
				`Assignees: monalisa`,
				`Milestones: big one.oh`,
				`Projects: roadmap`,
				`MaintainerCanModify: false`,
				`Body:`,
				``,
				`  BODY                                                                        `,
				``,
				``,
			},
			expectedErrOut: heredoc.Doc(`

			Dry Running pull request for feature into trunk in OWNER/REPO

		`),
		},
		{
			name: "dry-run-tty-with-empty-body",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "TITLE"
				opts.Body = ""
				opts.HeadBranch = "feature"
				opts.DryRun = true
				return func() {}
			},
			expectedOut: heredoc.Doc(`
				Would have created a Pull Request with:
				Title: TITLE
				Draft: false
				Base: master
				Head: feature
				MaintainerCanModify: false
				Body:
				No description provided
			`),
			expectedErrOut: heredoc.Doc(`

			Dry Running pull request for feature into master in OWNER/REPO

		`),
		},
		{
			name: "select a specific branch to push to on prompt",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
							{ "data": { "createPullRequest": { "pullRequest": {
								"URL": "https://github.com/OWNER/REPO/pull/12"
							} } } }`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "my title", input["title"].(string))
						assert.Equal(t, "my body", input["body"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "feature", input["headRefName"].(string))
						assert.Equal(t, false, input["draft"].(bool))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register("git rev-parse --symbolic-full-name feature@{push}", 0, "refs/remotes/origin/feature")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register("git show-ref --verify -- HEAD refs/remotes/origin/feature", 1, "")
				cs.Register(`git push --set-upstream origin HEAD:refs/heads/feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "skip pushing to branch on prompt",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
							{ "data": { "createPullRequest": { "pullRequest": {
								"URL": "https://github.com/OWNER/REPO/pull/12"
							} } } }`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "my title", input["title"].(string))
						assert.Equal(t, "my body", input["body"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "feature", input["headRefName"].(string))
						assert.Equal(t, false, input["draft"].(bool))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register("git rev-parse --symbolic-full-name feature@{push}", 0, "refs/remotes/origin/feature")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register("git show-ref --verify -- HEAD refs/remotes/origin/feature", 1, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return prompter.IndexFor(opts, "Skip pushing the branch")
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "project v2",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				opts.Projects = []string{"RoadmapV2"}
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				mockRetrieveProjects(t, reg)
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
							{ "data": { "createPullRequest": { "pullRequest": {
								"id": "PullRequest#1",
								"URL": "https://github.com/OWNER/REPO/pull/12"
							} } } }
							`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "my title", input["title"].(string))
						assert.Equal(t, "my body", input["body"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "feature", input["headRefName"].(string))
						assert.Equal(t, false, input["draft"].(bool))
					}))
				reg.Register(
					httpmock.GraphQL(`mutation UpdateProjectV2Items\b`),
					httpmock.GraphQLQuery(`
							{ "data": { "add_000": { "item": {
								"id": "1"
							} } } }
							`, func(mutations string, inputs map[string]interface{}) {
						variables, err := json.Marshal(inputs)
						assert.NoError(t, err)
						expectedMutations := "mutation UpdateProjectV2Items($input_000: AddProjectV2ItemByIdInput!) {add_000: addProjectV2ItemById(input: $input_000) { item { id } }}"
						expectedVariables := `{"input_000":{"contentId":"PullRequest#1","projectId":"ROADMAPV2ID"}}`
						assert.Equal(t, expectedMutations, mutations)
						assert.Equal(t, expectedVariables, string(variables))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register("git rev-parse --symbolic-full-name feature@{push}", 0, "refs/remotes/origin/feature")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register(`git push --set-upstream origin HEAD:refs/heads/feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "no maintainer modify",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
							{ "data": { "createPullRequest": { "pullRequest": {
								"URL": "https://github.com/OWNER/REPO/pull/12"
							} } } }
							`, func(input map[string]interface{}) {
						assert.Equal(t, false, input["maintainerCanModify"].(bool))
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "my title", input["title"].(string))
						assert.Equal(t, "my body", input["body"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "feature", input["headRefName"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register("git rev-parse --symbolic-full-name feature@{push}", 0, "refs/remotes/origin/feature")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register(`git push --set-upstream origin HEAD:refs/heads/feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "create fork",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "title"
				opts.Body = "body"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "monalisa"} } }`))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/forks"),
					httpmock.StatusStringResponse(201, `
							{ "node_id": "NODEID",
							  "name": "REPO",
							  "owner": {"login": "monalisa"}
							}`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
							{ "data": { "createPullRequest": { "pullRequest": {
								"URL": "https://github.com/OWNER/REPO/pull/12"
							}}}}`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "monalisa:feature", input["headRefName"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register("git rev-parse --symbolic-full-name feature@{push}", 1, "")
				cs.Register("git config remote.pushDefault", 1, "")
				cs.Register("git config push.default", 1, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register("git remote rename origin upstream", 0, "")
				cs.Register(`git remote add origin https://github.com/monalisa/REPO.git`, 0, "")
				cs.Register(`git push --set-upstream origin HEAD:refs/heads/feature`, 0, "")
				cs.Register(`git config --add remote.upstream.gh-resolved base`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return prompter.IndexFor(opts, "Create a fork of OWNER/REPO")
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for monalisa:feature into master in OWNER/REPO\n\nChanged OWNER/REPO remote to \"upstream\"\nAdded monalisa/REPO as remote \"origin\"\n! Repository monalisa/REPO set as the default repository. To learn more about the default repository, run: gh repo set-default --help\n",
		},
		{
			name: "pushed to non base repo",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "title"
				opts.Body = "body"
				opts.Remotes = func() (context.Remotes, error) {
					return context.Remotes{
						{
							Remote: &git.Remote{
								Name:     "upstream",
								Resolved: "base",
							},
							Repo: ghrepo.New("OWNER", "REPO"),
						},
						{
							Remote: &git.Remote{
								Name:     "origin",
								Resolved: "base",
							},
							Repo: ghrepo.New("monalisa", "REPO"),
						},
					}, nil
				}
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
							{ "data": { "createPullRequest": { "pullRequest": {
								"URL": "https://github.com/OWNER/REPO/pull/12"
							} } } }`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "monalisa:feature", input["headRefName"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register("git rev-parse --symbolic-full-name feature@{push}", 0, "refs/remotes/origin/feature")
				cs.Register("git show-ref --verify -- HEAD refs/remotes/origin/feature", 0, heredoc.Doc(`
				deadbeef HEAD
				deadbeef refs/remotes/origin/feature`))
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for monalisa:feature into master in OWNER/REPO\n\n",
		},
		{
			name: "pushed to different branch name",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "title"
				opts.Body = "body"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
			{ "data": { "createPullRequest": { "pullRequest": {
				"URL": "https://github.com/OWNER/REPO/pull/12"
			} } } }
			`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "master", input["baseRefName"].(string))
						assert.Equal(t, "my-feat2", input["headRefName"].(string))
					}))
			},
			customBranchConfig: true,
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp \^branch\\\.feature\\\.`, 0, heredoc.Doc(`
			branch.feature.remote origin
			branch.feature.merge refs/heads/my-feat2
		`))
				cs.Register("git rev-parse --symbolic-full-name feature@{push}", 0, "refs/remotes/origin/my-feat2")
				cs.Register("git show-ref --verify -- HEAD refs/remotes/origin/my-feat2", 0, heredoc.Doc(`
			deadbeef HEAD
			deadbeef refs/remotes/origin/my-feat2
		`))
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for my-feat2 into master in OWNER/REPO\n\n",
		},
		{
			name: "non legacy template",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.Title = "my title"
				opts.HeadBranch = "feature"
				opts.RootDirOverride = "./fixtures/repoWithNonLegacyPRTemplates"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query PullRequestTemplates\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "pullRequestTemplates": [
						{ "filename": "template1",
						  "body": "this is a bug" },
						{ "filename": "template2",
						  "body": "this is a enhancement" }
					] } } }`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
				{ "data": { "createPullRequest": { "pullRequest": {
					"URL": "https://github.com/OWNER/REPO/pull/12"
				} } } }
				`, func(input map[string]interface{}) {
						assert.Equal(t, "my title", input["title"].(string))
						assert.Equal(t, "- **commit 1**\n- **commit 0**\n\nthis is a bug", input["body"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "d3476a1\u0000commit 0\u0000\u0000\n7a6ea13\u0000commit 1\u0000\u0000")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.MarkdownEditorFunc = func(p, d string, ba bool) (string, error) {
					if p == "Body" {
						return d, nil
					} else {
						return "", prompter.NoSuchPromptErr(p)
					}
				}
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					switch p {
					case "What's next?":
						return 0, nil
					case "Choose a template":
						return prompter.IndexFor(opts, "template1")
					default:
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "metadata",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.Title = "TITLE"
				opts.BodyProvided = true
				opts.Body = "BODY"
				opts.HeadBranch = "feature"
				opts.Assignees = []string{"monalisa"}
				opts.Labels = []string{"bug", "todo"}
				opts.Projects = []string{"roadmap"}
				opts.Reviewers = []string{"hubot", "monalisa", "/core", "/robots"}
				opts.Milestone = "big one.oh"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
					httpmock.StringResponse(`
					{ "data": {
						"u000": { "login": "MonaLisa", "id": "MONAID" },
						"u001": { "login": "hubot", "id": "HUBOTID" },
						"repository": {
							"l000": { "name": "bug", "id": "BUGID" },
							"l001": { "name": "TODO", "id": "TODOID" }
						},
						"organization": {
							"t000": { "slug": "core", "id": "COREID" },
							"t001": { "slug": "robots", "id": "ROBOTID" }
						}
					} }
					`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryMilestoneList\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "milestones": {
						"nodes": [
							{ "title": "GA", "id": "GAID" },
							{ "title": "Big One.oh", "id": "BIGONEID" }
						],
						"pageInfo": { "hasNextPage": false }
					} } } }
					`))
				mockRetrieveProjects(t, reg)
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "createPullRequest": { "pullRequest": {
						"id": "NEWPULLID",
						"URL": "https://github.com/OWNER/REPO/pull/12"
					} } } }
				`, func(inputs map[string]interface{}) {
						assert.Equal(t, "TITLE", inputs["title"])
						assert.Equal(t, "BODY", inputs["body"])
						if v, ok := inputs["assigneeIds"]; ok {
							t.Errorf("did not expect assigneeIds: %v", v)
						}
						if v, ok := inputs["userIds"]; ok {
							t.Errorf("did not expect userIds: %v", v)
						}
					}))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreateMetadata\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "updatePullRequest": {
						"clientMutationId": ""
					} } }
				`, func(inputs map[string]interface{}) {
						assert.Equal(t, "NEWPULLID", inputs["pullRequestId"])
						assert.Equal(t, []interface{}{"MONAID"}, inputs["assigneeIds"])
						assert.Equal(t, []interface{}{"BUGID", "TODOID"}, inputs["labelIds"])
						assert.Equal(t, []interface{}{"ROADMAPID"}, inputs["projectIds"])
						assert.Equal(t, "BIGONEID", inputs["milestoneId"])
					}))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreateRequestReviews\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "requestReviews": {
						"clientMutationId": ""
					} } }
				`, func(inputs map[string]interface{}) {
						assert.Equal(t, "NEWPULLID", inputs["pullRequestId"])
						assert.Equal(t, []interface{}{"HUBOTID", "MONAID"}, inputs["userIds"])
						assert.Equal(t, []interface{}{"COREID", "ROBOTID"}, inputs["teamIds"])
						assert.Equal(t, true, inputs["union"])
					}))
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "already exists",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "title"
				opts.Body = "body"
				opts.HeadBranch = "feature"
				opts.Finder = shared.NewMockFinder("feature", &api.PullRequest{URL: "https://github.com/OWNER/REPO/pull/123"}, ghrepo.New("OWNER", "REPO"))
				return func() {}
			},
			wantErr: "a pull request for branch \"feature\" into branch \"master\" already exists:\nhttps://github.com/OWNER/REPO/pull/123",
		},
		{
			name: "web",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.WebMode = true
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
				cs.Register("git rev-parse --symbolic-full-name feature@{push}", 0, "refs/remotes/origin/feature")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register(`git push --set-upstream origin HEAD:refs/heads/feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedErrOut: "Opening https://github.com/OWNER/REPO/compare/master...feature in your browser.\n",
			expectedBrowse: "https://github.com/OWNER/REPO/compare/master...feature?body=&expand=1",
		},
		{
			name: "web project",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.WebMode = true
				opts.Projects = []string{"Triage"}
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				mockRetrieveProjects(t, reg)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
				cs.Register("git rev-parse --symbolic-full-name feature@{push}", 0, "refs/remotes/origin/feature")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 1, "")
				cs.Register(`git push --set-upstream origin HEAD:refs/heads/feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedErrOut: "Opening https://github.com/OWNER/REPO/compare/master...feature in your browser.\n",
			expectedBrowse: "https://github.com/OWNER/REPO/compare/master...feature?body=&expand=1&projects=ORG%2F1",
		},
		{
			name: "draft",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.Title = "my title"
				opts.HeadBranch = "feature"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query PullRequestTemplates\b`),
					httpmock.StringResponse(`
				{ "data": { "repository": { "pullRequestTemplates": [
					{ "filename": "template1",
					  "body": "this is a bug" },
					{ "filename": "template2",
					  "body": "this is a enhancement" }
				] } } }`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
			{ "data": { "createPullRequest": { "pullRequest": {
				"URL": "https://github.com/OWNER/REPO/pull/12"
			} } } }
			`, func(input map[string]interface{}) {
						assert.Equal(t, true, input["draft"].(bool))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry origin/master...feature`, 0, "")
				cs.Register(`git rev-parse --show-toplevel`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.MarkdownEditorFunc = func(p, d string, ba bool) (string, error) {
					if p == "Body" {
						return d, nil
					} else {
						return "", prompter.NoSuchPromptErr(p)
					}
				}
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					switch p {
					case "What's next?":
						return prompter.IndexFor(opts, "Submit as draft")
					case "Choose a template":
						return 0, nil
					default:
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "recover",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
					httpmock.StringResponse(`
			{ "data": {
				"u000": { "login": "jillValentine", "id": "JILLID" },
				"repository": {},
				"organization": {}
			} }
			`))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreateRequestReviews\b`),
					httpmock.GraphQLMutation(`
			{ "data": { "requestReviews": {
				"clientMutationId": ""
			} } }
		`, func(inputs map[string]interface{}) {
						assert.Equal(t, []interface{}{"JILLID"}, inputs["userIds"])
					}))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
			{ "data": { "createPullRequest": { "pullRequest": {
				"URL": "https://github.com/OWNER/REPO/pull/12"
			} } } }
			`, func(input map[string]interface{}) {
						assert.Equal(t, "recovered title", input["title"].(string))
						assert.Equal(t, "recovered body", input["body"].(string))
					}))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.InputFunc = func(p, d string) (string, error) {
					if p == "Title (required)" {
						return d, nil
					} else {
						return "", prompter.NoSuchPromptErr(p)
					}
				}
				pm.MarkdownEditorFunc = func(p, d string, ba bool) (string, error) {
					if p == "Body" {
						return d, nil
					} else {
						return "", prompter.NoSuchPromptErr(p)
					}
				}
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "What's next?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			setup: func(opts *CreateOptions, t *testing.T) func() {
				tmpfile, err := os.CreateTemp(t.TempDir(), "testrecover*")
				assert.NoError(t, err)
				state := shared.IssueMetadataState{
					Title:     "recovered title",
					Body:      "recovered body",
					Reviewers: []string{"jillValentine"},
				}
				data, err := json.Marshal(state)
				assert.NoError(t, err)
				_, err = tmpfile.Write(data)
				assert.NoError(t, err)

				opts.RecoverFile = tmpfile.Name()
				opts.HeadBranch = "feature"
				return func() { tmpfile.Close() }
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "web long URL",
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
			},
			setup: func(opts *CreateOptions, t *testing.T) func() {
				longBody := make([]byte, 9216)
				opts.Body = string(longBody)
				opts.BodyProvided = true
				opts.WebMode = true
				opts.HeadBranch = "feature"
				return func() {}
			},
			wantErr: "cannot open in browser: maximum URL length exceeded",
		},
		{
			name: "single commit title and body are used",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.HeadBranch = "feature"
				return func() {}
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(
					"git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry origin/master...feature",
					0,
					"3a9b48085046d156c5acce8f3b3a0532cd706a4a\u0000first commit of pr\u0000first commit description\u0000\n",
				)
				cs.Register(`git rev-parse --show-toplevel`, 0, "")
			},
			promptStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "Where should we push the 'feature' branch?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}

				pm.InputFunc = func(p, d string) (string, error) {
					if p == "Title (required)" {
						return d, nil
					} else if p == "Body" {
						return d, nil
					} else {
						return "", prompter.NoSuchPromptErr(p)
					}
				}

				pm.MarkdownEditorFunc = func(p, d string, ba bool) (string, error) {
					if p == "Body" {
						return d, nil
					} else {
						return "", prompter.NoSuchPromptErr(p)
					}
				}

				pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
					if p == "What's next?" {
						return 0, nil
					} else {
						return -1, prompter.NoSuchPromptErr(p)
					}
				}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query PullRequestTemplates\b`),
					httpmock.StringResponse(`{ "data": { "repository": { "pullRequestTemplates": [] } } }`),
				)
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{
						"data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
							} } }
						}
						`,
						func(input map[string]interface{}) {
							assert.Equal(t, "first commit of pr", input["title"], "pr title should be first commit message")
							assert.Equal(t, "first commit description", input["body"], "pr body should be first commit description")
						},
					),
				)
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "fill-first flag provided",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.FillFirst = true
				opts.HeadBranch = "feature"
				return func() {}
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(
					"git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry origin/master...feature",
					0,
					"56b6f8bb7c9e3a30093cd17e48934ce354148e80\u0000second commit of pr\u0000\u0000\n"+
						"3a9b48085046d156c5acce8f3b3a0532cd706a4a\u0000first commit of pr\u0000first commit description\u0000\n",
				)
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{
						"data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
							} } }
						}
						`,
						func(input map[string]interface{}) {
							assert.Equal(t, "first commit of pr", input["title"], "pr title should be first commit message")
							assert.Equal(t, "first commit description", input["body"], "pr body should be first commit description")
						},
					),
				)
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "fillverbose flag provided",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.FillVerbose = true
				opts.HeadBranch = "feature"
				return func() {}
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(
					"git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry origin/master...feature",
					0,
					"56b6f8bb7c9e3a30093cd17e48934ce354148e80\u0000second commit of pr\u0000second commit description\u0000\n"+
						"3a9b48085046d156c5acce8f3b3a0532cd706a4a\u0000first commit of pr\u0000first commit with super long description, with super long description, with super long description, with super long description.\u0000\n",
				)
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{
						"data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
							} } }
						}
						`,
						func(input map[string]interface{}) {
							assert.Equal(t, "feature", input["title"], "pr title should be branch name")
							assert.Equal(t, "- **first commit of pr**\n  first commit with super long description, with super long description, with super long description, with super long description.\n\n- **second commit of pr**\n  second commit description", input["body"], "pr body should be commits msg+body")
						},
					),
				)
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for feature into master in OWNER/REPO\n\n",
		},
		{
			name: "editor",
			httpStubs: func(r *httpmock.Registry, t *testing.T) {
				r.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{
						"data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
							} } }
						}
				`, func(inputs map[string]interface{}) {
						assert.Equal(t, "title", inputs["title"])
						assert.Equal(t, "body", inputs["body"])
					}))
			},
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.EditorMode = true
				opts.HeadBranch = "feature"
				opts.TitledEditSurvey = func(string, string) (string, string, error) { return "title", "body", nil }
				return func() {}
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register("git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry origin/master...feature", 0, "")
			},
			expectedOut: "https://github.com/OWNER/REPO/pull/12\n",
		},
		{
			name: "gh-merge-base",
			tty:  true,
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				opts.Branch = func() (string, error) {
					return "task1", nil
				}
				opts.Remotes = func() (context.Remotes, error) {
					return context.Remotes{
						{
							Remote: &git.Remote{
								Name:     "upstream",
								Resolved: "base",
							},
							Repo: ghrepo.New("OWNER", "REPO"),
						},
						{
							Remote: &git.Remote{
								Name: "origin",
							},
							Repo: ghrepo.New("monalisa", "REPO"),
						},
					}, nil
				}
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
							{ "data": { "createPullRequest": { "pullRequest": {
								"URL": "https://github.com/OWNER/REPO/pull/12"
							} } } }
							`, func(input map[string]interface{}) {
						assert.Equal(t, "REPOID", input["repositoryId"].(string))
						assert.Equal(t, "my title", input["title"].(string))
						assert.Equal(t, "my body", input["body"].(string))
						assert.Equal(t, "feature/feat2", input["baseRefName"].(string))
						assert.Equal(t, "monalisa:task1", input["headRefName"].(string))
					}))
			},
			customBranchConfig: true,
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git config --get-regexp \^branch\\\.task1\\\.\(remote\|merge\|pushremote\|gh-merge-base\)\$`, 0, heredoc.Doc(`
					branch.task1.remote origin
					branch.task1.merge refs/heads/task1
					branch.task1.gh-merge-base feature/feat2`)) // ReadBranchConfig
				cs.Register("git rev-parse --symbolic-full-name task1@{push}", 0, "refs/remotes/origin/task1")
				cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/task1`, 0, heredoc.Doc(`
					deadbeef HEAD
					deadbeef refs/remotes/origin/task1`))
			},
			expectedOut:    "https://github.com/OWNER/REPO/pull/12\n",
			expectedErrOut: "\nCreating pull request for monalisa:task1 into feature/feat2 in OWNER/REPO\n\n",
		},
		{
			name: "--head contains <user>:<branch> syntax",
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestCreate\b`),
					httpmock.GraphQLMutation(`
						{ "data": { "createPullRequest": { "pullRequest": {
							"URL": "https://github.com/OWNER/REPO/pull/12"
						} } } }`,
						func(input map[string]interface{}) {
							assert.Equal(t, "REPOID", input["repositoryId"])
							assert.Equal(t, "my title", input["title"])
							assert.Equal(t, "my body", input["body"])
							assert.Equal(t, "master", input["baseRefName"])
							assert.Equal(t, "otherowner:feature", input["headRefName"])
						}))
			},
			setup: func(opts *CreateOptions, t *testing.T) func() {
				opts.TitleProvided = true
				opts.BodyProvided = true
				opts.Title = "my title"
				opts.Body = "my body"
				opts.HeadBranch = "otherowner:feature"
				return func() {}
			},
			expectedOut: "https://github.com/OWNER/REPO/pull/12\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branch := "feature"
			reg := &httpmock.Registry{}
			reg.StubRepoInfoResponse("OWNER", "REPO", "master")
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg, t)
			}

			pm := &prompter.PrompterMock{}

			if tt.promptStubs != nil {
				tt.promptStubs(pm)
			}

			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			if !tt.customBranchConfig {
				cs.Register(`git config --get-regexp \^branch\\\..+\\\.\(remote\|merge\|pushremote\|gh-merge-base\)\$`, 0, "")
			}

			if tt.cmdStubs != nil {
				tt.cmdStubs(cs)
			}

			opts := CreateOptions{}
			opts.Detector = &fd.EnabledDetectorMock{}
			opts.Prompter = pm

			ios, _, stdout, stderr := iostreams.Test()
			// TODO do i need to bother with this
			ios.SetStdoutTTY(tt.tty)
			ios.SetStdinTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			browser := &browser.Stub{}
			opts.IO = ios
			opts.Browser = browser
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}
			opts.Remotes = func() (context.Remotes, error) {
				return context.Remotes{
					{
						Remote: &git.Remote{
							Name:     "origin",
							Resolved: "base",
						},
						Repo: ghrepo.New("OWNER", "REPO"),
					},
				}, nil
			}
			opts.Branch = func() (string, error) {
				return branch, nil
			}
			opts.Finder = shared.NewMockFinder(branch, nil, nil)
			opts.GitClient = &git.Client{
				GhPath:  "some/path/gh",
				GitPath: "some/path/git",
			}
			cleanSetup := func() {}
			if tt.setup != nil {
				cleanSetup = tt.setup(&opts, t)
			}
			defer cleanSetup()

			if opts.HeadBranch == "" {
				cs.Register(`git status --porcelain`, 0, "")
			}

			err := createRun(&opts)
			output := &test.CmdOut{
				OutBuf:     stdout,
				ErrBuf:     stderr,
				BrowsedURL: browser.BrowsedURL(),
			}
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				if tt.expectedOut != "" {
					assert.Equal(t, tt.expectedOut, output.String())
				}
				if len(tt.expectedOutputs) > 0 {
					assert.Equal(t, tt.expectedOutputs, strings.Split(output.String(), "\n"))
				}
				assert.Equal(t, tt.expectedErrOut, output.Stderr())
				assert.Equal(t, tt.expectedBrowse, output.BrowsedURL)
			}
		})
	}
}

func TestRemoteGuessing(t *testing.T) {
	// Given git config does not provide the necessary info to determine a remote
	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git status --porcelain`, 0, "")
	cs.Register(`git config --get-regexp \^branch\\\..+\\\.\(remote\|merge\|pushremote\|gh-merge-base\)\$`, 0, "")
	cs.Register(`git rev-parse --symbolic-full-name feature@{push}`, 1, "")
	cs.Register("git config remote.pushDefault", 1, "")
	cs.Register("git config push.default", 1, "")

	// And Given there is a remote on a SHA that matches the current HEAD
	cs.Register(`git show-ref --verify -- HEAD refs/remotes/upstream/feature refs/remotes/origin/feature`, 0, heredoc.Doc(`
	deadbeef HEAD
	deadb00f refs/remotes/upstream/feature
	deadbeef refs/remotes/origin/feature`))

	// When the command is run
	reg := &httpmock.Registry{}
	reg.StubRepoInfoResponse("OWNER", "REPO", "master")
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`mutation PullRequestCreate\b`),
		httpmock.GraphQLMutation(`
				{ "data": { "createPullRequest": { "pullRequest": {
					"URL": "https://github.com/OWNER/REPO/pull/12"
				} } } }`, func(input map[string]interface{}) {
			assert.Equal(t, "REPOID", input["repositoryId"].(string))
			assert.Equal(t, "master", input["baseRefName"].(string))
			assert.Equal(t, "OTHEROWNER:feature", input["headRefName"].(string))
		}))

	ios, _, _, _ := iostreams.Test()

	opts := CreateOptions{
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		Browser:  &browser.Stub{},
		IO:       ios,
		Prompter: &prompter.PrompterMock{},
		GitClient: &git.Client{
			GhPath:  "some/path/gh",
			GitPath: "some/path/git",
		},
		Finder: shared.NewMockFinder("feature", nil, nil),
		Remotes: func() (context.Remotes, error) {
			return context.Remotes{
				{
					Remote: &git.Remote{
						Name:     "upstream",
						Resolved: "base",
					},
					Repo: ghrepo.New("OWNER", "REPO"),
				},
				{
					Remote: &git.Remote{
						Name: "origin",
					},
					Repo: ghrepo.New("OTHEROWNER", "REPO-FORK"),
				},
			}, nil
		},
		Branch: func() (string, error) {
			return "feature", nil
		},

		TitleProvided: true,
		BodyProvided:  true,
		Title:         "my title",
		Body:          "my body",
	}

	require.NoError(t, createRun(&opts))

	// Then guessed remote is used for the PR head,
	// which annoyingly, is asserted above on the line:
	// assert.Equal(t, "OTHEROWNER:feature", input["headRefName"].(string))
	//
	// This is because OTHEROWNER relates to the "origin" remote, which has a
	// SHA that matches the HEAD ref in the `git show-ref` output.
}

func TestNoRepoCanBeDetermined(t *testing.T) {
	// Given no head repo can be determined from git config
	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git status --porcelain`, 0, "")
	cs.Register(`git config --get-regexp \^branch\\\..+\\\.\(remote\|merge\|pushremote\|gh-merge-base\)\$`, 0, "")
	cs.Register(`git rev-parse --symbolic-full-name feature@{push}`, 1, "")
	cs.Register("git config remote.pushDefault", 1, "")
	cs.Register("git config push.default", 1, "")

	// And Given there is no remote on the correct SHA
	cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, heredoc.Doc(`
	deadbeef HEAD
	deadb00f refs/remotes/origin/feature`))

	// When the command is run with no TTY
	reg := &httpmock.Registry{}
	reg.StubRepoInfoResponse("OWNER", "REPO", "master")
	defer reg.Verify(t)

	ios, _, _, stderr := iostreams.Test()

	opts := CreateOptions{
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		Browser:  &browser.Stub{},
		IO:       ios,
		Prompter: &prompter.PrompterMock{},
		GitClient: &git.Client{
			GhPath:  "some/path/gh",
			GitPath: "some/path/git",
		},
		Finder: shared.NewMockFinder("feature", nil, nil),
		Remotes: func() (context.Remotes, error) {
			return context.Remotes{
				{
					Remote: &git.Remote{
						Name:     "origin",
						Resolved: "base",
					},
					Repo: ghrepo.New("OWNER", "REPO"),
				},
			}, nil
		},
		Branch: func() (string, error) {
			return "feature", nil
		},

		TitleProvided: true,
		BodyProvided:  true,
		Title:         "my title",
		Body:          "my body",
	}

	// When we run the command
	err := createRun(&opts)

	// Then create fails
	require.Equal(t, cmdutil.SilentError, err)
	assert.Equal(t, "aborted: you must first push the current branch to a remote, or use the --head flag\n", stderr.String())
}

func mustParseQualifiedHeadRef(ref string) shared.QualifiedHeadRef {
	parsed, err := shared.ParseQualifiedHeadRef(ref)
	if err != nil {
		panic(err)
	}
	return parsed
}

func Test_generateCompareURL(t *testing.T) {
	tests := []struct {
		name              string
		ctx               CreateContext
		state             shared.IssueMetadataState
		httpStubs         func(*testing.T, *httpmock.Registry)
		projectsV1Support gh.ProjectsV1Support
		want              string
		wantErr           bool
	}{
		{
			name: "basic",
			ctx: CreateContext{
				PRRefs: &skipPushRefs{
					qualifiedHeadRef: shared.NewQualifiedHeadRefWithoutOwner("feature"),
					baseRefs: baseRefs{
						baseRepo:       api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
						baseBranchName: "main",
					},
				},
			},
			want:    "https://github.com/OWNER/REPO/compare/main...feature?body=&expand=1",
			wantErr: false,
		},
		{
			name: "with labels",
			ctx: CreateContext{
				PRRefs: &skipPushRefs{
					qualifiedHeadRef: shared.NewQualifiedHeadRefWithoutOwner("b"),
					baseRefs: baseRefs{
						baseRepo:       api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
						baseBranchName: "a",
					},
				},
			},
			state: shared.IssueMetadataState{
				Labels: []string{"one", "two three"},
			},
			want:    "https://github.com/OWNER/REPO/compare/a...b?body=&expand=1&labels=one%2Ctwo+three",
			wantErr: false,
		},
		{
			name: "'/'s in branch names/labels are percent-encoded",
			ctx: CreateContext{
				PRRefs: &skipPushRefs{
					qualifiedHeadRef: mustParseQualifiedHeadRef("ORIGINOWNER:feature"),
					baseRefs: baseRefs{
						baseRepo:       api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "UPSTREAMOWNER"}}, "github.com"),
						baseBranchName: "main/trunk",
					},
				},
			},
			want:    "https://github.com/UPSTREAMOWNER/REPO/compare/main%2Ftrunk...ORIGINOWNER:feature?body=&expand=1",
			wantErr: false,
		},
		{
			name: "Any of !'(),; but none of $&+=@ and : in branch names/labels are percent-encoded ",
			/*
				- Technically, per section 3.3 of RFC 3986, none of !$&'()*+,;= (sub-delims) and :[]@ (part of gen-delims) in path segments are optionally percent-encoded, but url.PathEscape percent-encodes !'(),; anyway
				- !$&'()+,;=@ is a valid Git branch name—essentially RFC 3986 sub-delims without * and gen-delims without :/?#[]
				- : is GitHub separator between a fork name and a branch name
				- See https://github.com/golang/go/issues/27559.
			*/
			ctx: CreateContext{
				PRRefs: &skipPushRefs{
					qualifiedHeadRef: mustParseQualifiedHeadRef("ORIGINOWNER:!$&'()+,;=@"),
					baseRefs: baseRefs{
						baseRepo:       api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "UPSTREAMOWNER"}}, "github.com"),
						baseBranchName: "main/trunk",
					},
				},
			},
			want:    "https://github.com/UPSTREAMOWNER/REPO/compare/main%2Ftrunk...ORIGINOWNER:%21$&%27%28%29+%2C%3B=@?body=&expand=1",
			wantErr: false,
		},
		{
			name: "with template",
			ctx: CreateContext{
				PRRefs: &skipPushRefs{
					qualifiedHeadRef: shared.NewQualifiedHeadRefWithoutOwner("feature"),
					baseRefs: baseRefs{
						baseRepo:       api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
						baseBranchName: "main",
					},
				},
			},
			state: shared.IssueMetadataState{
				Template: "story.md",
			},
			want:    "https://github.com/OWNER/REPO/compare/main...feature?body=&expand=1&template=story.md",
			wantErr: false,
		},
		// TODO projectsV1Deprecation
		// Clean up these tests, but probably keep one for general project ID resolution.
		{
			name: "with projects, no v1 support",
			ctx: CreateContext{
				PRRefs: &skipPushRefs{
					qualifiedHeadRef: shared.NewQualifiedHeadRefWithoutOwner("feature"),
					baseRefs: baseRefs{
						baseRepo:       api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
						baseBranchName: "main",
					},
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Ensure no v1 projects are requestd
				// ( is required to avoid matching projectsV2
				reg.Exclude(t, httpmock.GraphQL(`projects\(`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryProjectV2List\b`),
					httpmock.StringResponse(`
							{ "data": { "repository": { "projectsV2": {
								"nodes": [
									{ "title": "ProjectTitle", "id": "PROJECTV2ID", "resourcePath": "/OWNER/REPO/projects/3" }
								],
								"pageInfo": { "hasNextPage": false }
							} } } }
							`))
				reg.Register(
					httpmock.GraphQL(`query OrganizationProjectV2List\b`),
					httpmock.StringResponse(`
							{ "data": { "organization": { "projectsV2": {
								"nodes": [],
								"pageInfo": { "hasNextPage": false }
							} } } }
							`))
				reg.Register(
					httpmock.GraphQL(`query UserProjectV2List\b`),
					httpmock.StringResponse(`
							{ "data": { "viewer": { "projectsV2": {
								"nodes": [],
								"pageInfo": { "hasNextPage": false }
							} } } }
							`))
			},
			state: shared.IssueMetadataState{
				ProjectTitles: []string{"ProjectTitle"},
			},
			projectsV1Support: gh.ProjectsV1Unsupported,
			want:              "https://github.com/OWNER/REPO/compare/main...feature?body=&expand=1&projects=OWNER%2FREPO%2F3",
			wantErr:           false,
		},
		{
			name: "with projects, v1 support",
			ctx: CreateContext{
				PRRefs: &skipPushRefs{
					qualifiedHeadRef: shared.NewQualifiedHeadRefWithoutOwner("feature"),
					baseRefs: baseRefs{
						baseRepo:       api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
						baseBranchName: "main",
					},
				},
			},
			state: shared.IssueMetadataState{
				ProjectTitles: []string{"ProjectV1Title"},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// v1 project query responses
				reg.Register(
					httpmock.GraphQL(`query RepositoryProjectList\b`),
					httpmock.StringResponse(`
							{ "data": { "repository": { "projects": {
								"nodes": [
									{ "name": "ProjectV1Title", "id": "PROJECTV1ID", "resourcePath": "/OWNER/REPO/projects/1" }
								],
								"pageInfo": { "hasNextPage": false }
							} } } }
							`))
				reg.Register(
					httpmock.GraphQL(`query OrganizationProjectList\b`),
					httpmock.StringResponse(`
										{ "data": { "organization": { "projects": {
											"nodes": [],
											"pageInfo": { "hasNextPage": false }
										} } } }
										`))
				// v2 project query responses
				reg.Register(
					httpmock.GraphQL(`query RepositoryProjectV2List\b`),
					httpmock.StringResponse(`
							{ "data": { "repository": { "projectsV2": {
								"nodes": [],
								"pageInfo": { "hasNextPage": false }
							} } } }
							`))
				reg.Register(
					httpmock.GraphQL(`query OrganizationProjectV2List\b`),
					httpmock.StringResponse(`
							{ "data": { "organization": { "projectsV2": {
								"nodes": [],
								"pageInfo": { "hasNextPage": false }
							} } } }
							`))
				reg.Register(
					httpmock.GraphQL(`query UserProjectV2List\b`),
					httpmock.StringResponse(`
							{ "data": { "viewer": { "projectsV2": {
								"nodes": [],
								"pageInfo": { "hasNextPage": false }
							} } } }
							`))
			},
			projectsV1Support: gh.ProjectsV1Supported,
			want:              "https://github.com/OWNER/REPO/compare/main...feature?body=&expand=1&projects=OWNER%2FREPO%2F1",
			wantErr:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// If http stubs are provided, register them and inject the registry into a client
			// that is provided to generateCompareURL in the ctx.
			if tt.httpStubs != nil {
				reg := &httpmock.Registry{}
				defer reg.Verify(t)

				tt.httpStubs(t, reg)
				tt.ctx.Client = api.NewClientFromHTTP(&http.Client{Transport: reg})
			}

			got, err := generateCompareURL(tt.ctx, tt.state, tt.projectsV1Support)
			if (err != nil) != tt.wantErr {
				t.Errorf("generateCompareURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("generateCompareURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func mockRetrieveProjects(_ *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`query RepositoryProjectList\b`),
		httpmock.StringResponse(`
				{ "data": { "repository": { "projects": {
					"nodes": [
						{ "name": "Cleanup", "id": "CLEANUPID", "resourcePath": "/OWNER/REPO/projects/1" },
						{ "name": "Roadmap", "id": "ROADMAPID", "resourcePath": "/OWNER/REPO/projects/2" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
	reg.Register(
		httpmock.GraphQL(`query RepositoryProjectV2List\b`),
		httpmock.StringResponse(`
				{ "data": { "repository": { "projectsV2": {
					"nodes": [
						{ "title": "CleanupV2", "id": "CLEANUPV2ID", "resourcePath": "/OWNER/REPO/projects/3" },
						{ "title": "RoadmapV2", "id": "ROADMAPV2ID", "resourcePath": "/OWNER/REPO/projects/4" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
	reg.Register(
		httpmock.GraphQL(`query OrganizationProjectList\b`),
		httpmock.StringResponse(`
				{ "data": { "organization": { "projects": {
					"nodes": [
						{ "name": "Triage", "id": "TRIAGEID", "resourcePath": "/orgs/ORG/projects/1" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
	reg.Register(
		httpmock.GraphQL(`query OrganizationProjectV2List\b`),
		httpmock.StringResponse(`
				{ "data": { "organization": { "projectsV2": {
					"nodes": [
						{ "title": "TriageV2", "id": "TRIAGEV2ID", "resourcePath": "/orgs/ORG/projects/2" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
	reg.Register(
		httpmock.GraphQL(`query UserProjectV2List\b`),
		httpmock.StringResponse(`
				{ "data": { "viewer": { "projectsV2": {
					"nodes": [
						{ "title": "MonalisaV2", "id": "MONALISAV2ID", "resourcePath": "/user/MONALISA/projects/2" }
					],
					"pageInfo": { "hasNextPage": false }
				} } } }
				`))
}

// TODO projectsV1Deprecation
// Remove this test.
func TestProjectsV1Deprecation(t *testing.T) {

	t.Run("non-interactive submission", func(t *testing.T) {
		t.Run("when projects v1 is supported, queries for it", func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()

			reg := &httpmock.Registry{}
			reg.StubRepoInfoResponse("OWNER", "REPO", "main")
			reg.Register(
				// ( is required to avoid matching projectsV2
				httpmock.GraphQL(`projects\(`),
				// Simulate a GraphQL error to early exit the test.
				httpmock.StatusStringResponse(500, ""),
			)

			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			cs.Register(`git config --get-regexp \^branch\\\..+\\\.\(remote\|merge\|pushremote\|gh-merge-base\)\$`, 0, "")

			// Ignore the error because we have no way to really stub it without
			// fully stubbing a GQL error structure in the request body.
			_ = createRun(&CreateOptions{
				Detector: &fd.EnabledDetectorMock{},
				IO:       ios,
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				GitClient: &git.Client{
					GhPath:  "some/path/gh",
					GitPath: "some/path/git",
				},
				Remotes: func() (context.Remotes, error) {
					return context.Remotes{
						{
							Remote: &git.Remote{
								Name:     "upstream",
								Resolved: "base",
							},
							Repo: ghrepo.New("OWNER", "REPO"),
						},
					}, nil
				},
				Finder: shared.NewMockFinder("feature", nil, nil),

				HeadBranch: "feature",

				TitleProvided: true,
				BodyProvided:  true,
				Title:         "Test Title",
				Body:          "Test Body",

				// Required to force a lookup of projects
				Projects: []string{"Project"},
			})

			// Verify that our request contained projects
			reg.Verify(t)
		})

		t.Run("when projects v1 is not supported, does not query for it", func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()

			reg := &httpmock.Registry{}
			reg.StubRepoInfoResponse("OWNER", "REPO", "main")
			// ( is required to avoid matching projectsV2
			reg.Exclude(t, httpmock.GraphQL(`projects\(`))

			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			cs.Register(`git config --get-regexp \^branch\\\..+\\\.\(remote\|merge\|pushremote\|gh-merge-base\)\$`, 0, "")

			// Ignore the error because we're not really interested in it.
			_ = createRun(&CreateOptions{
				Detector: &fd.DisabledDetectorMock{},
				IO:       ios,
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				GitClient: &git.Client{
					GhPath:  "some/path/gh",
					GitPath: "some/path/git",
				},
				Remotes: func() (context.Remotes, error) {
					return context.Remotes{
						{
							Remote: &git.Remote{
								Name:     "upstream",
								Resolved: "base",
							},
							Repo: ghrepo.New("OWNER", "REPO"),
						},
					}, nil
				},
				Finder: shared.NewMockFinder("feature", nil, nil),

				HeadBranch: "feature",

				TitleProvided: true,
				BodyProvided:  true,
				Title:         "Test Title",
				Body:          "Test Body",

				// Required to force a lookup of projects
				Projects: []string{"Project"},
			})

			// Verify that our request contained projectCards
			reg.Verify(t)
		})
	})

	t.Run("interactive submission", func(t *testing.T) {
		t.Run("when projects v1 is supported, queries for it", func(t *testing.T) {
			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			cs.Register(`git config --get-regexp \^branch\\\..+\\\.\(remote\|merge\|pushremote\|gh-merge-base\)\$`, 0, "")
			cs.Register("git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry origin/master...feature", 0, "")
			cs.Register(`git rev-parse --show-toplevel`, 0, "")

			// When the command is run
			reg := &httpmock.Registry{}
			reg.StubRepoResponse("OWNER", "REPO")

			reg.Register(
				httpmock.GraphQL(`query PullRequestTemplates\b`),
				httpmock.StringResponse(`{ "data": { "repository": { "pullRequestTemplates": [] } } }`),
			)

			reg.Register(
				// ( is required to avoid matching projectsV2
				httpmock.GraphQL(`projects\(`),
				// Simulate a GraphQL error to early exit the test.
				httpmock.StatusStringResponse(500, ""),
			)

			// Register a handler to check for projects V2 just to avoid the registry panicking, even
			// though we return a 500 error. This is because the project lookup is done in parallel
			// so the previous error doesn't early exit.
			reg.Register(
				httpmock.GraphQL(`projectsV2`),
				// Simulate a GraphQL error to early exit the test.
				httpmock.StatusStringResponse(500, ""),
			)

			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			ios.SetStderrTTY(true)

			pm := &prompter.PrompterMock{}
			pm.InputFunc = func(p, _ string) (string, error) {
				if p == "Title (required)" {
					return "Test Title", nil
				} else {
					return "", prompter.NoSuchPromptErr(p)
				}
			}
			pm.MarkdownEditorFunc = func(p, _ string, ba bool) (string, error) {
				if p == "Body" {
					return "Test Body", nil
				} else {
					return "", prompter.NoSuchPromptErr(p)
				}
			}
			pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
				switch p {
				case "Choose a template":
					return 0, nil
				case "What's next?":
					return prompter.IndexFor(opts, "Add metadata")
				default:
					return -1, prompter.NoSuchPromptErr(p)
				}
			}
			pm.MultiSelectFunc = func(p string, _ []string, opts []string) ([]int, error) {
				return prompter.IndexesFor(opts, "Projects")
			}

			opts := CreateOptions{
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				Config: func() (gh.Config, error) {
					return config.NewBlankConfig(), nil
				},
				Browser:  &browser.Stub{},
				IO:       ios,
				Prompter: pm,
				GitClient: &git.Client{
					GhPath:  "some/path/gh",
					GitPath: "some/path/git",
				},
				Finder:   shared.NewMockFinder("feature", nil, nil),
				Detector: &fd.EnabledDetectorMock{},
				Remotes: func() (context.Remotes, error) {
					return context.Remotes{
						{
							Remote: &git.Remote{
								Name: "origin",
							},
							Repo: ghrepo.New("OWNER", "REPO"),
						},
					}, nil
				},
				Branch: func() (string, error) {
					return "feature", nil
				},

				HeadBranch: "feature",
			}

			// Ignore the error because we have no way to really stub it without
			// fully stubbing a GQL error structure in the request body.
			_ = createRun(&opts)

			// Verify that our request contained projects
			reg.Verify(t)
		})

		t.Run("when projects v1 is not supported, does not query for it", func(t *testing.T) {
			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			cs.Register(`git config --get-regexp \^branch\\\..+\\\.\(remote\|merge\|pushremote\|gh-merge-base\)\$`, 0, "")
			cs.Register("git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry origin/master...feature", 0, "")
			cs.Register(`git rev-parse --show-toplevel`, 0, "")

			// When the command is run
			reg := &httpmock.Registry{}
			reg.StubRepoResponse("OWNER", "REPO")

			reg.Register(
				httpmock.GraphQL(`query PullRequestTemplates\b`),
				httpmock.StringResponse(`{ "data": { "repository": { "pullRequestTemplates": [] } } }`),
			)

			// ( is required to avoid matching projectsV2
			reg.Exclude(t, httpmock.GraphQL(`projects\(`))

			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			ios.SetStderrTTY(true)

			pm := &prompter.PrompterMock{}
			pm.InputFunc = func(p, _ string) (string, error) {
				if p == "Title (required)" {
					return "Test Title", nil
				} else {
					return "", prompter.NoSuchPromptErr(p)
				}
			}
			pm.MarkdownEditorFunc = func(p, _ string, ba bool) (string, error) {
				if p == "Body" {
					return "Test Body", nil
				} else {
					return "", prompter.NoSuchPromptErr(p)
				}
			}
			pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
				switch p {
				case "Choose a template":
					return 0, nil
				case "What's next?":
					return prompter.IndexFor(opts, "Add metadata")
				default:
					return -1, prompter.NoSuchPromptErr(p)
				}
			}
			pm.MultiSelectFunc = func(p string, _ []string, opts []string) ([]int, error) {
				return prompter.IndexesFor(opts, "Projects")
			}

			opts := CreateOptions{
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				Config: func() (gh.Config, error) {
					return config.NewBlankConfig(), nil
				},
				Browser:  &browser.Stub{},
				IO:       ios,
				Prompter: pm,
				GitClient: &git.Client{
					GhPath:  "some/path/gh",
					GitPath: "some/path/git",
				},
				Finder:   shared.NewMockFinder("feature", nil, nil),
				Detector: &fd.DisabledDetectorMock{},
				Remotes: func() (context.Remotes, error) {
					return context.Remotes{
						{
							Remote: &git.Remote{
								Name: "origin",
							},
							Repo: ghrepo.New("OWNER", "REPO"),
						},
					}, nil
				},
				Branch: func() (string, error) {
					return "feature", nil
				},

				HeadBranch: "feature",
			}

			// Ignore the error because we have no way to really stub it without
			// fully stubbing a GQL error structure in the request body.
			_ = createRun(&opts)

			// Verify that our request did not contain projectCards
			reg.Verify(t)
		})
	})

	t.Run("web mode", func(t *testing.T) {
		t.Run("when projects v1 is supported, queries for it", func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()

			reg := &httpmock.Registry{}
			reg.StubRepoInfoResponse("OWNER", "REPO", "main")
			reg.Register(
				// ( is required to avoid matching projectsV2
				httpmock.GraphQL(`projects\(`),
				// Simulate a GraphQL error to early exit the test.
				httpmock.StatusStringResponse(500, ""),
			)

			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			cs.Register(`git config --get-regexp \^branch\\\..+\\\.\(remote\|merge\|pushremote\|gh-merge-base\)\$`, 0, "")

			// Ignore the error because we have no way to really stub it without
			// fully stubbing a GQL error structure in the request body.
			_ = createRun(&CreateOptions{
				Detector: &fd.EnabledDetectorMock{},
				IO:       ios,
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				GitClient: &git.Client{
					GhPath:  "some/path/gh",
					GitPath: "some/path/git",
				},
				Remotes: func() (context.Remotes, error) {
					return context.Remotes{
						{
							Remote: &git.Remote{
								Name:     "upstream",
								Resolved: "base",
							},
							Repo: ghrepo.New("OWNER", "REPO"),
						},
					}, nil
				},
				Finder: shared.NewMockFinder("feature", nil, nil),

				WebMode: true,

				HeadBranch: "feature",

				TitleProvided: true,
				BodyProvided:  true,
				Title:         "Test Title",
				Body:          "Test Body",

				// Required to force a lookup of projects
				Projects: []string{"Project"},
			})

			// Verify that our request contained projects
			reg.Verify(t)
		})

		t.Run("when projects v1 is not supported, does not query for it", func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()

			reg := &httpmock.Registry{}
			reg.StubRepoInfoResponse("OWNER", "REPO", "main")
			// ( is required to avoid matching projectsV2
			reg.Exclude(t, httpmock.GraphQL(`projects\(`))

			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			cs.Register(`git config --get-regexp \^branch\\\..+\\\.\(remote\|merge\|pushremote\|gh-merge-base\)\$`, 0, "")

			// Ignore the error because we're not really interested in it.
			_ = createRun(&CreateOptions{
				Detector: &fd.DisabledDetectorMock{},
				IO:       ios,
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				GitClient: &git.Client{
					GhPath:  "some/path/gh",
					GitPath: "some/path/git",
				},
				Remotes: func() (context.Remotes, error) {
					return context.Remotes{
						{
							Remote: &git.Remote{
								Name:     "upstream",
								Resolved: "base",
							},
							Repo: ghrepo.New("OWNER", "REPO"),
						},
					}, nil
				},
				Finder: shared.NewMockFinder("feature", nil, nil),

				WebMode: true,

				HeadBranch: "feature",

				TitleProvided: true,
				BodyProvided:  true,
				Title:         "Test Title",
				Body:          "Test Body",

				// Required to force a lookup of projects
				Projects: []string{"Project"},
			})

			// Verify that our request did not contain projectCards
			reg.Verify(t)
		})
	})
}
