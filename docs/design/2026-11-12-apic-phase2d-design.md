<!-- template_id: design; template_version: 1.2.0 -->
# apic 二期第四批：跨层取值、分页、清除选择、拉取定义跟随 --env、${exec:} 设计

> 状态：Draft 0.1 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-12 | Claude（inhere：下一批做 1–4 + `${exec:}`，分页参数名在站点配置里定义） | 初稿：五项的根因与方案 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 设计工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：解决 v0.4.0 试用后剩下的五个障碍：一次取不同层的字段，列表要手动翻页，保存的选择清不掉，拉取定义不跟随 `--env`，token 只能明文放在 `.env`。
- Scope freeze：只做 issue my-tools-ab5f 中用户确认的四项，加上 `${exec:}`。
  - 不做：`--columns` 与 overlay 默认列（由第 1 项覆盖）、响应头（`Link`/`X-Next-Page`）和响应体游标式翻页、按操作覆盖分页配置、`${keyring:}`、`${env:X:-默认值}`、oauth2、重定向跟随（deferred）。
- 暴露度：v0.4.0 已公开发布；第 5 项从配置执行外部命令取密钥。按高暴露处理（SR1409），走两轴独立评审。
- Review budget：一轮评审，两个方向：正确性与可实施性；安全与对外契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。

## 背景与目标

| # | 现象 | 根因（代码） |
|---|---|---|
| 1 | `--pick` 取不到 `code` 加 `data.username` 这种不在同一层的字段 | `model.Pick` 的投影只作用于最后一段，键都相对上一级（`model/pick.go` `projectPick`） |
| 2 | GitLab 默认每页 20 条，DeployHub 的 apps/hosts/tasks 都要翻页，只能一页一页手动调 | 每次调用只发一个请求（`commands/siterun.go` `run` → `request.Execute`） |
| 3 | `apic use` 保存的环境或认证无法清除，只能手改 `state.yaml` | `commands/use.go` 只会写入，不会删除 |
| 4 | 调用时给了 `--env`，拉取定义仍按保存的或默认的环境建 HTTP 客户端（TLS、`insecure`）和定义认证的缓存 | `wiring.FetchOptions` 与 `commands/site.go` `loadIndex` 只看 state 和 `default_env` |
| 5 | token 只能写进 `.env` 或系统环境变量（明文），或写一整个 exec 认证 | 值引用只有 `${env:}`（`config/expand.go`） |

用户可见的结果：

- `--pick '{code,user=data.username}'` 从根取多个路径，组成对象；投影里也能写 `data.username,$.code`。
- 站点配置 `paginate` 写上页码、条数参数名和列表路径，`--all-pages` 自动翻页，合并成一个数组输出。
- `apic use <site> --clear` 清除该站点保存的选择。
- `apic <site> <op> --env X` 拉取定义时也用环境 X 的设置。
- 凭证字段可写 `${exec:pass show gitlab}`，从密码管理器或系统凭据取值，不落盘。

## 名词

| 名词 | 含义 |
|---|---|
| 路径对象 | `--pick` 整体写成 `{...}`，每项是从根开始的路径，结果是对象 |
| 根引用 | 投影中以 `$.` 开头的项，从响应根取值 |
| 分页配置 | 站点的 `paginate`：页码参数名、条数参数名、列表路径等 |
| 值引用 | 配置值中的 `${env:NAME}` 和 `${exec:命令 参数...}` |
| 凭证字段 | 沿用第三批：`token`、`password`、`value`、`exec.command`、`exec.env`、`login.body` |

其余名词沿用一期设计（[apic-design.md](apic-design.md)）与二期各批设计（[第一批](2026-11-11-apic-phase2-design.md)、[第二批](2026-11-11-apic-phase2b-design.md)、[第三批](2026-11-12-apic-phase2c-design.md)）。

## 范围与非目标

范围（只改 apic 仓库）：

1. `--pick` 的路径对象 `{...}`，以及投影中的根引用 `$.`。
2. 站点级分页配置与 `--all-pages`、`--max-pages`。
3. `apic use <site> --clear`。
4. 拉取定义使用本次调用的 `--env`。
5. 凭证字段中的值引用 `${exec:}`。

非目标：

