package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v66/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

func main() {
	// Load .env file
	_ = godotenv.Load() // Ignore error; env vars might be set otherwise

	token := os.Getenv("GITHUB_TOKEN")
	username := os.Getenv("GITHUB_USERNAME")
	reposEnv := os.Getenv("REPOS")

	if token == "" || username == "" {
		fmt.Println("Please set GITHUB_TOKEN and GITHUB_USERNAME in .env or environment")
		os.Exit(1)
	}

	if len(os.Args) != 3 {
		fmt.Println("Usage: go run fetch.go <start_date> <end_date>")
		fmt.Println("Example: go run fetch.go 2025-10-01 2025-11-15")
		os.Exit(1)
	}

	startStr := os.Args[1]
	endStr := os.Args[2]

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		fmt.Printf("Invalid start_date format: %v\n", err)
		os.Exit(1)
	}
	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		fmt.Printf("Invalid end_date format: %v\n", err)
		os.Exit(1)
	}
	// End is inclusive; add one day and use Before for the range
	end = end.Add(24 * time.Hour)

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	var repos []*github.Repository

	if reposEnv != "" {
        repoNames := strings.Split(reposEnv, ",")
        for _, full := range repoNames {
            full = strings.TrimSpace(full)
            if full == "" {
                continue
            }
            parts := strings.SplitN(full, "/", 2)
            if len(parts) != 2 {
                fmt.Printf("Invalid repo format (expected owner/repo): %s\n", full)
                continue
            }
            owner := parts[0]
            repoName := parts[1]
            repo, _, err := client.Repositories.Get(ctx, owner, repoName)
            if err != nil {
                fmt.Printf("Error fetching %s: %v\n", full, err)
                continue
            }
            repos = append(repos, repo)
        }
    } else {
		fmt.Println("No REPOS provided, scanning all accessible repos. This may be slow...")
		opt := &github.RepositoryListOptions{Type: "all", ListOptions: github.ListOptions{PerPage: 100}}
		for {
			r, resp, err := client.Repositories.List(ctx, "", opt)
			if err != nil {
				fmt.Printf("Error fetching repositories: %v\n", err)
				os.Exit(1)
			}
			repos = append(repos, r...)
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
	}

	for _, repo := range repos {
		fmt.Printf("=== Fetching PRs for %s ===\n", repo.GetFullName())

		opt := &github.PullRequestListOptions{
			State: "all",
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}

		var prs []*github.PullRequest
		for {
			page, resp, err := client.PullRequests.List(ctx, repo.GetOwner().GetLogin(), repo.GetName(), opt)
			if err != nil {
				fmt.Printf("Error fetching PRs from %s: %v\n", repo.GetFullName(), err)
				break
			}
			prs = append(prs, page...)
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}

		found := false
		for _, pr := range prs {
			if pr.GetUser().GetLogin() != username {
				continue
			}
			created := pr.GetCreatedAt().Time
			if created.After(start) && created.Before(end) {
				fmt.Printf("- PR-%d: %s (%s)\n %s\n", pr.GetNumber(), pr.GetTitle(), repo.GetFullName(), pr.GetHTMLURL())
				found = true
			}
		}

		if !found {
			fmt.Println("No PRs found in this date range or no access.\n")
		} else {
			fmt.Println()
		}
	}
}