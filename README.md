[![CircleCI](https://dl.circleci.com/status-badge/img/gh/giantswarm/openssf-scorecard-exporter/tree/main.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/giantswarm/openssf-scorecard-exporter/tree/main)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/giantswarm/openssf-scorecard-exporter/badge)](https://securityscorecards.dev/viewer/?uri=github.com/giantswarm/openssf-scorecard-exporter)

# OpenSSF Scorecard Exporter

A Kubernetes operator that exports [OpenSSF Scorecard](https://securityscorecards.dev/) metrics for GitHub organizations as Prometheus metrics.

## Overview

The OpenSSF Scorecard Exporter is a kubebuilder-based Kubernetes operator that helps provide visibility into your organization's code security practices. It automatically:

1. Discovers public repositories in specified GitHub organizations
2. Fetches OpenSSF Scorecard data for each repository
3. Exposes the security scores as Prometheus metrics

The operator reconciles native Kubernetes ConfigMaps that are labeled with `openssf-scorecard.giantswarm.io/enabled=true`, making it easy to manage multiple organization configurations.

## Installation

### Using Helm

```bash
helm install openssf-scorecard-exporter ./helm/openssf-scorecard-exporter
```

### Using kubectl

```bash
make build-installer
kubectl apply -f dist/install.yaml
```

## Configuration

To monitor an organization's repositories, create a ConfigMap with the required label:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: giantswarm-scorecard-config
  namespace: default
  labels:
    openssf-scorecard.giantswarm.io/enabled: "true"
data:
  organization: "giantswarm"  # Required: GitHub organization name
```

### With GitHub Token (Recommended)

To avoid GitHub API rate limits, you can provide a GitHub token:

1. Create a secret with your GitHub token:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: github-token
  namespace: default
type: Opaque
stringData:
  token: "ghp_your_github_token_here"
```

2. Reference it in your ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: giantswarm-scorecard-config
  namespace: default
  labels:
    openssf-scorecard.giantswarm.io/enabled: "true"
data:
  organization: "giantswarm"
  tokenSecret: "github-token"        # Name of the secret
  tokenSecretKey: "token"             # Key in the secret (defaults to "token")
```

### ConfigMap Fields

| Field | Required | Description |
|-------|----------|-------------|
| `organization` | Yes | Organization/group name to monitor |
| `providerType` | No | VCS provider type: `github` (default) |
| `baseURL` | No | Custom VCS API base URL (for self-hosted instances) |
| `tokenSecret` | No | Name of the Kubernetes Secret containing the VCS token |
| `tokenSecretKey` | No | Key in the Secret containing the token (defaults to "token") |

## Metrics

The operator exposes the following Prometheus metrics:

### `openssf_scorecard_overall_score`

Overall OpenSSF Scorecard score for a repository (0-10 scale, -1 for unavailable).

**Labels:**
- `config`: Name of the ConfigMap managing this repository
- `organization`: GitHub organization
- `repository`: Repository name

**Special Values:**
- `-1`: Scorecard data not yet available for this repository

### `openssf_scorecard_check_score`

Score for individual OpenSSF Scorecard checks (0-10 scale, -1 for unavailable).

**Labels:**
- `config`: Name of the ConfigMap managing this repository
- `organization`: GitHub organization
- `repository`: Repository name
- `check`: Name of the security check (e.g., "Branch-Protection", "Code-Review")

### `openssf_scorecard_check_status`

Binary status of individual checks.

**Labels:**
- `config`: Name of the ConfigMap managing this repository
- `organization`: GitHub organization
- `repository`: Repository name
- `check`: Name of the security check

**Values:**
- `1`: Pass
- `0`: Fail
- `-1`: Unavailable/Unknown

### `openssf_scorecard_last_update_timestamp`

Unix timestamp of the last scorecard data update.

**Labels:**
- `config`: Name of the ConfigMap managing this repository
- `organization`: GitHub organization
- `repository`: Repository name

## Example Prometheus Queries

Get overall scores for all repositories:
```promql
openssf_scorecard_overall_score
```

Find repositories with low scores (excluding unavailable data):
```promql
openssf_scorecard_overall_score < 5 and openssf_scorecard_overall_score >= 0
```

Find repositories without scorecard data:
```promql
openssf_scorecard_overall_score == -1
```

Check Branch Protection status across all repos:
```promql
openssf_scorecard_check_score{check="Branch-Protection"}
```

Count failing checks per repository:
```promql
count by (organization, repository) (openssf_scorecard_check_status{status="0"})
```

## Development

### Prerequisites

- Go 1.23 or later
- Kubernetes cluster (or Kind for local development)
- kubectl configured to access your cluster

### Building

Build the operator binary:
```bash
make build
```

Build the Docker image:
```bash
make docker-build IMG=your-registry/openssf-scorecard-exporter:latest
```

### Running Locally

Run the operator outside the cluster (useful for development):
```bash
make run
```

### Testing

Run unit tests:
```bash
make test
```

Run e2e tests (requires Kind):
```bash
make test-e2e
```

### Linting

Run the linter:
```bash
make lint
```

Auto-fix linting issues:
```bash
make lint-fix
```

## Troubleshooting

### ConfigMap not reconciling

Check that the ConfigMap has the required label:
```bash
kubectl get configmap <name> -o jsonpath='{.metadata.labels}'
```

View operator logs:
```bash
kubectl logs -n openssf-scorecard-exporter-system deployment/openssf-scorecard-exporter-controller-manager
```

### No metrics appearing

1. Verify the ConfigMap is properly labeled
2. Check operator logs for errors
3. Verify the organization has public repositories
4. Check that repositories have scorecard data available

**Note:** Repositories without scorecard data will show a score of `-1`. This is normal for:
- New repositories not yet analyzed by OpenSSF Scorecard
- Repositories that don't meet scorecard analysis criteria
- Private repositories (scorecard only analyzes public repos)

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

This project is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.

## Links

- [OpenSSF Scorecard Project](https://securityscorecards.dev/)
- [OpenSSF Scorecard Documentation](https://github.com/ossf/scorecard)
- [Giant Swarm Documentation](https://docs.giantswarm.io/)
- [Kubebuilder](https://book.kubebuilder.io/)
