package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCommand(t *testing.T) {
	tests := []struct {
		name     string
		repoDir  string
		gitPath  string
		wantExe  string
		wantArgs []string
	}{
		{
			name:     "creates command",
			gitPath:  "path/to/git",
			wantExe:  "path/to/git",
			wantArgs: []string{"path/to/git", "ref-log"},
		},
		{
			name:     "adds repo directory configuration",
			repoDir:  "path/to/repo",
			gitPath:  "path/to/git",
			wantExe:  "path/to/git",
			wantArgs: []string{"path/to/git", "-C", "path/to/repo", "ref-log"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, out, errOut := &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
			client := Client{
				Stdin:   in,
				Stdout:  out,
				Stderr:  errOut,
				RepoDir: tt.repoDir,
				GitPath: tt.gitPath,
			}
			cmd, err := client.Command(context.Background(), "ref-log")
			assert.NoError(t, err)
			assert.Equal(t, tt.wantExe, cmd.Path)
			assert.Equal(t, tt.wantArgs, cmd.Args)
			assert.Equal(t, in, cmd.Stdin)
			assert.Equal(t, out, cmd.Stdout)
			assert.Equal(t, errOut, cmd.Stderr)
		})
	}
}

func TestClientAuthenticatedCommand(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pattern  CredentialPattern
		wantArgs []string
		wantErr  error
	}{
		{
			name:     "when credential pattern allows for anything, credential helper matches everything",
			path:     "path/to/gh",
			pattern:  AllMatchingCredentialsPattern,
			wantArgs: []string{"path/to/git", "-c", "credential.helper=", "-c", `credential.helper=!"path/to/gh" auth git-credential`, "fetch"},
		},
		{
			name:     "when credential pattern is set, credential helper only matches that pattern",
			path:     "path/to/gh",
			pattern:  CredentialPattern{pattern: "https://github.com"},
			wantArgs: []string{"path/to/git", "-c", "credential.https://github.com.helper=", "-c", `credential.https://github.com.helper=!"path/to/gh" auth git-credential`, "fetch"},
		},
		{
			name:     "fallback when GhPath is not set",
			pattern:  AllMatchingCredentialsPattern,
			wantArgs: []string{"path/to/git", "-c", "credential.helper=", "-c", `credential.helper=!"gh" auth git-credential`, "fetch"},
		},
		{
			name:    "errors when attempting to use an empty pattern that isn't marked all matching",
			pattern: CredentialPattern{allMatching: false, pattern: ""},
			wantErr: fmt.Errorf("empty credential pattern is not allowed unless provided explicitly"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GhPath:  tt.path,
				GitPath: "path/to/git",
			}
			cmd, err := client.AuthenticatedCommand(context.Background(), tt.pattern, "fetch")
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
				return
			}
			require.Equal(t, tt.wantArgs, cmd.Args)
		})
	}
}

