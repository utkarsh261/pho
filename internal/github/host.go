package github

// GitHubHostProfile holds resolved connection details for one authenticated GitHub host.
// Built once at startup by AuthService and cached in memory for the session.
type GitHubHostProfile struct {
	Host                   string // e.g. "github.com" or "github.example.com"
	GraphQLURL             string // e.g. "https://api.github.com/graphql"
	RESTURL                string // e.g. "https://api.github.com"
	Token                  string // resolved token, kept in memory only, never logged
	ViewerLogin            string // resolved from gh auth status
	SupportsTopLevelRollup bool   // PullRequest.statusCheckRollup available (probe result)
	SupportsHeadRefOID     bool   // PullRequest.headRefOid available (probe result)
}

func DefaultGraphQLURL(host string) string {
	if host == "github.com" {
		return "https://api.github.com/graphql"
	}
	return "https://" + host + "/api/graphql"
}

func DefaultRESTURL(host string) string {
	if host == "github.com" {
		return "https://api.github.com"
	}
	return "https://" + host + "/api/v3"
}
