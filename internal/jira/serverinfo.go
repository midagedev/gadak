package jira

import (
	"context"
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/config"
)

// serverInfo is the subset of /rest/api/2/serverInfo this package reads.
// The route is v2 deliberately: it is the one endpoint both deployments
// answer, so it can be asked before the deployment is known.
type serverInfo struct {
	DeploymentType string `json:"deploymentType"`
	Version        string `json:"version"`
	BaseURL        string `json:"baseUrl"`
}

// Deployment asks the origin which Jira it is and returns
// config.OriginJira (Cloud) or config.OriginJiraServer (GDK-1635).
//
// The answer comes from serverInfo's own deploymentType field, never from
// probing whether /rest/api/3 exists: a Server instance answers that route
// with 401, so a status-code probe reads a missing API as a bad credential.
//
// An unrecognized deploymentType is an error, not a default. Guessing Cloud
// here would send v3 requests to something that cannot answer them, and the
// resulting 401s would be reported as an authentication problem.
func (c *Client) Deployment(ctx context.Context) (string, error) {
	var info serverInfo
	if err := c.do(ctx, "GET", "/rest/api/2/serverInfo", nil, &info); err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(info.DeploymentType)) {
	case "cloud":
		return config.OriginJira, nil
	case "server", "datacenter", "data center":
		return config.OriginJiraServer, nil
	}
	return "", fmt.Errorf("jira: serverInfo reported an unknown deploymentType %q", info.DeploymentType)
}
