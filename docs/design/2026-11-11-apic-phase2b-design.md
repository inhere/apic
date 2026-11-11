<!-- template_id: design; template_version: 1.2.0 -->
# apic 二期第二批：定义覆盖（overlay）与 multipart 上传 设计

> 状态：Draft 0.1 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-11 | Claude（inhere：swagger 无需登录，主要用 CLI；第二批按推荐做 overlay 与 multipart） | 初稿：overlay 改名/隐藏/修参数；multipart 文件上传，修正 multipart 被当成 urlencoded 的问题 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 设计工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：两件事。
  - 内部 swagger 质量一般：没有 operationId、命令名长、参数写错、有不该暴露的接口。用户不改上游 spec，也能把命令和参数调整好用，对 agent 也更清楚。
  - 带文件上传的接口能直接用 apic 调用，不用再退回 curl。
- Scope freeze：只做 overlay 和 multipart 两项。
  - 不做：`--auth-for-spec`（用户确认 swagger 无需登录）、MCP 模式（用户主要用 CLI）。
  - 二期其余项（oauth2、`${keyring:}`/`${exec:}`、分页、重定向跟随）仍为 deferred。
- 暴露度：v0.2.0 已公开发布，用户可能存在；multipart 会读本地文件并发送出去，属于数据外发路径。因此按高暴露处理（SR1409），走两轴独立评审。
- Review budget：一轮评审，两个方向：正确性与可实施性；安全与对外契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。

## 背景与目标

一期和二期第一批已经能从 swagger 自动生成命令。实际接入内部服务时，还有两类问题：

1. **生成的命令不好用，也改不了**：
   - swaggo 生成的 spec 没有 operationId，命令名只能由路径生成（如 `back-cfg-get`），常常不直观；
   - 有些参数在 spec 里漏写了必填，或类型写错，枚举值也缺失；
   - 有些接口（内部管理、危险操作）不希望出现在 `ops`、补全和 `apic skill` 里；
   - 改上游 spec 要找对应服务的维护者，周期长；在本地改 spec 缓存，下次 `site refresh` 就被覆盖。
2. **文件上传接口调不了**：
   - 导入 Excel、上传附件、Jenkins 的文件参数等接口要求 `multipart/form-data`；
   - 现在 apic 把 spec 里的 `multipart/form-data` 当成普通表单，按 `application/x-www-form-urlencoded` 编码发送（`internal/spec/openapi.go` 的 `bodyType`），服务端收到的格式不对；
   - 也没有传文件的写法。

用户可见的结果：

- 在 `overlays/<site>.yaml` 里给操作改名、隐藏操作、修正参数和请求体类型。`ops`、调用、补全、`desc`、`apic skill` 都用覆盖后的结果，`site refresh` 后覆盖仍然有效。
- `apic <site> import-excel -F file=@./a.xlsx -F type=user` 能发送 multipart 请求；`--dry-run` 打印对应的 `curl -F`。

## 名词

| 名词 | 含义 |
|---|---|
| overlay | 用户为某个站点写的覆盖文件，加载操作索引后、使用前应用，不改动缓存的 spec |
| 操作键 | overlay 里定位操作的写法：`METHOD /path`（推荐，spec 变动时稳定），或生成的操作名 |
| multipart 部件 | multipart 请求体中的一项：普通字段（`-F k=v`）或文件（`-F k=@path`） |

其余名词沿用一期设计（[apic-design.md](apic-design.md)）和二期第一批设计（[2026-11-11-apic-phase2-design.md](2026-11-11-apic-phase2-design.md)）。

## 范围与非目标

**范围**（仅 `inhere-tools/apic`）：

1. overlay：
   - 改名、隐藏、修改摘要；
   - 修正参数：必填、类型、枚举、说明、隐藏，并可补充 spec 里漏写的参数；
   - 修正请求体类型；
   - 修改输出格式；
   - 配套的检查与提示。
2. multipart：
   - 新的请求体类型 `multipart`；
   - `-F` 选项；
   - spec 和简单定义的识别；
   - dry-run 与 `-v` 的显示；
   - 修正现有的错误映射（multipart 被当成 form）。

**非目标**：

- overlay 不支持新增操作。要新增操作，请用简单定义另建一个站点。
- overlay 不修改请求体的字段结构（schema），只修改请求体类型。字段说明仍以 spec 为准。
- multipart 不做流式上传：请求体在内存中组装，总大小上限为 64 MiB，更大的上传请用 curl。
- 不支持 `-F k=@-`（从 stdin 读文件）。
- 不支持 `-F k=<file`（curl 里把文件内容当作字段值的写法）。

## 已确认事实与规范

