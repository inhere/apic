<!-- template_id: design; template_version: 1.2.0 -->
# apic 二期第二批：定义覆盖（overlay）与 multipart 上传 设计

> 状态：Draft 0.2 / 已批准（用户 2026-11-11：“批准 apic 二期第二批设计 0.2（接受未复核修订）”；0.2 为第 1 轮评审后的未复核修订；批准只授权编写实施计划，不授权实施）

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-11 | Claude（inhere：swagger 无需登录，主要用 CLI；第二批按推荐做 overlay 与 multipart） | 初稿：overlay 改名/隐藏/修参数；multipart 文件上传，修正 multipart 被当成 urlencoded 的问题 |
| 0.2 | 2026-11-11 | Claude | 处理第 1 轮两轴评审（阻塞 C1/S4、S1、S2、S3；纠正 C2–C10、S5–S11；建议若干）：索引格式版本与本地重建；`-f` 永远是文本；打开后检查普通文件并限读；overlay init 用编码器生成、名字写成注释、O_EXCL 写入；部件头校验；curl 用 `--form-string`；文件读取移到写保护检查之后；隐藏参数拒绝传入；header 参数黑名单；补全静默；保留名 overlay；应用入口与改名冲突规则 |

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
- 评审状态：第 1 轮已完成，两个方向共 4 个阻塞项，0.2 已逐条修订。预算只有一轮，所以 0.2 属于未复核修订，见“评审处理”。

## 背景与目标

一期和二期第一批已经能从 swagger 自动生成命令。实际接入内部服务时，还有两类问题：

1. **生成的命令不好用，也改不了**：
   - swaggo 生成的 spec 没有 operationId，命令名只能由路径生成（如 `back-cfg-get`），常常不直观；
   - 有些参数在 spec 里漏写了必填，或类型写错，枚举值也缺失；
   - 有些接口不希望出现在 `ops`、补全和 `apic skill` 里；
   - 改上游 spec 要找对应服务的维护者，周期长；在本地改 spec 缓存，下次 `site refresh` 就被覆盖。
2. **文件上传接口调不了**：
   - 导入 Excel、上传附件、Jenkins 的文件参数等接口要求 `multipart/form-data`；
   - 现在 apic 把 spec 里的 `multipart/form-data` 当成普通表单，按 `application/x-www-form-urlencoded` 编码发送（`internal/spec/openapi.go` 的 `bodyType`），服务端收到的格式不对；
   - 也没有传文件的写法。

用户可见的结果：

- 在 `overlays/<site>.yaml` 里给操作改名、隐藏操作、修正参数和请求体类型。`ops`、调用、补全、`desc`、`apic skill` 都用覆盖后的结果，`site refresh` 后覆盖仍然有效。
- `apic <site> import-excel -F file=@./a.xlsx -F type=user` 能发送 multipart 请求；`--dry-run` 打印能原样复现的 `curl` 命令。
- 升级后，已缓存的 multipart 接口无需手动 refresh 即被正确识别。

## 名词

| 名词 | 含义 |
|---|---|
| overlay | 用户为某个站点写的覆盖文件，加载操作索引后、使用前应用，不改动缓存的 spec |
| 操作键 | overlay 里定位操作的写法：`METHOD /path`（推荐），或生成的操作名 |
| 原始索引 | spec 转换得到、未应用 overlay 的操作索引（即缓存内容） |
| 索引格式版本 | 写在缓存 meta 中的整数，表示 spec→索引转换规则的版本；规则变化时递增 |
| multipart 部件 | multipart 请求体中的一项：文本字段或文件 |

其余名词沿用一期设计（[apic-design.md](apic-design.md)）和二期第一批设计（[2026-11-11-apic-phase2-design.md](2026-11-11-apic-phase2-design.md)）。

## 范围与非目标

**范围**（仅 `inhere-tools/apic`）：

1. overlay：
   - 改名、隐藏、修改摘要；
   - 修正参数：必填、类型、枚举、说明、隐藏，并可补充 spec 漏写的 query/header 参数；
   - 修正请求体类型、输出格式；
   - `apic overlay init`；`config check` 的检查。
2. multipart：
   - 请求体类型 `multipart`、`-F` 选项；
   - spec 与简单定义的识别；dry-run 与 `-v` 的显示；
   - 修正现有的错误映射。
3. 索引格式版本与本地重建（为了让第 2 项对已有缓存生效）。

