package comment

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdComment(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := os.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name     string
		input    string
		stdin    string
		output   shared.CommentableOptions
		wantsErr bool
		isTTY    bool
	}{
		{
			name:     "no arguments",
			input:    "",
			output:   shared.CommentableOptions{},
			isTTY:    true,
			wantsErr: true,
		},
		{
			name:  "issue number",
			input: "1",
			output: shared.CommentableOptions{
				Interactive: true,
				InputType:   0,
				Body:        "",
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:  "issue url",
			input: "https://github.com/OWNER/REPO/issues/12",
			output: shared.CommentableOptions{
				Interactive: true,
				InputType:   0,
				Body:        "",
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:  "body flag",
			input: "1 --body test",
			output: shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeInline,
				Body:        "test",
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:  "body from stdin",
			input: "1 --body-file -",
			stdin: "this is on standard input",
			output: shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeInline,
				Body:        "this is on standard input",
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:  "body from file",
			input: fmt.Sprintf("1 --body-file '%s'", tmpFile),
			output: shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeInline,
				Body:        "a body from file",
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:  "editor flag",
			input: "1 --editor",
			output: shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeEditor,
				Body:        "",
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:  "web flag",
			input: "1 --web",
			output: shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeWeb,
				Body:        "",
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:  "edit last flag",
			input: "1 --edit-last",
			output: shared.CommentableOptions{
				Interactive: true,
				InputType:   shared.InputTypeEditor,
				Body:        "",
				EditLast:    true,
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:  "edit last flag with create if none",
			input: "1 --edit-last --create-if-none",
			output: shared.CommentableOptions{
				Interactive:  true,
				InputType:    shared.InputTypeEditor,
				Body:         "",
				EditLast:     true,
				CreateIfNone: true,
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:     "delete last flag non-interactive",
			input:    "1 --delete-last",
			isTTY:    false,
			wantsErr: true,
		},
		{
			name:  "delete last flag and pre-confirmation non-interactive",
			input: "1 --delete-last --yes",
			output: shared.CommentableOptions{
				DeleteLast:          true,
				DeleteLastConfirmed: true,
			},
			isTTY:    false,
			wantsErr: false,
		},
		{
			name:  "delete last flag interactive",
			input: "1 --delete-last",
			output: shared.CommentableOptions{
				Interactive: true,
				DeleteLast:  true,
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:  "delete last flag and pre-confirmation interactive",
			input: "1 --delete-last --yes",
			output: shared.CommentableOptions{
				Interactive:         true,
				DeleteLast:          true,
				DeleteLastConfirmed: true,
			},
			isTTY:    true,
			wantsErr: false,
		},
		{
			name:     "delete last flag and pre-confirmation with web flag",
			input:    "1 --delete-last --yes --web",
			isTTY:    true,
			wantsErr: true,
		},
		{
			name:     "delete last flag and pre-confirmation with editor flag",
			input:    "1 --delete-last --yes --editor",
			isTTY:    true,
			wantsErr: true,
		},
		{
			name:     "delete last flag and pre-confirmation with body flag",
			input:    "1 --delete-last --yes --body",
			isTTY:    true,
			wantsErr: true,
		},
		{
			name:     "delete pre-confirmation without delete last flag",
			input:    "1 --yes",
			isTTY:    true,
			wantsErr: true,
		},
		{
			name:     "body and body-file flags",
			input:    "1 --body 'test' --body-file 'test-file.txt'",
			output:   shared.CommentableOptions{},
			isTTY:    true,
			wantsErr: true,
		},
		{
			name:     "editor and web flags",
			input:    "1 --editor --web",
			output:   shared.CommentableOptions{},
			isTTY:    true,
			wantsErr: true,
		},
		{
			name:     "editor and body flags",
			input:    "1 --editor --body test",
			output:   shared.CommentableOptions{},
			isTTY:    true,
			wantsErr: true,
		},
		{
			name:     "web and body flags",
			input:    "1 --web --body test",
			output:   shared.CommentableOptions{},
			isTTY:    true,
			wantsErr: true,
		},
		{
			name:     "editor, web, and body flags",
			input:    "1 --editor --web --body test",
			output:   shared.CommentableOptions{},
			isTTY:    true,
			wantsErr: true,
		},
		{
			name:     "create-if-none flag without edit-last",
			input:    "1 --create-if-none",
			output:   shared.CommentableOptions{},
			isTTY:    true,
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, _, _ := iostreams.Test()
			isTTY := tt.isTTY
			ios.SetStdoutTTY(isTTY)
			ios.SetStdinTTY(isTTY)
			ios.SetStderrTTY(isTTY)

			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
				Browser:   &browser.Stub{},
			}

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			var gotOpts *shared.CommentableOptions
			cmd := NewCmdComment(f, func(opts *shared.CommentableOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.Interactive, gotOpts.Interactive)
			assert.Equal(t, tt.output.InputType, gotOpts.InputType)
			assert.Equal(t, tt.output.Body, gotOpts.Body)
			assert.Equal(t, tt.output.DeleteLast, gotOpts.DeleteLast)
			assert.Equal(t, tt.output.DeleteLastConfirmed, gotOpts.DeleteLastConfirmed)
		})
	}
}

func Test_commentRun(t *testing.T) {
	tests := []struct {
		name          string
		input         *shared.CommentableOptions
		emptyComments bool
		comments      api.Comments
		httpStubs     func(*testing.T, *httpmock.Registry)
		stdout        string
		stderr        string
		wantsErr      bool
	}{
		{
			name: "creating new comment with interactive editor succeeds",
			input: &shared.CommentableOptions{
				Interactive: true,
				InputType:   0,
				Body:        "",

				InteractiveEditSurvey: func(string) (string, error) { return "comment body", nil },
				ConfirmSubmitSurvey:   func() (bool, error) { return true, nil },
			},
			emptyComments: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentCreate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issues/123#issuecomment-456\n",
		},
		{
			name: "updating last comment with interactive editor fails if there are no comments and decline prompt to create",
			input: &shared.CommentableOptions{
				Interactive: true,
				InputType:   0,
				Body:        "",
				EditLast:    true,

				InteractiveEditSurvey:     func(string) (string, error) { return "comment body", nil },
				ConfirmSubmitSurvey:       func() (bool, error) { return true, nil },
				ConfirmCreateIfNoneSurvey: func() (bool, error) { return false, nil },
			},
			emptyComments: true,
			wantsErr:      true,
			stdout:        "no comments found for current user",
		},
		{
			name: "updating last comment with interactive editor succeeds if there are comments",
			input: &shared.CommentableOptions{
				Interactive: true,
				InputType:   0,
				Body:        "",
				EditLast:    true,

				InteractiveEditSurvey: func(string) (string, error) { return "comment body", nil },
				ConfirmSubmitSurvey:   func() (bool, error) { return true, nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentUpdate(t, reg)
			},
			emptyComments: false,
			stdout:        "https://github.com/OWNER/REPO/issues/123#issuecomment-111\n",
		},
		{
			name: "updating last comment with interactive editor creates new comment if there are no comments but --create-if-none",
			input: &shared.CommentableOptions{
				Interactive:  true,
				InputType:    0,
				Body:         "",
				EditLast:     true,
				CreateIfNone: true,

				InteractiveEditSurvey:     func(string) (string, error) { return "comment body", nil },
				ConfirmCreateIfNoneSurvey: func() (bool, error) { return true, nil },
				ConfirmSubmitSurvey:       func() (bool, error) { return true, nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentCreate(t, reg)
			},
			emptyComments: true,
			stderr:        "No comments found. Creating a new comment.\n",
			stdout:        "https://github.com/OWNER/REPO/issues/123#issuecomment-456\n",
		},
		{
			name: "creating new comment with non-interactive web opens issue in browser focusing on new comment",
			input: &shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeWeb,
				Body:        "",

				OpenInBrowser: func(string) error { return nil },
			},
			emptyComments: true,
			stderr:        "Opening https://github.com/OWNER/REPO/issues/123 in your browser.\n",
		},
		{
			name: "updating last comment with non-interactive web opens issue in browser focusing on the last comment",
			input: &shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeWeb,
				Body:        "",
				EditLast:    true,

				OpenInBrowser: func(u string) error {
					assert.Contains(t, u, "#issuecomment-111")
					return nil
				},
			},
			emptyComments: false,
			stderr:        "Opening https://github.com/OWNER/REPO/issues/123 in your browser.\n",
		},
		{
			name: "updating last comment with non-interactive web errors because there are no comments",
			input: &shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeWeb,
				Body:        "",
				EditLast:    true,
			},
			emptyComments: true,
			wantsErr:      true,
			stdout:        "no comments found for current user",
		},
		{
			name: "creating new comment with non-interactive editor succeeds",
			input: &shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeEditor,
				Body:        "",

				EditSurvey: func(string) (string, error) { return "comment body", nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentCreate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issues/123#issuecomment-456\n",
		},
		{
			name: "updating last comment with non-interactive editor fails if there are no comments",
			input: &shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeEditor,
				Body:        "",
				EditLast:    true,

				EditSurvey: func(string) (string, error) { return "comment body", nil },
			},
			emptyComments: true,
			wantsErr:      true,
			stdout:        "no comments found for current user",
		},
		{
			name: "updating last comment with non-interactive editor succeeds if there are comments",
			input: &shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeEditor,
				Body:        "",
				EditLast:    true,

				EditSurvey: func(string) (string, error) { return "comment body", nil },
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentUpdate(t, reg)
			},
			emptyComments: false,
			stdout:        "https://github.com/OWNER/REPO/issues/123#issuecomment-111\n",
		},
		{
			name: "updating last comment with non-interactive editor creates new comment if there are no comments but --create-if-none",
			input: &shared.CommentableOptions{
				Interactive:  false,
				InputType:    shared.InputTypeEditor,
				Body:         "",
				EditLast:     true,
				CreateIfNone: true,

				EditSurvey: func(string) (string, error) { return "comment body", nil },
			},
			emptyComments: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentCreate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issues/123#issuecomment-456\n",
		},
		{
			name: "creating new comment with non-interactive inline succeeds if comment body is provided",
			input: &shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeInline,
				Body:        "comment body",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentCreate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issues/123#issuecomment-456\n",
		},
		{
			name: "updating last comment with non-interactive inline succeeds if there are comments and comment body is provided",
			input: &shared.CommentableOptions{
				Interactive: false,
				InputType:   shared.InputTypeInline,
				Body:        "comment body",
				EditLast:    true,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentUpdate(t, reg)
			},
			emptyComments: false,
			stdout:        "https://github.com/OWNER/REPO/issues/123#issuecomment-111\n",
		},
		{
			name: "updating last comment with non-interactive inline creates new comment if there are no comments but --create-if-none",
			input: &shared.CommentableOptions{
				Interactive:  false,
				InputType:    shared.InputTypeInline,
				Body:         "comment body",
				EditLast:     true,
				CreateIfNone: true,
			},
			emptyComments: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentCreate(t, reg)
			},
			stdout: "https://github.com/OWNER/REPO/issues/123#issuecomment-456\n",
		},
		{
			name: "deleting last comment non-interactively without any comment",
			input: &shared.CommentableOptions{
				Interactive: false,
				DeleteLast:  true,
			},
			emptyComments: true,
			wantsErr:      true,
			stdout:        "no comments found for current user",
		},
		{
			name: "deleting last comment interactively without any comment",
			input: &shared.CommentableOptions{
				Interactive: true,
				DeleteLast:  true,
			},
			emptyComments: true,
			wantsErr:      true,
			stdout:        "no comments found for current user",
		},
		{
			name: "deleting last comment non-interactively and pre-confirmed",
			input: &shared.CommentableOptions{
				Interactive:         false,
				DeleteLast:          true,
				DeleteLastConfirmed: true,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentDelete(t, reg)
			},
			stderr: "Comment deleted\n",
		},
		{
			name: "deleting last comment interactively and pre-confirmed",
			input: &shared.CommentableOptions{
				Interactive:         true,
				DeleteLast:          true,
				DeleteLastConfirmed: true,
			},
			comments: api.Comments{Nodes: []api.Comment{
				{ID: "id1", Author: api.CommentAuthor{Login: "octocat"}, URL: "https://github.com/OWNER/REPO/pull/123#issuecomment-111", ViewerDidAuthor: true, Body: "comment body"},
			}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentDelete(t, reg)
			},
			stderr: "Comment deleted\n",
		},
		{
			name: "deleting last comment interactively and confirmed",
			input: &shared.CommentableOptions{
				Interactive: true,
				DeleteLast:  true,

				ConfirmDeleteLastComment: func(body string) (bool, error) {
					if body != "comment body" {
						return false, errors.New("unexpected comment body")
					}
					return true, nil
				},
			},
			comments: api.Comments{Nodes: []api.Comment{
				{ID: "id1", Author: api.CommentAuthor{Login: "octocat"}, URL: "https://github.com/OWNER/REPO/pull/123#issuecomment-111", ViewerDidAuthor: true, Body: "comment body"},
			}},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentDelete(t, reg)
			},
			stdout: "! Deleted comments cannot be recovered.\n",
			stderr: "Comment deleted\n",
		},
		{
			name: "deleting last comment interactively and confirmation declined",
			input: &shared.CommentableOptions{
				Interactive: true,
				DeleteLast:  true,

				ConfirmDeleteLastComment: func(body string) (bool, error) {
					if body != "comment body" {
						return false, errors.New("unexpected comment body")
					}
					return true, nil
				},
			},
			comments: api.Comments{Nodes: []api.Comment{
				{ID: "id1", Author: api.CommentAuthor{Login: "octocat"}, URL: "https://github.com/OWNER/REPO/pull/123#issuecomment-111", ViewerDidAuthor: true, Body: "comment body"},
			}},
			wantsErr: true,
			stdout:   "deletion not confirmed",
		},
		{
			name: "deleting last comment interactively and confirmed with long comment body",
			input: &shared.CommentableOptions{
				Interactive: true,
				DeleteLast:  true,

				ConfirmDeleteLastComment: func(body string) (bool, error) {
					if body != "Lorem ipsum dolor sit amet, consectet lo..." {
						return false, errors.New("unexpected comment body")
					}
					return true, nil
				},
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockCommentDelete(t, reg)
			},
			comments: api.Comments{Nodes: []api.Comment{
				{ID: "id1", Author: api.CommentAuthor{Login: "octocat"}, URL: "https://github.com/OWNER/REPO/pull/123#issuecomment-111", ViewerDidAuthor: true, Body: "Lorem ipsum dolor sit amet, consectet lorem ipsum again"},
			}},
			wantsErr: false,
			stdout:   "! Deleted comments cannot be recovered.\n",
			stderr:   "Comment deleted\n",
		},
	}
	for _, tt := range tests {
		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdoutTTY(true)
		ios.SetStdinTTY(true)
		ios.SetStderrTTY(true)

		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		if tt.httpStubs != nil {
			tt.httpStubs(t, reg)
		}

		tt.input.IO = ios
		tt.input.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		comments := api.Comments{Nodes: []api.Comment{
			{ID: "id1", Author: api.CommentAuthor{Login: "octocat"}, URL: "https://github.com/OWNER/REPO/issues/123#issuecomment-111", ViewerDidAuthor: true},
			{ID: "id2", Author: api.CommentAuthor{Login: "monalisa"}, URL: "https://github.com/OWNER/REPO/issues/123#issuecomment-222"},
		}}

		if tt.emptyComments {
			comments.Nodes = []api.Comment{}
		} else if len(tt.comments.Nodes) > 0 {
			comments = tt.comments
		}

		tt.input.RetrieveCommentable = func() (shared.Commentable, ghrepo.Interface, error) {
			return &api.Issue{
				ID:       "ISSUE-ID",
				URL:      "https://github.com/OWNER/REPO/issues/123",
				Comments: comments,
			}, ghrepo.New("OWNER", "REPO"), nil
		}

		t.Run(tt.name, func(t *testing.T) {
			err := shared.CommentableRun(tt.input)
			if tt.wantsErr {
				assert.Error(t, err)
				assert.Equal(t, tt.stderr, stderr.String())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())
		})
	}
}

func mockCommentCreate(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation CommentCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "addComment": { "commentEdge": { "node": {
			"url": "https://github.com/OWNER/REPO/issues/123#issuecomment-456"
		} } } } }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, "ISSUE-ID", inputs["subjectId"])
				assert.Equal(t, "comment body", inputs["body"])
			}),
	)
}

func mockCommentUpdate(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation CommentUpdate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "updateIssueComment": { "issueComment": {
			"url": "https://github.com/OWNER/REPO/issues/123#issuecomment-111"
		} } } }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, "id1", inputs["id"])
				assert.Equal(t, "comment body", inputs["body"])
			}),
	)
}

func mockCommentDelete(t *testing.T, reg *httpmock.Registry) {
	reg.Register(
		httpmock.GraphQL(`mutation CommentDelete\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "deleteIssueComment": {} } }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, "id1", inputs["id"])
			},
		),
	)
}
