package factory

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_BaseRepo(t *testing.T) {
	tests := []struct {
		name       string
		remotes    git.RemoteSet
		override   string
		wantsErr   bool
		wantsName  string
		wantsOwner string
		wantsHost  string
	}{
		{
			name: "matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://nonsense.com/owner/repo.git"),
			},
			wantsName:  "repo",
			wantsOwner: "owner",
			wantsHost:  "nonsense.com",
		},
		{
			name: "no matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://test.com/owner/repo.git"),
			},
			wantsErr: true,
		},
		{
			name: "override with matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://test.com/owner/repo.git"),
			},
			override:   "test.com",
			wantsName:  "repo",
			wantsOwner: "owner",
			wantsHost:  "test.com",
		},
		{
			name: "override with no matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://nonsense.com/owner/repo.git"),
			},
			override: "test.com",
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New("1")
			rr := &remoteResolver{
				readRemotes: func() (git.RemoteSet, error) {
					return tt.remotes, nil
				},
				getConfig: func() (gh.Config, error) {
					cfg := &ghmock.ConfigMock{}
					cfg.AuthenticationFunc = func() gh.AuthConfig {
						authCfg := &config.AuthConfig{}
						hosts := []string{"nonsense.com"}
						if tt.override != "" {
							hosts = append([]string{tt.override}, hosts...)
						}
						authCfg.SetHosts(hosts)
						authCfg.SetActiveToken("", "")
						authCfg.SetDefaultHost("nonsense.com", "hosts")
						if tt.override != "" {
							authCfg.SetDefaultHost(tt.override, "GH_HOST")
						}
						return authCfg
					}
					return cfg, nil
				},
			}
			f.Remotes = rr.Resolver()
			f.BaseRepo = BaseRepoFunc(f)
			repo, err := f.BaseRepo()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantsName, repo.RepoName())
			assert.Equal(t, tt.wantsOwner, repo.RepoOwner())
			assert.Equal(t, tt.wantsHost, repo.RepoHost())
		})
	}
}

func Test_SmartBaseRepo(t *testing.T) {
	pu, _ := url.Parse("https://test.com/newowner/newrepo.git")

	tests := []struct {
		name       string
		remotes    git.RemoteSet
		override   string
		wantsErr   bool
		wantsName  string
		wantsOwner string
		wantsHost  string
		tty        bool
		httpStubs  func(*httpmock.Registry)
	}{
		{
			name: "override with matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://test.com/owner/repo.git"),
			},
			override:   "test.com",
			wantsName:  "repo",
			wantsOwner: "owner",
			wantsHost:  "test.com",
		},
		{
			name: "override with matching remote and base resolution",
			remotes: git.RemoteSet{
				&git.Remote{Name: "origin",
					Resolved: "base",
					FetchURL: pu,
					PushURL:  pu},
			},
			override:   "test.com",
			wantsName:  "newrepo",
			wantsOwner: "newowner",
			wantsHost:  "test.com",
		},
		{
			name: "override with matching remote and nonbase resolution",
			remotes: git.RemoteSet{
				&git.Remote{Name: "origin",
					Resolved: "johnny/test",
					FetchURL: pu,
					PushURL:  pu},
			},
			override:   "test.com",
			wantsName:  "test",
			wantsOwner: "johnny",
			wantsHost:  "test.com",
		},
		{
			name: "override with no matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://example.com/owner/repo.git"),
			},
			override: "test.com",
			wantsErr: true,
		},

		{
			name: "only one remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://github.com/owner/repo.git"),
			},
			wantsName:  "repo",
			wantsOwner: "owner",
			wantsHost:  "github.com",
			tty:        true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL("RepositoryNetwork"),
					httpmock.StringResponse(`
						{
						  "data": {
						    "viewer": {
						      "login": "someone"
						    },
						    "repo_000": {
						      "id": "MDEwOlJlcG9zaXRvcnkxMDM3MjM2Mjc=",
						      "name": "repo",
						      "owner": {
						        "login": "owner"
						      },
						      "viewerPermission": "READ",
						      "defaultBranchRef": {
						        "name": "master"
						      },
						      "isPrivate": false,
						      "parent": null
						    }
						  }
						}
					`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New("1")
			rr := &remoteResolver{
				readRemotes: func() (git.RemoteSet, error) {
					return tt.remotes, nil
				},
				getConfig: func() (gh.Config, error) {
					cfg := &ghmock.ConfigMock{}
					cfg.AuthenticationFunc = func() gh.AuthConfig {
						authCfg := &config.AuthConfig{}
						hosts := []string{"nonsense.com"}
						if tt.override != "" {
							hosts = append([]string{tt.override}, hosts...)
						}
						authCfg.SetHosts(hosts)
						authCfg.SetActiveToken("", "")
						authCfg.SetDefaultHost("nonsense.com", "hosts")
						if tt.override != "" {
							authCfg.SetDefaultHost(tt.override, "GH_HOST")
						}
						return authCfg
					}
					return cfg, nil
				},
			}
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			f.IOStreams = ios
			f.HttpClient = func() (*http.Client, error) { return &http.Client{Transport: reg}, nil }
			f.Remotes = rr.Resolver()
			f.BaseRepo = SmartBaseRepoFunc(f)
			repo, err := f.BaseRepo()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantsName, repo.RepoName())
			assert.Equal(t, tt.wantsOwner, repo.RepoOwner())
			assert.Equal(t, tt.wantsHost, repo.RepoHost())
		})
	}
}

