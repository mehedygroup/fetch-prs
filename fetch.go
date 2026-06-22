package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

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
	Short: "Fetch GitHub work contributions within a date range",
	Args:  cobra.ExactArgs(2),
	RunE:  fetchPRs,
}

type WorkItem struct {
	Type      string `json:"type"`
	Status    string `json:"status"`
	Number    string `json:"number"`
	Title     string `json:"title"`
	Repo      string `json:"repo"`
	URL       string `json:"url"`
	CommitSHA string `json:"commitSha,omitempty"`
}

func init() {
	// Add flags to the fetch command
	fetchCmd.Flags().StringP("output", "o", "plain", "Legacy output format: plain or json")
	fetchCmd.Flags().Bool("plain", false, "Output invoice-friendly plain text")
	fetchCmd.Flags().Bool("json", false, "Output work items as JSON")
	_ = fetchCmd.Flags().MarkDeprecated("output", "use --plain or --json instead")
}

func fetchPRs(cmd *cobra.Command, args []string) error {
	// Load .env file
	_ = godotenv.Load() // Ignore error; env vars might be set otherwise

	token := os.Getenv("GITHUB_TOKEN")
	username := os.Getenv("GITHUB_USERNAME")
	reposEnv := os.Getenv("REPOS")
	outputFormat, err := resolveOutputFormat(cmd)
	if err != nil {
		return err
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

	var workItems []WorkItem
	for _, repo := range repos {
		logx.As().Info().Time("start", start).Time("end", end).Str("repo", repo.GetFullName()).
			Msgf("Fetching work items for %s", repo.GetFullName())

		prOpt := &github.PullRequestListOptions{
			State: "all",
			ListOptions: github.ListOptions{
				PerPage: 100,
			},
		}

		var prs []*github.PullRequest
		for {
			page, resp, err := client.PullRequests.List(ctx, repo.GetOwner().GetLogin(), repo.GetName(), prOpt)
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
			prOpt.Page = resp.NextPage
		}

		repoFound := false
		for _, pr := range prs {
			if pr.GetUser().GetLogin() != username {
				continue
			}

			workTime := prWorkTimestamp(pr)
			if workTime.IsZero() {
				continue
			}

			if !workTime.Before(start) && workTime.Before(end) {
				workItems = append(workItems, WorkItem{
					Type:   "pr",
					Status: prStatus(pr),
					Number: fmt.Sprintf("%d", pr.GetNumber()),
					Title:  pr.GetTitle(),
					Repo:   repo.GetFullName(),
					URL:    pr.GetHTMLURL(),
				})
				repoFound = true
			}
		}

		issueItems, err := fetchCommitClosedIssues(ctx, client, repo, username, start, end)
		if err != nil {
			logx.As().Warn().Err(err).
				Time("start", start).
				Time("end", end).
				Str("repo", repo.GetFullName()).
				Msg("Failed to fetch closed issues with valid commits")
		} else if len(issueItems) > 0 {
			workItems = append(workItems, issueItems...)
			repoFound = true
		}

		if !repoFound {
			logx.As().Info().
				Time("start", start).
				Time("end", end).
				Str("repo", repo.GetFullName()).
				Msgf("No work items found for %s", repo.GetFullName())
		}
	}

	if outputFormat == "json" {
		jsonOutput, err := json.MarshalIndent(workItems, "", "  ")
		if err != nil {
			return errorx.IllegalState.Wrap(err, "Error generating JSON output")
		}
		fmt.Println(string(jsonOutput))
	} else {
		for _, item := range workItems {
			fmt.Println(formatPlainWorkItem(item))
		}
	}

	return nil
}

func resolveOutputFormat(cmd *cobra.Command) (string, error) {
	plainFlag, _ := cmd.Flags().GetBool("plain")
	jsonFlag, _ := cmd.Flags().GetBool("json")
	outputFormat, _ := cmd.Flags().GetString("output")
	outputChanged := cmd.Flags().Changed("output")

	if plainFlag && jsonFlag {
		return "", errorx.IllegalArgument.New("cannot use --plain and --json together")
	}

	if outputChanged && (plainFlag || jsonFlag) {
		return "", errorx.IllegalArgument.New("cannot combine --output with --plain or --json")
	}

	if jsonFlag {
		return "json", nil
	}

	if plainFlag {
		return "plain", nil
	}

	if outputChanged {
		if outputFormat != "plain" && outputFormat != "json" {
			return "", errorx.IllegalArgument.New("Invalid output format %s. Use 'plain' or 'json'", outputFormat)
		}
		return outputFormat, nil
	}

	return "plain", nil
}

func fetchCommitClosedIssues(ctx context.Context, client *github.Client, repo *github.Repository, username string, start, end time.Time) ([]WorkItem, error) {
	owner := repo.GetOwner().GetLogin()
	repoName := repo.GetName()
	issueOpt := &github.IssueListByRepoOptions{
		State: "closed",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	commitCache := make(map[string]*github.RepositoryCommit)
	var items []WorkItem

	for {
		issues, resp, err := client.Issues.ListByRepo(ctx, owner, repoName, issueOpt)
		if err != nil {
			return nil, err
		}

		for _, issue := range issues {
			if issue == nil || issue.PullRequestLinks != nil {
				continue
			}

			closedAt := issue.GetClosedAt().Time
			if closedAt.IsZero() || closedAt.Before(start) || !closedAt.Before(end) {
				continue
			}

			commitSHA, ok, err := findValidCommitForIssue(ctx, client, owner, repoName, issue.GetNumber(), username, commitCache)
			if err != nil {
				logx.As().Warn().Err(err).
					Int("issue", issue.GetNumber()).
					Str("repo", repo.GetFullName()).
					Msg("Failed to validate commit for closed issue")
				continue
			}

			if !ok {
				continue
			}

			items = append(items, WorkItem{
				Type:      "issue_commit",
				Status:    "done",
				Number:    fmt.Sprintf("%d", issue.GetNumber()),
				Title:     issue.GetTitle(),
				Repo:      repo.GetFullName(),
				URL:       issue.GetHTMLURL(),
				CommitSHA: commitSHA,
			})
		}

		if resp.NextPage == 0 {
			break
		}
		issueOpt.Page = resp.NextPage
	}

	return items, nil
}

func findValidCommitForIssue(ctx context.Context, client *github.Client, owner, repo string, issueNumber int, username string, commitCache map[string]*github.RepositoryCommit) (string, bool, error) {
	timelineOpt := &github.ListOptions{PerPage: 100}

	for {
		events, resp, err := client.Issues.ListIssueTimeline(ctx, owner, repo, issueNumber, timelineOpt)
		if err != nil {
			return "", false, err
		}

		for _, event := range events {
			if event == nil {
				continue
			}

			switch event.GetEvent() {
			case "closed", "referenced":
			default:
				continue
			}

			commitSHA := event.GetCommitID()
			if commitSHA == "" {
				continue
			}

			commit, err := getRepositoryCommit(ctx, client, owner, repo, commitSHA, commitCache)
			if err != nil {
				return "", false, err
			}

			if commitMatchesUsername(commit, username) {
				return commitSHA, true, nil
			}
		}

		if resp.NextPage == 0 {
			break
		}
		timelineOpt.Page = resp.NextPage
	}

	return "", false, nil
}

func getRepositoryCommit(ctx context.Context, client *github.Client, owner, repo, sha string, cache map[string]*github.RepositoryCommit) (*github.RepositoryCommit, error) {
	if commit, ok := cache[sha]; ok {
		return commit, nil
	}

	commit, _, err := client.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		return nil, err
	}

	cache[sha] = commit
	return commit, nil
}

func commitMatchesUsername(commit *github.RepositoryCommit, username string) bool {
	if commit == nil {
		return false
	}

	if commit.Author != nil && commit.Author.GetLogin() == username {
		return true
	}

	if commit.Committer != nil && commit.Committer.GetLogin() == username {
		return true
	}

	return false
}

func formatPlainWorkItem(item WorkItem) string {
	if item.Type == "issue_commit" && item.CommitSHA != "" {
		return fmt.Sprintf("[%s] %s: %s, #%s (commit %s)", item.Status, item.Repo, item.Title, item.Number, shortenSHA(item.CommitSHA))
	}

	return fmt.Sprintf("[%s] %s: %s, #%s", item.Status, item.Repo, item.Title, item.Number)
}

func prStatus(pr *github.PullRequest) string {
	if !pr.GetMergedAt().Time.IsZero() {
		return "done"
	}

	return "wip"
}

func prWorkTimestamp(pr *github.PullRequest) time.Time {
	if pr == nil {
		return time.Time{}
	}

	if mergedAt := pr.GetMergedAt().Time; !mergedAt.IsZero() {
		return mergedAt
	}

	if updatedAt := pr.GetUpdatedAt().Time; !updatedAt.IsZero() {
		return updatedAt
	}

	return pr.GetCreatedAt().Time
}

func shortenSHA(sha string) string {
	if len(sha) <= 7 {
		return sha
	}

	return sha[:7]
}

