package spam

import (
	"fmt"

	"github.com/cli/go-gh"
	"github.com/cli/go-gh/pkg/api"
)

// GitHub User with profile info and contribution stats
type User struct {
	Name               string
	CreatedAt          string
	Bio                string
	Followers          int
	Following          int
	TotalContributions int
	ReposContributed   int
}

type Issue struct {
	Number            int
	Title             string
	Body              string
	Author            struct{ Login string }
	CreatedAt         string
	AuthorAssociation string
	IsSpam            bool
}

// Gets summary of GitHub user's account and contributions
func GetUserStats(username string) (User, error) {
	usr := User{}
	opts := &api.ClientOptions{EnableCache: true}
	client, err := gh.GQLClient(opts)
	if err != nil {
		return usr, err
	}

	query := `query GetUserStats($username: String!) {
  user(login: $username) {
    createdAt
    bio
    followers{ totalCount }
    following{ totalCount }
    contributionsCollection {
      contributionCalendar { totalContributions }
    }
    repositoriesContributedTo(
		first:100, 
		contributionTypes: [COMMIT, ISSUE, PULL_REQUEST], 
		orderBy: {field: UPDATED_AT,direction: DESC}){
      totalCount
    }
  }
}`

	variables := map[string]interface{}{"username": username}
	resp := struct {
		User struct {
			CreatedAt               string
			Bio                     string
			Followers               struct{ TotalCount int }
			Following               struct{ TotalCount int }
			ContributionsCollection struct {
				ContributionCalendar struct{ TotalContributions int }
			}
			RepositoriesContributedTo struct{ TotalCount int }
		}
	}{}
	err = client.Do(query, variables, &resp)
	if err != nil {
		return usr, err
	}

	usr = User{
		Name:               username,
		CreatedAt:          resp.User.CreatedAt,
		Followers:          resp.User.Followers.TotalCount,
		Following:          resp.User.Following.TotalCount,
		TotalContributions: resp.User.ContributionsCollection.ContributionCalendar.TotalContributions,
		ReposContributed:   resp.User.RepositoriesContributedTo.TotalCount,
	}
	return usr, nil
}

// Get contributors and the number of contributions for a repo
func GetContributors(owner, repo string) (map[string]int, error) {
	opts := &api.ClientOptions{EnableCache: true}
	client, err := gh.RESTClient(opts)
	if err != nil {
		return nil, err
	}

	resp := []struct {
		Login         string
		Contributions int
	}{}
	err = client.Get(fmt.Sprintf("repos/%s/%s/contributors", owner, repo), &resp)
	if err != nil {
		return nil, err
	}

	contribs := make(map[string]int)

	for _, usr := range resp {
		contribs[usr.Login] = usr.Contributions
	}

	return contribs, nil
}

// Gets issues opened by an author in a repo
func GetUserIssues(owner, repo, username string) ([]Issue, error) {
	searchQuery := fmt.Sprintf("repo:%s/%s is:issue author:%s", owner, repo, username)
	return issueSearchQuery(searchQuery)
}

// Finds issues that were likely closed as spam
func GetSpam(owner, repo string) ([]Issue, error) {
	searchQuery := fmt.Sprintf("repo:%s/%s is:issue is:closed comments:0 -linked:pr", owner, repo)
	issues, err := issueSearchQuery(searchQuery)
	if err != nil {
		return nil, err
	}

	for _, issue := range issues {
		issue.IsSpam = true
	}
	return issues, nil
}

// Get closed issues that were definitely not spam
func GetNonSpam(owner, repo string) ([]Issue, error) {
	searchQuery := fmt.Sprintf("repo:%s/%s is:issue is:closed linked:pr", owner, repo)
	return issueSearchQuery(searchQuery)
}

func issueSearchQuery(query string) ([]Issue, error) {
	opts := &api.ClientOptions{EnableCache: true}
	client, err := gh.GQLClient(opts)
	if err != nil {
		return nil, err
	}

	gqlQuery := `query GetSpamIssues($query: String!, $after: String) {
search(query: $query, after: $after, type: ISSUE, first: 100) {
    pageInfo {
	  startCursor
      hasNextPage
      endCursor
    }
    nodes {
      ... on Issue {
        author { login }
        title
		body
        number
        authorAssociation
		createdAt
      }
    }
  }
}`

	issues := []Issue{}
	variables := map[string]interface{}{"query": query}

	for {
		resp := struct {
			Search struct {
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
				Nodes []Issue
			}
		}{}

		err = client.Do(gqlQuery, variables, &resp)
		if err != nil {
			return nil, err
		}

		for _, issue := range resp.Search.Nodes {
			if issue.Title != "" && issue.Author.Login != "" {
				issues = append(issues, issue)
			}
		}

		if !resp.Search.PageInfo.HasNextPage {
			return issues, nil
		}
		variables["after"] = resp.Search.PageInfo.EndCursor
	}
}