- 分页只支持“页码 + 条数”式的 query 参数；不读 `Link`、`X-Next-Page` 等响应头，不支持响应体里的游标或 next 链接。
- 分页配置只到站点级，不能按操作覆盖（同一站点的列表接口结构不一致时，另建站点或等后续批次）。
- 不做并发翻页，不做流式输出（全部页拿齐后一次输出）。
- `${exec:}` 不缓存结果、不经过 shell、不支持引号和转义；需要复杂命令时写成脚本。不做 `${keyring:}`。
- `--pick` 仍不实现 jq 语法，不支持过滤和函数。
- `--columns`、overlay 默认列：路径对象已能选列、定列序、改列名。

## 已确认事实与规范

- `--pick` 在请求发出前由 `model.ParsePick` 解析，失败退出 2；求值失败退出 7（`output.postProcess`）。exec 认证的 `token.path`/`expires.path` 也用 `model.Pick`。投影的现有规则见第三批 D5：最后一段含逗号；字面键优先；缺键为 `null`；结果是 `model.Object`，键序即给出的顺序。
- `output.Table` 对对象数组按键首次出现的顺序取列；非对象元素回退为 JSON。
- 生成的参数选项会避开 `reservedOptNames`（`commands/globalopts.go`）；与保留名同名的参数只能用 `-p name=v` 传入。
- `request.Execute` 负责一次请求：写保护、认证、发送、401 重试一次、仍 401 时作废 token。超时 `--timeout` 作用于一次 `Execute`。
- `request.Build` 生成 `Prepared`（Method、URL、Header、Body）。query 参数在 URL 中。
- state 每个站点保存一份 `{env, auth}`（`config/state.go`）；`config.SaveState` 写回。
- run 路径先 `Prescan`（只扫描站点名之前的全局选项），再 `loadSiteIndex` 生成动态命令，最后才由 gcli 解析 `--env` 等选项（`commands/run.go`、`prescan.go`）。
- `loadIndex` 在拉取前按所选环境判断 `insecure` 并告警；`FetchOptions` 用该环境建客户端，并为 `source.auth` 建 provider（缓存键含环境名）。只有 run 路径（站点调用）有 `--env`；`site add/refresh`、`ops`、`desc`、`skill`、`overlay` 没有。
- exec 认证的命令执行（`auth/exec.go` `runExec`）：
  - 命令解析：相对路径按配置目录，裸名经 PATH，不匹配当前目录；Windows 有批处理参数检查。
  - 运行环境：不经过 shell；stdin 为空设备；输出上限 4 MiB；默认超时 30s；子进程环境为父环境加 `APIC_SITE/ENV/AUTH`。
- `config.Expand` 只认 `${env:}`。`config.ResolveAuth` 只解析所选的一个认证，凭证字段的值登记为密钥。dry-run 的 `ResolveAuthDryRun` 只对凭证字段容忍未设变量。
- `config check` 的 `isReference` 对任意 `${...}` 整串都视为引用，不报明文。
- 分层：config/auth/request/output/spec 只依赖 model；wiring 与 commands 在上层（`internal/layering_test.go`）。
- 规范：GR107–GR110、SR2003（gcli 参数先声明再取值）。

## 总体方案

五项彼此独立，不新增包、不改分层。

### 1. `--pick` 路径对象与根引用

**路径对象**：整个值以 `{` 开头、`}` 结尾。

```bash
apic dh me --pick '{code,data.username}'           # {"code":0,"data.username":"u"}
apic dh me --pick '{code,user=data.username}'      # {"code":0,"user":"u"}
apic gitlab projects --pick '{n=0.name,first=0.id}'
```

- 花括号内按逗号分项。每项写作 `路径`，或 `别名=路径`。路径从根开始，语法同普通 pick，可含 `*`，但不能再含逗号（不嵌套投影）。
- 结果是对象：
  - 键为别名，没写别名时就是路径原文；键序即书写顺序。
  - 某个路径取不到时，该键为 `null`，与投影的缺键一致。路径写错只会得到 `null`，不报错。
- 语法错误在请求前报出，退出 2：空项、别名或路径为空、键重复（含别名与路径原文重复）、花括号不配对、项内含逗号或花括号。
- 别名只允许 `[A-Za-z0-9_-]+`，所以表格列名和 YAML 键都安全。
- 以 `{` 开头的值一律按路径对象解析。根上恰好有以 `{` 开头的字面键时，这类键无法用 `--pick` 访问，与“键名含点无法访问”同类。

**根引用**：投影的某一项以 `$.` 开头时，从响应根取值。

