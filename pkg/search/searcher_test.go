package search

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestSearcherCode(t *testing.T) {
	query := Query{
		Keywords: []string{"keyword"},
		Kind:     "code",
		Limit:    30,
		Qualifiers: Qualifiers{
			Language: "go",
		},
	}

	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"30"},
		"q":        []string{"keyword language:go"},
	}

	tests := []struct {
		name      string
		host      string
		query     Query
		result    CodeResult
		wantErr   bool
		errMsg    string
		httpStubs func(reg *httpmock.Registry)
	}{
		{
			name:  "searches code",
			query: query,
			result: CodeResult{
				IncompleteResults: false,
				Items:             []Code{{Name: "file.go"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/code", values),
					httpmock.JSONResponse(map[string]interface{}{
						"incomplete_results": false,
						"total_count":        1,
						"items": []interface{}{
							map[string]interface{}{
								"name": "file.go",
							},
						},
					}),
				)
			},
		},
		{
			name:  "searches code for enterprise host",
			host:  "enterprise.com",
			query: query,
			result: CodeResult{
				IncompleteResults: false,
				Items:             []Code{{Name: "file.go"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "api/v3/search/code", values),
					httpmock.JSONResponse(map[string]interface{}{
						"incomplete_results": false,
						"total_count":        1,
						"items": []interface{}{
							map[string]interface{}{
								"name": "file.go",
							},
						},
					}),
				)
			},
		},
		{
			name:  "paginates results",
			query: query,
			result: CodeResult{
				IncompleteResults: false,
				Items:             []Code{{Name: "file.go"}, {Name: "file2.go"}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/code", values)
				firstRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        2,
					"items": []interface{}{
						map[string]interface{}{
							"name": "file.go",
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/code?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/code", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"q":        []string{"keyword language:go"},
				})
				secondRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        2,
					"items": []interface{}{
						map[string]interface{}{
							"name": "file2.go",
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "collect full and partial pages under total number of matching search results",
			query: Query{
				Keywords: []string{"keyword"},
				Kind:     "code",
				Limit:    110,
				Qualifiers: Qualifiers{
					Language: "go",
				},
			},
			result: CodeResult{
				IncompleteResults: false,
				Items: initialize(0, 110, func(i int) Code {
					return Code{
						Name: fmt.Sprintf("name%d.go", i),
					}
				}),
				Total: 287,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/code", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"100"},
					"q":        []string{"keyword language:go"},
				})
				firstRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(0, 100, func(i int) interface{} {
						return map[string]interface{}{
							"name": fmt.Sprintf("name%d.go", i),
						}
					}),
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/code?page=2&per_page=100&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/code", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"100"},
					"q":        []string{"keyword language:go"},
				})
				secondRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(100, 200, func(i int) interface{} {
						return map[string]interface{}{
							"name": fmt.Sprintf("name%d.go", i),
						}
					}),
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name:    "handles search errors",
			query:   query,
			wantErr: true,
			errMsg: heredoc.Doc(`
				Invalid search query "keyword language:go".
				"blah" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.`),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/code", values),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422,
							`{
								"message": "Validation Failed",
								"errors": [
                  {
                    "message":"\"blah\" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.",
                    "resource":"Search",
                    "field":"q",
                    "code":"invalid"
                  }
                ],
                "documentation_url":"https://developer.github.com/v3/search/"
							}`,
						), "Content-Type", "application/json"),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := &http.Client{Transport: reg}
			if tt.host == "" {
				tt.host = "github.com"
			}
			searcher := NewSearcher(client, tt.host)
			result, err := searcher.Code(tt.query)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.result, result)
		})
	}
}

