# pplx

Query Perplexity AI from the command line.

`pplx` is a single pure-Go binary. It reads public perplexity data
over plain HTTPS, shapes it into clean records, and prints output that pipes
into the rest of your tools. No API key, nothing to run alongside it.

The same package is also a [resource-URI driver](#use-it-as-a-resource-uri-driver),
so a host program like [ant](https://github.com/tamnd/ant) can address
perplexity as `perplexity://` URIs.

## Install

```bash
go install github.com/tamnd/perplexity-cli/cmd/pplx@latest
```

Or grab a prebuilt binary from the [releases](https://github.com/tamnd/perplexity-cli/releases), or run
the container image:

```bash
docker run --rm ghcr.io/tamnd/pplx:latest --help
```

## Usage

```bash
pplx page <path>                      # fetch one page as a record
pplx page <path> -o json              # as JSON, ready for jq
pplx page <path> --template '{{.Body}}'  # just the readable body text
pplx links <path>                     # the pages it links to, one per line
pplx --help                           # the whole command tree
```

Every command shares one output contract: `-o table|json|jsonl|csv|tsv|url|raw`,
`--fields` to pick columns, `--template` for a custom line, and `-n` to limit.
The default adapts to where output goes (a table on a terminal, JSONL in a
pipe), so the same command reads well by hand and parses cleanly downstream.

This is a fresh scaffold. It ships one example resource type, `page`, wired end
to end. Model the real perplexity records in `perplexity/` and declare their
operations in `perplexity/domain.go`; each one becomes a command, an HTTP
route, and an MCP tool at once.

## Serve it

The same operations are available over HTTP and as an MCP tool set for agents,
with no extra code:

```bash
pplx serve --addr :7777    # GET /v1/page/<path>  returns NDJSON
pplx mcp                   # speak MCP over stdio
```

## Use it as a resource-URI driver

`pplx` registers a `perplexity` domain the way a program registers a
database driver with `database/sql`. A host enables it with one blank import:

```go
import _ "github.com/tamnd/perplexity-cli/perplexity"
```

Then [ant](https://github.com/tamnd/ant) (or any program that links the package)
dereferences `perplexity://` URIs without knowing anything about perplexity:

```bash
ant get perplexity://page/<path>   # fetch the record
ant cat perplexity://page/<path>   # just the body text
ant ls  perplexity://page/<path>   # the pages it links to, each addressable
ant url perplexity://page/<path>   # the live https URL
```

## Development

```
cmd/pplx/   thin main: hands cli.NewApp to kit.Run
cli/                 assembles the kit App from the perplexity domain
perplexity/                the library: HTTP client, data models, and domain.go (the driver)
docs/                tago documentation site
```

```bash
make build      # ./bin/pplx
make test       # go test ./...
make vet        # go vet ./...
```

## Releasing

Push a version tag and GitHub Actions runs GoReleaser, which builds the
archives, Linux packages, the multi-arch GHCR image, checksums, SBOMs, and a
cosign signature:

```bash
git tag v0.1.0
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so the first
release works with no extra secrets.

## License

Apache-2.0. See [LICENSE](LICENSE).