**非目标**：

- overlay 不新增操作（用简单定义另建站点），不修改请求体字段结构（schema），不新增 path 参数，不改变已有参数的位置。
- hide 不是访问控制：隐藏的操作仍可用原始路径调用。危险接口靠 `protect` 与 `allow_methods`。
- multipart 不做流式上传：请求体在内存中组装，总大小上限 64 MiB，更大的上传请用 curl。
- 不支持从 stdin 读文件（`-F k=@-`），也不支持 curl 的 `-F k=<file` 写法。
- swagger 2.0 的 formData 请求体，kin-openapi 转换后不保留请求体的必填标记（评审探针实测），所以这类接口不做“请求体必填”检查。

## 已确认事实与规范

1. **索引加载的调用方**（`internal/commands`）：
   - 用索引的地方：普通调用和站点命令注册（`run.go`）、`ops`、`desc`、`skill`、补全（`run.go` 的补全分支经 `loadIndexCached`）。
   - 要原始索引的地方：`site add`（`site.go:155`）和 `site refresh`（`site.go:516`，输出操作数），也经 `loadIndex`。
2. **索引缓存**：`spec.Index` 把转换后的索引缓存在 `cache/specs/<site>.index.json`，同目录还有原文 `.raw` 和 `.meta.json`。以下三种情况直接返回旧索引，不重新转换（`cache.go:43,57,69-81`）：
   - CacheOnly；
   - TTL 内命中缓存；
   - 收到 304。

   因此只改 `bodyType`，对已有缓存不生效。
3. **请求体类型**：`openapi.go` 的 `bodyType` 把 urlencoded 和 multipart 都映射成 `form`；`request/body.go` 的 `buildBody` 对 `form` 一律按 urlencoded 编码，`-f` 的值都按字面量处理，`k:=json` 的 JSON 字符串会去掉引号。
4. **请求的构造与发送**：
   - `request.Build` 在 `wiring.NewCall` 中执行，早于 `request.Execute` 中的写保护检查（`execute.go:66`）；
   - dry-run 和 401 重发都直接复用 `Prepared.Body` 的字节；`Curl` 会输出 Prepared 里的全部请求头；
   - `-v` 只打印请求行和请求头。
5. **`-p` 寻址**：`collectValues` 遇到操作里没有的参数名时，只要写成 `query.x` 等带位置的形式就照常接受（`build.go:200-215`）。
6. **路径与 basePath**：`Operation.Path` 是 spec 中的原始路径，不含 basePath，与 `ops` 的 PATH 列一致；`desc` 显示的是 basePath 加路径。简单定义允许不同操作名使用同一个 `METHOD /path`。
7. **mime/multipart**：Go 的实现只转义部件头中的 `\` 和 `"`，不过滤 CR/LF；`CreateFormFile` 把文件的 Content-Type 固定为 `application/octet-stream`。
8. **参数选项**：单字母参数不生成选项（`dynamic.go` 的 `optNamePattern`）；gcli 能同时注册 `-f` 和 `-F`，区分大小写（评审探针实测）。
9. **保留名**：管理命令名在 `config.reservedNames` 和 `managementCommands` 中；新增保留名时，与已有站点冲突的处理方式是“跳过该站点并警告”（二期第一批 D15）。
10. **适用规范**：GR104、GR107–GR110、SR2002–SR2004；分层规则沿用（领域包只依赖 `model`）。

## 总体方案

| 项 | 落点 | 对外新增 |
|---|---|---|
| overlay | `model`：类型；`config`：加载与校验；`spec`：`ApplyOverlay`；`commands`：应用入口、`overlay init`、`config check` | `overlays/<site>.yaml`；`apic overlay init <site>`；保留名 `overlay` |
| multipart | `spec`：识别；`request`：部件解析、组装、curl 与 `-v` 显示；`commands`：`-F` | `-F`；请求体类型 `multipart` |
| 索引格式版本 | `spec/cache.go` | 无 |

### 1. overlay

**文件位置**：`<配置目录>/overlays/<site>.yaml`，按站点名自动查找；文件不存在等于没有 overlay。overlay 完全由用户维护，apic 不会改写它（`overlay init` 只在文件不存在时创建）。

**格式**：