1. **加载入口**：操作索引的加载只有两处，`commands.loadIndex`（普通调用、`ops`、`desc`、`skill`）和 `commands.loadIndexCached`（补全），都在 `internal/commands/site.go` 中。overlay 在这两处应用，就能覆盖所有使用面。
2. **索引缓存**：`spec.Index` 把转换后的索引缓存在 `cache/specs/<site>.index.json`。overlay 在读出索引之后才应用，不写回缓存，所以改了 overlay 不需要 refresh。
3. **请求体类型**：
   - `internal/spec/openapi.go` 的 `bodyType` 把 `application/x-www-form-urlencoded` 和 `multipart/form-data` 都映射成 `form`；
   - `internal/request/body.go` 的 `buildBody` 对 `form` 一律按 urlencoded 编码。
4. **请求体的传递方式**：`request.Prepared.Body` 是 `[]byte`，curl 渲染（`request/curl.go`）和 401 重发都直接复用这段字节。
5. **简单定义的请求体**：简单定义已有 `body: {type: ...}` 字段（`internal/spec/simple.go` 的 `simpleBody`）。
6. **命名规则**：没有 operationId 时由路径生成名字，重名时依次消歧（一期 D16）。消歧结果会随 spec 变化而变化，所以 overlay 推荐用 `METHOD /path` 定位操作。
7. **参数选项冲突**：参数名与全局选项冲突时，只能用 `-p` 传（一期“参数映射”）。新增的 `-F` 是站点选项，会占用参数名 `F`；单字母参数本来就不生成选项（`dynamic.go` 要求选项名至少 2 个字符），所以不会冲突。
8. **适用规范**：GR104、GR107–GR110、SR2002–SR2004；分层规则沿用（领域包只依赖 `model`）。

## 总体方案

| 项 | 落点 | 对外新增 |
|---|---|---|
| overlay | `model`：`Overlay` 类型；`config`：加载与校验 overlay 文件；`spec`：`ApplyOverlay` 应用到索引；`commands`：在两个加载入口应用 | `overlays/<site>.yaml`；`apic overlay init <site>` |
| multipart | `spec`：识别 multipart；`request`：组装 multipart 请求体、curl 渲染；`commands`：`-F` 选项 | `-F`；请求体类型 `multipart` |

### 1. overlay

**文件位置**：`<配置目录>/overlays/<site>.yaml`，按站点名自动查找；文件不存在等于没有 overlay。overlay 和 `sites/<site>.yaml` 分开存放：后者由 `site add` 写入，overlay 完全由用户维护，apic 不会改写它。

**格式**：

```yaml
operations:
  "POST /back/cfg/get":          # 操作键：METHOD /path（推荐），或生成的操作名
    name: cfg-get                # 改名
    summary: 查询打印配置          # 改摘要
  "POST /back/cfg/delete":
    hide: true                   # 隐藏：不出现在 ops、补全、skill 中，也不能调用
  users-list:                    # 用生成的操作名定位也可以
    output: raw
    body: multipart              # 修正请求体类型：json | form | multipart | xml | raw
    params:
      status: {required: true, enum: [active, banned], desc: 用户状态}
      page:   {type: integer}
      debug:  {hide: true}       # 隐藏参数：不生成选项，也不能传
      tenant: {in: header, required: true}   # spec 里没有的参数：必须写 in
```

**应用规则**：

- **应用时机**：加载索引后、注册命令前，按操作键找到操作后修改，产生一份新索引；缓存不变。
- **定位**：操作键含空格时按 `METHOD /path` 精确匹配，方法不区分大小写；否则按生成的操作名匹配。同一个操作同时被两种写法命中时报错。
- **改名**：
  - 新名字必须符合操作名规则（`^[a-z0-9]+(-[a-z0-9]+)*$`）；
  - 改名后的名字不能与其他操作重名，否则报错；
  - 改名不影响其他操作的名字，不重新跑消歧。
- **隐藏**：
  - 隐藏的操作从索引中移除，等同于不存在；
  - 原始路径调用（`apic <site> -X POST /back/cfg/delete`）不受影响，因为原始路径本来就不经过操作索引；
  - 写保护照常生效。
- **参数**：
  - 按参数名定位；同名参数出现在不同位置时，用 `in.name`（如 `query.id`）定位；
  - 可修改的字段：`required`、`type`、`enum`、`desc`、`hide`；
  - 参数在 spec 里不存在时视为新增，必须写 `in`（`query`、`header` 或 `path`）。新增 `path` 参数时，路径里必须有对应的 `{name}`。
- **请求体**：`body` 替换请求体类型。spec 原本没有请求体时，`body` 会新建一个非必填的请求体。
- **输出**：`output` 与简单定义里的含义相同。

