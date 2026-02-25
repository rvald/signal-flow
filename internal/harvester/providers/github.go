package providers

import (
	"context"

	"github.com/rvald/signal-flow/internal/domain"
)

// GitHubHarvester collects signals from GitHub REST API v3.
// Fetches the user's starred repositories along with README and description.
type GitHubHarvester struct {
	// Client will be injected when GitHub API client is configured.
}

// NewGitHubHarvester creates a new GitHubHarvester.
func NewGitHubHarvester() *GitHubHarvester {
	return &GitHubHarvester{}
}

// Harvest fetches newly starred repositories from GitHub.
// Captures the repo URL, README content, and "About" description.
func (h *GitHubHarvester) Harvest(ctx context.Context, cred *domain.Credential) ([]domain.RawSignal, error) {
	// TODO: Implement using net/http + GitHub REST v3
	// 1. Authenticate with cred token
	// 2. GET /users/{username}/starred (sorted by most recent)
	// 3. For each repo: fetch README via GET /repos/{owner}/{repo}/readme
	// 4. Convert to []domain.RawSignal with SourceURL = repo URL
	return nil, nil
}

// Provider returns the GitHub provider identifier.
func (h *GitHubHarvester) Provider() string {
	return domain.ProviderGitHub
}
