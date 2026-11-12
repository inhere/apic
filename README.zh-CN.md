# apic

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/inhere/apic?style=flat-square)
[![GitHub tag (latest SemVer)](https://img.shields.io/github/tag/inhere/apic)](https://github.com/inhere/apic)
[![Unit-Tests](https://github.com/inhere/apic/actions/workflows/go.yml/badge.svg)](https://github.com/inhere/apic)

---

[English](./README.md) | 简体中文

`apic` 是一个在命令行里调用任意 HTTP API 的工具。它加载接口定义（OpenAPI 3.x / Swagger 2.0，或几行简单 YAML），自动生成命令，不必再给每个服务单独写 CLI。base URL、环境和认证也由它管理。

人和 AI agent 都能直接用：stdout 只输出响应体，退出码固定，生产环境的写操作有保护，密钥在所有输出里都会打码。

## 功能

- 从 OpenAPI / Swagger 的 URL 或文件注册站点，也可以用简单的 YAML 定义注册
- 两种调用方式：
  - 按操作名调用：`apic pet list-users --page 2`
  - 按原始路径调用：`apic pet -X POST /api/users -f name=tom`
- 命令名取自 `operationId`；没有 operationId 时由路径生成（`POST /back/cfg/get` → `back-cfg-get`）
- 环境（base URL）与认证分开配置，用 `apic use` 切换
- 认证类型：`none`、`bearer`、`basic`、`apikey`（请求头或 query）、`login`（先登录拿 token，token 按环境缓存，遇到 401 自动重登）
- 密钥放在 `.env`，配置里用 `${env:NAME}` 引用；`-v`、`--dry-run`、错误信息和 `config show` 里都会打码
- `--dry-run` 打印等价的 `curl` 命令，不发请求
- 写保护：
  - 标记了 `protect: true` 的环境，写操作必须加 `--yes`
  - `allow_methods` 可以把某个认证限制为只读
- 快捷命令：`apic link pet --name petctl` 生成 `petctl` 脚本（Windows 上同时生成 `.cmd`）
- shell 补全（bash、zsh、PowerShell）：站点、操作、参数、枚举值和 `--env`/`--auth` 的取值，快捷命令也能补全
- `--pick 'data.{id,name},code'` 从 JSON 响应里取值：逗号分项从根取，`{...}` 分组，`别名=路径`；`-o yaml`、`-o table` 换格式，数字和字段顺序保持原样
- `--all-pages` 按站点 `paginate` 配置翻页，把各页合并成一个数组
- `apic use <site> --clear` 清除保存的环境和认证
- `apic skill pet` 生成 `SKILL.md`，教 AI agent 使用这个站点；`apic skill`（不带站点）生成 apic 自身的 skill
- `exec` 认证：从外部命令（vault、SSO 工具等）取 token
- overlay：在 `overlays/<site>.yaml` 里给操作改名、隐藏操作、修正参数，不用改上游 spec
- 文件上传：`-F file=@./a.xlsx` 发送 `multipart/form-data`

## 安装

推荐用 [eget](https://github.com/inherelab/eget) 安装预编译的二进制（支持 Linux、macOS 与 Windows，会校验 checksum）：

```bash
eget install inhere/apic
```

也可以从 [Releases](https://github.com/inhere/apic/releases/latest) 下载，或者用 Go 安装：

```bash
go install github.com/inhere/apic/cmd/apic@latest
```

或者从源码构建：

```bash
make install   # 构建到 dist/ 并复制到 $GOPATH/bin
```

## 快速开始

```bash
# 用 OpenAPI / Swagger 文档注册站点
apic site add pet --spec https://pet.example.com/swagger/doc.json
# 文档里没有 host 时，自己给 base URL
apic site add pet --spec ./swagger.json --base-url http://127.0.0.1:8080

apic ops pet                 # 列出操作
apic desc pet create-user    # 查看参数、请求体与响应字段

apic pet list-users --page 2                    # 按操作调用
apic pet create-user -f name=tom -f age:=3      # 用字段拼 JSON 请求体
apic pet create-user -d @user.json              # 从文件读请求体（- 表示 stdin）
apic pet -X GET /api/users -p page=2            # 按原始路径调用
apic pet list-users --dry-run                   # 只打印 curl 命令
```

`-f` 的几种写法：
- `-f key=value` 传字符串；
- `-f key:=<json>` 按 JSON 类型传值；
- `-f a.b=v` 拼出嵌套对象。

参数名与全局选项重名时，用 `-p name=value` 或 `-p query.name=value` 传入。`all-pages`、`max-pages` 也是保留的选项名。

## 常用场景

下面的例子都假设有个站点 `pet`，它的列表接口返回
`{"code":0,"data":{"list":[{"id":1,"name":"tom","dept":{"name":"dev"}}],"page":1,"size":10,"total":42}}`。

### 几行 YAML 接入一个小服务

没有 OpenAPI 文档时，把接口写下来再注册：

```yaml
# ~/.config/apic/defs/pet.yaml
operations:
  users:
    get: /api/users
    summary: list users
    query: [keyword, page, size]
  user:
    get: /api/users/{id}             # 生成必填的 --id
  create-user:
    post: /api/users
```

```bash
apic site add pet --def ~/.config/apic/defs/pet.yaml --base-url http://127.0.0.1:8080
apic ops pet
apic pet users --keyword tom
apic pet user --id 7
apic pet create-user -f name=tom -f age:=3      # {"name":"tom","age":3}
```

### 把列表输出成表格

```bash
apic pet users --pick 'data.list.*.{id,name,dept=dept.name}' -o table
# id  name  dept
# 1   tom   dev
apic pet users --pick data.total                         # 42
apic pet users --pick 'total=data.total,first=data.list.0.name'   # {"total":42,"first":"tom"}
```

### 取全部分页

先在站点上配一次 `paginate`：

```yaml
sites:
  pet:
    paginate: {page: page, size: size, items: data.list, total: data.total}
```

```bash
apic pet users --all-pages --pick '*.{id,name}' -o table   # 路径从合并后的数组开始
apic pet users --size 100 --max-pages 5                    # 最多 5 页；还有剩余时给出警告
apic pet users --all-pages | jq length
```

加了 `--all-pages` 后，输出是各页 `data.list` 合并成的数组，所以 `--pick` 里不再写 `data.list.` 前缀。

### 在脚本里用

```bash
name=$(apic pet user --id 7 --pick data.name)   # 字符串直接输出：tom
apic pet user --id 999 >/dev/null 2>&1; echo $?  # 4（HTTP 4xx）
```

按[退出码](#输出与退出码)分支：3 网络错误，4 HTTP 4xx，5 HTTP 5xx，6 被写保护拦下，7 请求成功但 `--pick` 或 `-o` 失败（不要重试）。

### 保护生产环境

```yaml
    envs:
      dev:  {base_url: http://127.0.0.1:8080}
      prod: {base_url: https://pet.example.com, protect: true}
```

```bash
apic pet users --env prod                          # 读不受限制
apic pet create-user --env prod -f name=tom        # 退出 6：POST on a protected environment requires --yes
apic pet create-user --env prod -f name=tom --yes
```

### 凭证不写进配置

```yaml
    auths:
      admin: {type: bearer, token: "${env:PET_TOKEN}"}             # 来自 .env 或 shell 环境变量
      vault: {type: bearer, token: "${exec:pass show pet/token}"}  # 来自密码管理器
```

```bash
apic pet users --auth vault
```

### 看清发出去的请求

```bash
apic pet create-user -f name=tom --dry-run   # 打印 curl 命令，token 显示为 ***
apic pet user --id 7 -v -i                   # 请求头与响应头输出到 stderr
apic pet -X GET /api/users/7 -H 'X-Trace: 1' # 定义里没有的路径也能调
```

### 快捷命令与 AI agent

```bash
apic link pet --name petctl && petctl users --keyword tom
apic skill          # 生成教 agent 使用 apic 的 SKILL.md
apic skill pet      # 生成 pet 站点的 SKILL.md
```

## 配置

配置目录默认是 `~/.config/apic`，可以用 `APIC_CONFIG_DIR` 覆盖：

```
~/.config/apic/
  config.yaml          # 你的配置和站点（apic 不会改写它）
  .env                 # 密钥：KEY=VALUE；已导出的系统环境变量优先
  sites/<name>.yaml    # 一个文件一个站点，由 `apic site add` 写入
  defs/<name>.yaml     # 简单定义
  state.yaml           # 各站点当前的 env/auth、已生成的快捷命令
  cache/               # 缓存的接口定义与登录 token（权限 0600）
```

下面这个站点有两个环境、三个认证：

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
apic use pet --env prod --auth ro        # 切换当前选择
apic use pet --clear                     # 清除保存的环境和认证
apic pet list-users --env dev --auth me  # 只对这一次请求覆盖
apic auth status pet                     # 查看登录状态（不会输出 token）
```

### 按环境指定认证

环境可以用 `envs.<env>.auth` 声明自己使用的认证：

```yaml
    envs:
      build:   {base_url: http://127.0.0.1:18090, auth: build}
      factory: {base_url: http://127.0.0.1:18080, auth: factory, protect: true}
```

- **选择优先级**：`--auth`，然后 `envs.<env>.auth`，然后 `apic use` 保存的认证，最后 `default_auth`。
- **建议**：多环境站点给每个环境都写 `auth`。没写的环境会使用保存的认证；写了的环境不会被保存的选择改走，凭证不会发到别的环境。
- **显示**：`apic use` 输出 `auth: factory (env)` 这样的来源后缀；保存的 `--auth` 被环境声明遮蔽时，stderr 给出提示。
- **旧缓存**：环境加上 `auth` 后改用新认证名的 token 缓存；旧名字下残留的缓存不再被使用，可用 `apic auth logout <site> --env <env> --auth <旧名>` 清除。
- **dry-run 缺变量**：凭证字段（`token`、`password`、`value`、`exec.command`、`exec.env`、`login.body`）里未设的 `${env:NAME}` 不再让 `--dry-run` 失败，退出 0，并为每个变量在 stderr 写一行 `warning: dry-run: NAME is not set; a real call would fail`。
- **dry-run 的限制**：
  - 真实调用和 `apic auth status` 仍然失败；其他字段（如 `username`、`allow_methods`）缺变量也仍然失败。
  - 占位显示为 `***`（头的值）、URL 编码形式（query）或 `<exec-token>`/`<login-token>`，是否缺变量以警告为准。
  - dry-run 不执行 `exec` 命令，也不写、不删 token 缓存。
- **401 作废**：重发后仍是 401 时，`login`/`exec` 认证缓存的 token 会被删除，`apic auth status` 显示 logged out，下次调用重新取 token。

### 简单定义

接口不多的服务（Jenkins、小工具服务），写一个简短的 YAML 就行，不用写 swagger：

```yaml
# defs/jenkins.yaml
operations:
  job-info:
    get: /job/{name}/api/json        # {name} 自动成为必填参数
    summary: 查看 job 信息
  job-build:
    post: /job/{name}/buildWithParameters
    query: [BRANCH, ENV]             # 可选的字符串 query 参数
  job-log:
    get: /job/{name}/{build}/consoleText
    output: raw
  job-list:
    get: /api/json
    params:
      - {name: view, in: query, enum: [all, failed, running]}   # 允许的取值
```

`enum` 按字符串比较：数字和布尔值按类型转换（`1.0` 得到 `"1"`，`0x10` 得到 `"16"`），需要原样比较时请加引号；列表不能为空，元素不能为空字符串或重复。

```bash
apic site add jenkins --def ~/.config/apic/defs/jenkins.yaml --base-url https://ci.example.com
apic jenkins job-build --name demo --BRANCH main
```

可直接复制使用的只读 GitLab 站点（定义文件、带 `PRIVATE-TOKEN` 认证的站点文件、示例命令）见 [examples/gitlab](examples/gitlab/)。

### 分页

```yaml
sites:
  gitlab:
    paginate:
      page: page          # 页码参数名（必填）
      size: per_page      # 每页条数参数名（可选）
      size_value: 100     # 用户没给条数时自动带上的值（需要 size）
      start: 1            # 首页页码，默认 1，允许 0
      items: data.list    # 列表在响应中的路径；不写表示响应本身就是数组
      total: data.total   # 总数在响应中的路径（可选）
```

```bash
apic gitlab projects --all-pages --pick '*.{id,path_with_namespace}' -o table
apic gitlab projects --per_page 100 --max-pages 3
```

- **开启方式**：`--all-pages` 翻页，不加时行为不变；`--max-pages N` 设置上限，并隐含 `--all-pages`。
- **前提检查**：只支持 GET；站点没配 `paginate`/`page`、方法不是 GET、页码或条数不是整数，都退出 2。这些检查在解析认证之前做，所以用法错误不会去跑 `${exec:}` 或 `exec` 命令。
- **参数**：页码和条数不需要操作声明，用 `-p <名>=` 或 `-p query.<名>=` 传入；没给时用 `page`/`size_value` 自动补上。
- **每页都是普通请求**：写保护、认证、401 重试和 `--timeout` 都按页生效；`insecure` 告警只打印一次。
- **合并结果**：合并后的数组就是响应体，`--pick` 的路径相对它（写 `*.{id,name}`，不是 `data.list.*.{id,name}`）。状态码和响应头取最后一页，所以 `-i` 的 `Content-Length` 与合并后的 body 不一致。
- **停止条件**：本页 0 条；配了 `total` 时已取够总数；没配 `total` 时本页条数少于第一页实际返回的条数（服务端常会限制条数，所以以第一页为准）。
- **重复页**：相邻两页的非空列表完全相同则退出 7，提示 `page N repeats page N-1; check paginate.page`，用来发现页码参数名写错。
- **某页失败**：非 2xx 时停止，该页原样输出，前面的页丢弃，stderr 提示 `note: page N returned HTTP ...`，退出码按状态码映射（3xx 算 7）。2xx 但不是 JSON、`items` 不是数组或 `total` 不是整数时退出 7。
- **上限**：显式 `--max-pages` 截断时，已取到的结果显示出来，stderr 提示 `warning: stopped after N pages (--max-pages); more may exist`，退出 0；默认上限 100 页截断时什么都不输出，退出 7，避免脚本把截断的列表当成完整结果。
- **dry-run**：只打印第一页的 curl 命令，并提示还会请求后续页。全部页都留在内存里，最坏耗时是页数乘 `--timeout`。

### 由外部命令提供 token（`exec`）

```yaml
auths:
  sso:
    type: exec
    exec:
      command: [/usr/local/bin/corp-sso, token, --audience, pet]  # argv，不经过 shell
      env: {SSO_SECRET: "${env:PET_SSO_SECRET}"}                   # 传给命令的环境变量
      ttl: 30m          # 缓存时长；没有 ttl 或 expires 时每次请求都执行命令
      # format: json, token: {path: data.token}, expires: {path: expires_in}
    apply: {header: Authorization, value: "Bearer {token}"}
```

- **执行方式**：命令不经过 shell，stdin 为空，工作目录是配置目录，相对路径也以配置目录为基准。
- **超时**：超过 `timeout`（默认 30s）就会被终止。
- **输出与重试**：命令参数不会出现在任何输出里；收到 401 时重新执行一次命令。

> **安全提示**：
> - **配置文件等同脚本**：有了 `exec`，`config.yaml` 和 `sites/*.yaml` 都能让 apic 执行命令，不要导入不信任的站点文件。
> - **Windows 限制**：token 缓存的 0600 权限不生效；命令是 `.bat`/`.cmd` 时，含 `& | < > ^ % ! "` 的参数会被拒绝。

### 凭证值来自命令（`${exec:}`）

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

`${exec:命令 参数...}` 执行命令并取其 stdout。只能用凭证值中：`token`、`password`、`value`、`exec.env` 的值、`login.body` 里的字符串。放在其他字段（`exec.command`、`apply.value`、`login.apply.value`、`token.path`、`allow_methods`、`username` 等）会在执行任何东西之前退出 1。内容按空白切分，不经过 shell，不支持引号和转义；内容为空或含 `${` 会报错。可以和文字、`${env:NAME}` 混写，如 `"Bearer ${exec:get-token}"`。

- **只解析被选中的认证**：其他认证里的引用不会执行。同一引用在一个进程内最多执行一次；失败不缓存。
- **会执行的入口**：站点调用、`auth login`、`auth status`、`auth logout`，以及需要拉取定义且配了 `source.auth` 时的 `ops`、`desc`、`skill`、`overlay init`、`site add`、`site refresh` 和站点调用（含 `--dry-run`）。补全、`site show`、`auth ls`、`config check`、`link` 创建不执行。
- **运行方式**：超时固定 30s（与 `--timeout` 无关），stdin 为空，环境变量继承父进程（不设 `APIC_*`），工作目录是配置目录，所以相对路径以配置目录为基准。
- **输出要求**：只能是一行，末尾的 `\r`/`\n` 会去掉，首尾空格保留。结果为空或含控制字符退出 1；命令输出多行时请包一层脚本。
- **脱敏**：结果登记为密钥；少于 4 个字符的结果不会在任何输出里打码。
- **dry-run**：不执行，自定义头、query 或请求体里显示占位 `<exec>`；本来就整体遮蔽的头（`Authorization`、`Cookie`）显示 `***`。
- **失败**：退出 1，消息为 `auth "pat" value: ${exec:} failed (exit 3): <stderr 最后一行>`，命令文本不会出现在任何消息里。
- **config check**：每处 `${exec:}` 输出 `runs ${exec:}`；放错位置和未闭合的引用也会提示。

> **回滚到 v0.4.x**：先把所有 `${exec:...}` 改回 `${env:...}`。v0.4 会把命令文本当作凭证发出去（`apikey` 放在 query 时还会进访问日志和代理日志），它的 `config check` 也不会提示。

## 用 overlay 修正定义

生成的命令不好用，或者 spec 写错了，就写 `<配置目录>/overlays/<site>.yaml`，不用改 spec。`site refresh` 后仍然有效，调用、`ops`、`desc`、补全和 `apic skill` 都按覆盖后的结果工作。

```yaml
operations:
  "POST /back/cfg/get":          # METHOD /path，与 apic ops 的 PATH 列一致
    name: cfg-get
    summary: 查询打印配置
  "POST /back/cfg/delete":
    hide: true
  users-list:                    # 也可以用生成的操作名
    output: raw
    body: multipart              # json | form | multipart | xml | raw
    params:
      status: {required: true, enum: [active, banned]}
      debug:  {hide: true}       # 不生成选项，-p debug=... 会被拒绝
      tenant: {in: header, required: true}   # 新增参数：in 为 query 或 header
```

```bash
apic overlay init pet          # 生成 overlays/pet.yaml，每个操作一条空条目
apic overlay init pet --stdout
apic config check              # 检查 overlay 的错误和找不到的键
```

- **找不到的键**：只警告。
- **结构错误**：取值非法、改名冲突、隐藏 path 参数或必填参数、新增 `Authorization`/`Cookie`/`Host`/`Content-*` 等头，都属于结构错误。这时该站点的命令都退出 1，`site refresh/show/rm` 和 `overlay init` 除外；`config check` 也退出 1。
- **desc 标注**：改过的字段标 `(overlay)`，隐藏参数标 `(hidden)`；`desc -o json` 增加 `overlay` 数组。
- **hide 不是访问控制**：隐藏的操作仍能用原始路径调用，隐藏的参数仍能用 `-H` 或原始路径调用带上。危险接口请用 `protect` 和 `allow_methods`。
- **保留名**：名为 `overlay` 的站点会被跳过并警告。

## 文件上传（multipart）

```bash
apic pet import-users -F file=@./users.xlsx -F type=user
apic pet upload -F 'file=@./a.bin;type=application/zip;filename=report.zip'
apic pet upload -F 'file=@"./a;b.bin";filename=ab.bin'   # 路径含 ; 或 " 时加双引号
apic pet -X POST /upload -F file=@./a.txt                # 原始路径调用
```

- **哪些操作走 multipart**：spec 里写了 `multipart/form-data` 的操作按 multipart 发送；其他操作要用 overlay 改请求体类型（`body: multipart`）。
- **只有 `-F name=@path` 读文件**：`-F name=value`、`-f name=value` 都是文本，`-f name=@x` 发送的就是 `@x` 这个字面值。不支持 `-F name=@-` 和 curl 的 `-F name=<file`，以 `<` 开头的值按文本发送。
- **type 与名字**：`type` 只能是不带参数的 `type/subtype`；不写时按内置扩展名表推断，推断不出用 `application/octet-stream`。字段名和文件名不能含 `"`、`\` 和控制字符；路径最后一段含这些字符时，用 `;filename=` 另给文件名。
- **组合限制**：`-F` 不能和 `-d`、`-f` 或 `Content-Type` 头一起用；multipart 操作上单独用 `-d` 时，必须用 `-H 'Content-Type: ...'` 给出类型。
- **读取限制**：只读普通文件，写保护检查通过后才读；整个请求体上限 64 MiB，更大的上传请用 curl。`--dry-run` 打印的 curl 命令发出的部件与直接发送相同；`-v` 只列出各部件，不打印内容。

> **安全提示**：`-F name=@path` 和 `-d @file` 都会把本地文件发出去。`-d @file` 在写保护检查之前读取，也没有大小上限。Windows 上 UNC 路径（`\\host\share\...`）在打开时就会向该主机认证。

## shell 补全

```bash
eval "$(apic --gen-completion bash)"        # zsh 同理；PowerShell：apic --gen-completion pwsh | Invoke-Expression
eval "$(apic link ls --completion bash)"    # 快捷命令；link / link rm 之后重新执行
```

- **无副作用**：补全只读缓存的定义，不发请求、不登录、不执行 `exec` 命令。
- **已知限制**：选项值之后（`apic pet --env dev list-users --<TAB>`）只能补出站点层的选项。

## 取值与换格式

```bash
apic pet list-users --pick data.items.*.name            # ["tom","amy"]
apic pet get-user --id 7 --pick data.name               # tom（字符串不带引号）
apic pet list-users --pick 'data.items.*.{id,name}' -o table
apic pet me --pick 'code,data.username'                 # {"code":0,"username":"u"}
apic pet me --pick 'data.{username,id},code'            # {"username":"u","id":7,"code":0}
apic pet me --pick 'uid=data.id,code'                   # {"uid":7,"code":0}
apic pet list-users -o yaml
```

- **路径写法**：路径用点号分段，段可以是键名、数组下标（`items.0`）或 `*`。
- **`*` 的规则**：`*` 表示每个元素，缺少的字段填 `null`，空数组得到 `[]`。
- **分项**：`--pick` 是逗号分隔的多项，每项都从根开始，所以 `code,data.username` 一次就能取到两个层级的字段。
- **分组**：路径最后一段可以写成 `{...}`，组内各项相对该前缀。`data.{username,id},code` 在一个对象里给出三个键；`data.items.*.{id,name}` 给出对象数组（表格的两列）。
- **别名**：`别名=路径` 用来改键名（`uid=data.id`），别名只能用 `[A-Za-z0-9_-]`。
- **键名**：取别名，否则取最后一个键名段（`0.name` 得到 `name`）。没有键名段的路径（`*`、`0`）必须写别名；同一对象里键名重复是用法错误。
- **缺失**：只有一项且不是可展开的分组时，路径缺失退出 7，与普通单路径一致。多项或在分组里缺失得 `null`；`*` 之下元素错误也得 `null`。
- **语法错误在请求前退出 2**：空项或空段、花括号不配对、分组不在最后一段、以逗号开头或结尾。
- **`-o table`**：结果不是对象数组时回退为 JSON 并提示。`--pick`、`-o yaml`、`-o table` 只对 2xx 响应生效，其他响应原样输出。

**与 v0.4.0 不兼容**（旧的投影写法已取消）：`data.items.*.id,name` 现在表示两项、都从根取，要写成 `data.items.*.{id,name}`。“字面键优先于投影”的规则不再存在；键名本身含 `.` `,` `=` `{` `}` 时无法用 `--pick` 访问。

## 生成给 AI agent 用的 skill

```bash
apic skill                           # apic 自身的 skill：<配置目录>/skills/apic/SKILL.md
apic skill pet                       # 站点 skill：<配置目录>/skills/apic-pet/SKILL.md
skillc install -S ~/.config/apic/skills apic-pet
```

- **站点 skill 内容**：怎么调用、退出码、env 和 auth 的名字、操作清单；操作数超过 `--max-ops` 时按 tag 汇总。
- **不含**：base URL 和凭证。
- **分享前**：先检查一遍。
- **不带站点名**：得到通用的 apic skill，讲清 apic 用来做什么、配置目录布局、`site add`、简易定义格式与完整示例、站点配置、怎么调用、输出与退出码。它不依赖本机，可拷给装了 apic、还没有站点的 agent。`--out`、`--name`（默认 `apic`）与站点 skill 相同；生成时不读站点配置，配置目录不存在或有错也能生成。

## 快捷命令

```bash
apic link pet --name petctl                 # petctl list-users 等于 apic --api pet list-users
apic link pet --name pet-prod --env prod --auth ro
apic link ls
apic link rm petctl
```

- **生成位置**：脚本默认写到 `defaults.link_dir`，未配置时为 `~/.local/bin`；Windows 上为 `%USERPROFILE%\bin`。
- **Windows**：会额外生成 `.cmd` 版。`cmd.exe` 会把 `%*` 重新解析一遍，所以在 Windows 上传 JSON 请求体请用 `-d @file`。

## 输出与退出码

- **stdout**：只输出响应体，其他信息都写到 stderr，所以 `apic ... | jq` 可以直接用。
- **`-i`**：输出状态行和响应头。
- **`-v`**：输出请求行和请求头，密钥会打码。

| 码 | 含义 |
|---|---|
| 0 | 2xx / 3xx（不跟随重定向） |
| 1 | 本地错误：配置、定义、参数校验 |
| 2 | 用法错误：未知选项、命令或操作 |
| 3 | 网络错误或超时 |
| 4 | HTTP 4xx |
| 5 | HTTP 5xx |
| 6 | 被写保护拒绝 |
| 7 | 请求已成功（2xx），但 `--pick` 或 `-o yaml\|table` 处理失败，不要重试 |

## 开发

```bash
make test     # go vet + 单元测试
make lint     # gofmt（装了 staticcheck 时一并检查）
make build    # 生成 dist/apic
```

设计与计划：[设计文档](docs/design/apic-design.md)、[一期计划](docs/plans/2026-11-10-apic-phase1-plan.md)。

## License

[MIT](LICENSE)
