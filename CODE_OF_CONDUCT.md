# Code of Conduct

Billtap is a technical project for building and testing billing integrations. The community should stay focused on practical, reproducible work and clear boundaries around payment safety.

## Expected Behavior

- Be respectful and direct.
- Discuss ideas, implementation tradeoffs, and evidence rather than personal traits.
- Keep issues and pull requests actionable, reproducible, and scoped.
- Assume good intent, but accept correction when project boundaries or safety rules are involved.
- Avoid sharing private company data, real customer data, real card data, live credentials, or production payment payloads.

## Unacceptable Behavior

- Harassment, threats, insults, or discriminatory language.
- Publishing private information without explicit permission.
- Pressuring maintainers or contributors to handle real payment data in public channels.
- Using the project to process, proxy, or validate live payment success paths.
- Repeatedly derailing issues or pull requests after maintainers have clarified scope.

## Product Safety Boundary

Billtap is a billing sandbox, not a payment processor. Community discussion and contributions must preserve that boundary:

- no real card processing
- no live payment credentials in examples, logs, screenshots, issues, or pull requests
- no production payment dependency hidden behind Billtap
- no full Stripe parity claims without fixture-backed tests and documented limitations

If a report includes a vulnerability or a possible production-boundary bypass, use the private process in `SECURITY.md` instead of a public issue.

## Enforcement

Maintainers may edit, hide, or remove comments, issues, pull requests, or other contributions that violate this code of conduct or expose sensitive payment data. Maintainers may also temporarily or permanently limit participation for repeated or severe violations.

For moderation concerns, contact the maintainers through the repository owner profile. For security issues, follow `SECURITY.md`.
