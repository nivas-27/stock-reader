# Contributing

Thanks for taking a look. This is a small project, so the process is small too.

## Getting set up

See [Quick start](README.md#quick-start) in the README. You need Go 1.26+,
a local MongoDB and a free Twelve Data API key.

```bash
cd backend && go test ./...
```

Tests use a fake HTTP server, so they run without an API key or a network.

## Asking for a watchlist

You do not need to open a pull request to get a list added. Open an issue with
the list name and its symbols, or email the address behind "Request a list" on
the page.

## Pull requests

- One change per pull request, with a short description of the why.
- Run `gofmt` on the files you touch and make sure `go test ./...` passes.
- Add a test when you change resolving, trend or rate-limiting logic — those
  have tests already and they are cheap to extend.

## House style

A few conventions this codebase follows. They keep diffs small and reviewable:

- Prefer a new, small function over reworking an existing handler. Existing
  handlers carry conditions that are easy to break and hard to notice.
- Reuse helpers, models and constants by calling them as they are.
- `models/` holds database structs only. Request and response structs are
  declared inside the handler that uses them.
- Comments are short: two or three lines on a function, one on a model field.
- Keep changes to existing files additive, so unrelated lines do not move.

## Reporting a security issue

Please do not open a public issue for a vulnerability. Email the address behind
"Request a list" on the page instead.

Note that the write endpoints under `/api/lists` are unauthenticated by design
for local use — that is documented in the README rather than treated as a bug.

## License

By contributing you agree that your contribution is licensed under the
[MIT License](LICENSE).