func TestSearcherCommits(t *testing.T) {
	query := Query{
		Keywords: []string{"keyword"},
		Kind:     "commits",
		Limit:    30,
		Order:    "desc",
		Sort:     "committer-date",
		Qualifiers: Qualifiers{
			Author:        "foobar",
			CommitterDate: ">2021-02-28",
		},
	}

	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"30"},
		"order":    []string{"desc"},
		"sort":     []string{"committer-date"},
		"q":        []string{"keyword author:foobar committer-date:>2021-02-28"},
	}

	tests := []struct {
		name      string
		host      string
		query     Query
		result    CommitsResult
		wantErr   bool
		errMsg    string
		httpStubs func(*httpmock.Registry)
	}{
		{
			name:  "searches commits",
			query: query,
			result: CommitsResult{
				IncompleteResults: false,
				Items:             []Commit{{Sha: "abc"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/commits", values),
					httpmock.JSONResponse(map[string]interface{}{
						"incomplete_results": false,
						"total_count":        1,
						"items": []interface{}{
							map[string]interface{}{
								"sha": "abc",
							},
						},
					}),
				)
			},
		},
		{
			name:  "searches commits for enterprise host",
			host:  "enterprise.com",
			query: query,
			result: CommitsResult{
				IncompleteResults: false,
				Items:             []Commit{{Sha: "abc"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "api/v3/search/commits", values),
					httpmock.JSONResponse(map[string]interface{}{
						"incomplete_results": false,
						"total_count":        1,
						"items": []interface{}{
							map[string]interface{}{
								"sha": "abc",
							},
						},
					}),
				)
			},
		},
		{
			name:  "paginates results",
			query: query,
			result: CommitsResult{
				IncompleteResults: false,
				Items:             []Commit{{Sha: "abc"}, {Sha: "def"}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/commits", values)
				firstRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        2,
					"items": []interface{}{
						map[string]interface{}{
							"sha": "abc",
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/commits?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/commits", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"committer-date"},
					"q":        []string{"keyword author:foobar committer-date:>2021-02-28"},
				})
				secondRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        2,
					"items": []interface{}{
						map[string]interface{}{
							"sha": "def",
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "collect full and partial pages under total number of matching search results",
			query: Query{
				Keywords: []string{"keyword"},
				Kind:     "commits",
				Limit:    110,
				Order:    "desc",
				Sort:     "committer-date",
				Qualifiers: Qualifiers{
					Author:        "foobar",
					CommitterDate: ">2021-02-28",
				},
			},
			result: CommitsResult{
				IncompleteResults: false,
				Items: initialize(0, 110, func(i int) Commit {
					return Commit{
						Sha: strconv.Itoa(i),
					}
				}),
				Total: 287,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/commits", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"committer-date"},
					"q":        []string{"keyword author:foobar committer-date:>2021-02-28"},
				})
				firstRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(0, 100, func(i int) map[string]interface{} {
						return map[string]interface{}{
							"sha": strconv.Itoa(i),
						}
					}),
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/commits?page=2&per_page=100&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/commits", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"committer-date"},
					"q":        []string{"keyword author:foobar committer-date:>2021-02-28"},
				})
				secondRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(100, 200, func(i int) map[string]interface{} {
						return map[string]interface{}{
							"sha": strconv.Itoa(i),
						}
					}),
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name:    "handles search errors",
			query:   query,
			wantErr: true,
			errMsg: heredoc.Doc(`
				Invalid search query "keyword author:foobar committer-date:>2021-02-28".
				"blah" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.`),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/commits", values),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422,
							`{
                "message":"Validation Failed",
                "errors":[
                  {
                    "message":"\"blah\" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.",
                    "resource":"Search",
                    "field":"q",
                    "code":"invalid"
                  }
                ],
                "documentation_url":"https://docs.github.com/v3/search/"
              }`,
						), "Content-Type", "application/json"),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := &http.Client{Transport: reg}
			if tt.host == "" {
				tt.host = "github.com"
			}
			searcher := NewSearcher(client, tt.host)
			result, err := searcher.Commits(tt.query)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.result, result)
		})
	}
}

