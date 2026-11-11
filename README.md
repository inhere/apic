# apic

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/inhere/apic?style=flat-square)
[![GitHub tag (latest SemVer)](https://img.shields.io/github/tag/inhere/apic)](https://github.com/inhere/apic)
[![Unit-Tests](https://github.com/inhere/apic/actions/workflows/go.yml/badge.svg)](https://github.com/inhere/apic)

---

English | [简体中文](./README.zh-CN.md)

`apic` calls any HTTP API from the command line. It loads the API's
definition (OpenAPI 3.x / Swagger 2.0, or a few lines of simple YAML) and
builds commands from it, so you do not have to write a CLI for each service.
It also handles base URLs, environments and authentication.

It is designed for people and AI agents alike: stable JSON output on stdout,
documented exit codes, write protection for production environments and
secrets masked everywhere.

## Features

- Register a site from an OpenAPI / Swagger URL or file, or from a simple YAML definition
- Call operations by name (`apic pet list-users --page 2`) or by raw path
  (`apic pet -X POST /api/users -f name=tom`)
- Command names come from `operationId`, or from the path when there is none
  (`POST /back/cfg/get` → `back-cfg-get`)
- Separate environments (base URLs) and authentications; switch with `apic use`
- Auth types: `none`, `bearer`, `basic`, `apikey` (header or query) and `login`
  (log in, cache the token per environment, re-login on 401)
- Secrets live in `.env` and are referenced as `${env:NAME}`; they are masked in
  `-v`, `--dry-run`, error messages and `config show`
- `--dry-run` prints the equivalent `curl` command
- Write protection: `protect: true` environments need `--yes` for writes;
  `allow_methods` restricts an authentication to read-only methods
- Shortcut commands: `apic link pet --name petctl` creates a `petctl` script
  (`.cmd` on Windows)
- Shell completion (bash, zsh, PowerShell) for sites, operations, parameters,
  enum values and `--env`/`--auth`, including shortcut commands
- `--pick data.items.*.id` extracts values from a JSON response; `-o yaml` and
  `-o table` reformat it, keeping numbers and field order intact
- `apic skill pet` generates a `SKILL.md` that teaches an AI agent to use the site
- `exec` authentication: get the token from an external command (vault, SSO CLI…)
- Overlays: rename, hide or fix operations and parameters in `overlays/<site>.yaml`
  without touching the upstream spec
- File uploads: `-F file=@./a.xlsx` sends `multipart/form-data`

## Install

```bash
go install github.com/inhere/apic/cmd/apic@latest
```

Or build from source:

```bash
make install   # builds into dist/ and copies to $GOPATH/bin
```

## Quick start

```bash
# register a site from an OpenAPI / Swagger document
apic site add pet --spec https://pet.example.com/swagger/doc.json
# the document has no host? give the base URL yourself
apic site add pet --spec ./swagger.json --base-url http://127.0.0.1:8080

apic ops pet                 # list operations
apic desc pet create-user    # parameters, request body, response fields

apic pet list-users --page 2                    # call by operation
apic pet create-user -f name=tom -f age:=3      # JSON body from fields
apic pet create-user -d @user.json              # JSON body from a file (- for stdin)
apic pet -X GET /api/users -p page=2            # call by raw path
apic pet list-users --dry-run                   # print the curl command only
```

`-f key=value` sends a string, `-f key:=<json>` sends a typed JSON value and
`-f a.b=v` builds nested objects. A parameter whose name clashes with a global
option is passed with `-p name=value` (or `-p query.name=value`).

## Configuration

The config dir is `~/.config/apic` (override with `APIC_CONFIG_DIR`):

```
~/.config/apic/
  config.yaml          # your settings and sites (apic never rewrites it)
  .env                 # secrets: KEY=VALUE; exported OS env vars win
  sites/<name>.yaml    # one site per file, written by `apic site add`
  defs/<name>.yaml     # simple definitions
  state.yaml           # current env/auth per site and generated shortcuts
  cache/               # cached definitions and login tokens (0600)
```

A site with two environments and three authentications:

```yaml
sites:
  pet:
    source: {type: openapi, url: https://pet.example.com/swagger/doc.json, ttl: 24h}
    envs:
      dev:  {base_url: http://127.0.0.1:8080}
      prod: {base_url: https://pet.example.com, protect: true}
    default_env: dev
    auths:
      admin: {type: bearer, token: "${env:PET_ADMIN_TOKEN}"}
      ro:    {type: bearer, token: "${env:PET_RO_TOKEN}", allow_methods: [GET, HEAD]}
      me:
        type: login
        login:
          method: POST
          path: /api/login
          body: {username: "${env:PET_USER}", password: "${env:PET_PASS}"}
        token: {from: body, path: data.token}
        expires: {from: body, path: data.expires_in}
        apply: {header: Authorization, value: "Bearer {token}"}
    default_auth: me
```

```bash
apic use pet --env prod --auth ro        # switch the current selection
apic pet list-users --env dev --auth me  # override for one request
apic auth status pet                     # login state (token never printed)
```

### Simple definitions

For services with only a few endpoints (Jenkins, small internal tools),
write a short YAML file instead of a Swagger document:

```yaml
# defs/jenkins.yaml
operations:
  job-info:
    get: /job/{name}/api/json        # {name} becomes a required parameter
    summary: show job info
  job-build:
    post: /job/{name}/buildWithParameters
    query: [BRANCH, ENV]             # optional string query parameters
  job-log:
    get: /job/{name}/{build}/consoleText
    output: raw
```

```bash
apic site add jenkins --def ~/.config/apic/defs/jenkins.yaml --base-url https://ci.example.com
apic jenkins job-build --name demo --BRANCH main
```

### Token from an external command (`exec`)

```yaml
auths:
  sso:
    type: exec
    exec:
      command: [/usr/local/bin/corp-sso, token, --audience, pet]  # argv, no shell
      env: {SSO_SECRET: "${env:PET_SSO_SECRET}"}                   # passed to the command
      ttl: 30m          # cache the token; without ttl/expires it runs on every request
      # format: json, token: {path: data.token}, expires: {path: expires_in}
    apply: {header: Authorization, value: "Bearer {token}"}
```

The command runs without a shell, with an empty stdin, in the config directory
(relative paths are resolved from there) and is killed after `timeout` (30s).
Its argv never appears in any output. On a 401 the command runs again once.

> **Security:** with `exec`, `config.yaml` and `sites/*.yaml` can make apic run
> commands, so treat them like scripts and do not import untrusted site files.
> On Windows the 0600 permission of the token cache is not enforced, and
> arguments with `& | < > ^ % ! "` are refused when the command is a `.bat`/`.cmd`.

## Fixing a definition with an overlay

When a generated command is awkward or a spec is wrong, write
`<config dir>/overlays/<site>.yaml` instead of editing the spec. It survives
`site refresh` and applies to calls, `ops`, `desc`, completion and `apic skill`.

```yaml
operations:
  "POST /back/cfg/get":          # METHOD /path, as in the PATH column of `apic ops`
    name: cfg-get
    summary: get the print config
  "POST /back/cfg/delete":
    hide: true
  users-list:                    # or the generated operation name
    output: raw
    body: multipart              # json | form | multipart | xml | raw
    params:
      status: {required: true, enum: [active, banned]}
      debug:  {hide: true}       # no option; -p debug=... is refused
      tenant: {in: header, required: true}   # new parameter: in is query or header
```

```bash
apic overlay init pet          # writes overlays/pet.yaml with one empty entry per operation
apic overlay init pet --stdout
apic config check              # reports overlay errors and keys that match nothing
```

- A key that matches no operation only warns. A structural error (bad value,
  name clash, hiding a path or required parameter, a new `Authorization`/`Cookie`/
  `Host`/`Content-*` header…) makes every command of that site exit 1, except
  `site refresh/show/rm` and `overlay init`; `config check` then exits 1 too.
- `desc` marks changed fields with `(overlay)` and hidden parameters with
  `(hidden)`; `desc -o json` adds an `overlay` array.
- **`hide` is not access control**: a hidden operation can still be called by
  its raw path, and a hidden parameter can still be sent with `-H` or a raw
  path call. Use `protect` and `allow_methods` for dangerous endpoints.
- `overlay` is a reserved name: a site called `overlay` is skipped with a warning.

## File uploads (multipart)

```bash
apic pet import-users -F file=@./users.xlsx -F type=user
apic pet upload -F 'file=@./a.bin;type=application/zip;filename=report.zip'
apic pet upload -F 'file=@"./a;b.bin";filename=ab.bin'   # quote paths containing ; or "
apic pet -X POST /upload -F file=@./a.txt                # raw path call
```

- Operations whose spec says `multipart/form-data` are sent as multipart; for
  others fix the body type with an overlay (`body: multipart`).
- Only `-F name=@path` reads a file. `-F name=value` and `-f name=value` are
  text (`-f name=@x` sends the literal `@x`). `-F name=@-` and curl's
  `-F name=<file` are not supported; a value starting with `<` is sent as text.
- `type` must be a plain `type/subtype` (no parameters); otherwise it is guessed
  from a builtin extension table, falling back to `application/octet-stream`.
  Names and file names cannot contain `"`, `\` or control characters — pass
  `;filename=` when the path's last segment has them.
- `-F` cannot be combined with `-d`, `-f` or a `Content-Type` header. On a
  multipart operation, `-d` alone needs `-H 'Content-Type: ...'`.
- Only regular files are read, after the write-protection check, and the whole
  body is limited to 64 MiB (use curl for larger uploads). `--dry-run` prints a
  curl command that sends the same parts; `-v` lists the parts without content.

> **Security:** `-F name=@path` and `-d @file` send local files. `-d @file` is
> read before the write-protection check and has no size limit. On Windows a UNC
> path (`\\host\share\...`) authenticates to that host when it is opened.

## Shortcut commands

```bash
apic link pet --name petctl                 # petctl list-users == apic --api pet list-users
apic link pet --name pet-prod --env prod --auth ro
apic link ls
apic link rm petctl
```

Scripts go to `defaults.link_dir`, `~/.local/bin` by default (`%USERPROFILE%\bin`
on Windows). On Windows a `.cmd` file is also generated; `cmd.exe` re-parses
`%*`, so pass JSON bodies with `-d @file` there.

## Shell completion

```bash
eval "$(apic --gen-completion bash)"        # or zsh; PowerShell: apic --gen-completion pwsh | Invoke-Expression
eval "$(apic link ls --completion bash)"    # shortcut commands; re-run after link / link rm
```

Completion only reads cached definitions: it never sends requests, logs in or
runs `exec` commands. Known limit: after an option value
(`apic pet --env dev list-users --<TAB>`) only site-level options are offered.

## Picking and formatting output

```bash
apic pet list-users --pick data.items.*.name     # ["tom","amy"]
apic pet get-user --id 7 --pick data.name        # tom   (strings print bare)
apic pet list-users --pick data.items -o table
apic pet list-users -o yaml
```

A path is dot separated: keys, array indexes (`items.0`) and `*` (every element;
missing fields become `null`, an empty array gives `[]`). `--pick`, `-o yaml`
and `-o table` apply to 2xx responses only; other responses print unchanged.

## Generating a skill for AI agents

```bash
apic skill pet                       # writes <config dir>/skills/apic-pet/SKILL.md
skillc install -S ~/.config/apic/skills apic-pet
```

The skill lists how to call the site, the exit codes, environment and
authentication names (never base URLs or credentials) and the operations
(grouped by tag when there are more than `--max-ops`). Review it before sharing.

## Output and exit codes

stdout carries only the response body; everything else goes to stderr, so
`apic ... | jq` works. `-i` prints the status line and headers, `-v` the
request line and headers (secrets masked).

| Code | Meaning |
|---|---|
| 0 | 2xx / 3xx (redirects are not followed) |
| 1 | local error: config, definition, parameter validation |
| 2 | usage error: unknown option, command or operation |
| 3 | network error or timeout |
| 4 | HTTP 4xx |
| 5 | HTTP 5xx |
| 6 | refused by write protection |
| 7 | the request succeeded (2xx) but `--pick` / `-o yaml\|table` failed — do not retry |

## Development

```bash
make test     # go vet + unit tests
make lint     # gofmt (+ staticcheck when installed)
make build    # dist/apic
```

Design and plans: [design](docs/design/apic-design.md),
[phase 1 plan](docs/plans/2026-11-10-apic-phase1-plan.md).

## License

[MIT](LICENSE)