```bash
apic dh me --pick 'data.username,id,$.code'                    # {"username":…,"id":…,"code":0}
apic dh apps --pick 'data.list.*.id,name,$.data.total' -o table
```

- 根引用项的键名为路径的最后一段（`$.data.total` 的键为 `total`），与同一投影中的其他键重复时报错，退出 7（与现有投影语法错误一致，因为投影的语法检查推迟到求值时）。需要自定义键名时改用路径对象。
- 在 `*` 之下，根引用对每个元素取同一个根值，每行都相同。
- 解析：只有含逗号的投影段才识别 `$.`。`ParsePick` 先找到第一个逗号，从它往后的项中，以 `$.` 开头的项一直延伸到下一个逗号，不再按点切分。不含逗号时 `$` 仍是普通键名，与现在一致。
- 字面键优先规则不变：对象恰有与整段同名的字面键时取该键。

**实现**：`model.Pick` 增加路径对象与根引用两种形态，`Apply` 时把根值一路传下去。exec 的 `token.path` 同样会接受这两种写法，但结果是对象，会被现有的“token 必须是字符串”校验拒绝，不另加限制。

### 2. 分页

```yaml
sites:
  gitlab:
    paginate:
      page: page          # 页码参数名（必填）
      size: per_page      # 每页条数参数名（可选）
      size_value: 100     # 用户没给条数参数时自动带上的值（可选，需要 size）
      start: 1            # 首页页码，默认 1
      items: ""           # 列表在响应中的路径；空表示响应本身就是数组
  dh:
    paginate: {page: page, size: size, size_value: 100, items: data.list, total: data.total}
```

- 配置项：
  - `page`：必填。
  - `size`、`size_value`、`total`：可选。
  - `start`：默认 1，允许 0。
  - `items`、`total`：都是 pick 路径（不允许投影、路径对象和 `*`），加载时校验，错误退出 1。
- 只有加 `--all-pages` 才翻页；不加时行为完全不变。`--max-pages N` 设上限，默认 100，同时隐含 `--all-pages`。两个都加入 `reservedOptNames`。
- 前提检查（退出 2）：
  - 站点没配 `paginate`；
  - 方法不是 GET。
  - 页码与条数参数不要求操作声明过，直接写入 URL query；原始路径调用同样适用。
- 翻页循环（`request` 新增 `Paginate`，每页调用一次 `Execute`，401 重试与作废逐页生效）：
  1. 起始页码取用户给的页码参数，否则取 `start`。
  2. 条数取用户给的条数参数，否则取 `size_value`，都没有就不带。
  3. 每页把页码写入 URL query（覆盖同名参数），发送。
  4. 非 2xx：停止，按普通响应原样输出该页（退出码由状态码决定），前面各页丢弃，stderr 提示 `note: page N returned HTTP 500; M earlier pages discarded`。
  5. 网络错误：直接返回该错误。
  6. 2xx：解析 JSON，按 `items` 取列表，取不到或不是数组时退出 7（`--all-pages failed: page N: items "data.list" is not an array`）。
  7. 追加列表。满足以下任一条件即停止：
     - 本页 0 条；
     - 已知条数且本页条数少于它；
     - 配了 `total` 且已取到的条数达到总数；
     - 达到上限，此时 stderr 警告 `warning: stopped after N pages (--max-pages); more may exist`，退出码不变。
- 输出：
  - 合并后的数组作为响应体，`--pick` 和 `-o` 作用在这个数组上，例如 `--all-pages --pick '*.id,name' -o table`。注意路径相对合并后的数组，不再带 `data.list` 前缀。
  - 状态码与响应头取最后一页（`-i` 打印最后一页的头）。
- dry-run：只打印第一页的 curl，stderr 提示 `note: dry-run: --all-pages would request further pages`。
- `-v`：每页的请求行照常打印。

### 3. 清除保存的选择

- `apic use <site> --clear` 删除 `state.yaml` 中该站点的选择（环境与认证都删），写回 state 后按清除后的状态打印，与无参 `use` 一致。
- 与 `--env`/`--auth` 同时给出时报用法错误，退出 2。
- 站点没有保存的选择时同样成功，幂等。
- 快捷命令自带 `--env`/`--auth`，不受影响。

### 4. 拉取定义使用本次 `--env`

