# Contributing to tf-graph-analysis

Thank you for your interest in contributing to **tf-graph-analysis**!

## Development Setup

### Prerequisites
- [Go](https://go.dev) 1.23 or newer
- [Terraform](https://www.terraform.io) 1.5+ (optional, for generating test fixtures)

### Building from Source
```bash
git clone https://github.com/kheiakiyama/tf-graph-analysis.git
cd tf-graph-analysis
go build -o tf-graph-analysis ./cmd/tf-graph-analysis
```

### Running Tests
```bash
go test -v -race ./...
```

## Pull Request Guidelines

1. **Create an Issue**: Before submitting non-trivial PRs, open an issue to discuss your proposed changes.
2. **Follow Go Conventions**: Run `go fmt ./...` and `go vet ./...` before committing.
3. **Add Tests**: Ensure new features or bug fixes include unit/integration tests in the appropriate package.
4. **Clean Commits**: Write clear, descriptive commit messages.

## Code of Conduct
Please be respectful, constructive, and collaborative in all project interactions.