```yaml
operations:
  "POST /back/cfg/get":          # 操作键：METHOD /path（与 ops 的 PATH 列一致，不含 basePath）
    name: cfg-get                # 改名
    summary: 查询打印配置
  "POST /back/cfg/delete":
    hide: true                   # 隐藏：不出现在 ops、补全、skill 中，也不能按操作名调用
  users-list:                    # 也可以用生成的操作名定位
    output: raw
    body: multipart              # json | form | multipart | xml | raw
    params:
      status: {required: true, enum: [active, banned], desc: 用户状态}
      page.size: {type: integer} # 参数名本身带点号时，先按完整名字精确匹配
      debug:  {hide: true}       # 隐藏参数：不生成选项，-p 传入也会被拒绝
      tenant: {in: header, required: true}   # spec 里没有的参数：新增，必须写 in（query 或 header）
```

**应用入口**（D10）：

- 原始索引仍由 `loadIndex`/`loadIndexCached` 返回。
- 新增包装函数 `loadSiteIndex`，在原始索引上应用 overlay。
- 只有使用索引的地方调用这个包装函数：普通调用、站点命令注册、`ops`、`desc`、`skill`、补全。
- `site add`、`site refresh`、`overlay init` 使用原始索引。即使 overlay 写错，`site refresh` 也能照常运行，输出的也是原始操作数。

**定位**：

- **操作键**：含空格时按 `METHOD /path` 匹配，方法不区分大小写，路径与原始索引中的 `Operation.Path` 完全一致；否则按原始索引中的操作名匹配。
- **匹配结果**：
  - 一个键命中多个操作（简单定义里可能出现），报错；
  - 两个键命中同一个操作，报错；
  - 找不到的键只警告，不报错。
- **参数定位**：先按完整参数名精确匹配，再按 `in.name` 匹配，与 `collectValues` 的顺序一致。
  - `in` 只用于新增参数，或在同名参数出现在多个位置时消除歧义；
  - 写了 `in` 但与已有参数的位置不一致时报错，不能借此改变参数位置。

**修改规则**：

- **改名**：
  - 先在原始索引上解析所有键，再统一计算最终名字的集合，所以两个操作互换名字是允许的；
  - 新名字必须符合操作名规则，不能是 `help`；
  - overlay 显式指定的名字优先。生成名与之冲突的其他操作，按一期消歧规则依次追加 `-2`、`-3`，并给警告；
  - 两个 overlay 名字互相冲突时报错。
- **隐藏操作**：从索引中移除。原始路径调用不受影响，写保护照常生效。
- **参数**：
  - 可修改 `required`、`type`、`enum`、`desc`、`hide`；
  - `type` 只能是 string、integer、number、boolean、array；
  - 隐藏参数在 `Param` 上加 `Hidden` 标记：不生成选项，`-p` 传入时退出 2；
  - path 参数不能隐藏，否则这个操作就调不通了，加载时报错；
  - 新增参数的 `in` 只能是 `query` 或 `header`；
  - 新增 header 参数的名字必须是合法的 header token，且不能是 `Host`、`Content-Type`、`Content-Length`、`Transfer-Encoding`、`Authorization`、`Cookie`、`Proxy-Authorization`（凭证走 auth）。
- **请求体与输出**：
  - `body` 替换请求体类型；spec 原本没有请求体时，新建一个非必填的请求体；
  - `output` 的取值与 `-o` 相同。

**错误与提示**：

- **结构错误**：以下情况属于结构错误，命令退出 1，消息里带文件名和操作键：
  - YAML 语法错误、未知键、非法取值；
  - 改名冲突、隐藏 path 参数、新增参数非法、键命中多个操作等。
- **结构错误的影响范围**：该站点所有使用索引的命令都退出 1，包括原始路径调用；`site refresh`、`site show`、`site rm`、`overlay init` 照常可用。
- **找不到的键**：普通命令在 stderr 给警告。
- **补全模式**：overlay 的任何错误和警告都静默，此时该站点不提供操作候选，即按失败处理，不退回原始索引，以免把隐藏的操作补全出来。
- **`config check`**：在 `commands/config.go` 中组合实现：对每个有 overlay 的站点，读原始缓存索引（只读缓存，不联网）并应用 overlay，列出结构错误和找不到的键；没有缓存的站点报告“无缓存索引，先 site refresh”。

**`apic overlay init <site>`**：