- `loadIndex` 与 `wiring.FetchOptions` 增加参数 `envFlag`，环境按 `envFlag` > 保存的 > `default_env` 选，`insecure` 告警也按这个环境判断。
- run 路径在 gcli 解析参数之前就加载定义（要靠它生成动态命令），这时 `g.Env` 还没有值。因此 `Prescan` 在找到站点后，继续扫描站点之后的参数（到 `--` 为止），取最后一个 `--env X` 或 `--env=X`，交给 `loadSiteIndex`。
  - 其余调用点传空串，行为不变。
  - 极端情况下（例如某个选项的值本身就是字面量 `--env`），预扫描结果可能与 gcli 解析结果不同。这只影响拉取定义时用哪个环境的 TLS 设置，与现状同类，不另做校验。
- `--env` 写错时由 `Resolve` 报错。拉取在 `Resolve` 之前，所以 `loadIndex` 遇到不存在的环境也报同样的错误（`site %q has no environment %q`，退出 1），不再静默用零值环境。
- `source.auth` 的选择不变（第三批 D 非目标：拉取定义的认证不受环境声明影响），只是缓存键里的环境名跟着变。

### 5. 值引用 `${exec:}`

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

- 语法：`${exec:命令 参数...}`，内容按空白切分，不支持引号和转义，内容中不能出现 `}`。同一串中可与文字、`${env:}` 混写，如 `"Bearer ${exec:get-token}"`。
- 允许出现的位置：`token`、`password`、`value`、`exec.env`、`login.body` 中的字符串。出现在其他字段（含 `exec.command`、`allow_methods`、`login.path` 等）时解析报错，退出 1：`${exec:} is only allowed in credential values`。
- 执行：复用 exec 认证的执行器，命令解析、空 stdin、4 MiB 上限、Windows 批处理检查都相同。
  - 超时固定 30s，但不超过本次请求剩余的时间。
  - 工作目录为配置目录。
  - 环境为父进程环境加 `APIC_SITE`、`APIC_ENV`、`APIC_AUTH`。
- 输出：
  - 取 stdout，去掉末尾的 `\r`、`\n`；结果为空，或含其他控制字符时报错。
  - 结果登记为密钥，参与脱敏。
- 失败：退出 1，消息为 `auth "pat": ${exec:pass}: exit 1: <stderr 最后一行>`。只显示命令名，不显示参数，整条消息经脱敏。
- 只解析所选的认证，没被选中的认证里的 `${exec:}` 不执行。同一进程内，同一引用文本只执行一次，例如拉取定义和调用都用到同一认证时。不跨进程缓存；需要缓存时改用 `type: exec` 并设 `ttl`。
- dry-run：不执行，保留 `${exec:...}` 原文、不登记、不告警（命令文本不是密钥）；认证头仍显示为 `***`。
- `config check`：对每个 `${exec:}` 输出 `site S auth A <字段>: runs <命令名>`，与 exec 认证的提示一致。
- 实现：
  - `config.Env` 增加可注入的执行函数字段，由 `wiring.Load` 填入 auth 包导出的执行器。config 不新增依赖。
  - 字段为空时，引用报 `${exec:} is not available`，用于测试和非调用路径。
  - 执行器从 `runExec` 中抽出，exec 认证改为调用它，行为不变。

## 架构

不新增包，依赖方向不变。

| 项 | 改动位置 |
|---|---|
| 1 | `model/pick.go`（解析与求值）；README；`skillgen` 示例一条 |
| 2 | `model.Site.Paginate`（`model/config.go`）；`config/load.go` 校验；`request` 新增 `paginate.go`；`commands/globalopts.go`（两个选项、保留名）；`commands/siterun.go` 选择单页或翻页 |
| 3 | `commands/use.go` |
| 4 | `commands/prescan.go`（返回站点后的 `--env`）、`commands/site.go` `loadIndex`、`wiring.FetchOptions`、`commands/run.go` 调用点 |
| 5 | `config/expand.go`（引用识别、字段白名单、dry-run 保留）；`config/dotenv.go` `Env` 加执行函数；`config/check.go`；`auth/exec.go` 抽出执行器；`wiring.Load` 注入 |

## 关键流程

### 分页

