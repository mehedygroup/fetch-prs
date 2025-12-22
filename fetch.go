package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/automa-saga/logx"
	"github.com/google/go-github/v66/github"
	"github.com/joho/godotenv"
	"github.com/joomcode/errorx"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

// Add the fetch command
var fetchCmd = &cobra.Command{
	Use:   "fetch [start_date] [end_date]",
	Short: "Fetch pull requests within a date range",
	Args:  cobra.ExactArgs(2),
	RunE:  fetchPRs,
}

func init() {
	// Add flags to the fetch command
	fetchCmd.Flags().StringP("output", "o", "plain", "Output format: plain, yaml or json")
}

func fetchPRs(cmd *cobra.Command, args []string) error {
	// Load .env file
	_ = godotenv.Load() // Ignore error; env vars might be set otherwise

	token := os.Getenv("GITHUB_TOKEN")
	username := os.Getenv("GITHUB_USERNAME")
	reposEnv := os.Getenv("REPOS")
	outputFormat, _ := cmd.Flags().GetString("output")

	if outputFormat != "plain" && outputFormat != "json" && outputFormat != "yaml" {
		return errorx.IllegalArgument.New("Invalid output format %s. Use 'plain', 'yaml' or 'json'", outputFormat)
	}

	if token == "" || username == "" {
		return errorx.IllegalArgument.New("Missing GITHUB_TOKEN or GITHUB_USERNAME in .env or environment")
	}

	var startStr, endStr string
	if len(args) == 2 {
		startStr = args[0]
		endStr = args[1]
	} else {
		logx.As().Info().Msg("Using default date range from environment variables or fallback values.")
		startStr = os.Getenv("START_DATE")
		endStr = os.Getenv("END_DATE")
		if startStr == "" || endStr == "" {
			return errorx.IllegalArgument.New("Missing date range")
		}
	}

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return errorx.IllegalFormat.Wrap(err, "Invalid start_date format")
	}

	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		return errorx.IllegalFormat.Wrap(err, "Invalid end_date format")
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
				logx.As().Warn().Msgf("Invalid repo format (expected owner/repo): %s\n", full)
				continue
			}
			owner := parts[0]
			repoName := parts[1]
			repo, _, err := client.Repositories.Get(ctx, owner, repoName)
			if err != nil {
				logx.As().Warn().Err(err).Msgf("Error fetching %s\n", full)
				continue
			}
			repos = append(repos, repo)
		}
	} else {
		logx.As().Info().Msg("No REPOS provided, scanning all accessible repos. This may be slow...")
		opt := &github.RepositoryListOptions{Type: "all", ListOptions: github.ListOptions{PerPage: 100}}
		for {
			r, resp, err := client.Repositories.List(ctx, "", opt)
			if err != nil {
				return errorx.IllegalState.New("Failed to list repositories: %v", err)
			}

			repos = append(repos, r...)
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
	}

	var prData []map[string]string
	for _, repo := range repos {
		logx.As().Info().Time("start", start).Time("end", end).Str("repo", repo.GetFullName()).
			Msgf("Fetching PRs for %s", repo.GetFullName())

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
				logx.As().Error().Err(err).
					Time("start", start).
					Time("end", end).
					Str("repo", repo.GetFullName()).
					Msg("Failed to fetch PRs")
				break
			}
			prs = append(prs, page...)
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}

		// Modify the PR output logic to handle different formats
		found := false
		for _, pr := range prs {
			if pr.GetUser().GetLogin() != username {
				continue
			}
			created := pr.GetCreatedAt().Time
			if created.After(start) && created.Before(end) {
				prData = append(prData, map[string]string{
					"number": fmt.Sprintf("%d", pr.GetNumber()),
					"title":  pr.GetTitle(),
					"repo":   repo.GetFullName(),
					"url":    pr.GetHTMLURL(),
				})
				found = true
			}
		}

		if !found {
			logx.As().Info().
				Time("start", start).
				Time("end", end).
				Str("repo", repo.GetFullName()).
				Msgf("No PRs found for %s", repo.GetFullName())
		}
	}

	if outputFormat == "json" {
		jsonOutput, err := json.MarshalIndent(prData, "", "  ")
		if err != nil {
			return errorx.IllegalState.Wrap(err, "Error generating JSON output")
		}
		fmt.Println(string(jsonOutput))
	} else if outputFormat == "yaml" {
		yamlOutput, err := yaml.Marshal(prData)
		if err != nil {
			return errorx.InternalError.Wrap(err, "Error generating YAML output")
		}
		fmt.Println(string(yamlOutput))
	} else {
		for _, pr := range prData {
			fmt.Printf("%s, #%s: %s (%s)\n", pr["repo"], pr["number"], pr["title"], pr["url"])
		}
	}

	return nil
}
