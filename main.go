package main

import (
	"context"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
	"time"

	"github.com/automa-saga/logx"
	"github.com/google/go-github/v66/github"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

func main() {
	err := logx.Initialize(logx.LoggingConfig{
		Level:          "debug",
		ConsoleLogging: true,
	})
	if err != nil {
		fmt.Printf("Error initializing logger: %v\n", err)
		os.Exit(1)
	}

	// Initialize the root command
	var rootCmd = &cobra.Command{
		Use:   "fetch-prs",
		Short: "Fetch pull requests from GitHub repositories",
		Long:  `A CLI tool to fetch and manage pull requests from GitHub repositories based on date range and other criteria.`,
	}

	// Add the fetch command
	var fetchCmd = &cobra.Command{
		Use:   "fetch [start_date] [end_date]",
		Short: "Fetch pull requests within a date range",
		Args:  cobra.ExactArgs(2),
		Run:   fetchPRs,
	}

	// Add flags to the fetch command
	fetchCmd.Flags().StringP("output", "o", "plain", "Output format: plain or json")
	rootCmd.AddCommand(fetchCmd)

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		logx.As().Info().Msg(err.Error())
		os.Exit(1)
	}
}

func fetchPRs(cmd *cobra.Command, args []string) {
	// Load .env file
	_ = godotenv.Load() // Ignore error; env vars might be set otherwise

	token := os.Getenv("GITHUB_TOKEN")
	username := os.Getenv("GITHUB_USERNAME")
	reposEnv := os.Getenv("REPOS")
	outputFormat, _ := cmd.Flags().GetString("output")

	if outputFormat != "plain" && outputFormat != "json" && outputFormat != "yaml" {
		logx.As().Fatal().Msg("Invalid output format. Use 'plain', 'yaml' or 'json'")
	}

	if token == "" || username == "" {
		logx.As().Info().Msg("Please set GITHUB_TOKEN and GITHUB_USERNAME in .env or environment")
		os.Exit(1)
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
			logx.As().Info().Msg("Please provide START_DATE and END_DATE in the environment or as CLI arguments.")
			os.Exit(1)
		}
	}

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
		logx.As().Info().Msg("No REPOS provided, scanning all accessible repos. This may be slow...")
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
			fmt.Printf("Error generating JSON output: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonOutput))
	} else if outputFormat == "yaml" {
		yamlOutput, err := yaml.Marshal(prData)
		if err != nil {
			fmt.Printf("Error generating YAML output: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(yamlOutput))
	} else if outputFormat == "" || outputFormat == "plain" {
		for _, pr := range prData {
			fmt.Printf("%s, #%s: %s (%s)\n", pr["repo"], pr["number"], pr["title"], pr["url"])
		}
	} else {
		logx.As().Fatal().Msg("Invalid output format. Use 'plain', 'json'")
	}
}
