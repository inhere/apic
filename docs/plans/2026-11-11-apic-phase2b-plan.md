<!-- template_id: plan; template_version: 1.3.0 -->
# apic 二期第二批（overlay 与 multipart）实施计划

> 状态：Draft 0.1 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-11 | Claude（inhere：“批准 apic 二期第二批设计 0.2（接受未复核修订）”） | 初稿：3 个波次、7 个任务，覆盖设计 0.2 的 overlay、multipart 与索引格式版本 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 规划工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：交付设计 0.2 的三项并通过验收：overlay、multipart 上传、索引格式版本与本地重建。
- Scope freeze：只做设计 0.2 的范围。设计“非目标”和二期其余 deferred 项（oauth2、`${keyring:}`/`${exec:}`、分页、重定向跟随、`--auth-for-spec`、MCP）实施中即使发现需要，也只记为 deferred。
- 评审预算：与设计相同，按高暴露处理（SR1409），做一轮两轴独立评审：一是正确性与可执行性，二是安全与契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。
- 评审状态：未评审。

## 目标与完成定义

**目标**：v0.2.0 的用户升级后：

- 写一个 `overlays/<site>.yaml` 就能改名、隐藏、修参数、改请求体类型和输出格式，`ops`、调用、补全、`desc`、`skill` 都按覆盖后的结果工作；
- 用 `-F` 调用文件上传接口，`--dry-run` 给出能原样复现的 curl；
- 已缓存的 multipart 接口不用手动 refresh 就被正确识别。

行为符合设计 0.2 的各节和 D1–D13。

**完成定义（验收）**：

1. **B1 overlay**：经真实分发（`commands.Run`）覆盖：
   - 改名（含两个操作互换名字、生成名与 overlay 名冲突时加后缀并警告、两个 overlay 名冲突报错、改名为 `help` 报错）；
   - 隐藏操作：`ops`、补全、`skill` 中都没有，按名调用报未知操作，原始路径调用仍可发出；
   - 参数：改 required/type/enum/desc；新增 query 与 header 参数；隐藏参数不生成选项，`-p debug=1` 和 `-p query.debug=1` 都退出 2；
   - 隐藏 path 参数、隐藏必填参数、新增参数写成 path、新增 header 用黑名单名字、`in` 与已有位置不一致：都是结构错误；
   - 结构错误时，该站点的调用、原始路径调用、`ops`、`desc`、`skill` 都退出 1，`site refresh` 照常成功，补全输出为空且 stderr 为空；
   - 找不到的键只在 stderr 警告；
   - `desc` 表格的 `(overlay)` 标注，`desc -o json` 的 `overlay` 数组；没有 overlay 时 `desc -o json` 与 v0.2.0 逐字节一致；
   - `overlay init`：文件内容能被 `LoadOverlay` 读回且等于“无改动”；spec 的 summary 含换行和 `x: {hide: true}` 时不产生任何条目；文件已存在、`overlays` 是符号链接时退出 1；权限 0600/0700（Unix）；`--stdout`；
   - `config check` 报出结构错误和找不到的键；
   - 名为 `overlay` 的站点被跳过并警告。
2. **B2 multipart**：
   - httptest 服务端用 `mime/multipart` 解析，断言部件的名字、值、filename、Content-Type 和文件内容；
   - `-f` 在 multipart 操作上发文本部件，`-f k=@/etc/passwd` 发的是字面值；`-f k:=json` 与 form 分支一致；
   - 组合限制：`-F` 与 `-d`/`-f`/`-H Content-Type` 退出 2；非 multipart 操作用 `-F` 退出 2；multipart 操作单独 `-d` 不带 Content-Type 退出 2，带了则原样发送；原始路径调用用 `-F` 发 multipart；
   - 名字、filename、type 含控制字符或 type 无法解析时退出 2；
   - 目录、`/dev/zero`、FIFO（Unix）和 `NUL`（Windows）都退出 1，且不阻塞；总量超过 64 MiB 退出 1；
   - 写保护拒绝时（退出 6）不打开文件：用一个不存在的路径证明退出码是 6 而不是 1；
   - 401 重发的请求体与首次相同；
   - `-v` 每个部件一行，哨兵文件内容不出现在 stderr；
   - **real-env**：用容器里真实的 `curl` 执行 dry-run 输出的命令，发往同一个 httptest 服务，收到的部件与 apic 直接发送时一致（文本值含 `@`、`;type=`，路径含 `;` 和 `"`）。
3. **B3 索引格式版本**：
   - 单测：旧缓存（meta 无 `index_version`，index 中是 `form`）在 TTL 内、CacheOnly、304 三种情况下都被本地重建为 `multipart`，服务端请求计数为 0；`.raw` 缺失时 CacheOnly 报错、非 CacheOnly 正常拉取；
   - **real-env**：用 v0.2.0 源码构建的二进制建好缓存，停掉 spec 服务后，新二进制的 `desc` 显示 `BODY multipart`，`fetched_at` 不变。