func TestSearcherRepositories(t *testing.T) {
	query := Query{
		Keywords: []string{"keyword"},
		Kind:     "repositories",
		Limit:    30,
		Order:    "desc",
		Sort:     "stars",
		Qualifiers: Qualifiers{
			Stars: ">=5",
			Topic: []string{"topic"},
		},
	}

	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"30"},
		"order":    []string{"desc"},
		"sort":     []string{"stars"},
		"q":        []string{"keyword stars:>=5 topic:topic"},
	}

	tests := []struct {
		name      string
		host      string
		query     Query
		result    RepositoriesResult
		wantErr   bool
		errMsg    string
		httpStubs func(*httpmock.Registry)
	}{
		{
			name:  "searches repositories",
			query: query,
			result: RepositoriesResult{
				IncompleteResults: false,
				Items:             []Repository{{Name: "test"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/repositories", values),
					httpmock.JSONResponse(map[string]interface{}{
						"incomplete_results": false,
						"total_count":        1,
						"items": []interface{}{
							map[string]interface{}{
								"name": "test",
							},
						},
					}),
				)
			},
		},
		{
			name:  "searches repositories for enterprise host",
			host:  "enterprise.com",
			query: query,
			result: RepositoriesResult{
				IncompleteResults: false,
				Items:             []Repository{{Name: "test"}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "api/v3/search/repositories", values),
					httpmock.JSONResponse(map[string]interface{}{
						"incomplete_results": false,
						"total_count":        1,
						"items": []interface{}{
							map[string]interface{}{
								"name": "test",
							},
						},
					}),
				)
			},
		},
		{
			name:  "paginates results",
			query: query,
			result: RepositoriesResult{
				IncompleteResults: false,
				Items:             []Repository{{Name: "test"}, {Name: "cli"}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/repositories", values)
				firstRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        2,
					"items": []interface{}{
						map[string]interface{}{
							"name": "test",
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/repositories?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/repositories", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"stars"},
					"q":        []string{"keyword stars:>=5 topic:topic"},
				})
				secondRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        2,
					"items": []interface{}{
						map[string]interface{}{
							"name": "cli",
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "collect full and partial pages under total number of matching search results",
			query: Query{
				Keywords: []string{"keyword"},
				Kind:     "repositories",
				Limit:    110,
				Order:    "desc",
				Sort:     "stars",
				Qualifiers: Qualifiers{
					Stars: ">=5",
					Topic: []string{"topic"},
				},
			},
			result: RepositoriesResult{
				IncompleteResults: false,
				Items: initialize(0, 110, func(i int) Repository {
					return Repository{
						Name: fmt.Sprintf("name%d", i),
					}
				}),
				Total: 287,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/repositories", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"stars"},
					"q":        []string{"keyword stars:>=5 topic:topic"},
				})
				firstRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(0, 100, func(i int) interface{} {
						return map[string]interface{}{
							"name": fmt.Sprintf("name%d", i),
						}
					}),
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/repositories?page=2&per_page=100&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/repositories", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"stars"},
					"q":        []string{"keyword stars:>=5 topic:topic"},
				})
				secondRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(100, 200, func(i int) interface{} {
						return map[string]interface{}{
							"name": fmt.Sprintf("name%d", i),
						}
					}),
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name:    "handles search errors",
			query:   query,
			wantErr: true,
			errMsg: heredoc.Doc(`
				Invalid search query "keyword stars:>=5 topic:topic".
				"blah" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.`),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/repositories", values),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422,
							`{
                "message":"Validation Failed",
                "errors":[
                  {
                    "message":"\"blah\" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.",
                    "resource":"Search",
                    "field":"q",
                    "code":"invalid"
                  }
                ],
                "documentation_url":"https://docs.github.com/v3/search/"
              }`,
						), "Content-Type", "application/json"),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := &http.Client{Transport: reg}
			if tt.host == "" {
				tt.host = "github.com"
			}
			searcher := NewSearcher(client, tt.host)
			result, err := searcher.Repositories(tt.query)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.result, result)
		})
	}
}

