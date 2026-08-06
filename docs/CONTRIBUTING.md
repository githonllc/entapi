# Contributing to EntDomain

Thank you for your interest in contributing to EntDomain!

## Prerequisites

- Go 1.23+
- [golangci-lint](https://golangci-lint.run/) (for linting)

## Development Workflow

1. Fork and clone the repository
2. Create a feature branch
3. Make your changes
4. Run tests and checks:

```bash
make check    # runs fmt + vet + test
make cover    # shows test coverage
make lint     # runs golangci-lint
```

## Code Style

- Follow standard Go conventions
- All exported symbols must have godoc comments
- Code comments and commit messages in English
- Run `make fmt` before committing

## Testing

- Write tests for all new functionality
- Maintain test coverage above 85%
- Use table-driven tests where appropriate
- Add `Example*` functions for public API changes

## Pull Requests

- Keep PRs focused on a single change
- Include a clear description of what changed and why
- Ensure all checks pass before requesting review