4. **B4 工程与兼容性**：
   - `go vet ./...`、`go test ./... -count=1`、分层守卫测试通过，单个文件不超过 1000 行（GR104）；
   - Windows 主机上 `go test ./internal/request/ ./internal/commands/ ./internal/spec/ ./internal/config/ -count=1` 通过；
   - 用 v0.2.0 源码构建的基线二进制和新二进制，在 `cfg`、`lincfg` 上跑二期第一批的 `compat.txt` dry-run 清单，输出逐字节一致（两个配置都没有 overlay、真实 spec 中没有 multipart）。

## 范围、排除项与授权

**范围**：仓库 `inhere-tools/apic`（Git root）下的代码、README 和 docs。

**排除项**：

- 设计 0.2 的非目标；二期其余 deferred 项。
- 修改 gookit 各库。
- push、tag、发布 v0.3.0：属于外部动作，需要另行授权。

**授权**：

- 计划批准后，还需要用户另行发出当前执行请求，才能开始实施。
- 按功能点本地提交是默认授权。
- 主机动作只有 T07 的 Windows `go test`：
  - 源码用 `git archive HEAD` 导出到 `<WP>/tmp/apic-p2b-accept/src`，不在主机建 worktree，不动主机上的仓库；
  - 测试只用 `t.TempDir()`；除 Go 自己的构建缓存外，只读写 `<WP>/tmp/apic-p2b-accept/`。

- `host_or_non_offline_action=REQUIRED`

## 输入与批准证据

- 设计：`docs/design/2026-11-11-apic-phase2b-design.md` Draft 0.2，已批准。
  - 批准原话（2026-11-11）：“批准 apic 二期第二批设计 0.2（接受未复核修订）”。
  - 记录批准的提交：`d4f59aa`。
- 设计第 1 轮评审的 C1–C10、S1–S16 已并入 0.2，见设计“评审处理”。本计划按 0.2 的条款执行，不重述理由。
- 二期第一批验收产物：`<WP>/tmp/apic-p2-accept/`（`compat.txt`、`cfg`、`lincfg`）；真实 spec：`<WP>/tmp/apix-specs/`。
- 跟踪 issue：`my-tools-thv4`。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | overlay 文件的严格解码 | goccy/go-yaml（已依赖，`yaml.Strict()` 拒绝未知键） | 是 | 在 `config` 加 `LoadOverlay` | OWNER_EXTENSION | none | 无 |
| CAP-02 | 在索引上应用 overlay、改名消歧 | `spec.AssignNames` 的 `-N` 后缀规则 | 部分 | 在 `spec` 加 `ApplyOverlay`，后缀规则与 `AssignNames` 末段一致 | OWNER_EXTENSION | `AssignNames` 从路径生成名字，不处理“显式名字优先” | 两处后缀写法一致；不抽公共函数（只有两处） |
| CAP-03 | multipart 组装 | `mime/multipart`（`Writer.CreatePart`）、`net/textproto` | 是（stdlib） | 在 `request` 加 `multipart.go` | OWNER_EXTENSION | none | 不用 `CreateFormFile`（固定 octet-stream，事实 7） |
| CAP-04 | 部件 type 推断 | `mime.TypeByExtension`（读系统配置，结果因机器而异，设计已否决） | 否 | 内置扩展名表 | OWNER_EXTENSION | 结果必须跨机器一致 | 只在 `request` 内 |
| CAP-05 | type 校验 | `mime.ParseMediaType` | 是（stdlib） | 无 | DIRECT_REUSE | none | 无 |
| CAP-06 | overlay init 模板中 spec 文本的单行净化 | `skillgen.PlainText`（领域包之间不能互相依赖） | 否 | 在 `config/overlay.go` 写约 15 行的注释净化函数 | OWNER_EXTENSION | 分层规则禁止 `config` 导入 `skillgen` | 与 `PlainText` 规则相近；不移动 `PlainText`（超出范围） |
| CAP-07 | 索引重建 | `spec.loadOpenAPI`（kin-openapi 默认不解析外部引用，不联网） | 是 | `cache.go` 中按版本号调用 | DIRECT_REUSE | none | 无 |

### New module candidates

| candidate_id | capability_id | proposed_module | searched_candidates | direct_reuse_gap | thin_adapter_or_owner_extension_gap | proven_gap | unique_owner_and_lifecycle | deletion_or_merge_handling |
|---|---|---|---|---|---|---|---|---|
| NONE | none | none | none | none | none | none | none | none |

### Rejected new tools