// Defined in pkg/cmdutil/repo_override.go but test it along with other BaseRepo functions
func Test_OverrideBaseRepo(t *testing.T) {
	tests := []struct {
		name        string
		remotes     git.RemoteSet
		config      gh.Config
		envOverride string
		argOverride string
		wantsErr    bool
		wantsName   string
		wantsOwner  string
		wantsHost   string
	}{
		{
			name:        "override from argument",
			argOverride: "override/test",
			wantsHost:   "github.com",
			wantsOwner:  "override",
			wantsName:   "test",
		},
		{
			name:        "override from environment",
			envOverride: "somehost.com/override/test",
			wantsHost:   "somehost.com",
			wantsOwner:  "override",
			wantsName:   "test",
		},
		{
			name: "no override",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://nonsense.com/owner/repo.git"),
			},
			config:     defaultConfig(),
			wantsHost:  "nonsense.com",
			wantsOwner: "owner",
			wantsName:  "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envOverride != "" {
				t.Setenv("GH_REPO", tt.envOverride)
			}
			f := New("1")
			rr := &remoteResolver{
				readRemotes: func() (git.RemoteSet, error) {
					return tt.remotes, nil
				},
				getConfig: func() (gh.Config, error) {
					return tt.config, nil
				},
			}
			f.Remotes = rr.Resolver()
			f.BaseRepo = cmdutil.OverrideBaseRepoFunc(f, tt.argOverride)
			repo, err := f.BaseRepo()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantsName, repo.RepoName())
			assert.Equal(t, tt.wantsOwner, repo.RepoOwner())
			assert.Equal(t, tt.wantsHost, repo.RepoHost())
		})
	}
}

func Test_ioStreams_pager(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		config    gh.Config
		wantPager string
	}{
		{
			name: "GH_PAGER and PAGER set",
			env: map[string]string{
				"GH_PAGER": "GH_PAGER",
				"PAGER":    "PAGER",
			},
			wantPager: "GH_PAGER",
		},
		{
			name: "GH_PAGER and config pager set",
			env: map[string]string{
				"GH_PAGER": "GH_PAGER",
			},
			config:    pagerConfig(),
			wantPager: "GH_PAGER",
		},
		{
			name: "config pager and PAGER set",
			env: map[string]string{
				"PAGER": "PAGER",
			},
			config:    pagerConfig(),
			wantPager: "CONFIG_PAGER",
		},
		{
			name: "only PAGER set",
			env: map[string]string{
				"PAGER": "PAGER",
			},
			wantPager: "PAGER",
		},
		{
			name: "GH_PAGER set to blank string",
			env: map[string]string{
				"GH_PAGER": "",
				"PAGER":    "PAGER",
			},
			wantPager: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != nil {
				for k, v := range tt.env {
					t.Setenv(k, v)
				}
			}
			f := New("1")
			f.Config = func() (gh.Config, error) {
				if tt.config == nil {
					return config.NewBlankConfig(), nil
				} else {
					return tt.config, nil
				}
			}
			io := ioStreams(f)
			assert.Equal(t, tt.wantPager, io.GetPager())
		})
	}
}

func Test_ioStreams_prompt(t *testing.T) {
	tests := []struct {
		name           string
		config         gh.Config
		promptDisabled bool
		env            map[string]string
	}{
		{
			name:           "default config",
			promptDisabled: false,
		},
		{
			name:           "config with prompt disabled",
			config:         disablePromptConfig(),
			promptDisabled: true,
		},
		{
			name:           "prompt disabled via GH_PROMPT_DISABLED env var",
			env:            map[string]string{"GH_PROMPT_DISABLED": "1"},
			promptDisabled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != nil {
				for k, v := range tt.env {
					t.Setenv(k, v)
				}
			}
			f := New("1")
			f.Config = func() (gh.Config, error) {
				if tt.config == nil {
					return config.NewBlankConfig(), nil
				} else {
					return tt.config, nil
				}
			}
			io := ioStreams(f)
			assert.Equal(t, tt.promptDisabled, io.GetNeverPrompt())
		})
	}
}

