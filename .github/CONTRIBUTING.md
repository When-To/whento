# Contributing to WhenTo

Thank you for your interest in contributing to WhenTo!

## How to Contribute

### Reporting Bugs

1. Check existing [issues](https://github.com/When-To/whento/issues) to avoid duplicates
2. Use the bug report template
3. Include reproduction steps, expected vs actual behavior

### Suggesting Features

1. Open a [discussion](https://github.com/When-To/whento/discussions) first
2. Explain the use case and why it would be valuable
3. If approved, create an issue with the feature request template

### Pull Requests

1. **Fork** the repository
2. **Create a branch** from `main`: `git checkout -b feature/your-feature`
3. **Make your changes** following the code style guidelines
4. **Test your changes**: `make test`
5. **Commit** with clear messages
6. **Push** to your fork
7. **Open a Pull Request** against `main`

### Code Style

#### Go

- Follow standard Go conventions
- Use `gofmt` for formatting
- Add tests for new functionality
- Use meaningful variable names

#### Frontend (Vue/TypeScript)

- Follow existing component patterns
- Use TypeScript strict mode
- Run `npm run lint` before committing

### Commit Messages

Use clear, descriptive commit messages:

```text
feat: add calendar sharing via email
fix: correct timezone handling in ICS export
docs: update deployment instructions
refactor: simplify availability calculation
```

## Development Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/whento.git
cd whento

# Start development environment
make dev-db          # Start PostgreSQL + Redis
make dev-fullstack   # Start backend + frontend

# Run tests
make test
```

## Questions?

- Open a [Discussion](https://github.com/When-To/whento/discussions)
- Check the [README](../README.md) for documentation

## License

By contributing, you agree that your contributions will be licensed under the BSL-1.1 license.