| rejected_candidate | capability_id | deletion_test_and_reason |
|---|---|---|
| 把 `PlainText` 移到 `model` 供两处共用 | CAP-06 | 删掉它不缺能力；属于重构，超出本批范围 |
| JSON Patch / JSON Merge Patch 库 | CAP-02 | 删掉它不缺能力；overlay 字段有限，直接按字段改写更清楚 |

## 前置检查与 fail-closed 条件

运行目录均为 `inhere-tools/apic`。

1. **P1 工作树**：`git status --short` 为空，HEAD 至少包含本计划的批准提交。否则停下，查清归属。
2. **P2 Go 版本**：`go.mod` 写 `go 1.26.0`，容器内 `GOTOOLCHAIN=auto` 得到 go1.26.x。
3. **P3 基线**：`git archive v0.2.0 | tar -x -C <WP>/tmp/apic-p2b-accept/base` 后构建 `base/apic`。不用 worktree（容器里不能对主机仓库执行 `git worktree add`）。
4. **P4 curl**：容器里有 `curl` 且支持 `--form-string`（`curl --help all | grep form-string`）。没有时 B2 的 real-env 项标 `UNVERIFIED_REAL_ENV`，改用解析 dry-run 输出并逐项比对作为替代证据。
5. **P5 goccy Strict**：T01 开工前用探针确认 `yaml.UnmarshalWithOptions(data, &v, yaml.Strict())` 对嵌套 map 中的结构体也拒绝未知键，探针放 `<WP>/tmp/apic-p2b-probe/`。不成立时改为先解码到 `map[string]any` 再逐层校验键名，属于 Corrective。
6. **P6 主机可用**：T07 的 Windows `go test` 需要 gofer server 和本项目 worker 在线，否则 B4 的 Windows 项标 `UNVERIFIED_REAL_ENV`，停在完成 Gate。

## 波次与依赖

| 波次 | 任务 | 依赖 |
|---|---|---|
| W1 领域 | T01 → T02；T03 ∥ T04 | T02 用 T01 的类型；T03、T04 与 T01/T02 无共同文件；T04 的隐藏参数拒绝只用 `Param.Hidden`，在 T01 合并后开工 |
| W2 接线 | T05 ∥ T06 | T05 依赖 T01、T02、T03；T06 依赖 T04。两者没有共同文件 |
| W3 验收 | T07 | T01–T06 |

**文件归属**：除 `internal/config/overlay.go` 外每个文件只归一个任务；该文件由 T01 创建，T05 在 T01 合并后的 main 上追加模板与写入函数。

**执行方式**：沿用二期第一批的做法，用 gofer plan 跟进。

- 实施方：T01、T03 派 `-a suga`（`openai-codex/gpt-6-sol`，`--worktree`）；T02、T04、T05、T06 派 sonnet 子 agent，在主 checkout 中按任务串行；T07 我自己做。
- 我审查 diff、grep 调用点确认接线，并做验收。
- 任务书写明：分层规则 G021–G024 与 idevstd；禁止 web_fetch；go.mod/go.sum 在允许修改的范围内；禁止删注释和无关重排；测试不在 export 了 `GIT_DIR` 的 shell 中运行。
- 每个任务一次原子提交（`feat(<pkg>): ... (P2B-Tnn)`）。

## 任务

所有命令都在 `inhere-tools/apic` 下运行，并带上 `GOFLAGS=-buildvcs=false GOWORK=off GOTOOLCHAIN=auto`。

### T01 overlay 类型、加载与校验；保留名 overlay（config 侧）

- 文件：`internal/model/overlay.go`（新）、`internal/model/operation.go`、`internal/config/overlay.go`（新）、`internal/config/load.go` 及测试。
- 动作：
  - **类型**（`model`）：
    ```go
    type Overlay struct {
        File string                     // 绝对路径，只用于消息
        Ops  map[string]*OpOverlay      // 键：METHOD /path 或操作名
    }
    type OpOverlay struct {
        Name, Summary, Body, Output *string
        Hide   bool
        Params map[string]*ParamOverlay // 键：参数名或 in.name
    }
    type ParamOverlay struct {
        In       string                 // 仅新增或消歧时写
        Required *bool
        Type, Desc *string
        Enum     []string               // nil 表示不改
        Hide     bool
    }
    type OverlayChange struct{ Op, Field string } // Field 见 T02
    ```
    `Param` 增加 `Hidden bool \`json:",omitempty"\``，保证缓存和 `desc -o json` 在没有 overlay 时不变。
  - **加载**（`config`）：`LoadOverlay(dir, site string) (*model.Overlay, error)`：
    - 读 `<dir>/overlays/<site>.yaml`；不存在返回 `(nil, nil)`；
    - 用 `yaml.Strict()` 解码，YAML 键为 `operations`；
    - 取值校验：`body` ∈ json/form/multipart/xml/raw；`output` ∈ auto/json/raw/yaml/table；`type` ∈ string/integer/number/boolean/array；`in` ∈ query/header（`path` 也接受，但只用于消歧，新增时由 T02 报错）；`name` 匹配 `^[a-z0-9]+(?:-[a-z0-9]+)*$` 且不是 `help`；`in: header` 的参数名必须是 header token（RFC 7230 tchar），不能是设计列出的 7 个头（不区分大小写）；
    - 所有错误为 `ExitLocal`，消息格式 `overlay <文件>: <操作键>[.<参数键>]: <原因>`。
  - **保留名**：`config.reservedNames` 加 `overlay`。
