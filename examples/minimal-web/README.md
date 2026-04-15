# minimal-web example

This is the smallest useful example adapter in the kit.

Use it to learn the lane contract before looking at the more opinionated examples.

Try:

```bash
go run ./cmd/devlane inspect --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web --json
go run ./cmd/devlane prepare --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web
go run ./cmd/devlane up --config examples/minimal-web/devlane.yaml --cwd examples/minimal-web --dry-run
```