func TestClientRemotes(t *testing.T) {
	tempDir := t.TempDir()
	initRepo(t, tempDir)
	gitDir := filepath.Join(tempDir, ".git")
	remoteFile := filepath.Join(gitDir, "config")
	remotes := `
[remote "origin"]
	url = git@example.com:monalisa/origin.git
[remote "test"]
	url = git://github.com/hubot/test.git
	gh-resolved = other
[remote "upstream"]
	url = https://github.com/monalisa/upstream.git
	gh-resolved = base
[remote "github"]
	url = git@github.com:hubot/github.git
`
	f, err := os.OpenFile(remoteFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	assert.NoError(t, err)
	_, err = f.Write([]byte(remotes))
	assert.NoError(t, err)
	err = f.Close()
	assert.NoError(t, err)
	client := Client{
		RepoDir: tempDir,
	}
	rs, err := client.Remotes(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 4, len(rs))
	assert.Equal(t, "upstream", rs[0].Name)
	assert.Equal(t, "base", rs[0].Resolved)
	assert.Equal(t, "github", rs[1].Name)
	assert.Equal(t, "", rs[1].Resolved)
	assert.Equal(t, "origin", rs[2].Name)
	assert.Equal(t, "", rs[2].Resolved)
	assert.Equal(t, "test", rs[3].Name)
	assert.Equal(t, "other", rs[3].Resolved)
}

func TestClientRemotes_no_resolved_remote(t *testing.T) {
	tempDir := t.TempDir()
	initRepo(t, tempDir)
	gitDir := filepath.Join(tempDir, ".git")
	remoteFile := filepath.Join(gitDir, "config")
	remotes := `
[remote "origin"]
	url = git@example.com:monalisa/origin.git
[remote "test"]
	url = git://github.com/hubot/test.git
[remote "upstream"]
	url = https://github.com/monalisa/upstream.git
[remote "github"]
	url = git@github.com:hubot/github.git
`
	f, err := os.OpenFile(remoteFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	assert.NoError(t, err)
	_, err = f.Write([]byte(remotes))
	assert.NoError(t, err)
	err = f.Close()
	assert.NoError(t, err)
	client := Client{
		RepoDir: tempDir,
	}
	rs, err := client.Remotes(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 4, len(rs))
	assert.Equal(t, "upstream", rs[0].Name)
	assert.Equal(t, "github", rs[1].Name)
	assert.Equal(t, "origin", rs[2].Name)
	assert.Equal(t, "", rs[2].Resolved)
	assert.Equal(t, "test", rs[3].Name)
}

func TestParseRemotes(t *testing.T) {
	remoteList := []string{
		"mona\tgit@github.com:monalisa/myfork.git (fetch)",
		"origin\thttps://github.com/monalisa/octo-cat.git (fetch)",
		"origin\thttps://github.com/monalisa/octo-cat-push.git (push)",
		"upstream\thttps://example.com/nowhere.git (fetch)",
		"upstream\thttps://github.com/hubot/tools (push)",
		"zardoz\thttps://example.com/zed.git (push)",
		"koke\tgit://github.com/koke/grit.git (fetch)",
		"koke\tgit://github.com/koke/grit.git (push)",
	}

	r := parseRemotes(remoteList)
	assert.Equal(t, 5, len(r))

	assert.Equal(t, "mona", r[0].Name)
	assert.Equal(t, "ssh://git@github.com/monalisa/myfork.git", r[0].FetchURL.String())
	assert.Nil(t, r[0].PushURL)

	assert.Equal(t, "origin", r[1].Name)
	assert.Equal(t, "/monalisa/octo-cat.git", r[1].FetchURL.Path)
	assert.Equal(t, "/monalisa/octo-cat-push.git", r[1].PushURL.Path)

	assert.Equal(t, "upstream", r[2].Name)
	assert.Equal(t, "example.com", r[2].FetchURL.Host)
	assert.Equal(t, "github.com", r[2].PushURL.Host)

	assert.Equal(t, "zardoz", r[3].Name)
	assert.Nil(t, r[3].FetchURL)
	assert.Equal(t, "https://example.com/zed.git", r[3].PushURL.String())

	assert.Equal(t, "koke", r[4].Name)
	assert.Equal(t, "/koke/grit.git", r[4].FetchURL.Path)
	assert.Equal(t, "/koke/grit.git", r[4].PushURL.Path)
}

func TestClientUpdateRemoteURL(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			name:        "update remote url",
			wantCmdArgs: `path/to/git remote set-url test https://test.com`,
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git remote set-url test https://test.com`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.UpdateRemoteURL(context.Background(), "test", "https://test.com")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientSetRemoteResolution(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			name:        "set remote resolution",
			wantCmdArgs: `path/to/git config --add remote.origin.gh-resolved base`,
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git config --add remote.origin.gh-resolved base`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.SetRemoteResolution(context.Background(), "origin", "base")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientCurrentBranch(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
		wantBranch    string
	}{
		{
			name:        "branch name",
			cmdStdout:   "branch-name\n",
			wantCmdArgs: `path/to/git symbolic-ref --quiet HEAD`,
			wantBranch:  "branch-name",
		},
		{
			name:        "ref",
			cmdStdout:   "refs/heads/branch-name\n",
			wantCmdArgs: `path/to/git symbolic-ref --quiet HEAD`,
			wantBranch:  "branch-name",
		},
		{
			name:        "escaped ref",
			cmdStdout:   "refs/heads/branch\u00A0with\u00A0non\u00A0breaking\u00A0space\n",
			wantCmdArgs: `path/to/git symbolic-ref --quiet HEAD`,
			wantBranch:  "branch\u00A0with\u00A0non\u00A0breaking\u00A0space",
		},
		{
			name:          "detached head",
			cmdExitStatus: 1,
			wantCmdArgs:   `path/to/git symbolic-ref --quiet HEAD`,
			wantErrorMsg:  "failed to run git: not on any branch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			branch, err := client.CurrentBranch(context.Background())
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
			assert.Equal(t, tt.wantBranch, branch)
		})
	}
}

func TestClientShowRefs(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantRefs      []Ref
		wantErrorMsg  string
	}{
		{
			name:          "show refs with one valid ref and one invalid ref",
			cmdExitStatus: 128,
			cmdStdout:     "9ea76237a557015e73446d33268569a114c0649c refs/heads/valid",
			cmdStderr:     "fatal: 'refs/heads/invalid' - not a valid ref",
			wantCmdArgs:   `path/to/git show-ref --verify -- refs/heads/valid refs/heads/invalid`,
			wantRefs: []Ref{{
				Hash: "9ea76237a557015e73446d33268569a114c0649c",
				Name: "refs/heads/valid",
			}},
			wantErrorMsg: "failed to run git: fatal: 'refs/heads/invalid' - not a valid ref",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			refs, err := client.ShowRefs(context.Background(), []string{"refs/heads/valid", "refs/heads/invalid"})
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			assert.EqualError(t, err, tt.wantErrorMsg)
			assert.Equal(t, tt.wantRefs, refs)
		})
	}
}

func TestClientConfig(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantOut       string
		wantErrorMsg  string
	}{
		{
			name:        "get config key",
			cmdStdout:   "test",
			wantCmdArgs: `path/to/git config credential.helper`,
			wantOut:     "test",
		},
		{
			name:          "get unknown config key",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git config credential.helper`,
			wantErrorMsg:  "failed to run git: unknown config key credential.helper",
		},
		{
			name:          "git error",
			cmdExitStatus: 2,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git config credential.helper`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			out, err := client.Config(context.Background(), "credential.helper")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
			assert.Equal(t, tt.wantOut, out)
		})
	}
}

func TestClientUncommittedChangeCount(t *testing.T) {
	tests := []struct {
		name            string
		cmdExitStatus   int
		cmdStdout       string
		cmdStderr       string
		wantCmdArgs     string
		wantChangeCount int
	}{
		{
			name:            "no changes",
			wantCmdArgs:     `path/to/git status --porcelain`,
			wantChangeCount: 0,
		},
		{
			name:            "one change",
			cmdStdout:       " M poem.txt",
			wantCmdArgs:     `path/to/git status --porcelain`,
			wantChangeCount: 1,
		},
		{
			name:            "untracked file",
			cmdStdout:       " M poem.txt\n?? new.txt",
			wantCmdArgs:     `path/to/git status --porcelain`,
			wantChangeCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			ucc, err := client.UncommittedChangeCount(context.Background())
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			assert.NoError(t, err)
			assert.Equal(t, tt.wantChangeCount, ucc)
		})
	}
}

type stubbedCommit struct {
	Sha   string
	Title string
	Body  string
}

type stubbedCommitsCommandData struct {
	ExitStatus int

	ErrMsg string

	Commits []stubbedCommit
}

func TestClientCommits(t *testing.T) {
	tests := []struct {
		name         string
		testData     stubbedCommitsCommandData
		wantCmdArgs  string
		wantCommits  []*Commit
		wantErrorMsg string
	}{
		{
			name: "single commit no body",
			testData: stubbedCommitsCommandData{
				Commits: []stubbedCommit{
					{
						Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
						Title: "testing testability test",
						Body:  "",
					},
				},
			},
			wantCmdArgs: `path/to/git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry SHA1...SHA2`,
			wantCommits: []*Commit{{
				Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
				Title: "testing testability test",
			}},
		},
		{
			name: "single commit with body",
			testData: stubbedCommitsCommandData{
				Commits: []stubbedCommit{
					{
						Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
						Title: "testing testability test",
						Body:  "This is the body",
					},
				},
			},
			wantCmdArgs: `path/to/git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry SHA1...SHA2`,
			wantCommits: []*Commit{{
				Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
				Title: "testing testability test",
				Body:  "This is the body",
			}},
		},
		{
			name: "multiple commits with bodies",
			testData: stubbedCommitsCommandData{
				Commits: []stubbedCommit{
					{
						Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
						Title: "testing testability test",
						Body:  "This is the body",
					},
					{
						Sha:   "7a6872b918c601a0e730710ad8473938a7516d31",
						Title: "testing testability test 2",
						Body:  "This is the body 2",
					},
				},
			},
			wantCmdArgs: `path/to/git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry SHA1...SHA2`,
			wantCommits: []*Commit{
				{
					Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
					Title: "testing testability test",
					Body:  "This is the body",
				},
				{
					Sha:   "7a6872b918c601a0e730710ad8473938a7516d31",
					Title: "testing testability test 2",
					Body:  "This is the body 2",
				},
			},
		},
		{
			name: "multiple commits mixed bodies",
			testData: stubbedCommitsCommandData{
				Commits: []stubbedCommit{
					{
						Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
						Title: "testing testability test",
					},
					{
						Sha:   "7a6872b918c601a0e730710ad8473938a7516d31",
						Title: "testing testability test 2",
						Body:  "This is the body 2",
					},
				},
			},
			wantCmdArgs: `path/to/git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry SHA1...SHA2`,
			wantCommits: []*Commit{
				{
					Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
					Title: "testing testability test",
				},
				{
					Sha:   "7a6872b918c601a0e730710ad8473938a7516d31",
					Title: "testing testability test 2",
					Body:  "This is the body 2",
				},
			},
		},
		{
			name: "multiple commits newlines in bodies",
			testData: stubbedCommitsCommandData{
				Commits: []stubbedCommit{
					{
						Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
						Title: "testing testability test",
						Body:  "This is the body\nwith a newline",
					},
					{
						Sha:   "7a6872b918c601a0e730710ad8473938a7516d31",
						Title: "testing testability test 2",
						Body:  "This is the body 2",
					},
				},
			},
			wantCmdArgs: `path/to/git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry SHA1...SHA2`,
			wantCommits: []*Commit{
				{
					Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
					Title: "testing testability test",
					Body:  "This is the body\nwith a newline",
				},
				{
					Sha:   "7a6872b918c601a0e730710ad8473938a7516d31",
					Title: "testing testability test 2",
					Body:  "This is the body 2",
				},
			},
		},
		{
			name: "no commits between SHAs",
			testData: stubbedCommitsCommandData{
				Commits: []stubbedCommit{},
			},
			wantCmdArgs:  `path/to/git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry SHA1...SHA2`,
			wantErrorMsg: "could not find any commits between SHA1 and SHA2",
		},
		{
			name: "git error",
			testData: stubbedCommitsCommandData{
				ErrMsg:     "git error message",
				ExitStatus: 1,
			},
			wantCmdArgs:  `path/to/git -c log.ShowSignature=false log --pretty=format:%H%x00%s%x00%b%x00 --cherry SHA1...SHA2`,
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommitsCommandContext(t, tt.testData)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			commits, err := client.Commits(context.Background(), "SHA1", "SHA2")
			require.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg != "" {
				require.EqualError(t, err, tt.wantErrorMsg)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantCommits, commits)
		})
	}
}

func TestCommitsHelperProcess(t *testing.T) {
	if os.Getenv("GH_WANT_HELPER_PROCESS") != "1" {
		return
	}

	var td stubbedCommitsCommandData
	_ = json.Unmarshal([]byte(os.Getenv("GH_COMMITS_TEST_DATA")), &td)

	if td.ErrMsg != "" {
		fmt.Fprint(os.Stderr, td.ErrMsg)
	} else {
		var sb strings.Builder
		for _, commit := range td.Commits {
			sb.WriteString(commit.Sha)
			sb.WriteString("\u0000")
			sb.WriteString(commit.Title)
			sb.WriteString("\u0000")
			sb.WriteString(commit.Body)
			sb.WriteString("\u0000")
			sb.WriteString("\n")
		}
		fmt.Fprint(os.Stdout, sb.String())
	}

	os.Exit(td.ExitStatus)
}

func createCommitsCommandContext(t *testing.T, testData stubbedCommitsCommandData) (*exec.Cmd, commandCtx) {
	t.Helper()

	b, err := json.Marshal(testData)
	require.NoError(t, err)

	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=TestCommitsHelperProcess", "--")
	cmd.Env = []string{
		"GH_WANT_HELPER_PROCESS=1",
		"GH_COMMITS_TEST_DATA=" + string(b),
	}
	return cmd, func(ctx context.Context, exe string, args ...string) *exec.Cmd {
		cmd.Args = append(cmd.Args, exe)
		cmd.Args = append(cmd.Args, args...)
		return cmd
	}
}

func TestClientLastCommit(t *testing.T) {
	client := Client{
		RepoDir: "./fixtures/simple.git",
	}
	c, err := client.LastCommit(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "6f1a2405cace1633d89a79c74c65f22fe78f9659", c.Sha)
	assert.Equal(t, "Second commit", c.Title)
}

func TestClientCommitBody(t *testing.T) {
	client := Client{
		RepoDir: "./fixtures/simple.git",
	}
	body, err := client.CommitBody(context.Background(), "6f1a2405cace1633d89a79c74c65f22fe78f9659")
	assert.NoError(t, err)
	assert.Equal(t, "I'm starting to get the hang of things\n", body)
}

func TestClientReadBranchConfig(t *testing.T) {
	tests := []struct {
		name             string
		cmds             mockedCommands
		branch           string
		wantBranchConfig BranchConfig
		wantError        *GitError
	}{
		{
			name: "when the git config has no (remote|merge|pushremote|gh-merge-base) keys, it should return an empty BranchConfig and no error",
			cmds: mockedCommands{
				`path/to/git config --get-regexp ^branch\.trunk\.(remote|merge|pushremote|gh-merge-base)$`: {
					ExitStatus: 1,
				},
			},
			branch:           "trunk",
			wantBranchConfig: BranchConfig{},
			wantError:        nil,
		},
		{
			name: "when the git fails to read the config, it should return an empty BranchConfig and the error",
			cmds: mockedCommands{
				`path/to/git config --get-regexp ^branch\.trunk\.(remote|merge|pushremote|gh-merge-base)$`: {
					ExitStatus: 2,
					Stderr:     "git error",
				},
			},
			branch:           "trunk",
			wantBranchConfig: BranchConfig{},
			wantError: &GitError{
				ExitCode: 2,
				Stderr:   "git error",
			},
		},
		{
			name: "when the config is read, it should return the correct BranchConfig",
			cmds: mockedCommands{
				`path/to/git config --get-regexp ^branch\.trunk\.(remote|merge|pushremote|gh-merge-base)$`: {
					Stdout: heredoc.Doc(`
						branch.trunk.remote upstream
						branch.trunk.merge refs/heads/trunk
						branch.trunk.pushremote origin
						branch.trunk.gh-merge-base gh-merge-base
					`),
				},
			},
			branch: "trunk",
			wantBranchConfig: BranchConfig{
				RemoteName:     "upstream",
				PushRemoteName: "origin",
				MergeRef:       "refs/heads/trunk",
				MergeBase:      "gh-merge-base",
			},
			wantError: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdCtx := createMockedCommandContext(t, tt.cmds)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			branchConfig, err := client.ReadBranchConfig(context.Background(), tt.branch)
			if tt.wantError != nil {
				var gitError *GitError
				require.ErrorAs(t, err, &gitError)
				assert.Equal(t, tt.wantError.ExitCode, gitError.ExitCode)
				assert.Equal(t, tt.wantError.Stderr, gitError.Stderr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantBranchConfig, branchConfig)
		})
	}
}

func Test_parseBranchConfig(t *testing.T) {
	tests := []struct {
		name             string
		configLines      []string
		wantBranchConfig BranchConfig
	}{
		{
			name:        "remote branch",
			configLines: []string{"branch.trunk.remote origin"},
			wantBranchConfig: BranchConfig{
				RemoteName: "origin",
			},
		},
		{
			name:        "merge ref",
			configLines: []string{"branch.trunk.merge refs/heads/trunk"},
			wantBranchConfig: BranchConfig{
				MergeRef: "refs/heads/trunk",
			},
		},
		{
			name:        "merge base",
			configLines: []string{"branch.trunk.gh-merge-base gh-merge-base"},
			wantBranchConfig: BranchConfig{
				MergeBase: "gh-merge-base",
			},
		},
		{
			name:        "pushremote",
			configLines: []string{"branch.trunk.pushremote pushremote"},
			wantBranchConfig: BranchConfig{
				PushRemoteName: "pushremote",
			},
		},
		{
			name: "remote and pushremote are specified by name",
			configLines: []string{
				"branch.trunk.remote upstream",
				"branch.trunk.pushremote origin",
			},
			wantBranchConfig: BranchConfig{
				RemoteName:     "upstream",
				PushRemoteName: "origin",
			},
		},
		{
			name: "remote and pushremote are specified by url",
			configLines: []string{
				"branch.trunk.remote git@github.com:UPSTREAMOWNER/REPO.git",
				"branch.trunk.pushremote git@github.com:ORIGINOWNER/REPO.git",
			},
			wantBranchConfig: BranchConfig{
				RemoteURL: &url.URL{
					Scheme: "ssh",
					User:   url.User("git"),
					Host:   "github.com",
					Path:   "/UPSTREAMOWNER/REPO.git",
				},
				PushRemoteURL: &url.URL{
					Scheme: "ssh",
					User:   url.User("git"),
					Host:   "github.com",
					Path:   "/ORIGINOWNER/REPO.git",
				},
			},
		},
		{
			name: "remote, pushremote, gh-merge-base, and merge ref all specified",
			configLines: []string{
				"branch.trunk.remote remote",
				"branch.trunk.pushremote pushremote",
				"branch.trunk.gh-merge-base gh-merge-base",
				"branch.trunk.merge refs/heads/trunk",
			},
			wantBranchConfig: BranchConfig{
				RemoteName:     "remote",
				PushRemoteName: "pushremote",
				MergeBase:      "gh-merge-base",
				MergeRef:       "refs/heads/trunk",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			branchConfig := parseBranchConfig(tt.configLines)
			assert.Equalf(t, tt.wantBranchConfig.RemoteName, branchConfig.RemoteName, "unexpected RemoteName")
			assert.Equalf(t, tt.wantBranchConfig.MergeRef, branchConfig.MergeRef, "unexpected MergeRef")
			assert.Equalf(t, tt.wantBranchConfig.MergeBase, branchConfig.MergeBase, "unexpected MergeBase")
			assert.Equalf(t, tt.wantBranchConfig.PushRemoteName, branchConfig.PushRemoteName, "unexpected PushRemoteName")
			if tt.wantBranchConfig.RemoteURL != nil {
				assert.Equalf(t, tt.wantBranchConfig.RemoteURL.String(), branchConfig.RemoteURL.String(), "unexpected RemoteURL")
			}
			if tt.wantBranchConfig.PushRemoteURL != nil {
				assert.Equalf(t, tt.wantBranchConfig.PushRemoteURL.String(), branchConfig.PushRemoteURL.String(), "unexpected PushRemoteURL")
			}
		})
	}
}

func Test_parseRemoteURLOrName(t *testing.T) {
	tests := []struct {
		name           string
		value          string
		wantRemoteURL  *url.URL
		wantRemoteName string
	}{
		{
			name:           "empty value",
			value:          "",
			wantRemoteURL:  nil,
			wantRemoteName: "",
		},
		{
			name:  "remote URL",
			value: "git@github.com:foo/bar.git",
			wantRemoteURL: &url.URL{
				Scheme: "ssh",
				User:   url.User("git"),
				Host:   "github.com",
				Path:   "/foo/bar.git",
			},
			wantRemoteName: "",
		},
		{
			name:           "remote name",
			value:          "origin",
			wantRemoteURL:  nil,
			wantRemoteName: "origin",
		},
		{
			name:           "remote name is from filesystem",
			value:          "./path/to/repo",
			wantRemoteURL:  nil,
			wantRemoteName: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remoteURL, remoteName := parseRemoteURLOrName(tt.value)
			assert.Equal(t, tt.wantRemoteURL, remoteURL)
			assert.Equal(t, tt.wantRemoteName, remoteName)
		})
	}
}

func TestClientPushDefault(t *testing.T) {
	tests := []struct {
		name            string
		commandResult   commandResult
		wantPushDefault PushDefault
		wantError       *GitError
	}{
		{
			name: "push default is not set",
			commandResult: commandResult{
				ExitStatus: 1,
				Stderr:     "error: key does not contain a section: remote.pushDefault",
			},
			wantPushDefault: PushDefaultSimple,
			wantError:       nil,
		},
		{
			name: "push default is set to current",
			commandResult: commandResult{
				ExitStatus: 0,
				Stdout:     "current",
			},
			wantPushDefault: PushDefaultCurrent,
			wantError:       nil,
		},
		{
			name: "push default errors",
			commandResult: commandResult{
				ExitStatus: 128,
				Stderr:     "fatal: git error",
			},
			wantPushDefault: "",
			wantError: &GitError{
				ExitCode: 128,
				Stderr:   "fatal: git error",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdCtx := createMockedCommandContext(t, mockedCommands{
				`path/to/git config push.default`: tt.commandResult,
			},
			)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			pushDefault, err := client.PushDefault(context.Background())
			if tt.wantError != nil {
				var gitError *GitError
				require.ErrorAs(t, err, &gitError)
				assert.Equal(t, tt.wantError.ExitCode, gitError.ExitCode)
				assert.Equal(t, tt.wantError.Stderr, gitError.Stderr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantPushDefault, pushDefault)
		})
	}
}

func TestClientRemotePushDefault(t *testing.T) {
	tests := []struct {
		name                  string
		commandResult         commandResult
		wantRemotePushDefault string
		wantError             *GitError
	}{
		{
			name: "remote.pushDefault is not set",
			commandResult: commandResult{
				ExitStatus: 1,
				Stderr:     "error: key does not contain a section: remote.pushDefault",
			},
			wantRemotePushDefault: "",
			wantError:             nil,
		},
		{
			name: "remote.pushDefault is set to origin",
			commandResult: commandResult{
				ExitStatus: 0,
				Stdout:     "origin",
			},
			wantRemotePushDefault: "origin",
			wantError:             nil,
		},
		{
			name: "remote.pushDefault errors",
			commandResult: commandResult{
				ExitStatus: 128,
				Stderr:     "fatal: git error",
			},
			wantRemotePushDefault: "",
			wantError: &GitError{
				ExitCode: 128,
				Stderr:   "fatal: git error",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdCtx := createMockedCommandContext(t, mockedCommands{
				`path/to/git config remote.pushDefault`: tt.commandResult,
			},
			)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			pushDefault, err := client.RemotePushDefault(context.Background())
			if tt.wantError != nil {
				var gitError *GitError
				require.ErrorAs(t, err, &gitError)
				assert.Equal(t, tt.wantError.ExitCode, gitError.ExitCode)
				assert.Equal(t, tt.wantError.Stderr, gitError.Stderr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantRemotePushDefault, pushDefault)
		})
	}
}

func TestClientParsePushRevision(t *testing.T) {
	tests := []struct {
		name                   string
		branch                 string
		commandResult          commandResult
		wantParsedPushRevision RemoteTrackingRef
		wantError              error
	}{
		{
			name:   "@{push} resolves to refs/remotes/origin/branchName",
			branch: "branchName",
			commandResult: commandResult{
				ExitStatus: 0,
				Stdout:     "refs/remotes/origin/branchName",
			},
			wantParsedPushRevision: RemoteTrackingRef{Remote: "origin", Branch: "branchName"},
		},
		{
			name: "@{push} doesn't resolve",
			commandResult: commandResult{
				ExitStatus: 128,
				Stderr:     "fatal: git error",
			},
			wantParsedPushRevision: RemoteTrackingRef{},
			wantError: &GitError{
				ExitCode: 128,
				Stderr:   "fatal: git error",
			},
		},
		{
			name: "@{push} resolves to something surprising",
			commandResult: commandResult{
				ExitStatus: 0,
				Stdout:     "not/a/valid/remote/ref",
			},
			wantParsedPushRevision: RemoteTrackingRef{},
			wantError:              fmt.Errorf("could not parse push revision: remote tracking branch must have format refs/remotes/<remote>/<branch> but was: not/a/valid/remote/ref"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := fmt.Sprintf("path/to/git rev-parse --symbolic-full-name %s@{push}", tt.branch)
			cmdCtx := createMockedCommandContext(t, mockedCommands{
				args(cmd): tt.commandResult,
			})
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			trackingRef, err := client.PushRevision(context.Background(), tt.branch)
			if tt.wantError != nil {
				var wantErrorAsGit *GitError
				if errors.As(err, &wantErrorAsGit) {
					var gitError *GitError
					require.ErrorAs(t, err, &gitError)
					assert.Equal(t, wantErrorAsGit.ExitCode, gitError.ExitCode)
					assert.Equal(t, wantErrorAsGit.Stderr, gitError.Stderr)
				} else {
					assert.Equal(t, err, tt.wantError)
				}
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantParsedPushRevision, trackingRef)
		})
	}
}

func TestRemoteTrackingRef(t *testing.T) {
	t.Run("parsing", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name                  string
			remoteTrackingRef     string
			wantRemoteTrackingRef RemoteTrackingRef
			wantError             error
		}{
			{
				name:              "valid remote tracking ref without slash in branch name",
				remoteTrackingRef: "refs/remotes/origin/branchName",
				wantRemoteTrackingRef: RemoteTrackingRef{
					Remote: "origin",
					Branch: "branchName",
				},
			},
			{
				name:              "valid remote tracking ref with slash in branch name",
				remoteTrackingRef: "refs/remotes/origin/branch/name",
				wantRemoteTrackingRef: RemoteTrackingRef{
					Remote: "origin",
					Branch: "branch/name",
				},
			},
			// TODO: Uncomment when we support slashes in remote names
			// {
			// 	name: "valid remote tracking ref with slash in remote name",
			// 	remoteTrackingRef: "refs/remotes/my/origin/branchName",
			// 	wantRemoteTrackingRef: RemoteTrackingRef{
			// 		Remote: "my/origin",
			// 		Branch: "branchName",
			// 	},
			// },
			// {
			// 	name: 			"valid remote tracking ref with slash in remote name and branch name",
			// 	remoteTrackingRef: "refs/remotes/my/origin/branch/name",
			// 	wantRemoteTrackingRef: RemoteTrackingRef{
			// 		Remote: "my/origin",
			// 		Branch: "branch/name",
			// 	},
			// },
			{
				name:                  "incorrect parts",
				remoteTrackingRef:     "refs/remotes/origin",
				wantRemoteTrackingRef: RemoteTrackingRef{},
				wantError:             fmt.Errorf("remote tracking branch must have format refs/remotes/<remote>/<branch> but was: refs/remotes/origin"),
			},
			{
				name:                  "incorrect prefix type",
				remoteTrackingRef:     "invalid/remotes/origin/branchName",
				wantRemoteTrackingRef: RemoteTrackingRef{},
				wantError:             fmt.Errorf("remote tracking branch must have format refs/remotes/<remote>/<branch> but was: invalid/remotes/origin/branchName"),
			},
			{
				name:                  "incorrect ref type",
				remoteTrackingRef:     "refs/invalid/origin/branchName",
				wantRemoteTrackingRef: RemoteTrackingRef{},
				wantError:             fmt.Errorf("remote tracking branch must have format refs/remotes/<remote>/<branch> but was: refs/invalid/origin/branchName"),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				trackingRef, err := ParseRemoteTrackingRef(tt.remoteTrackingRef)
				if tt.wantError != nil {
					require.Equal(t, tt.wantError, err)
					return
				}

				require.NoError(t, err)
				assert.Equal(t, tt.wantRemoteTrackingRef, trackingRef)
			})
		}
	})

	t.Run("stringifying", func(t *testing.T) {
		t.Parallel()

		remoteTrackingRef := RemoteTrackingRef{
			Remote: "origin",
			Branch: "branchName",
		}

		require.Equal(t, "refs/remotes/origin/branchName", remoteTrackingRef.String())
	})
}

func TestClientDeleteLocalTag(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			name:        "delete local tag",
			wantCmdArgs: `path/to/git tag -d v1.0`,
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git tag -d v1.0`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.DeleteLocalTag(context.Background(), "v1.0")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientDeleteLocalBranch(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			name:        "delete local branch",
			wantCmdArgs: `path/to/git branch -D trunk`,
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git branch -D trunk`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.DeleteLocalBranch(context.Background(), "trunk")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientHasLocalBranch(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantOut       bool
	}{
		{
			name:        "has local branch",
			wantCmdArgs: `path/to/git rev-parse --verify refs/heads/trunk`,
			wantOut:     true,
		},
		{
			name:          "does not have local branch",
			cmdExitStatus: 1,
			wantCmdArgs:   `path/to/git rev-parse --verify refs/heads/trunk`,
			wantOut:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			out := client.HasLocalBranch(context.Background(), "trunk")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			assert.Equal(t, out, tt.wantOut)
		})
	}
}

func TestClientCheckoutBranch(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			name:        "checkout branch",
			wantCmdArgs: `path/to/git checkout trunk`,
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git checkout trunk`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.CheckoutBranch(context.Background(), "trunk")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientCheckoutNewBranch(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			name:        "checkout new branch",
			wantCmdArgs: `path/to/git checkout -b trunk --track origin/trunk`,
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git checkout -b trunk --track origin/trunk`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.CheckoutNewBranch(context.Background(), "origin", "trunk")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientToplevelDir(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantDir       string
		wantErrorMsg  string
	}{
		{
			name:        "top level dir",
			cmdStdout:   "/path/to/repo",
			wantCmdArgs: `path/to/git rev-parse --show-toplevel`,
			wantDir:     "/path/to/repo",
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git rev-parse --show-toplevel`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			dir, err := client.ToplevelDir(context.Background())
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
			assert.Equal(t, tt.wantDir, dir)
		})
	}
}

func TestClientGitDir(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantDir       string
		wantErrorMsg  string
	}{
		{
			name:        "git dir",
			cmdStdout:   "/path/to/repo/.git",
			wantCmdArgs: `path/to/git rev-parse --git-dir`,
			wantDir:     "/path/to/repo/.git",
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git rev-parse --git-dir`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			dir, err := client.GitDir(context.Background())
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
			assert.Equal(t, tt.wantDir, dir)
		})
	}
}

func TestClientPathFromRoot(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
		wantDir       string
	}{
		{
			name:        "current path from root",
			cmdStdout:   "some/path/",
			wantCmdArgs: `path/to/git rev-parse --show-prefix`,
			wantDir:     "some/path",
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git rev-parse --show-prefix`,
			wantDir:       "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			dir := client.PathFromRoot(context.Background())
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			assert.Equal(t, tt.wantDir, dir)
		})
	}
}