- 验证：`go test ./internal/config/ -run 'TestLoadOverlay|TestReservedOverlay' -count=1`，覆盖：文件不存在；未知键（顶层、操作、参数三层）；每个枚举的非法值；header 黑名单的大小写变体；非 token 的 header 名；`help`；名为 `overlay` 的站点被跳过并有警告。
- 完成标准：用例通过；`model` 只导入标准库，`config` 只导入 `model`（分层测试）。
- 依赖：P5。

### T02 ApplyOverlay

- 文件：`internal/spec/overlay.go`（新）及测试。
- 动作：
  ```go
  // ApplyOverlay 返回新索引（深拷贝，不改 idx）、改动记录、警告、结构错误。ov 为 nil 时原样返回 idx 的拷贝。
  func ApplyOverlay(idx *model.Index, ov *model.Overlay) (*model.Index, []model.OverlayChange, []string, error)
  ```
  按以下顺序处理，键按字典序遍历，保证消息确定：
  1. **解析键**：含空格时按 `METHOD /path` 匹配（方法不区分大小写，路径与 `Operation.Path` 完全相等），否则按原始操作名匹配。命中多个操作、两个键命中同一操作：结构错误。找不到：警告 `overlay <文件>: <键> matches no operation`。
  2. **隐藏操作**：`hide: true` 的操作移出索引。
  3. **参数**：对每个参数键，先按完整名字精确匹配（`in` 为空时要求只命中一个位置，多个位置报错要求写 `in`），找不到再按 `in.name` 拆分匹配；写了 `in` 但与命中参数的位置不同：结构错误。
     - 命中：改 required/type/enum/desc；`hide` 设 `Hidden`。隐藏 path 参数、隐藏后仍为必填（spec 必填且 overlay 没有改成 `required: false`）：结构错误。
     - 未命中：作为新增参数，`in` 必须是 query 或 header，否则结构错误；类型默认 string。
  4. **请求体与输出**：`body` 替换 `Body.Type`，原来没有请求体时新建 `&BodySpec{Type: body}`；`output` 替换 `Output`。
  5. **改名**：先收集所有显式名字，彼此重复：结构错误。再给其余操作按原顺序检查：名字与显式名字或已占用的名字冲突时，依次尝试 `-2`、`-3`……（跳过所有已占用名字），并警告 `operation <M> <path> renamed "<a>" to "<b>"`。
  6. **改动记录**：`Field` 取值 `name`、`summary`、`body`、`output`、`params.<in>.<name>.<attr>`（attr ∈ required/type/enum/desc/hide/added），`Op` 为最终名字。
- 验证：`go test ./internal/spec/ -run TestApplyOverlay -count=1`，表驱动覆盖上述每条规则和每种结构错误；原索引在调用后不变（深拷贝）；`ov == nil` 时结果与原索引相等。
- 完成标准：用例通过；`spec` 只导入 `model`。
- 依赖：T01。

### T03 multipart 识别与索引格式版本

- 文件：`internal/spec/openapi.go`、`internal/spec/simple.go`、`internal/spec/cache.go` 及测试。
- 动作：
  - `bodyType`：`multipart/form-data`（含参数）返回 `multipart`；`sortedMedia` 排序名次 json 0、form 1、multipart 2、xml 3、其他 4。
  - `simple.go`：`body.type` 为空时取 json，否则必须是 json/form/multipart/xml/raw，否则 `ExitLocal`：`<文件> operation <name>: body.type must be one of json, form, multipart, xml, raw`。
  - `cache.go`：
    - `const indexVersion = 2`；`cacheMeta` 加 `IndexVersion int \`json:"index_version,omitempty"\``；写 meta 的两处都写入当前版本。
    - 先读 meta，再判断 CacheOnly/TTL/304。读到旧索引且 `meta.IndexVersion != indexVersion` 时，读 `.raw` 并调用 `loadOpenAPI` 本地重建；成功则写回 index 和 meta（保留 ETag 与 FetchedAt，只改版本号），用重建结果继续后面的流程；`.raw` 缺失或重建失败时视为没有旧缓存（`haveOld=false`），不写任何文件。
    - 重建产生的命名警告丢弃（与 TTL 命中时一致）。
