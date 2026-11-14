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
- `--pick 'data.{id,name},code'` extracts values from a JSON response: comma
  separated items from the root, `{...}` groups, `alias=path`; `-o yaml` and
  `-o table` reformat it, keeping numbers and field order intact
- `--all-pages` follows a paginated list (site `paginate` configuration) and
  merges the pages into one array
- `apic use <site> --clear` drops the saved environment and authentication
- `apic skill pet` generates a `SKILL.md` that teaches an AI agent to use the site;
  `apic skill` (no site) generates the generic apic skill
- `exec` authentication: get the token from an external command (vault, SSO CLI…)
- Overlays: rename, hide or fix operations and parameters in `overlays/<site>.yaml`
  without touching the upstream spec
- File uploads: `-F file=@./a.xlsx` sends `multipart/form-data`

## Install

Recommended: install the prebuilt binary with [eget](https://github.com/inherelab/eget)
(Linux, macOS and Windows; the checksum is verified):

```bash
eget install inhere/apic
```

Or download it from [Releases](https://github.com/inhere/apic/releases/latest),
or install with Go:

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
`-f a.b=v` builds nested objects. When the definition declares a top-level
body field as `boolean`, `integer` or `number` (the OpenAPI schema, or
`body.fields` in a simple definition), `-f key=value` is converted to that type
(`-f mock=true` sends `true`) and an invalid value exits 1. A parameter whose name clashes with a global
option is passed with `-p name=value` (or `-p query.name=value`); `all-pages`
and `max-pages` are reserved option names too.

## Recipes

Each recipe assumes a site `pet` whose list API answers
`{"code":0,"data":{"list":[{"id":1,"name":"tom","dept":{"name":"dev"}}],"page":1,"size":10,"total":42}}`.

### Wrap a small API in a few lines

No OpenAPI document? Write the endpoints down and register them:

```yaml
# ~/.config/apic/defs/pet.yaml
operations:
  users:
    get: /api/users
    summary: list users
    query: [keyword, page, size]
  user:
    get: /api/users/{id}             # --id is required
  create-user:
    post: /api/users
    body: {fields: {age: integer, admin: boolean}}  # optional: types for -f key=value
```

```bash
apic site add pet --def ~/.config/apic/defs/pet.yaml --base-url http://127.0.0.1:8080
apic ops pet
apic pet users --keyword tom
apic pet user --id 7
apic pet create-user -f name=tom -f age=3 -f admin=false   # {"admin":false,"age":3,"name":"tom"}
```

### Read a list as a table

```bash
apic pet users --pick 'data.list.*.{id,name,dept=dept.name}' -o table
# id  name  dept
# 1   tom   dev
apic pet users --pick data.total                         # 42
apic pet users --pick 'total=data.total,first=data.list.0.name'   # {"total":42,"first":"tom"}
```

### Fetch every page

Add `paginate` to the site once:

```yaml
sites:
  pet:
    paginate: {page: page, size: size, items: data.list, total: data.total}
```

```bash
apic pet users --all-pages --pick '*.{id,name}' -o table   # paths start at the merged array
apic pet users --size 100 --max-pages 5                    # at most 5 pages; warns if more remain
apic pet users --all-pages | jq length
```

With `--all-pages` the output is the merged `data.list` of every page, so the
`data.list.` prefix is dropped from `--pick`.

### Use it in scripts

```bash
name=$(apic pet user --id 7 --pick data.name)   # strings print bare: tom
apic pet user --id 999 >/dev/null 2>&1; echo $?  # 4 (HTTP 4xx)
```

Branch on the [exit codes](#output-and-exit-codes): 3 network, 4 HTTP 4xx,
5 HTTP 5xx, 6 refused by write protection, 7 the call succeeded but `--pick` or `-o` failed (do not retry).

### Guard production

```yaml
    envs:
      dev:  {base_url: http://127.0.0.1:8080}
      prod: {base_url: https://pet.example.com, protect: true}
```

```bash
apic pet users --env prod                          # reads are allowed
apic pet create-user --env prod -f name=tom        # exit 6: POST on a protected environment requires --yes
apic pet create-user --env prod -f name=tom --yes
```

### Keep tokens out of the config

```yaml
    auths:
      admin: {type: bearer, token: "${env:PET_TOKEN}"}             # from .env or the shell
      vault: {type: bearer, token: "${exec:pass show pet/token}"}  # from a password manager
```

```bash
apic pet users --auth vault
```

### See what is sent

```bash
apic pet create-user -f name=tom --dry-run   # the curl command, token shown as ***
apic pet user --id 7 -v -i                   # request and response headers on stderr
apic pet -X GET /api/users/7 -H 'X-Trace: 1' # a path that is not in the definition
```

### Shortcuts and AI agents

```bash
apic link pet --name petctl && petctl users --keyword tom
apic skill          # SKILL.md teaching an agent to use apic
apic skill pet      # SKILL.md for the pet site
```

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
apic use pet --clear                     # drop the saved environment and auth
apic pet list-users --env dev --auth me  # override for one request
apic auth status pet                     # login state (token never printed)
```

### Authentication per environment

An environment can declare the authentication it uses with `envs.<env>.auth`:

```yaml
    envs:
      build:   {base_url: http://127.0.0.1:18090, auth: build}
      factory: {base_url: http://127.0.0.1:18080, auth: factory, protect: true}
```

The auth for a call is chosen by `--auth`, then `envs.<env>.auth`, then the
auth saved by `apic use`, then `default_auth`. Give every environment of a
multi-environment site its own `auth`: an environment without one uses the
saved auth, so a saved choice can never send another environment's credential
to an environment that declares its own. `apic use` prints the source as
`auth: factory (env)` and a note when a saved `--auth` is shadowed by the
environment. Adding `auth` to an environment switches it to the token cache of
that auth name; a cache left under the old name is no longer used and can be
removed with `apic auth logout <site> --env <env> --auth <old-name>`.

Other behaviours of authentication:

- **Dry run with unset variables**: `--dry-run` tolerates `${env:NAME}` that is
  not set in credential fields (`token`, `password`, `value`, `exec.command`,
  `exec.env`, `login.body`); it exits 0 and prints one stderr line
  `warning: dry-run: NAME is not set; a real call would fail` per variable.
  Real calls and `apic auth status` still fail. The placeholder is shown as
  `***` (header values), URL-encoded in a query string, or as `<exec-token>` /
  `<login-token>`; the warning is the source of truth. Other fields (such as
  `username` or `allow_methods`) with unset variables still fail. A dry run
  never runs `exec` commands and never writes or removes a token cache.
- **401 drops the token**: when the retried request is still 401, the cached
  token of a `login`/`exec` auth is deleted, so `apic auth status` shows
  logged out and the next call logs in again.

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
  job-list:
    get: /api/json
    params:
      - {name: view, in: query, enum: [all, failed, running]}   # allowed values
```

`enum` values are compared as strings: numbers and booleans are converted by
type (`1.0` becomes `"1"`, `0x10` becomes `"16"`), so quote a value to keep it
as written. The list must be non-empty with distinct, non-empty values.

```bash
apic site add jenkins --def ~/.config/apic/defs/jenkins.yaml --base-url https://ci.example.com
apic jenkins job-build --name demo --BRANCH main
```

A ready-to-copy read-only GitLab site (definition, site file with a
`PRIVATE-TOKEN` auth, sample commands) is in [examples/gitlab](examples/gitlab/).

### Pagination

```yaml
sites:
  gitlab:
    paginate:
      page: page          # query parameter of the page number (required)
      size: per_page      # query parameter of the page size (optional)
      size_value: 100     # sent when the call gives no size (needs `size`)
      start: 1            # first page, default 1; 0 is allowed
      items: data.list    # path of the list inside the body; empty: the body is the list
      total: data.total   # optional path of the total count
```

```bash
apic gitlab projects --all-pages --pick '*.{id,path_with_namespace}' -o table
apic gitlab projects --per_page 100 --max-pages 3
```

- `--all-pages` walks the pages; without it nothing changes. `--max-pages N`
  sets the limit and implies `--all-pages`.
- Only GET calls are paginated. A site without `paginate`/`page`, a non-GET
  call or a page/size value that is not an integer exits 2. These checks run
  before the authentication is resolved, so no `${exec:}` or `exec` command
  runs for a usage error.
- The page and size parameters need no declaration: give them with
  `-p <name>=` or `-p query.<name>=`, or let `page`/`size_value` fill them in.
- Every page is a normal request: write protection, authentication, the 401
  retry and `--timeout` apply per page; the `insecure` warning is printed once.
- The merged array is the response body, so `--pick` paths are relative to it
  (`*.{id,name}`, not `data.list.*.{id,name}`). Status and headers come from the
  last page, so `-i` shows a `Content-Length` that does not match the body.
- The run stops on an empty page, when `total` says everything is collected, or
  when a page has fewer items than the first page returned. The first page is
  the reference because servers cap the size they return.
- Two adjacent pages with the same non-empty list exit 7,
  `page N repeats page N-1; check paginate.page`; it catches a wrong page
  parameter name.
- A non-2xx page stops the run: that page is printed as usual, earlier pages
  are dropped, stderr gets `note: page N returned HTTP ...` and the exit code
  follows the status (`3xx` counts as 7). A 2xx page that is not JSON, whose
  `items` is not an array or whose `total` is not an integer exits 7.
- At the limit: with an explicit `--max-pages` the pages collected so far are
  printed with `warning: stopped after N pages (--max-pages); more may exist`
  and exit 0; at the default limit of 100 pages nothing is printed and the run
  exits 7, so a truncated list never looks complete.
- `--dry-run` prints the curl command of the first page only and notes that
  further pages would be requested. All pages stay in memory, so the worst case
  is pages × `--timeout`.

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

### Credential values from a command (`${exec:}`)

```yaml
auths:
  pat:
    type: apikey
    in: header
    name: PRIVATE-TOKEN
    value: ${exec:pass show gitlab/token}
  ad:
    type: login
    login:
      body: {username: me, password: "${exec:powershell -NoProfile -File bin/secret.ps1 ad}"}
```

`${exec:command arg...}` runs a command and uses its stdout. It is allowed in
credential values only: `token`, `password`, `value`, the values of `exec.env`
and the strings of `login.body`. Anywhere else (`exec.command`, `apply.value`,
`login.apply.value`, `token.path`, `allow_methods`, `username`, ...) it exits 1
before anything runs. The text is split on whitespace — no shell, no quoting,
no escaping — and may not be empty or contain `${`. It can be mixed with text
and `${env:NAME}`, as in `"Bearer ${exec:get-token}"`.

- Only the authentication a call selects is resolved, so references in other
  identities never run. Each reference runs at most once per process; a failure
  is not cached.
- It runs when the credential is needed: a site call, `auth login`, `auth
  status`, `auth logout`, and — when the definition has to be fetched with
  `source.auth` — `ops`, `desc`, `skill`, `overlay init`, `site add`,
  `site refresh` and site calls, including `--dry-run`. Completion, `site show`,
  `auth ls`, `config check` and `link` never run it.
- Fixed 30s timeout (unrelated to `--timeout`), empty stdin, the parent
  environment (no `APIC_*` variables) and the config dir as working directory,
  so relative paths resolve there.
- stdout must be one line: trailing `\r`/`\n` are stripped, inner leading and
  trailing spaces are kept. An empty value or a control character exits 1; wrap
  a script if the command prints more than one line.
- The value is registered as a secret. Values shorter than 4 characters are not
  masked in any output.
- `--dry-run` does not run it: a custom header, query or body shows the
  placeholder `<exec>`, while a header that is masked anyway (`Authorization`,
  `Cookie`) shows `***`.
- A failure exits 1 with
  `auth "pat" value: ${exec:} failed (exit 3): <last stderr line>`; the command
  text never appears in a message.
- `config check` prints `runs ${exec:}` for every reference and reports the
  misplaced and the unclosed ones.

> **Rolling back to v0.4.x:** turn every `${exec:...}` back into `${env:...}`
> first. v0.4 sends the command text as the credential (for an `apikey` in the
> query it also reaches access and proxy logs), and its `config check` says
> nothing about it.

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
apic pet list-users --pick data.items.*.name            # ["tom","amy"]
apic pet get-user --id 7 --pick data.name               # tom   (strings print bare)
apic pet list-users --pick 'data.items.*.{id,name}' -o table
apic pet me --pick 'code,data.username'                 # {"code":0,"username":"u"}
apic pet me --pick 'data.{username,id},code'            # {"username":"u","id":7,"code":0}
apic pet me --pick 'uid=data.id,code'                   # {"uid":7,"code":0}
apic pet list-users -o yaml
```

A path is dot separated: keys, array indexes (`items.0`) and `*` (every
element; a missing field becomes `null`, an empty array gives `[]`).

- **Items**: `--pick` is a list of comma separated items and each item starts
  at the root, so `code,data.username` reaches both levels in one call.
- **Groups**: the last segment of a path may be `{...}`; its items are relative
  to that prefix. `data.{username,id},code` gives three keys in one object, and
  `data.items.*.{id,name}` gives an array of objects (two table columns).
- **Aliases**: `alias=path` renames the key (`uid=data.id`); an alias may use
  `[A-Za-z0-9_-]` only.
- **Key names**: the alias, else the last key segment (`0.name` gives `name`).
  A path without a key segment (`*`, `0`) needs an alias; duplicate keys in one
  object are a usage error.
- **Missing values**: a single item that is not an expandable group exits 7
  when the path is missing, exactly like a plain path. With several items, or
  inside a group, a missing path is `null`; under `*` a bad element is `null`
  too.
- **Syntax errors exit 2 before the request**: empty item or segment, unbalanced
  braces, a group that is not the last segment, a leading or trailing comma.
- **`-o table`** on something that is not an array of objects falls back to JSON
  with a note. `--pick`, `-o yaml` and `-o table` apply to 2xx responses only;
  other responses print unchanged.

**Not compatible with v0.4.0** (the old projection is gone): `data.items.*.id,name`
now means two items, both read from the root, so write `data.items.*.{id,name}`.
"A literal key wins over a projection" no longer exists, and a key containing
`.` `,` `=` `{` or `}` cannot be picked at all.

## Generating a skill for AI agents

```bash
apic skill                           # apic's own skill: <config dir>/skills/apic/SKILL.md
apic skill pet                       # a site skill: <config dir>/skills/apic-pet/SKILL.md
skillc install -S ~/.config/apic/skills apic-pet
```

The site skill lists how to call the site, the exit codes, environment and
authentication names (never base URLs or credentials) and the operations
(grouped by tag when there are more than `--max-ops`). Review it before sharing.

Without a site name you get the generic skill: what apic is for, the config
directory layout, `site add`, the simple-definition format with a worked
example, the site configuration, how to call operations, output and exit codes.
It is the same for every machine, so it can be copied to an agent that has apic
installed but no site yet. `--out` and `--name` (default `apic`) apply as for a
site skill; it never reads the site configuration, so a missing or broken config
directory does not matter.

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
make publish  # build-all, then upload dist/ to a dufs server with apic (examples/dufs)
```

Design and plans: [design](docs/design/apic-design.md),
[phase 1 plan](docs/plans/2026-11-10-apic-phase1-plan.md).

## License

[MIT](LICENSE)