- `<site>` 必须是已配置的站点；读取原始索引（按一期规则，可能刷新）。
- 用 YAML 编码器生成文件内容，每个操作一条，以 `METHOD /path` 为键。
  - 生成名不写成 `name:` 字段，而是写成注释 `# name: back-cfg-get`。这样不会把所有生成名固定下来，以后上游新增操作导致消歧变化时，也不会冲突。
  - summary 也写成注释，经单行净化处理：去掉控制字符和换行，截断到 120 字符。
- **写入方式**：
  - 写文件用 `O_CREATE|O_EXCL`，文件权限 0600，目录权限 0700；
  - 文件已存在时退出 1；
  - 不跟随符号链接。
- `--stdout`：输出到 stdout，内容同样经过净化。

**修改标注**：`ApplyOverlay` 另外返回一份改动记录，内容是操作和字段的改动列表；`model.Operation` 不加字段，所以缓存和 `desc -o json` 的结构都不变。`desc` 表格中被修改的字段后面标 `(overlay)`；`desc -o json` 在顶层增加 `overlay` 数组，列出改动的字段。

**保留名**：`overlay` 加入两处保留名列表。已有名为 `overlay` 的站点，按 D15 跳过并警告。

### 2. multipart 上传

**识别**：

- spec 中请求体为 `multipart/form-data` 时，类型为 `multipart`；`sortedMedia` 给 multipart 一个确定的排序名次。
- 简单定义的 `body: {type: multipart}`；简单定义的 `body.type` 改为枚举校验：json、form、multipart、xml、raw。
- overlay 的 `body: multipart`。
- 原始路径调用时，出现 `-F` 即为 multipart；`rawCall` 判断“有没有请求体”时，也要把 `-F` 算进去。

**`-F` 的写法**（站点选项，可重复）：

| 写法 | 含义 |
|---|---|
| `-F name=value` | 文本字段。值不以 `@` 开头时，等号后的整段都是字面值（包括其中的 `;`） |
| `-F file=@./a.xlsx` | 文件 |
| `-F 'file=@./a.bin;type=application/zip;filename=report.bin'` | 文件，附带参数 |
| `-F 'file=@"./a;b.bin";type=text/plain'` | 路径含 `;` 或 `"` 时用双引号括起，`\"` 表示引号 |

- **文件参数**：只有 `@` 后面的部分才解析参数，参数以 `;` 分隔，只支持 `type` 和 `filename`，其他参数退出 2。
- **字段名**：不能为空，不能含控制字符；`-F x`（没有 `=`）退出 2。
- **filename 与 type**：都不能含控制字符，否则退出 2，防止注入部件头。`type` 必须能用 `mime.ParseMediaType` 解析。
  - filename 默认取路径的最后一段，同样要过控制字符检查，Linux 文件名可以含换行。
  - type 默认用内置的扩展名表推断（xlsx、xls、csv、json、txt、zip、pdf、png、jpg、gif 等），推断不出时用 `application/octet-stream`。不用 `mime.TypeByExtension`：它会读系统配置，结果因机器而异。

**与 `-d`、`-f`、`-H` 的关系**：

- **`-f` 的值永远是文本**：multipart 下 `-f k=v` 发送一个文本部件，值里的 `@`、`;type=` 等都不解析，与 v0.2.0 中 form 的行为一致。只有 `-F k=@path` 会读文件。
- **`-f k:=json`**：与 form 分支的行为一致：JSON 字符串去掉引号，其他 JSON 值以紧凑的 JSON 文本发送。
- **组合限制**：
  - `-F` 与 `-d`、`-f` 不能同时使用，退出 2；
  - `-F` 与 `-H 'Content-Type: …'` 同时出现时退出 2，否则会破坏 boundary。
- **multipart 操作上用 `-d`**：要求同时用 `-H` 给出 Content-Type，按原样发送；不给时退出 2，并提示改用 `-F`。
- **类型不匹配**：操作的请求体类型不是 multipart 而用了 `-F` 时退出 2，并提示用 overlay 修正请求体类型。

**组装与发送**：

- **Build 阶段**：`request.Build` 只解析部件，把列表存进 `Prepared.Parts`，每项含名字、文本值或文件路径、filename、type。这时不读文件。
- **Execute 阶段**：写保护检查通过后，再组装请求体：
  - 每个文件用 `os.Open` 打开，再对打开后的文件 `Stat`，要求是普通文件（`Mode().IsRegular()`），否则退出 1。这条同时排除了 FIFO、设备文件、`/proc` 文件，以及 Windows 上的 `NUL`、`CON`。
  - 用 `io.LimitReader` 按剩余额度读取。所有部件头和内容的总量超过 64 MiB 就退出 1，并提示改用 curl。
  - 部件用 `CreatePart` 写出，自行设置头。各字段在解析阶段已经校验过，不含控制字符。
  - 组装结果存进 `Prepared.Body`，Content-Type 带 boundary；401 重发复用这段字节。