- 验证：`go test ./internal/spec/ -run 'TestBodyType|TestSimpleBodyType|TestIndexVersion' -count=1`，覆盖：
  - multipart 识别和排序（同时有 json 与 multipart 时取 json）；
  - simple 的非法 `body.type`；
  - 旧缓存在 TTL 内、CacheOnly、304 下被重建，httptest 计数在 TTL 和 CacheOnly 下为 0，304 下为 1；重建后 meta 的 `fetched_at` 和 `etag` 不变、`index_version` 为 2；
  - `.raw` 缺失：CacheOnly 报 `no cached index`，非 CacheOnly 正常拉取；
  - 新写的缓存带版本号，再次读取不重建（用修改 `.raw` 的方法证明没有重建）。
- 完成标准：用例通过；`openapi_test.go` 中现有用例不改期望值也能通过，form 的用例仍为 form。
- 依赖：无。

### T04 multipart 请求：-F 解析、组装、显示；隐藏参数拒绝

- 文件：`internal/request/multipart.go`（新）、`internal/request/build.go`、`internal/request/body.go`、`internal/request/execute.go`、`internal/request/curl.go` 及测试；Windows 专用用例放 `multipart_windows_test.go`，Unix 专用（FIFO、`/dev/zero`）放 `multipart_unix_test.go`。
- 动作：
  - **类型与解析**（`multipart.go`）：
    ```go
    type Part struct {
        Name     string
        Value    string // 文本部件
        File     string // 文件部件：路径；非空即为文件
        Filename string // 文件部件：默认 filepath.Base(File)
        Type     string // 文件部件：默认按内置表推断，推断不出为 application/octet-stream
        Size     int64  // Assemble 后填写，供 -v 显示
    }
    const MaxMultipartBytes = 64 << 20
    func ParsePart(s string) (Part, error)              // -F 语法，错误为 ExitUsage
    func TextPart(name, value string) Part              // -f 用
    func CheckFiles(parts []Part) error                 // 打开 + Stat 普通文件后关闭；dry-run 用
    func Assemble(parts []Part) (body []byte, contentType string, err error) // 填写 Size
    ```
    - `ParsePart`：按第一个 `=` 拆出名字；值不以 `@` 开头时整段为文本值。以 `@` 开头时，路径可以是 `"..."`（`\"`、`\\` 转义）或不含 `;` 的裸路径，之后是 `;key=value` 参数，只接受 `type`、`filename`，参数值也可加双引号。名字为空、没有 `=`、未知参数、引号未闭合、路径为空：退出 2。名字、filename、type 含控制字符（`unicode.IsControl`）：退出 2。type 用 `mime.ParseMediaType` 校验。默认 filename 同样做控制字符检查。
    - **内置类型表**（扩展名不区分大小写）：xlsx、xls、csv、json、txt、xml、zip、gz、pdf、png、jpg/jpeg、gif、webp、doc、docx。
    - `CheckFiles`/`Assemble`：`os.Open` 后对文件句柄 `Stat`，`!Mode().IsRegular()` 返回 `ExitLocal`：`-F <name>: <path> is not a regular file`。`Assemble` 用 `multipart.Writer.CreatePart` 自己写 `Content-Disposition`（名字和 filename 只转义 `\` 与 `"`）和文件部件的 `Content-Type`；写入一个超过上限即报错的缓冲区，文件内容经 `io.LimitReader(f, 剩余额度+1)` 读入，总量超过 `MaxMultipartBytes` 返回 `ExitLocal`：`multipart body exceeds 64 MiB; use curl for large uploads`。错误消息不含文件内容。
  - **Build**（`build.go`、`body.go`）：
    - `Input` 加 `Forms []string`（-F 原始值）；`Prepared` 加 `Parts []Part`。
    - `buildBody` 改为返回 `(body []byte, ctype string, parts []Part, err error)`，判断顺序：
      1. `-F` 与 `-d` 或 `-f` 同时出现：退出 2 `-F cannot be combined with -d or -f`；
      2. 有 `-F`：操作存在且请求体类型不是 multipart 时退出 2 `operation body is <type>, not multipart; fix it with an overlay (body: multipart)`；否则逐个 `ParsePart`；
      3. 类型为 multipart 且有 `-f`：每个字段转成 `TextPart`，值不解析 `@`；`k:=json` 时 JSON 字符串去引号，其他值用 `json.Compact` 后的文本；
      4. 类型为 multipart 且有 `-d`：交给 Build 检查 Content-Type（见下）；
      5. 类型为 multipart、没有 `-F`/`-f`/`-d` 且请求体必填：退出 1 `request body is required (use -F, -d or -f)`。
      其余分支不变。原始路径调用（`op == nil`）有 `-F` 即为 multipart。
    - Build 处理 `-H` 时：有 parts 且出现 `Content-Type` 头（不区分大小写）退出 2；类型为 multipart、用 `-d` 且没有 `-H Content-Type` 退出 2 `multipart body with -d needs -H 'Content-Type: ...'; or use -F`。有 parts 时不设置 Content-Type 头。
    - **隐藏参数**：`collectValues` 中，`-p` 的名字（无论直接写名字还是 `in.name`）命中 `Hidden` 参数时退出 2 `parameter %q is hidden by overlay`；`--name` 选项不会出现隐藏参数（T06 不生成）。
  - **Execute**（`execute.go`）：
    - 写保护检查之后：dry-run 时有 parts 则 `CheckFiles`；非 dry-run 且有 parts、`Body == nil` 时 `Assemble`，把结果写回 `Prepared.Body` 和 `Prepared.Header` 的 Content-Type，之后再 `newRequest`；401 重发复用这段字节。
    - `-v`：请求头之后，每个部件一行：文件 `> part <name>: file=<filename> type=<type> size=<n>`，文本 `> part <name>: <打码后的值>`。
  - **Curl**（`curl.go`）：签名改为 `Curl(req *http.Request, body []byte, parts []Part, env model.Env, r *model.Redactor) string`。有 parts 时不输出 `Content-Type` 头和 `--data-raw`；文本部件 `--form-string <shQuote(name=打码后的值)>`；文件部件 `-F <shQuote(name=@"<路径>";type=<type>;filename="<filename>")>`，路径和 filename 中的 `\`、`"` 用 `\` 转义。
