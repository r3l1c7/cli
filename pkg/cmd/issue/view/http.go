package view

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

func preloadIssueComments(client *http.Client, repo ghrepo.Interface, issue *api.Issue) error {
	type response struct {
		Node struct {
			Issue struct {
				Comments *api.Comments `graphql:"comments(first: 100, after: $endCursor)"`
			} `graphql:"...on Issue"`
			PullRequest struct {
				Comments *api.Comments `graphql:"comments(first: 100, after: $endCursor)"`
			} `graphql:"...on PullRequest"`
		} `graphql:"node(id: $id)"`
	}

	variables := map[string]interface{}{
		"id":        githubv4.ID(issue.ID),
		"endCursor": (*githubv4.String)(nil),
	}
	if issue.Comments.PageInfo.HasNextPage {
		variables["endCursor"] = githubv4.String(issue.Comments.PageInfo.EndCursor)
	} else {
		issue.Comments.Nodes = issue.Comments.Nodes[0:0]
	}

	gql := api.NewClientFromHTTP(client)
	for {
		var query response
		err := gql.Query(repo.RepoHost(), "CommentsForIssue", &query, variables)
		if err != nil {
			return err
		}

		comments := query.Node.Issue.Comments
		if comments == nil {
			comments = query.Node.PullRequest.Comments
		}

		issue.Comments.Nodes = append(issue.Comments.Nodes, comments.Nodes...)
		if !comments.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(comments.PageInfo.EndCursor)
	}

	issue.Comments.PageInfo.HasNextPage = false
	return nil
}

func preloadClosedByPullRequestsReferences(client *http.Client, repo ghrepo.Interface, issue *api.Issue) error {
	if !issue.ClosedByPullRequestsReferences.PageInfo.HasNextPage {
		return nil
	}

	type response struct {
		Node struct {
			Issue struct {
				ClosedByPullRequestsReferences api.ClosedByPullRequestsReferences `graphql:"closedByPullRequestsReferences(first: 100, after: $endCursor)"`
			} `graphql:"...on Issue"`
		} `graphql:"node(id: $id)"`
	}

	variables := map[string]interface{}{
		"id":        githubv4.ID(issue.ID),
		"endCursor": githubv4.String(issue.ClosedByPullRequestsReferences.PageInfo.EndCursor),
	}

	gql := api.NewClientFromHTTP(client)

	for {
		var query response
		err := gql.Query(repo.RepoHost(), "closedByPullRequestsReferences", &query, variables)
		if err != nil {
			return err
		}

		issue.ClosedByPullRequestsReferences.Nodes = append(issue.ClosedByPullRequestsReferences.Nodes, query.Node.Issue.ClosedByPullRequestsReferences.Nodes...)

		if !query.Node.Issue.ClosedByPullRequestsReferences.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Node.Issue.ClosedByPullRequestsReferences.PageInfo.EndCursor)
	}

	issue.ClosedByPullRequestsReferences.PageInfo.HasNextPage = false
	return nil
}