```text
siterun: --all-pages? ──否──> Execute 一次（不变）
              │是
前提检查（paginate、GET）──失败──> 退出 2
              │
page = 用户值 或 start；size = 用户值 或 size_value
loop:
  写入 page → Execute ──错误──> 返回
       │非 2xx ──> 原样输出该页 + note，结束
       │2xx
  取 items ──非数组──> 退出 7
  追加；0 条 / 不足 size / 达 total ──> 结束
  达 max ──> 警告，结束
  page++
合并数组 → output.Render（--pick、-o）
```

### `${exec:}` 解析

```text
ResolveAuth(所选认证)
  凭证字段：${env:} 查变量；${exec:} → 运行命令（同文本只运行一次）→ 登记密钥
  其他字段：遇到 ${exec:} → 退出 1
dry-run：${exec:} 原样保留，不运行
```

## 安全、数据、运维与回滚

- **执行命令的信任边界**：`${exec:}` 与 exec 认证一样，执行的是用户配置目录里写明的命令，不扩大信任面。
  - 不经过 shell，参数不做展开。
  - 只解析所选认证的凭证字段，补全、`ops`、`desc`、`site show` 都不执行。
  - dry-run 不执行。
  - `config check` 列出会执行的命令。
- **密钥**：
  - 命令输出登记为密钥。
  - 失败消息只给命令名和 stderr 最后一行，并经脱敏。stderr 最后一行仍可能含敏感信息，与 exec 认证的现状相同。
  - 结果不落盘。
- **分页**：
  - 只允许 GET。
  - 默认上限 100 页，防止无限翻页；每页都过写保护（GET 不受影响）和认证。
  - 全部页留在内存中，与现有单个响应的处理方式相同；大列表请用 `--max-pages` 控制。
- **行为变化**（写进 release notes）：
  - `--pick` 以 `{` 开头的值改按路径对象解析。以前它是一个以 `{` 开头的键名，几乎不可能被这样用。
  - 投影中以 `$.` 开头的项改为根引用。以前 `$` 后面的点会切出新段，几乎必然 not found（退出 7）。
  - `all-pages`、`max-pages` 成为保留选项名，同名的操作参数改为只能用 `-p` 传入。
  - 调用时给 `--env`，拉取定义改用该环境的 TLS 设置；`--env` 写错时拉取前就报错（以前先用默认环境拉取，再在 `Resolve` 报同样的错）。
  - 凭证值中的 `${exec:...}` 文本以前被当作字面值发出，现在会执行命令。
- **回滚到 v0.4.x**：
  - 旧版本忽略 `paginate`。
  - 旧版本把 `${exec:...}` 当作字面值发出，认证会失败（401）。
  - 回退前改回 `${env:}`。
  - state、缓存格式不变。

## 决策

| ID | 决策 | 理由 |
|---|---|---|
| D1 | 跨层取值用路径对象 `{...}`，同时支持投影中的 `$.` | 用户确认两种都要；花括号消除与投影的歧义 |
| D2 | 路径对象缺路径为 `null`，语法错误退出 2 | 与投影缺键一致；语法问题在请求前发现 |
| D3 | 分页参数名、列表路径由站点配置定义，只支持页码 + 条数 | 用户确认；覆盖 GitLab 与 DeployHub；响应头和游标留到以后 |
| D4 | 翻页显式开启（`--all-pages`），默认上限 100 页 | 不改变现有调用；防止无限翻页 |
| D5 | 某页失败即整体失败，输出该页，不输出部分结果 | 脚本拿到的要么是完整列表，要么是明确的错误 |
| D6 | `use --clear` 同时清环境与认证 | 最小命令面；只清一项的需求可以先清再设 |
| D7 | 拉取定义跟随本次 `--env`，`source.auth` 选择不变 | 修正客户端设置；不扩大第三批已确定的范围 |
| D8 | `${exec:}` 只在凭证字段、只解析所选认证、不跨进程缓存 | 避免意外执行；需要缓存的用 exec 认证 |
| D9 | `${exec:}` 复用 exec 认证的执行器，通过注入函数接入 config | 不重复实现进程管理；不改分层 |
| D10 | 选项名用 `--all-pages`/`--max-pages`，不用 `--all` | `all` 更可能与操作参数撞名 |

## 待确认事项

- Q1（非阻塞）：分页的默认上限 100 页是否合适；改成可在 `paginate.max_pages` 配置，按需再加。

## 结论与人工计划 Gate

本设计确定五项的行为与落点。下一步做第 1 轮两轴评审（正确性与可实施性；安全与对外契约），处理后请用户批准设计。批准只授权编写实施计划，不授权实施。