- 验证：`go test ./internal/request/ -run 'TestParsePart|TestAssemble|TestMultipart|TestCurlMultipart|TestHiddenParam' -count=1`，覆盖：
  - `ParsePart` 的语法表（设计中的 4 种写法、引号转义、每种退出 2 的情况）；
  - 类型推断与默认 filename；
  - 目录、FIFO、`/dev/zero`（Unix）、`NUL`（Windows）退出 1，FIFO 用例在 2 秒内结束；
  - 64 MiB 上限：用 `Truncate` 造 64 MiB+1 的稀疏文件；
  - 写保护拒绝时（退出 6）不打开文件：`-F f=@<不存在的路径>` 在受保护环境的 POST 上返回 6；
  - 401 重发两次收到的请求体相同；
  - 组合限制的每一条；
  - `-v` 不含哨兵文件内容；
  - 隐藏参数的两种 `-p` 写法；
  - 现有 `request_test.go` 中 Curl 的用例只改调用签名，期望输出不变。
- 完成标准：用例通过；`request` 只导入 `model`。
- 依赖：T01（`Param.Hidden`）。

### T05 overlay 接线：loadSiteIndex、overlay init、desc 标注、config check、保留名（commands 侧）

- 文件：`internal/commands/site.go`、`internal/commands/overlay.go`（新）、`internal/commands/run.go`、`internal/commands/ops.go`、`internal/commands/skill.go`、`internal/commands/config.go`、`internal/commands/app.go`、`internal/config/overlay.go`（只加模板与写入函数）及测试（`internal/commands/overlay_test.go`，新）。
- 动作：
  - **入口**（`site.go`）：
    ```go
    // 原始索引 + overlay；警告包含 loadIndex 的警告和 overlay 的警告。
    func loadSiteIndex(d *wiring.Deps, name string, site *model.Site) (*model.Index, []model.OverlayChange, []string, error)
    // 补全用：只读缓存；overlay 有任何错误都返回 error，不返回警告。
    func loadSiteIndexCached(d *wiring.Deps, name string, site *model.Site) (*model.Index, error)
    ```
    结构错误包装为 `ExitLocal`，消息经 Redactor。
  - **调用点**：`run.go` 的站点命令注册改用 `loadSiteIndex`；补全分支改用 `loadSiteIndexCached`，出错时用空索引（不退回原始索引）；`ops`、`desc`、`skill` 改用 `loadSiteIndex`。`site add`、`site refresh` 不变。`managementCommands` 加 `overlay`。
  - **desc 标注**（`ops.go`）：
    - 表格：方法与路径之后，若有 `name`/`summary`/`output` 改动，加一行 `OVERLAY: name, output`（按出现顺序）；被改动或新增的参数，NAME 列写成 `<name> (overlay)`；`BODY <type> (overlay)`。
    - `-o json`：有改动时输出 `struct{ *model.Operation; Overlay []string \`json:"overlay"\` }`，`Overlay` 为改动的 `Field` 列表；没有改动时仍输出 `op` 本身。
  - **config check**（`config.go`）：在现有警告之后，对 `overlays/` 中有文件的每个已配置站点，`loadIndexCached` 加 `LoadOverlay` 加 `ApplyOverlay`：没有缓存时输出 `site <s> overlay: no cached index; run apic site refresh <s>`；结构错误输出 `site <s> overlay: <err>`；找不到的键输出其警告。存在结构错误时退出 1（消息已写到 stdout），否则沿用现有行为。
  - **overlay init**（`overlay.go`、`config/overlay.go`）：
    - `apic overlay init <site> [--stdout]`；站点不存在退出 1；读原始索引（`loadIndex`，按一期规则可能刷新）。
    - `config.OverlayTemplate(site string, idx *model.Index) []byte`：
      ```yaml
      # apic overlay for site <site>; edit and keep only what you change.
      operations:
        "POST /back/cfg/get": {}  # name: back-cfg-get; <净化后的 summary>
      ```
      键用 `json.Marshal(METHOD + " " + path)`（JSON 字符串也是合法的 YAML 双引号标量）；同一 `METHOD /path` 有多个操作时（简单定义），改用操作名作键。注释经单行净化：控制字符和 U+2028/U+2029 替换为空格，截断到 120 字符。
      生成后用与 `LoadOverlay` 相同的严格解码自检：条目数等于操作数，且每条都是空；不符时退出 1，不写文件。
    - `config.WriteOverlay(dir, site string, data []byte) (path string, err error)`：`overlays` 已存在且是符号链接时退出 1；`MkdirAll(0700)`；`OpenFile(O_WRONLY|O_CREATE|O_EXCL, 0600)`；已存在时退出 1 `overlay file <path> already exists`。成功后 stdout 输出路径。
    - `--stdout` 只输出模板，不写文件。
  - **app.go**：注册 `overlayCommand()`。