**错误与提示**：

- 以下情况在加载时报错，退出 1，消息里带文件名和操作键，应用到的命令都无法执行：
  - YAML 语法错误；
  - 未知键；
  - 新名字非法或重名；
  - 新增参数缺少 `in`；
  - 新增的 path 参数在路径里没有对应的占位符。
- 操作键在当前索引里找不到时（通常是上游 spec 改了路径），只在 stderr 给警告，不报错，避免上游一改 spec 整个站点就不能用。补全模式下不输出这类警告（二期第一批 D10）。
- `config check` 列出每个站点 overlay 中找不到的操作键。

**`apic overlay init <site>`**：生成 overlay 模板，方便用户起步。

- 每个操作写一条，用 `METHOD /path` 作键，`name` 预填当前的生成名，加上 summary 注释。
- 文件已存在时拒绝覆盖，退出 1。
- `--stdout` 改为输出到 stdout，不写文件。

**使用面**：`ops`、`desc`、调用、补全、`apic skill` 都用覆盖后的索引。`desc` 中被 overlay 修改过的字段标注 `(overlay)`，便于排查。

### 2. multipart 上传

**识别**：

- spec 中请求体为 `multipart/form-data` 时，类型为 `multipart`；`application/x-www-form-urlencoded` 仍为 `form`。这修正了现有的错误映射。
- 简单定义的 `body: {type: multipart}`，以及 overlay 的 `body: multipart`。
- 原始路径调用时，出现 `-F` 即为 multipart。

**`-F` 选项**（站点选项，可重复，写法与 curl 的 `-F` 一致）：

| 写法 | 含义 |
|---|---|
| `-F name=value` | 普通字段 |
| `-F file=@./a.xlsx` | 文件；文件名取路径的最后一段，Content-Type 按扩展名推断，推断不出时用 `application/octet-stream` |
| `-F 'file=@./a.bin;type=application/zip'` | 指定 Content-Type |
| `-F 'file=@./a.bin;filename=report.bin'` | 指定发给服务端的文件名 |

**规则**：

- **类型校验**：
  - `-F` 只能用于 multipart 请求体，与 `-d`、`-f` 同时出现时退出 2；
  - 操作的请求体类型是 multipart 时，用 `-f k=v` 也可以传普通字段（与 `-F k=v` 等价），但 `-f k:=json` 退出 2，因为 multipart 字段只能是文本；
  - 操作的请求体类型不是 multipart 而用了 `-F` 时退出 2，并提示用 overlay 修正请求体类型。
- **文件**：
  - 路径相对当前工作目录；
  - 文件不存在、是目录、或读不了时退出 1，消息里带路径；
  - 总大小（所有部件的头和内容）超过 64 MiB 时退出 1，提示改用 curl。
- **组装**：用 `mime/multipart` 在内存中组装请求体，Content-Type 带 boundary。组装结果存进 `Prepared.Body`，401 重发直接复用，与现有路径一致。
- **必填**：spec 标记请求体必填时，至少要有一个部件。
- **显示**：
  - **dry-run**：打印 `curl -F 'name=value' -F 'file=@<路径>'`，不读取文件内容（只检查文件是否存在）；字段值经过打码；
  - **`-v`**：只打印每个部件的名字、文件名和大小，不打印文件内容。
- **打码**：普通字段的值与 `-f` 一样走打码流程；文件内容不经过打码，也不出现在任何输出里。

**参数名冲突**：`-F` 是站点选项，`F` 加入保留选项名列表。按已确认事实 7，没有参数会因此被挤到 `-p`。

## 架构

### 代码组织

沿用现有结构（SR2004），不新增包：

```
internal/model/overlay.go        # Overlay、OpOverlay、ParamOverlay 类型
internal/config/overlay.go       # LoadOverlay(dir, site) (*model.Overlay, error)：读取、未知键与值校验
internal/spec/overlay.go         # ApplyOverlay(idx *model.Index, ov *model.Overlay) (*model.Index, []string, error)
internal/spec/openapi.go         # bodyType：multipart/form-data → multipart
internal/request/multipart.go    # 组装 multipart 请求体；Input 新增 Files（-F 原始值）
internal/request/body.go         # buildBody 接入 multipart 分支
internal/request/curl.go         # multipart 时渲染 -F
internal/commands/site.go        # loadIndex / loadIndexCached 应用 overlay；overlay init 命令
internal/commands/globalopts.go  # siteOpts 加 -F；reservedOptNames 加 F
internal/commands/ops.go         # desc 标注 (overlay)
internal/config/check.go         # 列出找不到的操作键
```