func TestSearcherIssues(t *testing.T) {
	query := Query{
		Keywords: []string{"keyword"},
		Kind:     "issues",
		Limit:    30,
		Order:    "desc",
		Sort:     "comments",
		Qualifiers: Qualifiers{
			Language: "go",
			Is:       []string{"public", "locked"},
		},
	}

	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"30"},
		"order":    []string{"desc"},
		"sort":     []string{"comments"},
		"q":        []string{"keyword is:locked is:public language:go"},
	}

	tests := []struct {
		name      string
		host      string
		query     Query
		result    IssuesResult
		wantErr   bool
		errMsg    string
		httpStubs func(*httpmock.Registry)
	}{
		{
			name:  "searches issues",
			query: query,
			result: IssuesResult{
				IncompleteResults: false,
				Items:             []Issue{{Number: 1234}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/issues", values),
					httpmock.JSONResponse(map[string]interface{}{
						"incomplete_results": false,
						"total_count":        1,
						"items": []interface{}{
							map[string]interface{}{
								"number": 1234,
							},
						},
					}),
				)
			},
		},
		{
			name:  "searches issues for enterprise host",
			host:  "enterprise.com",
			query: query,
			result: IssuesResult{
				IncompleteResults: false,
				Items:             []Issue{{Number: 1234}},
				Total:             1,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "api/v3/search/issues", values),
					httpmock.JSONResponse(map[string]interface{}{
						"incomplete_results": false,
						"total_count":        1,
						"items": []interface{}{
							map[string]interface{}{
								"number": 1234,
							},
						},
					}),
				)
			},
		},
		{
			name:  "paginates results",
			query: query,
			result: IssuesResult{
				IncompleteResults: false,
				Items:             []Issue{{Number: 1234}, {Number: 5678}},
				Total:             2,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/issues", values)
				firstRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        2,
					"items": []interface{}{
						map[string]interface{}{
							"number": 1234,
						},
					},
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/issues?page=2&per_page=30&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/issues", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"30"},
					"order":    []string{"desc"},
					"sort":     []string{"comments"},
					"q":        []string{"keyword is:locked is:public language:go"},
				})
				secondRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        2,
					"items": []interface{}{
						map[string]interface{}{
							"number": 5678,
						},
					},
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name: "collect full and partial pages under total number of matching search results",
			query: Query{
				Keywords: []string{"keyword"},
				Kind:     "issues",
				Limit:    110,
				Order:    "desc",
				Sort:     "comments",
				Qualifiers: Qualifiers{
					Language: "go",
					Is:       []string{"public", "locked"},
				},
			},
			result: IssuesResult{
				IncompleteResults: false,
				Items: initialize(0, 110, func(i int) Issue {
					return Issue{
						Number: i,
					}
				}),
				Total: 287,
			},
			httpStubs: func(reg *httpmock.Registry) {
				firstReq := httpmock.QueryMatcher("GET", "search/issues", url.Values{
					"page":     []string{"1"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"comments"},
					"q":        []string{"keyword is:locked is:public language:go"},
				})
				firstRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(0, 100, func(i int) interface{} {
						return map[string]interface{}{
							"number": i,
						}
					}),
				})
				firstRes = httpmock.WithHeader(firstRes, "Link", `<https://api.github.com/search/issues?page=2&per_page=100&q=org%3Agithub>; rel="next"`)
				secondReq := httpmock.QueryMatcher("GET", "search/issues", url.Values{
					"page":     []string{"2"},
					"per_page": []string{"100"},
					"order":    []string{"desc"},
					"sort":     []string{"comments"},
					"q":        []string{"keyword is:locked is:public language:go"},
				})
				secondRes := httpmock.JSONResponse(map[string]interface{}{
					"incomplete_results": false,
					"total_count":        287,
					"items": initialize(100, 200, func(i int) interface{} {
						return map[string]interface{}{
							"number": i,
						}
					}),
				})
				reg.Register(firstReq, firstRes)
				reg.Register(secondReq, secondRes)
			},
		},
		{
			name:    "handles search errors",
			query:   query,
			wantErr: true,
			errMsg: heredoc.Doc(`
				Invalid search query "keyword is:locked is:public language:go".
				"blah" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.`),
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "search/issues", values),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422,
							`{
                "message":"Validation Failed",
                "errors":[
                  {
                    "message":"\"blah\" is not a recognized date/time format. Please provide an ISO 8601 date/time value, such as YYYY-MM-DD.",
                    "resource":"Search",
                    "field":"q",
                    "code":"invalid"
                  }
                ],
                "documentation_url":"https://docs.github.com/v3/search/"
              }`,
						), "Content-Type", "application/json"),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := &http.Client{Transport: reg}
			if tt.host == "" {
				tt.host = "github.com"
			}
			searcher := NewSearcher(client, tt.host)
			result, err := searcher.Issues(tt.query)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.result, result)
		})
	}
}

func TestSearcherURL(t *testing.T) {
	query := Query{
		Keywords: []string{"keyword"},
		Kind:     "repositories",
		Limit:    30,
		Order:    "desc",
		Sort:     "stars",
		Qualifiers: Qualifiers{
			Stars: ">=5",
			Topic: []string{"topic"},
		},
	}

	tests := []struct {
		name  string
		host  string
		query Query
		url   string
	}{
		{
			name:  "outputs encoded query url",
			query: query,
			url:   "https://github.com/search?order=desc&q=keyword+stars%3A%3E%3D5+topic%3Atopic&sort=stars&type=repositories",
		},
		{
			name:  "supports enterprise hosts",
			host:  "enterprise.com",
			query: query,
			url:   "https://enterprise.com/search?order=desc&q=keyword+stars%3A%3E%3D5+topic%3Atopic&sort=stars&type=repositories",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.host == "" {
				tt.host = "github.com"
			}
			searcher := NewSearcher(nil, tt.host)
			assert.Equal(t, tt.url, searcher.URL(tt.query))
		})
	}
}

// initialize generate slices over a range for test scenarios using the provided initializer.
func initialize[T any](start int, stop int, initializer func(i int) T) []T {
	results := make([]T, 0, (stop - start))
	for i := start; i < stop; i++ {
		results = append(results, initializer(i))
	}
	return results
}