- 验证：`go test ./internal/commands/ -run 'TestOverlay' -count=1`，经 `commands.Run` 覆盖 B1 的每一项（补全一项用 `--in-completion`，断言 stdout 不含隐藏操作、stderr 为空）。
- 完成标准：用例通过；grep 确认 `run.go`、`ops.go`、`skill.go` 已改用 `loadSiteIndex`，补全分支用 `loadSiteIndexCached`；`site.go` 中 `addSite`、`refreshSite` 仍用 `loadIndex`；分层测试通过。
- 依赖：T01、T02、T03（T03 让 `body: multipart` 的端到端用例成立）。

### T06 multipart 接线：-F 选项、隐藏参数选项、skill 提示

- 文件：`internal/commands/globalopts.go`、`internal/commands/dynamic.go`、`internal/commands/siterun.go`、`internal/skillgen/skillgen.go` 及测试（`internal/commands/multipart_test.go`，新；`internal/skillgen/skillgen_test.go`）。
- 动作：
  - `siteOpts` 加 `Forms []string`，绑定 `-F`（`Multipart field name=value or name=@file[;type=..][;filename=..] (repeatable)`）；`reservedOptNames` 加 `F`。
  - `siterun.go`：`in.Forms = so.Forms`。
  - `dynamic.go`：`rawCall` 判断“没有路径却给了请求参数”时算上 `-F`；`paramOptions` 跳过 `Hidden` 参数。
  - `skillgen.go`：How to call 加一条：`-F name=@path` 和 `-d @file` 会读取并发送本地文件，只用于用户明确给出的路径；`-f` 的值永远是文本。操作列表中 multipart 操作的行尾加 ` (multipart: use -F name=@path)`。
- 验证：`go test ./internal/commands/ -run 'TestMultipart' -count=1` 和 `go test ./internal/skillgen/ -count=1`，经 `commands.Run` 覆盖 B2 中除 real-env 之外的各项；skill 的两处新文字出现。
- 完成标准：用例通过；`--in-completion` 下 `-F` 出现在选项候选中，隐藏参数不出现。
- 依赖：T04。

### T07 验收、README、发布说明草稿

- 文件：
  - 本计划：只改“进度记录”。
  - `README.md`、`README.zh-CN.md`：overlay（格式、键的写法、hide 不是访问控制、`overlay init`、`config check`）；multipart（`-F` 写法、`-f` 永远是文本、64 MiB 上限、不支持 `-F k=<file`，以 `<` 开头的值按文本发送）。
  - `docs/release-notes/v0.3.0.md`：新功能；兼容性变化：保留名 `overlay`、multipart 接口改为 multipart 编码（缓存自动重建）、multipart 操作单独 `-d` 需带 Content-Type、简单定义的 `body.type` 改为枚举校验；回滚说明。
  - `<WP>/tmp/apic-p2b-accept/`：验收脚本、基线二进制和输出，不进仓库。
