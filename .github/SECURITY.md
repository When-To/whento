# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to: <security@whento.be>

Include:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Any suggested fixes (optional)

You should receive a response within 48 hours. If the issue is confirmed, we will:

1. Work on a fix
2. Release a patch as soon as possible
3. Credit you in the release notes (unless you prefer anonymity)

## Security Best Practices for Self-Hosted Deployments

1. **Keep WhenTo updated** - Always use the latest version
2. **Use HTTPS** - Deploy behind a reverse proxy with TLS
3. **Secure your database** - Use strong passwords, limit network access
4. **Protect JWT keys** - Store private keys securely, never commit them
5. **Enable rate limiting** - Set `RATE_LIMIT_ENABLED=true`
6. **Restrict registration** - Use `ALLOWED_EMAILS` to limit who can register