func Test_ioStreams_spinnerDisabled(t *testing.T) {
	tests := []struct {
		name            string
		config          gh.Config
		spinnerDisabled bool
		env             map[string]string
	}{
		{
			name:            "default config",
			spinnerDisabled: false,
		},
		{
			name:            "config with spinner disabled",
			config:          disableSpinnersConfig(),
			spinnerDisabled: true,
		},
		{
			name:            "config with spinner enabled",
			config:          enableSpinnersConfig(),
			spinnerDisabled: false,
		},
		{
			name:            "spinner disabled via GH_SPINNER_DISABLED env var = 0",
			env:             map[string]string{"GH_SPINNER_DISABLED": "0"},
			spinnerDisabled: false,
		},
		{
			name:            "spinner disabled via GH_SPINNER_DISABLED env var = false",
			env:             map[string]string{"GH_SPINNER_DISABLED": "false"},
			spinnerDisabled: false,
		},
		{
			name:            "spinner disabled via GH_SPINNER_DISABLED env var = no",
			env:             map[string]string{"GH_SPINNER_DISABLED": "no"},
			spinnerDisabled: false,
		},
		{
			name:            "spinner enabled via GH_SPINNER_DISABLED env var = 1",
			env:             map[string]string{"GH_SPINNER_DISABLED": "1"},
			spinnerDisabled: true,
		},
		{
			name:            "spinner enabled via GH_SPINNER_DISABLED env var = true",
			env:             map[string]string{"GH_SPINNER_DISABLED": "true"},
			spinnerDisabled: true,
		},
		{
			name:            "config enabled but env disabled, respects env",
			config:          enableSpinnersConfig(),
			env:             map[string]string{"GH_SPINNER_DISABLED": "true"},
			spinnerDisabled: true,
		},
		{
			name:            "config disabled but env enabled, respects env",
			config:          disableSpinnersConfig(),
			env:             map[string]string{"GH_SPINNER_DISABLED": "false"},
			spinnerDisabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			f := New("1")
			f.Config = func() (gh.Config, error) {
				if tt.config == nil {
					return config.NewBlankConfig(), nil
				} else {
					return tt.config, nil
				}
			}
			io := ioStreams(f)
			assert.Equal(t, tt.spinnerDisabled, io.GetSpinnerDisabled())
		})
	}
}

func Test_ioStreams_accessiblePrompterEnabled(t *testing.T) {
	tests := []struct {
		name                      string
		config                    gh.Config
		accessiblePrompterEnabled bool
		env                       map[string]string
	}{
		{
			name:                      "default config",
			accessiblePrompterEnabled: false,
		},
		{
			name:                      "config with accessible prompter enabled",
			config:                    enableAccessiblePrompterConfig(),
			accessiblePrompterEnabled: true,
		},
		{
			name:                      "config with accessible prompter disabled",
			config:                    disableAccessiblePrompterConfig(),
			accessiblePrompterEnabled: false,
		},
		{
			name:                      "accessible prompter enabled via GH_ACCESSIBLE_PROMPTER env var = 1",
			env:                       map[string]string{"GH_ACCESSIBLE_PROMPTER": "1"},
			accessiblePrompterEnabled: true,
		},
		{
			name:                      "accessible prompter enabled via GH_ACCESSIBLE_PROMPTER env var = true",
			env:                       map[string]string{"GH_ACCESSIBLE_PROMPTER": "true"},
			accessiblePrompterEnabled: true,
		},
		{
			name:                      "accessible prompter disabled via GH_ACCESSIBLE_PROMPTER env var = 0",
			env:                       map[string]string{"GH_ACCESSIBLE_PROMPTER": "0"},
			accessiblePrompterEnabled: false,
		},
		{
			name:                      "config disabled but env enabled, respects env",
			config:                    disableAccessiblePrompterConfig(),
			env:                       map[string]string{"GH_ACCESSIBLE_PROMPTER": "true"},
			accessiblePrompterEnabled: true,
		},
		{
			name:                      "config enabled but env disabled, respects env",
			config:                    enableAccessiblePrompterConfig(),
			env:                       map[string]string{"GH_ACCESSIBLE_PROMPTER": "false"},
			accessiblePrompterEnabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			f := New("1")
			f.Config = func() (gh.Config, error) {
				if tt.config == nil {
					return config.NewBlankConfig(), nil
				} else {
					return tt.config, nil
				}
			}
			io := ioStreams(f)
			assert.Equal(t, tt.accessiblePrompterEnabled, io.AccessiblePrompterEnabled())
		})
	}
}