func TestClientUnsetRemoteResolution(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			name:        "unset remote resolution",
			wantCmdArgs: `path/to/git config --unset remote.origin.gh-resolved`,
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git config --unset remote.origin.gh-resolved`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.UnsetRemoteResolution(context.Background(), "origin")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientSetRemoteBranches(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			name:        "set remote branches",
			wantCmdArgs: `path/to/git remote set-branches origin trunk`,
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git remote set-branches origin trunk`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.SetRemoteBranches(context.Background(), "origin", "trunk")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientFetch(t *testing.T) {
	tests := []struct {
		name         string
		mods         []CommandModifier
		commands     mockedCommands
		wantErrorMsg string
	}{
		{
			name: "fetch",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential fetch origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "accepts command modifiers",
			mods: []CommandModifier{WithRepoDir("/path/to/repo")},
			commands: map[args]commandResult{
				`path/to/git -C /path/to/repo -c credential.helper= -c credential.helper=!"gh" auth git-credential fetch origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "git error on fetch",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential fetch origin trunk`: {
					ExitStatus: 1,
					Stderr:     "fetch error message",
				},
			},
			wantErrorMsg: "failed to run git: fetch error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdCtx := createMockedCommandContext(t, tt.commands)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.Fetch(context.Background(), "origin", "trunk", tt.mods...)
			if tt.wantErrorMsg == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientPull(t *testing.T) {
	tests := []struct {
		name         string
		mods         []CommandModifier
		commands     mockedCommands
		wantErrorMsg string
	}{
		{
			name: "pull",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential pull --ff-only origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "accepts command modifiers",
			mods: []CommandModifier{WithRepoDir("/path/to/repo")},
			commands: map[args]commandResult{
				`path/to/git -C /path/to/repo -c credential.helper= -c credential.helper=!"gh" auth git-credential pull --ff-only origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "git error on pull",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential pull --ff-only origin trunk`: {
					ExitStatus: 1,
					Stderr:     "pull error message",
				},
			},
			wantErrorMsg: "failed to run git: pull error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdCtx := createMockedCommandContext(t, tt.commands)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.Pull(context.Background(), "origin", "trunk", tt.mods...)
			if tt.wantErrorMsg == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientPush(t *testing.T) {
	tests := []struct {
		name         string
		mods         []CommandModifier
		commands     mockedCommands
		wantErrorMsg string
	}{
		{
			name: "push",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential push --set-upstream origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "accepts command modifiers",
			mods: []CommandModifier{WithRepoDir("/path/to/repo")},
			commands: map[args]commandResult{
				`path/to/git -C /path/to/repo -c credential.helper= -c credential.helper=!"gh" auth git-credential push --set-upstream origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "git error on push",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential push --set-upstream origin trunk`: {
					ExitStatus: 1,
					Stderr:     "push error message",
				},
			},
			wantErrorMsg: "failed to run git: push error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdCtx := createMockedCommandContext(t, tt.commands)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.Push(context.Background(), "origin", "trunk", tt.mods...)
			if tt.wantErrorMsg == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientClone(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		mods          []CommandModifier
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantTarget    string
		wantErrorMsg  string
	}{
		{
			name:        "clone",
			args:        []string{},
			wantCmdArgs: `path/to/git -c credential.https://github.com.helper= -c credential.https://github.com.helper=!"gh" auth git-credential clone https://github.com/cli/cli`,
			wantTarget:  "cli",
		},
		{
			name:        "accepts command modifiers",
			args:        []string{},
			mods:        []CommandModifier{WithRepoDir("/path/to/repo")},
			wantCmdArgs: `path/to/git -C /path/to/repo -c credential.https://github.com.helper= -c credential.https://github.com.helper=!"gh" auth git-credential clone https://github.com/cli/cli`,
			wantTarget:  "cli",
		},
		{
			name:          "git error",
			args:          []string{},
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git -c credential.https://github.com.helper= -c credential.https://github.com.helper=!"gh" auth git-credential clone https://github.com/cli/cli`,
			wantErrorMsg:  "failed to run git: git error message",
		},
		{
			name:        "bare clone",
			args:        []string{"--bare"},
			wantCmdArgs: `path/to/git -c credential.https://github.com.helper= -c credential.https://github.com.helper=!"gh" auth git-credential clone --bare https://github.com/cli/cli`,
			wantTarget:  "cli.git",
		},
		{
			name:        "bare clone with explicit target",
			args:        []string{"cli-bare", "--bare"},
			wantCmdArgs: `path/to/git -c credential.https://github.com.helper= -c credential.https://github.com.helper=!"gh" auth git-credential clone --bare https://github.com/cli/cli cli-bare`,
			wantTarget:  "cli-bare",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			target, err := client.Clone(context.Background(), "https://github.com/cli/cli", tt.args, tt.mods...)
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
			assert.Equal(t, tt.wantTarget, target)
		})
	}
}

func TestParseCloneArgs(t *testing.T) {
	type wanted struct {
		args []string
		dir  string
	}
	tests := []struct {
		name string
		args []string
		want wanted
	}{
		{
			name: "args and target",
			args: []string{"target_directory", "-o", "upstream", "--depth", "1"},
			want: wanted{
				args: []string{"-o", "upstream", "--depth", "1"},
				dir:  "target_directory",
			},
		},
		{
			name: "only args",
			args: []string{"-o", "upstream", "--depth", "1"},
			want: wanted{
				args: []string{"-o", "upstream", "--depth", "1"},
				dir:  "",
			},
		},
		{
			name: "only target",
			args: []string{"target_directory"},
			want: wanted{
				args: []string{},
				dir:  "target_directory",
			},
		},
		{
			name: "no args",
			args: []string{},
			want: wanted{
				args: []string{},
				dir:  "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, dir := parseCloneArgs(tt.args)
			got := wanted{args: args, dir: dir}
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestClientAddRemote(t *testing.T) {
	tests := []struct {
		title         string
		name          string
		url           string
		branches      []string
		dir           string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			title:       "fetch all",
			name:        "test",
			url:         "URL",
			dir:         "DIRECTORY",
			branches:    []string{},
			wantCmdArgs: `path/to/git -C DIRECTORY remote add test URL`,
		},
		{
			title:       "fetch specific branches only",
			name:        "test",
			url:         "URL",
			dir:         "DIRECTORY",
			branches:    []string{"trunk", "dev"},
			wantCmdArgs: `path/to/git -C DIRECTORY remote add -t trunk -t dev test URL`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				RepoDir:        tt.dir,
				commandContext: cmdCtx,
			}
			_, err := client.AddRemote(context.Background(), tt.name, tt.url, tt.branches)
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			assert.NoError(t, err)
		})
	}
}

func initRepo(t *testing.T, dir string) {
	errBuf := &bytes.Buffer{}
	inBuf := &bytes.Buffer{}
	outBuf := &bytes.Buffer{}
	client := Client{
		RepoDir: dir,
		Stderr:  errBuf,
		Stdin:   inBuf,
		Stdout:  outBuf,
	}
	cmd, err := client.Command(context.Background(), []string{"init", "--quiet"}...)
	assert.NoError(t, err)
	_, err = cmd.Output()
	assert.NoError(t, err)
}

type args string

type commandResult struct {
	ExitStatus int    `json:"exitStatus"`
	Stdout     string `json:"out"`
	Stderr     string `json:"err"`
}

type mockedCommands map[args]commandResult

// TestCommandMocking is an invoked test helper that emulates expected behavior for predefined shell commands, erroring when unexpected conditions are encountered.
func TestCommandMocking(t *testing.T) {
	if os.Getenv("GH_WANT_HELPER_PROCESS_RICH") != "1" {
		return
	}

	jsonVar, ok := os.LookupEnv("GH_HELPER_PROCESS_RICH_COMMANDS")
	if !ok {
		fmt.Fprint(os.Stderr, "missing GH_HELPER_PROCESS_RICH_COMMANDS")
		// Exit 1 is used for empty key values in the git config. This is non-breaking in those use cases,
		// so this is returning a non-zero exit code to avoid suppressing this error for those use cases.
		os.Exit(16)
	}

	var commands mockedCommands
	if err := json.Unmarshal([]byte(jsonVar), &commands); err != nil {
		fmt.Fprint(os.Stderr, "failed to unmarshal GH_HELPER_PROCESS_RICH_COMMANDS")
		// Exit 1 is used for empty key values in the git config. This is non-breaking in those use cases,
		// so this is returning a non-zero exit code to avoid suppressing this error for those use cases.
		os.Exit(16)
	}

	// The discarded args are those for the go test binary itself, e.g. `-test.run=TestHelperProcessRich`
	realArgs := os.Args[3:]

	commandResult, ok := commands[args(strings.Join(realArgs, " "))]
	if !ok {
		fmt.Fprintf(os.Stderr, "unexpected command: %s\n", strings.Join(realArgs, " "))
		// Exit 1 is used for empty key values in the git config. This is non-breaking in those use cases,
		// so this is returning a non-zero exit code to avoid suppressing this error for those use cases.
		os.Exit(16)
	}

	if commandResult.Stdout != "" {
		fmt.Fprint(os.Stdout, commandResult.Stdout)
	}

	if commandResult.Stderr != "" {
		fmt.Fprint(os.Stderr, commandResult.Stderr)
	}

	os.Exit(commandResult.ExitStatus)
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GH_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if err := func(args []string) error {
		fmt.Fprint(os.Stdout, os.Getenv("GH_HELPER_PROCESS_STDOUT"))
		exitStatus := os.Getenv("GH_HELPER_PROCESS_EXIT_STATUS")
		if exitStatus != "0" {
			return errors.New("error")
		}
		return nil
	}(os.Args[3:]); err != nil {
		fmt.Fprint(os.Stderr, os.Getenv("GH_HELPER_PROCESS_STDERR"))
		exitStatus := os.Getenv("GH_HELPER_PROCESS_EXIT_STATUS")
		i, err := strconv.Atoi(exitStatus)
		if err != nil {
			os.Exit(1)
		}
		os.Exit(i)
	}
	os.Exit(0)
}

func TestCredentialPatternFromGitURL(t *testing.T) {
	tests := []struct {
		name                  string
		gitURL                string
		wantErr               bool
		wantCredentialPattern CredentialPattern
	}{
		{
			name:   "Given a well formed gitURL, it returns the corresponding CredentialPattern",
			gitURL: "https://github.com/OWNER/REPO.git",
			wantCredentialPattern: CredentialPattern{
				pattern:     "https://github.com",
				allMatching: false,
			},
		},
		{
			name: "Given a malformed gitURL, it returns an error",
			// This pattern is copied from the tests in ParseURL
			// Unexpectedly, a non URL-like string did not error in ParseURL
			gitURL:  "ssh://git@[/tmp/git-repo",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentialPattern, err := CredentialPatternFromGitURL(tt.gitURL)
			if tt.wantErr {
				assert.ErrorContains(t, err, "failed to parse remote URL")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCredentialPattern, credentialPattern)
			}
		})
	}
}

func TestCredentialPatternFromHost(t *testing.T) {
	tests := []struct {
		name                  string
		host                  string
		wantCredentialPattern CredentialPattern
	}{
		{
			name: "Given a well formed host, it returns the corresponding CredentialPattern",
			host: "github.com",
			wantCredentialPattern: CredentialPattern{
				pattern:     "https://github.com",
				allMatching: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentialPattern := CredentialPatternFromHost(tt.host)
			require.Equal(t, tt.wantCredentialPattern, credentialPattern)
		})
	}
}

func TestPushDefault(t *testing.T) {
	t.Run("it parses valid values correctly", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			value               string
			expectedPushDefault PushDefault
		}{
			{"nothing", PushDefaultNothing},
			{"current", PushDefaultCurrent},
			{"upstream", PushDefaultUpstream},
			{"tracking", PushDefaultTracking},
			{"simple", PushDefaultSimple},
			{"matching", PushDefaultMatching},
		}

		for _, test := range tests {
			t.Run(test.value, func(t *testing.T) {
				t.Parallel()

				pushDefault, err := ParsePushDefault(test.value)
				require.NoError(t, err)
				assert.Equal(t, test.expectedPushDefault, pushDefault)
			})
		}
	})

	t.Run("it returns an error for invalid values", func(t *testing.T) {
		t.Parallel()

		_, err := ParsePushDefault("invalid")
		require.Error(t, err)
	})
}

func createCommandContext(t *testing.T, exitStatus int, stdout, stderr string) (*exec.Cmd, commandCtx) {
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=TestHelperProcess", "--")
	cmd.Env = []string{
		"GH_WANT_HELPER_PROCESS=1",
		fmt.Sprintf("GH_HELPER_PROCESS_STDOUT=%s", stdout),
		fmt.Sprintf("GH_HELPER_PROCESS_STDERR=%s", stderr),
		fmt.Sprintf("GH_HELPER_PROCESS_EXIT_STATUS=%v", exitStatus),
	}
	return cmd, func(ctx context.Context, exe string, args ...string) *exec.Cmd {
		cmd.Args = append(cmd.Args, exe)
		cmd.Args = append(cmd.Args, args...)
		return cmd
	}
}

func createMockedCommandContext(t *testing.T, commands mockedCommands) commandCtx {
	marshaledCommands, err := json.Marshal(commands)
	require.NoError(t, err)

	// invokes helper within current test binary, emulating desired behavior
	return func(ctx context.Context, exe string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=TestCommandMocking", "--")
		cmd.Env = []string{
			"GH_WANT_HELPER_PROCESS_RICH=1",
			fmt.Sprintf("GH_HELPER_PROCESS_RICH_COMMANDS=%s", string(marshaledCommands)),
		}

		cmd.Args = append(cmd.Args, exe)
		cmd.Args = append(cmd.Args, args...)
		return cmd
	}
}
