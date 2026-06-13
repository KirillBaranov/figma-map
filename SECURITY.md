# Security Policy

## Supported versions

figma-map is pre-1.0. Security fixes are applied to the latest released version
only.

## Reporting a vulnerability

Please **do not** open a public issue for security problems.

Instead, report privately via GitHub's
[private vulnerability reporting](https://github.com/KirillBaranov/figma-map/security/advisories/new)
(Security → Report a vulnerability).

Include:

- a description of the issue and its impact,
- steps to reproduce,
- affected version(s).

You can expect an initial response within a few days. Once a fix is available,
a release and advisory will be published.

## Scope notes

figma-map reads a local Figma bridge, screenshots a local Storybook, and sends
images to the LLM endpoint you configure. Be mindful that:

- **Screenshots are sent to your configured LLM provider.** Point `llm.baseURL`
  at a self-hosted/local model if your designs are confidential.
- **The API key is read from an environment variable**, never stored in the
  config file. Do not commit keys.