func Test_ioStreams_colorLabels(t *testing.T) {
	tests := []struct {
		name               string
		config             gh.Config
		colorLabelsEnabled bool
		env                map[string]string
	}{
		{
			name:               "default config",
			colorLabelsEnabled: false,
		},
		{
			name:               "config with colorLabels enabled",
			config:             enableColorLabelsConfig(),
			colorLabelsEnabled: true,
		},
		{
			name:               "config with colorLabels disabled",
			config:             disableColorLabelsConfig(),
			colorLabelsEnabled: false,
		},
		{
			name:               "colorLabels enabled via `1` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "1"},
			colorLabelsEnabled: true,
		},
		{
			name:               "colorLabels enabled via `true` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "true"},
			colorLabelsEnabled: true,
		},
		{
			name:               "colorLabels enabled via `yes` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "yes"},
			colorLabelsEnabled: true,
		},
		{
			name:               "colorLabels disable via empty string in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": ""},
			colorLabelsEnabled: false,
		},
		{
			name:               "colorLabels disabled via `0` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "0"},
			colorLabelsEnabled: false,
		},
		{
			name:               "colorLabels disabled via `false` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "false"},
			colorLabelsEnabled: false,
		},
		{
			name:               "colorLabels disabled via `no` in GH_COLOR_LABELS env var",
			env:                map[string]string{"GH_COLOR_LABELS": "no"},
			colorLabelsEnabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != nil {
				for k, v := range tt.env {
					t.Setenv(k, v)
				}
			}
			f := New("1")
			f.Config = func() (gh.Config, error) {
				if tt.config == nil {
					return config.NewBlankConfig(), nil
				} else {
					return tt.config, nil
				}
			}
			io := ioStreams(f)
			assert.Equal(t, tt.colorLabelsEnabled, io.ColorLabels())
		})
	}
}

func TestSSOURL(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		sso        string
		wantStderr string
		wantSSO    string
	}{
		{
			name:       "SSO challenge in response header",
			host:       "github.com",
			sso:        "required; url=https://github.com/login/sso?return_to=xyz&param=123abc; another",
			wantStderr: "",
			wantSSO:    "https://github.com/login/sso?return_to=xyz&param=123abc",
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sso := r.URL.Query().Get("sso"); sso != "" {
			w.Header().Set("X-GitHub-SSO", sso)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New("1")
			f.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}
			ios, _, _, stderr := iostreams.Test()
			f.IOStreams = ios
			client, err := httpClientFunc(f, "v1.2.3")()
			require.NoError(t, err)
			req, err := http.NewRequest("GET", ts.URL, nil)
			if tt.sso != "" {
				q := req.URL.Query()
				q.Set("sso", tt.sso)
				req.URL.RawQuery = q.Encode()
			}
			req.Host = tt.host
			require.NoError(t, err)

			res, err := client.Do(req)
			require.NoError(t, err)

			assert.Equal(t, 204, res.StatusCode)
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantSSO, SSOURL())
		})
	}
}

func TestNewGitClient(t *testing.T) {
	tests := []struct {
		name          string
		config        gh.Config
		executable    string
		wantAuthHosts []string
		wantGhPath    string
	}{
		{
			name:          "creates git client",
			config:        defaultConfig(),
			executable:    filepath.Join("path", "to", "gh"),
			wantAuthHosts: []string{"nonsense.com"},
			wantGhPath:    filepath.Join("path", "to", "gh"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New("1")
			f.Config = func() (gh.Config, error) {
				if tt.config == nil {
					return config.NewBlankConfig(), nil
				} else {
					return tt.config, nil
				}
			}
			f.ExecutableName = tt.executable
			ios, _, _, _ := iostreams.Test()
			f.IOStreams = ios
			c := newGitClient(f)
			assert.Equal(t, tt.wantGhPath, c.GhPath)
			assert.Equal(t, ios.In, c.Stdin)
			assert.Equal(t, ios.Out, c.Stdout)
			assert.Equal(t, ios.ErrOut, c.Stderr)
		})
	}
}

func defaultConfig() *ghmock.ConfigMock {
	cfg := config.NewFromString("")
	cfg.Set("nonsense.com", "oauth_token", "BLAH")
	return cfg
}

func pagerConfig() gh.Config {
	return config.NewFromString("pager: CONFIG_PAGER")
}

func disablePromptConfig() gh.Config {
	return config.NewFromString("prompt: disabled")
}

func enableAccessiblePrompterConfig() gh.Config {
	return config.NewFromString("accessible_prompter: enabled")
}

func disableAccessiblePrompterConfig() gh.Config {
	return config.NewFromString("accessible_prompter: disabled")
}

func disableSpinnersConfig() gh.Config {
	return config.NewFromString("spinner: disabled")
}

func enableSpinnersConfig() gh.Config {
	return config.NewFromString("spinner: enabled")
}

func disableColorLabelsConfig() gh.Config {
	return config.NewFromString("color_labels: disabled")
}

func enableColorLabelsConfig() gh.Config {
	return config.NewFromString("color_labels: enabled")
}