- 动作：
  1. **B1、B2**：`go test ./internal/commands/ ./internal/request/ ./internal/spec/ ./internal/config/ -run 'Overlay|Multipart|ParsePart|Assemble|Hidden|IndexVersion' -count=1`。
  2. **B2 real-env**：脚本 `curl-replay.sh` 启动一个记录 multipart 部件的小服务（`go run` 一个 tmp 下的 `main.go`），先用 apic 直接发送，再执行 apic `--dry-run` 输出的 curl 命令，比对两次收到的部件（名字、值、filename、type、内容 sha256）。
  3. **B3 real-env**：用一个含 multipart 操作的测试 spec，由本地服务提供；基线二进制 `site add` 建缓存；停掉服务；新二进制 `desc` 显示 `BODY multipart`；比较重建前后 meta 的 `fetched_at`。
  4. **B4**：
     - `go vet ./... && go test ./... -count=1 && go test ./internal/ -run TestLayering`；检查文件行数；
     - 基线与新二进制在 `cfg`、`lincfg` 上跑 `compat.txt`，比对输出；
     - 主机 job：`git archive HEAD` 导出到 `<WP>/tmp/apic-p2b-accept/src`，提交 `gofer job run -p my-tools-dev -a exec --runner local --sync --cwd tmp/apic-p2b-accept/src -- go test ./internal/request/ ./internal/commands/ ./internal/spec/ ./internal/config/ -count=1`。
- 验证：各项的命令和输出摘要写进“进度记录”，完整输出放在 `<WP>/tmp/apic-p2b-accept/`。
- 完成标准：B1–B4 全部通过，或明确标注 UNVERIFIED 并说明原因（P4、P6）。
- 依赖：T01–T06。

## 偏离登记

| 偏离 | 规则或设计条款 | 理由 |
|---|---|---|
| 隐藏后仍为必填的参数是结构错误 | 设计只写了“path 参数不能隐藏” | 与 D13 同理：必填参数隐藏后无法传入（`-p` 被拒绝），操作就调不通了 |
| `-p in.name` 写法命中隐藏参数同样拒绝 | 设计“隐藏参数用 `-p` 传入时退出 2” | 事实 5：带位置的写法会绕过按名字查找；不拒绝的话隐藏承诺不成立 |
| `overlay init` 的键用 JSON 字符串编码、生成后严格解码自检，而不是调用 YAML 编码器整体生成 | 设计“用 YAML 编码器生成文件内容” | 模板需要注释，编码器不便输出；JSON 字符串是合法的 YAML 标量，自检保证 spec 文本无法注入条目，达到设计意图 |
| `config check` 在 overlay 有结构错误时退出 1 | 设计未规定退出码 | 结构错误会让站点不可用，脚本需要能检出；没有 overlay 错误时行为不变 |
| `overlay init` 的注释净化在 `config` 内另写，不复用 `skillgen.PlainText` | 设计 S3 处理“单行净化” | 分层规则禁止领域包互相导入；规则与 `PlainText` 一致 |

## 回滚与恢复

- 每个任务各自一个提交，恢复点写进“进度记录”；回滚某个任务用 `git revert <提交>`。
- 不涉及数据迁移。缓存重建写回的文件旧版也能读取（多出的 meta 字段被忽略）；测试只用 `t.TempDir()`，不读写真实的 `~/.config/apic`。
- 实施中遇到 Semantic Amendment（例如 P5 不成立且无法局部修正、curl 回放显示设计的渲染规则不可行），停在当前任务，回到 plan review，并请求人工批准。

## 人工 Gate

1. 计划批准：本文档。
2. 当前执行请求：计划批准后，由用户另行发出。
3. 外部动作：push、tag 和发布 v0.3.0 不在本计划内，完成后另行批准。

## 可追溯性

| 设计条款与验收 | 任务 | 验证 |
|---|---|---|
| §1 overlay 格式、定位、修改规则；D1–D5、D11、D13 | T01、T02 | T01、T02 测试；B1 |
| §1 应用入口、错误影响范围、补全静默；D10 | T05 | T05 测试；B1 |
| §1 overlay init、修改标注、config check、保留名 | T01、T05 | T05 测试；B1 |
| §2 识别；D6 | T03 | T03 测试；B3 |
| §2 `-F` 写法、与 `-d/-f/-H` 的关系、组装、显示；D7–D9、D12 | T04、T06 | T04、T06 测试；B2（含 real-env） |
| §3 索引格式版本；D6 | T03 | T03 测试；B3（含 real-env） |
| 安全：文件外发、部件头注入、overlay 信任边界、header 黑名单 | T01、T04、T05、T06 | 对应测试；B1、B2 |
| 兼容性与回滚 | T03、T07 | B4；发布说明 |
| 工程规范 GR104、GR107–GR110、SR2002–SR2004 | 全部任务 | 分层测试；B4 |

## 完成 Gate 与剩余工作

- 完成条件：B1–B4 全部满足，或者用户接受其中标注为 UNVERIFIED 的项。
- 完成时在“进度记录”里写一段不超过 8 行的总结。
- 剩余工作：发布 v0.3.0（外部动作 Gate）；二期其余 deferred 项；gcli issue `my-tools-74ur`。

## 进度记录

（未开始）