依赖方向不变：`config` 和 `spec` 只依赖 `model`；由 `commands` 把两者串起来（先 `config.LoadOverlay`，再 `spec.ApplyOverlay`）。

## 关键流程

### 加载索引（含 overlay）

```
loadIndex / loadIndexCached
  ─▶ spec.Index（缓存或拉取）
  ─▶ config.LoadOverlay(dir, site)    # 文件不存在 → nil
  ─▶ spec.ApplyOverlay(idx, ov)       # 返回新索引与警告；错误退出 1
  ─▶ 注册命令 / ops / desc / skill
```

### multipart 请求

```
-F 解析 ─▶ 类型检查（不是 multipart 退出 2）─▶ 检查文件存在
  ─▶ dry-run？─是─▶ 渲染 curl -F（不读文件内容）
       │否
       ▼
     计算总大小（超过 64 MiB 退出 1）─▶ mime/multipart 组装 ─▶ Prepared.Body ─▶ 发送（401 重发复用）
```

## 安全、数据、运维与回滚

- **文件外发**：multipart 会把本地文件发给目标服务，必须由用户显式写 `-F k=@path`，apic 不会自动选择或扫描文件。写保护（`protect`、`allow_methods`、`--yes`）照常在发送前检查。
- **输出不泄露文件内容**：dry-run、`-v` 和错误消息都不包含文件内容，只出现路径、文件名和大小。
- **overlay 来源**：overlay 只来自用户自己的配置目录，spec 和简单定义不能提供 overlay。overlay 只能修改操作的名字、可见性、参数和请求体类型，不能修改认证、base URL 或请求地址的主机部分。新增 path 参数必须对应路径里已有的占位符，所以 overlay 也无法改变请求路径。
- **兼容性**：
  - 没有 overlay 文件时，行为与 v0.2.0 完全一致；
  - multipart 识别修正后，原来被当成 form 发送的 multipart 接口改为 multipart 编码。原来的发法本来就是错的，这条写进发布说明；
  - 原来用 `-f` 调用这类接口的命令仍然可用：`-f k=v` 在 multipart 下等同于 `-F k=v`；
  - `-F` 是新增的站点选项。
- **回滚**：回退到 v0.2.0 二进制即可，overlay 文件会被忽略；用了 `-F` 的命令在旧版本上会报未知选项。
- **发布**：做完后发 v0.3.0，需要用户另行授权。

## 决策

| ID | 决策 | 理由 |
|---|---|---|
| D1 | overlay 放在单独的 `overlays/<site>.yaml`，按站点名自动查找 | `sites/*.yaml` 由 `site add` 写入；分开存放才不会被覆盖，也一眼能看出是用户自己的修改 |
| D2 | 推荐用 `METHOD /path` 作操作键，也接受生成的操作名 | 生成名会随 spec 消歧而变化，`METHOD /path` 更稳定；操作名写起来短，两种都支持 |
| D3 | overlay 在读出索引之后应用，不写回缓存 | 改了 overlay 立即生效，无需 refresh；缓存始终保持 spec 的原样 |
| D4 | 找不到的操作键只警告，结构错误才报错 | 上游 spec 改动不应让整个站点不可用；结构错误则必须让用户改正 |
| D5 | overlay 不支持新增操作，也不修改请求体字段结构 | 新增操作用简单定义；修改 schema 的复杂度高，收益低 |
| D6 | multipart 单独成为一种请求体类型，修正现有的错误映射 | urlencoded 与 multipart 编码不同，混用就是现有的 bug |
| D7 | `-F` 写法与 curl 一致，请求体在内存中组装，上限 64 MiB | 用户熟悉 curl 写法；内存组装能复用现有的 `[]byte` 链路（401 重发、curl 渲染），内部上传通常不大；流式上传等有真实需求再做 |
| D8 | dry-run 不读文件内容 | dry-run 不应产生读取大文件的开销，也不应让内容出现在输出里 |
| D9 | multipart 下 `-f k=v` 等同于 `-F k=v` | 让原来用 `-f` 调用这类接口的命令继续可用 |

## 待确认事项

无阻塞项。以下在计划阶段核实：

- 扩展名推断 Content-Type 用 `mime.TypeByExtension`，在 Windows 上它会读注册表，同一扩展名在不同机器上的结果可能不同。评估是否改用一张内置的小表。
- `desc` 标注 `(overlay)` 的具体输出格式（表格和 `-o json` 两种）。

## 结论与人工计划 Gate

本设计确定二期第二批的范围（overlay、multipart）、对外接口（`overlays/<site>.yaml`、`apic overlay init`、`-F`、请求体类型 `multipart`）和兼容性影响。两轴评审完成后等待人工批准；批准只授权编写实施计划，不授权实施。