- **必填**：spec 标记请求体必填时，至少要有一个部件；swagger 2 的情况见非目标。

**显示**：

- **dry-run**：不组装请求体，但对每个文件做同样的普通文件检查，保证 dry-run 与实际发送的结果一致。
  - 渲染方式：文本部件用 `--form-string 'name=value'`，避免 curl 把 `@`、`;type` 当成文件或参数；文件部件用 `-F 'name=@"<路径>";type=<推断或指定的类型>;filename=<名字>'`，type 总是写出来，保证回放结果相同。
  - 不输出 Content-Type 请求头，由 curl 自己生成 boundary。
  - 字段值和路径都经过打码。
- **`-v`**：在请求头之后，每个部件打印一行：名字、文件名、type、大小；不打印文件内容。文本部件的值经过打码。
- **错误消息**：只包含路径和文件名，不包含文件内容。

### 3. 索引格式版本与本地重建

- **版本号**：`cache/specs/<site>.meta.json` 增加 `index_version`。当前转换规则（multipart 单独识别）定为版本 2；没有这个字段的旧缓存视为版本 1。
- **触发**：`spec.Index` 读到旧缓存时，若版本号不是当前值，不论 TTL、CacheOnly 或 304，都从同目录的 `.raw` 本地重新转换，写回 index 和 meta。不联网。
  - `.raw` 缺失或解析失败时，按“没有缓存”处理：CacheOnly 返回错误，其他情况正常拉取。
- **简单定义**：每次加载都重新解析，不受影响。
- 以后转换规则再有变化时，递增这个版本号即可。

## 架构

### 代码组织

沿用现有结构（SR2004），不新增包：

```
internal/model/overlay.go        # Overlay、OpOverlay、ParamOverlay、OverlayChange 类型；Param.Hidden
internal/config/overlay.go       # LoadOverlay(dir, site) (*model.Overlay, error)：读取、未知键与取值校验
internal/config/load.go          # 保留名 overlay
internal/spec/overlay.go         # ApplyOverlay(idx *model.Index, ov *model.Overlay) (*model.Index, []model.OverlayChange, []string, error)
internal/spec/openapi.go         # bodyType 与 sortedMedia：multipart
internal/spec/simple.go          # body.type 枚举校验
internal/spec/cache.go           # index_version；旧版本时从 .raw 本地重建
internal/request/multipart.go    # ParsePart（-F 语法）、Assemble（打开、检查、限读、组装）、内置类型表
internal/request/build.go        # Prepared.Parts；Hidden 参数在 -p 中拒绝；-F 与 -d/-f/-H 的组合校验
internal/request/body.go         # multipart 分支：-f 文本部件、k:=json
internal/request/execute.go      # 写保护检查之后组装；-v 打印部件；dry-run 检查文件
internal/request/curl.go         # multipart：--form-string / -F 渲染，不输出 Content-Type
internal/commands/site.go        # loadSiteIndex；overlay init 命令
internal/commands/run.go         # managementCommands 加 overlay；调用 loadSiteIndex；补全时静默
internal/commands/globalopts.go  # siteOpts 加 -F；reservedOptNames 加 F
internal/commands/dynamic.go     # rawCall 的请求体判断算上 -F；Hidden 参数不生成选项
internal/commands/ops.go         # desc 的 (overlay) 标注
internal/commands/config.go      # config check 组合 overlay 检查
internal/wiring/wiring.go        # NewCall 传入 Parts（如有需要）
internal/skillgen/               # multipart 操作提示用 -F；说明 -F/-d @ 会外发本地文件
```

依赖方向不变：`config`、`spec`、`request` 只依赖 `model`；由 `commands` 把它们串起来。

## 关键流程

### 加载索引（含 overlay）

```
loadSiteIndex(site)
  ─▶ loadIndex / loadIndexCached ─▶ spec.Index
        └─ meta.index_version 过旧？─是─▶ 从 .raw 本地重建
  ─▶ config.LoadOverlay(dir, site)    # 文件不存在 → nil
  ─▶ spec.ApplyOverlay(原始索引, ov)   # 新索引、改动记录、警告；结构错误 → 退出 1（补全时静默且无候选）
  ─▶ 调用 / ops / desc / skill / 补全
site add / site refresh / overlay init ─▶ 只用原始索引
```

### multipart 请求

```
Build：解析 -F/-f 为 Parts（校验名字、filename、type、组合）─▶ Prepared.Parts
Execute：
  写保护检查 ─▶ dry-run？─是─▶ 检查每个文件是普通文件 ─▶ 渲染 curl（--form-string / -F）
                 │否
                 ▼
              逐个 Open + Stat(普通文件) + LimitReader（总量 ≤ 64 MiB）─▶ CreatePart 组装
              ─▶ Prepared.Body ─▶ 发送（401 重发复用）
```

## 安全、数据、运维与回滚

- **文件外发**：
  - 只有 `-F k=@path` 会读文件，`-f` 的值永远是文本；agent 透传不可信文本时，也不会意外读出本地文件。
  - 文件在写保护检查通过之后才读取。
  - 只读普通文件，限读 64 MiB。
  - dry-run、`-v` 和错误消息都不包含文件内容。
  - `apic skill` 生成的说明里写明：`-F k=@path` 和 `-d @file` 会读取并外发本地文件，只能用于用户明确给出的路径。
- **部件头注入**：字段名、filename、type 不能含控制字符，type 还要能被解析，从源头上不会产生 CRLF 注入。
- **overlay 的信任边界**：
  - overlay 只来自用户自己的配置目录。
  - `overlay init` 写入的来自 spec 的文本只出现在注释里，且经过单行净化，spec 无法借此向 overlay 注入条目。
  - overlay 写的 summary、desc 进入 SKILL.md 时，照常按不可信文本净化。
- **overlay 能改什么**：
  - 不能改主机、base path、认证；
  - 不能新增 path 参数或改变参数位置；
  - 新增 header 参数时，不能使用会影响请求编码的头和凭证类头。
  - 请求路径中 path 参数的值经 `url.PathEscape` 转义，不受 overlay 影响（`..` 不转义，这是一期就有的行为，主机不会变）。
- **hide**：只控制可见性，不是访问控制，文档中写明。
- **补全**：overlay 的错误和警告都静默；按失败处理，不会补全出隐藏的操作。
- **兼容性**：
  - 没有 overlay 文件时，行为与 v0.2.0 一致；`overlay` 成为保留名；
  - 已有缓存会在本地自动重建一次，multipart 接口改为 multipart 编码。原来的 urlencoded 发法本来就是错的，这条写进发布说明；
  - 原来在这类接口上用 `-f`（含 `k:=json`）的命令继续可用，值仍按文本发送；
  - multipart 操作上单独用 `-d` 时，现在必须带 `-H Content-Type`。原来会被当成 urlencoded 发送，本来就是错的，同样写进发布说明。
- **回滚**：回退到 v0.2.0 二进制即可。旧版会忽略 overlay 文件；本地重建过的缓存旧版仍能读取（多出的 meta 字段会被忽略）；用了 `-F` 的命令在旧版上报未知选项。
- **发布**：做完后发 v0.3.0，需要用户另行授权。

## 决策

| ID | 决策 | 理由 |
|---|---|---|
| D1 | overlay 放在单独的 `overlays/<site>.yaml`，按站点名自动查找 | `sites/*.yaml` 由 `site add` 写入，分开存放不会被覆盖 |
| D2 | 推荐 `METHOD /path` 作键，路径与 `ops` 的 PATH 列一致；也接受生成的操作名 | 生成名会随消歧变化，`METHOD /path` 更稳定 |
| D3 | overlay 在读出原始索引之后应用，不写回缓存 | 改了 overlay 立即生效，缓存保持 spec 原样 |
| D4 | 找不到的键只警告；结构错误让该站点使用索引的命令都退出 1 | 上游改动不应让站点不可用；结构错误必须由用户改正 |
| D5 | overlay 不新增操作、不改请求体 schema、不新增 path 参数、不改参数位置 | 控制复杂度；也保证 overlay 改不了请求地址 |
| D6 | multipart 单独成为一种请求体类型，并用索引格式版本触发本地重建 | 修正现有错误映射；不重建的话对已有缓存无效 |
| D7 | `-F` 写法参照 curl；请求体在内存中组装，上限 64 MiB | 用户熟悉；复用现有 `[]byte` 链路；内部上传通常不大 |
| D8 | 文件在写保护检查之后才读取；只读普通文件；按剩余额度限读 | 被保护或拒绝的请求不读文件；防止读取无尽的设备或 FIFO |
| D9 | `-f` 的值永远是文本；只有 `-F k=@path` 读文件 | 不悄悄引入读文件外发；保持 v0.2.0 中 `-f` 的语义 |
| D10 | overlay 只在使用索引的入口应用，add/refresh/init 用原始索引 | overlay 写错时仍能 refresh；refresh 输出原始操作数 |
| D11 | `overlay init` 把生成名写成注释，overlay 显式名字优先，冲突的生成名按消歧规则加后缀 | 不把生成名固定下来，避免上游新增操作后冲突 |
| D12 | dry-run 用 `--form-string` 渲染文本部件、显式写出 type、不输出 Content-Type 头 | 渲染出的 curl 命令能原样复现请求 |
| D13 | 隐藏参数在 `-p` 中拒绝；path 参数不可隐藏；hide 不是访问控制 | 与文档承诺一致；不会产生调不通的操作；不给人错误的安全感 |

## 评审处理（第 1 轮）

| 评审项 | 处理 |
|---|---|
| C1 / S4 已有缓存不生效 | §3 索引格式版本与本地重建（D6） |
| S1 / C9 `-f k=@path` 读文件；`-F` 语法未定义 | D9；`-F` 语法表；`--form-string`（D12） |
| S2 用 stat 判断大小拦不住设备与 FIFO | 打开后检查普通文件、LimitReader、dry-run 同样检查（D8） |
| S3 overlay init 写入 spec 文本 | 用编码器生成、注释净化、O_EXCL、0600/0700 |
| C2 应用入口 | `loadSiteIndex`（D10）；明确结构错误的影响范围 |
| C3 config check 违反分层 | 改在 `commands/config.go` 组合实现 |
| C4 保留名 overlay | 加入两处保留名列表，沿用 D15 |
| C5 / S7 隐藏参数仍可传、隐藏 path 参数 | `Param.Hidden` 在 `-p` 中拒绝；path 参数不可隐藏（D13） |
| C6 参数定位歧义 | 先按完整名字匹配，再按 `in.name`；`in` 不能改位置 |
| C7 键的路径语义、一键多操作 | 与 `ops` 的 PATH 列一致；一键命中多个操作时报错 |
| C8 dry-run 与 `-v` 缺数据 | `Prepared.Parts`；Execute 阶段组装；不输出 Content-Type；代码组织补全 |
| C10 改名冲突 | init 把名字写成注释；最终名字集合检查；冲突的生成名加后缀（D11） |
| S5 部件头注入 | 控制字符检查与 `ParseMediaType` |
| S6 curl 渲染 | 引号规则、显式 type、不输出 Content-Type |
| S8 新增 header 参数 | header token 规则加黑名单；`-F` 与 `-H Content-Type` 互斥 |
| S9 补全静默 | 补全时 overlay 错误静默，且不提供候选 |
| S10 兼容性矛盾 | `k:=json` 与 form 一致；multipart 上用 `-d` 的规则 |
| S11 改名为 help | 拒绝 |
| 安全建议 S12–S16、正确性建议 S1–S7 | 均已采纳：保护检查后才读文件、内置类型表、skill 说明、措辞修正、simple 枚举、swagger 2 必填限制写进非目标、sortedMedia、取值校验、改动记录、skill 提示 `-F` |

## 待确认事项

无阻塞项。以下在计划阶段核实：

- 内置扩展名类型表的具体条目。
- `desc` 表格中 `(overlay)` 标注的具体排版。

## 结论与人工计划 Gate

本设计确定二期第二批的范围、对外接口和兼容性影响：

- **范围**：overlay、multipart，以及索引格式版本。
- **对外接口**：`overlays/<site>.yaml`、`apic overlay init`、`-F`、请求体类型 `multipart`、保留名 `overlay`。

第 1 轮两轴评审的阻塞项都已在 0.2 修订，但修订本身未经复核。可以选择以下任一方式批准：

- 点名接受未复核修订并批准 0.2；
- 只批准最后评审过的 0.1；
- 先要求再评审一轮。

批准只授权编写实施计划，不授权实施。
