<!-- template_id: plan; template_version: 1.3.0 -->
# apic 二期第二批（overlay 与 multipart）实施计划

> 状态：Draft 0.2 / 已批准（用户 2026-11-11：“批准 apic 二期第二批计划 0.2（接受未复核修订）；开始实施”；0.2 为第 1 轮评审后的未复核修订）

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-11 | Claude（inhere：“批准 apic 二期第二批设计 0.2（接受未复核修订）”） | 初稿：3 个波次、7 个任务，覆盖设计 0.2 的 overlay、multipart 与索引格式版本 |
| 0.2 | 2026-11-11 | Claude | 处理第 1 轮两轴评审（阻塞 P1/Q2、P2/Q4、Q1；纠正 P3–P13、Q3、Q5–Q8；建议若干）：打开文件前先 Stat，Unix 上非阻塞打开；type 只接受不带参数的 `type/subtype`；名字与 filename 禁止 `"` 和 `\`，路径禁止控制字符；`@-` 拒绝；`-f` 字段名校验；`k:=json` 与 form 分支共用转换；重建时清空 ETag；header 黑名单移到新增分支；overlay 类型标签与 null 条目；T06 先于 T05；Unix 测试加 build tag；curl 与 `-v` 对路径和 filename 打码；desc 标出隐藏参数；compat 用复制的配置；发布说明补全 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 规划工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：交付设计 0.2 的三项并通过验收：overlay、multipart 上传、索引格式版本与本地重建。
- Scope freeze：只做设计 0.2 的范围。设计“非目标”和二期其余 deferred 项（oauth2、`${keyring:}`/`${exec:}`、分页、重定向跟随、`--auth-for-spec`、MCP）实施中即使发现需要，也只记为 deferred。
- 评审预算：与设计相同，按高暴露处理（SR1409），做一轮两轴独立评审：一是正确性与可执行性，二是安全与契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。
- 评审状态：第 1 轮已完成，两个方向共提出 4 个阻塞项（其中 FIFO 与部件头转义两项两轴重复），0.2 已逐条修订。预算只有一轮，所以 0.2 属于未复核修订，见“评审处理”。

## 目标与完成定义

**目标**：v0.2.0 的用户升级后：

- 写一个 `overlays/<site>.yaml` 就能改名、隐藏、修参数、改请求体类型和输出格式，`ops`、调用、补全、`desc`、`skill` 都按覆盖后的结果工作；
- 用 `-F` 调用文件上传接口，`--dry-run` 给出能原样复现的 curl；
- 已缓存的 multipart 接口不用手动 refresh 就被正确识别。

行为符合设计 0.2 的各节和 D1–D13，以及本计划“偏离登记”中的细化与事实纠正。

**完成定义（验收）**：

1. **B1 overlay**：经真实分发（`commands.Run`）覆盖：
   - 改名（含两个操作互换名字、生成名与 overlay 名冲突时加后缀并警告、两个 overlay 名冲突报错、改名为 `help` 报错）；
   - 隐藏操作：`ops`、补全、`skill` 中都没有，按名调用报未知操作，原始路径调用仍可发出；
   - 参数：改 required/type/enum/desc；新增 query 与 header 参数；隐藏参数不生成选项、`desc` 中标 `(hidden)`，`-p debug=1` 和 `-p query.debug=1` 都退出 2；
   - 隐藏 path 参数、隐藏必填参数、新增参数写成 path、新增 header 用黑名单名字、`in` 与已有位置不一致：都是结构错误；spec 自带的 `Authorization` 参数写 `in: header` 消歧不报错；
   - 条目值为 null（`"GET /a":`）等同于空条目；
   - 结构错误时，该站点的调用、原始路径调用、`ops`、`desc`、`skill` 都退出 1，`site refresh` 照常成功，补全输出为空且 stderr 为空；
   - 找不到的键只在 stderr 警告；
   - `desc` 表格的 `(overlay)` 标注，`desc -o json` 的 `overlay` 数组；没有 overlay 时 `desc -o json` 与 v0.2.0 逐字节一致；
   - `overlay init`：文件能被读回，应用后 0 改动、0 警告；spec 的 summary 含换行和 `x: {hide: true}` 时不产生任何条目；文件已存在、`overlays` 不是目录（符号链接）时退出 1；权限 0600/0700（Unix）；`--stdout`；
   - `config check` 报出结构错误和找不到的键，有结构错误时退出 1；
   - 名为 `overlay` 的站点被跳过并警告。
2. **B2 multipart**：
   - httptest 服务端用 `mime/multipart` 解析，断言部件的名字、值、filename、Content-Type 和文件内容；
   - `-f` 在 multipart 操作上发文本部件，`-f k=@/etc/passwd` 发的是字面值；`-f k:=json` 与 form 分支一致；`-f` 字段名含 CR/LF 退出 2；
   - 组合限制：`-F` 与 `-d`/`-f`/`-H Content-Type`/`-p header.Content-Type` 退出 2；非 multipart 操作用 `-F` 退出 2；multipart 操作单独 `-d` 不带 Content-Type 退出 2，带了则原样发送；原始路径调用用 `-F` 发 multipart；
   - 退出 2 的输入：名字、filename、路径含控制字符；名字或 filename 含 `"`、`\`（包括默认 filename）；type 不是 `type/subtype` 形式（含 `;`、`,`、`"`、空格）；路径为 `-`；
   - 目录、`/dev/zero`、FIFO（Unix）和 `NUL`（Windows）都退出 1，FIFO 用例 2 秒内结束；总量超过 64 MiB 退出 1，dry-run 同样退出 1；
   - 写保护拒绝时（退出 6）不打开文件：用一个不存在的路径证明退出码是 6 而不是 1；
   - 401 重发的请求体与首次相同；
   - `-v` 每个部件一行，哨兵文件内容不出现在 stderr；
   - **real-env**：用容器里真实的 `curl` 执行 dry-run 输出的命令，发往同一个记录服务，收到的部件与 apic 直接发送时一致。用例：文本值含 `@`、`;type=`；路径含 `;`、`,` 和 `"`（filename 显式给出）；type 为 `text/plain`。
3. **B3 索引格式版本**：
   - 单测：旧缓存（meta 无 `index_version`，index 中是 `form`）在 TTL 内、CacheOnly 下被本地重建为 `multipart`，请求计数为 0，`fetched_at` 不变、`etag` 被清空、`index_version` 为 2；TTL 过期时先重建再拉取，请求不带 `If-None-Match`，计数为 1，结果为 multipart；`.raw` 缺失时 CacheOnly 报错、非 CacheOnly 不带 `If-None-Match` 正常拉取；
   - **real-env**：用 v0.2.0 源码构建的二进制建好缓存，停掉 spec 服务后，新二进制的 `desc` 显示 `BODY multipart`，`fetched_at` 不变。
4. **B4 工程与兼容性**：
   - `go vet ./...`、`go test ./... -count=1`、分层守卫测试通过，单个文件不超过 1000 行（GR104）；
   - Windows 主机上 `go test ./internal/request/ ./internal/commands/ ./internal/spec/ ./internal/config/ -count=1` 通过；
   - 用 v0.2.0 源码构建的基线二进制和新二进制，各用一份复制的 `cfg`、`lincfg`，跑二期第一批的 `compat.txt` dry-run 清单，输出逐字节一致。

## 范围、排除项与授权

**范围**：仓库 `inhere-tools/apic`（Git root）下的代码、README 和 docs。

**排除项**：

- 设计 0.2 的非目标；二期其余 deferred 项。
- `-d @file` 在写保护检查之前读取、没有大小上限、遇到 FIFO 会阻塞：一期就有的行为，本批不改，登记为 deferred，README 写明与 `-F` 的差异。
- 修改 gookit 各库。
- push、tag、发布 v0.3.0：属于外部动作，需要另行授权。

**授权**：

- 计划批准后，还需要用户另行发出当前执行请求，才能开始实施。
- 按功能点本地提交是默认授权。
- 主机动作只有 T07 的 Windows `go test`：
  - 源码用 `git archive HEAD` 导出到 `<WP>/tmp/apic-p2b-accept/src`，不在主机建 worktree，不动主机上的仓库；
  - 测试只用 `t.TempDir()`；除 Go 自己的构建缓存外，只读写 `<WP>/tmp/apic-p2b-accept/`。
- 一期和二期第一批的验收产物只读，需要的配置先复制到 `<WP>/tmp/apic-p2b-accept/` 再用。

- `host_or_non_offline_action=REQUIRED`

## 输入与批准证据

- 设计：`docs/design/2026-11-11-apic-phase2b-design.md` Draft 0.2，已批准。
  - 批准原话（2026-11-11）：“批准 apic 二期第二批设计 0.2（接受未复核修订）”。
  - 记录批准的提交：`d4f59aa`。
- 设计第 1 轮评审的 C1–C10、S1–S16 已并入 0.2，见设计“评审处理”。本计划按 0.2 的条款执行，不重述理由。
- 计划第 1 轮评审：`<WP>/tmp/apic-p2b-plan-review/{correctness,security}.md`，探针在同目录下。
- 二期第一批验收产物：`<WP>/tmp/apic-p2-accept/`（`compat.txt`，配置在 `c1/{cfg,lincfg}`）；真实 spec：`<WP>/tmp/apix-specs/`。
- 跟踪 issue：`my-tools-thv4`。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | overlay 文件的严格解码 | goccy/go-yaml（已依赖，`yaml.Strict()` 拒绝未知键与重复键，评审探针已证实三层都生效） | 是 | 在 `config` 加 `LoadOverlay` | OWNER_EXTENSION | none | 无 |
| CAP-02 | 在索引上应用 overlay、改名消歧 | `spec.AssignNames` 的 `-N` 后缀规则 | 部分 | 在 `spec` 加 `ApplyOverlay`，预留规则与 `naming.go:87-90` 一致 | OWNER_EXTENSION | `AssignNames` 从路径生成名字，不处理“显式名字优先” | 两处后缀写法一致；不抽公共函数（只有两处） |
| CAP-03 | multipart 组装 | `mime/multipart`（`Writer.CreatePart`）、`net/textproto` | 是（stdlib） | 在 `request` 加 `multipart.go` | OWNER_EXTENSION | none | 不用 `CreateFormFile`（固定 octet-stream，事实 7） |
| CAP-04 | 部件 type 推断 | `mime.TypeByExtension`（读系统配置，结果因机器而异，设计已否决） | 否 | 内置扩展名表 | OWNER_EXTENSION | 结果必须跨机器一致 | 只在 `request` 内 |
| CAP-05 | 不阻塞地打开文件 | `os.Stat`、`os.OpenFile` + `syscall.O_NONBLOCK`（Unix）、`os.SameFile` | 是（stdlib） | `request` 内按 build tag 分两个小文件 | OWNER_EXTENSION | 评审探针：FIFO 上 `os.Open` 阻塞 | 无 |
| CAP-06 | overlay init 模板中 spec 文本的单行净化 | `skillgen.PlainText`（领域包之间不能互相依赖） | 否 | 在 `config/overlay.go` 写约 15 行的注释净化函数 | OWNER_EXTENSION | 分层规则禁止 `config` 导入 `skillgen` | 规则与 `PlainText` 一致；不移动 `PlainText`（超出范围） |
| CAP-07 | 索引重建 | `spec.loadOpenAPI`（kin-openapi 默认不解析外部引用，`ToV3` 也不联网，评审已核实） | 是 | `cache.go` 中按版本号调用 | DIRECT_REUSE | none | 无 |

### New module candidates

| candidate_id | capability_id | proposed_module | searched_candidates | direct_reuse_gap | thin_adapter_or_owner_extension_gap | proven_gap | unique_owner_and_lifecycle | deletion_or_merge_handling |
|---|---|---|---|---|---|---|---|---|
| NONE | none | none | none | none | none | none | none | none |

### Rejected new tools

| rejected_candidate | capability_id | deletion_test_and_reason |
|---|---|---|
| 把 `PlainText` 移到 `model` 供两处共用 | CAP-06 | 删掉它不缺能力；属于重构，超出本批范围 |
| JSON Patch / JSON Merge Patch 库 | CAP-02 | 删掉它不缺能力；overlay 字段有限，直接按字段改写更清楚 |
| 按 WHATWG 规则对名字与 filename 百分号编码 | CAP-03 | 删掉它不缺能力：直接拒绝 `"` 和 `\` 更简单，并且 apic 直发与 curl 回放的结果自然一致 |

## 前置检查与 fail-closed 条件

运行目录均为 `inhere-tools/apic`。

1. **P1 工作树**：`git status --short` 为空，HEAD 至少包含本计划的批准提交。否则停下，查清归属。
2. **P2 Go 版本**：`go.mod` 写 `go 1.26.0`，容器内 `GOTOOLCHAIN=auto` 得到 go1.26.x。
3. **P3 基线与兼容性素材**：
   - `git archive v0.2.0 | tar -x -C <WP>/tmp/apic-p2b-accept/base` 后构建 `base/apic`。不用 worktree（容器里不能对主机仓库执行 `git worktree add`）。
   - 把 `<WP>/tmp/apic-p2-accept/c1/{cfg,lincfg}` 分别复制到 `<WP>/tmp/apic-p2b-accept/compat/{base,new}/`，两个二进制各用一份，避免新二进制重建缓存时改写对方或原始产物。
   - `grep -l multipart` 复制后各 `cache/specs/*.raw` 为空，确认真实 spec 中没有 multipart（评审时已为空）；不为空时把涉及的命令从比对清单中列出并说明。
4. **P4 curl**：容器里有 `curl` 且支持 `--form-string`（评审已用 curl 8.5.0 证实）。没有时 B2 的 real-env 项标 `UNVERIFIED_REAL_ENV`，改用解析 dry-run 输出并逐项比对作为替代证据。
5. **P5 主机可用**：T07 的 Windows `go test` 需要 gofer server 和本项目 worker 在线，否则 B4 的 Windows 项标 `UNVERIFIED_REAL_ENV`，停在完成 Gate。

## 波次与依赖

| 波次 | 任务 | 依赖 |
|---|---|---|
| W1 领域 | T01 → T02；T03 ∥ T04 | T02 用 T01 的类型；T03、T04 与 T01/T02 无共同文件；T04 的隐藏参数拒绝用 `Param.Hidden`，在 T01 合并后开工 |
| W2 接线 | T06 → T05 | T06 依赖 T04；T05 依赖 T01、T02、T03、T06（B1 中隐藏参数的端到端用例需要 T04 的拒绝和 T06 的选项过滤） |
| W3 验收 | T07 | T01–T06 |

**文件归属**：除 `internal/config/overlay.go` 外每个文件只归一个任务；该文件由 T01 创建，T05 在 T01 合并后的 main 上追加模板与写入函数。`internal/commands` 下新建的测试文件各自定义的辅助函数加任务前缀（`ovl…`、`mp…`），复用 `dynamic_test.go` 中已有的 `setup`、`runIO`、`newFakeAPI`。

**执行方式**：沿用二期第一批的做法，用 gofer plan 跟进。

- 实施方：T01、T03 派 `-a suga`（`openai-codex/gpt-6-sol`，`--worktree`），可并行；T02、T04、T06、T05 派 sonnet 子 agent，在主 checkout 中按这个顺序串行（T04 可在 T02 进行时开工，二者无共同文件）；T07 我自己做。
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
        File string                 `yaml:"-"`          // 绝对路径，只用于消息
        Ops  map[string]*OpOverlay  `yaml:"operations"` // 键：METHOD /path 或操作名
    }
    type OpOverlay struct {
        Name    *string                  `yaml:"name"`
        Summary *string                  `yaml:"summary"`
        Body    *string                  `yaml:"body"`
        Output  *string                  `yaml:"output"`
        Hide    bool                     `yaml:"hide"`
        Params  map[string]*ParamOverlay `yaml:"params"` // 键：参数名或 in.name
    }
    type ParamOverlay struct {
        In       string   `yaml:"in"`       // 仅新增或消歧时写
        Required *bool    `yaml:"required"`
        Type     *string  `yaml:"type"`
        Desc     *string  `yaml:"desc"`
        Enum     []string `yaml:"enum"`     // nil 表示不改
        Hide     bool     `yaml:"hide"`
    }
    type OverlayChange struct{ Op, Field string } // Field 见 T02
    ```
    `Param` 增加 `Hidden bool \`json:",omitempty"\``，保证缓存和 `desc -o json` 在没有 overlay 时不变。
  - **加载**（`config`）：`LoadOverlay(dir, site string) (*model.Overlay, error)`：
    - 读 `<dir>/overlays/<site>.yaml`；不存在返回 `(nil, nil)`；
    - 用 `yaml.Strict()` 解码；值为 null 的操作或参数条目替换为空结构体；
    - 取值校验：`body` ∈ json/form/multipart/xml/raw；`output` ∈ auto/json/raw/yaml/table；`type` ∈ string/integer/number/boolean/array；`in` ∈ query/header/path；`name` 匹配 `^[a-z0-9]+(?:-[a-z0-9]+)*$` 且不是 `help`。header 名的 token 检查与黑名单只对新增参数生效，放在 T02；
    - 所有错误为 `ExitLocal`。取值错误的消息格式 `overlay <文件>: <操作键>[.<参数键>]: <原因>`；解码错误只取 goccy 错误的第一行，格式 `overlay <文件>: <第一行>`。
  - **保留名**：`config.reservedNames` 加 `overlay`。
- 验证：`go test ./internal/config/ -run 'TestLoadOverlay|TestReservedOverlay' -count=1`，覆盖：文件不存在；未知键与重复键（顶层、操作、参数三层）；`file:` 键被拒绝；null 条目；每个枚举的非法值；`help`；解码错误为单行；名为 `overlay` 的站点被跳过并有警告。
- 完成标准：用例通过；`model` 只导入标准库，`config` 只导入 `model`（分层测试）。
- 依赖：无。

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
     - 未命中：作为新增参数，`in` 必须是 query 或 header，否则结构错误；`in: header` 时名字必须是 header token（RFC 7230 tchar），且不能是设计列出的 7 个头（不区分大小写）；类型默认 string。
  4. **请求体与输出**：`body` 替换 `Body.Type`，原来没有请求体时新建 `&BodySpec{Type: body}`；`output` 替换 `Output`。
  5. **改名**：先收集所有显式名字，彼此重复：结构错误。已占用集合 = 显式名字 + 其余所有操作的当前名字（整体预留，同 `naming.go:87-90`）。再给其余操作按原顺序检查：名字与显式名字冲突时，依次尝试 `-2`、`-3`……，跳过已占用名字，选中后加入集合，并警告 `operation <M> <path> renamed "<a>" to "<b>"`。
  6. **改动记录**：`Field` 取值 `name`、`summary`、`body`、`output`、`params.<in>.<name>.<attr>`（attr ∈ required/type/enum/desc/hide/added），`Op` 为最终名字。
- 验证：`go test ./internal/spec/ -run TestApplyOverlay -count=1`，表驱动覆盖上述每条规则和每种结构错误；已有 `Authorization` header 参数写 `in: header` 消歧时不报错；原索引在调用后不变（深拷贝）；`ov == nil` 时结果与原索引相等；改名结果与键的遍历顺序无关。
- 完成标准：用例通过；`spec` 只导入 `model`。
- 依赖：T01。

### T03 multipart 识别与索引格式版本

- 文件：`internal/spec/openapi.go`、`internal/spec/simple.go`、`internal/spec/cache.go` 及测试（含 `internal/spec/openapi_test.go` 现有期望的修改）。
- 动作：
  - `bodyType`：`multipart/form-data`（含参数）返回 `multipart`；`sortedMedia` 排序名次 json 0、form 1、multipart 2、xml 3、其他 4。
  - `simple.go`：`body.type` 为空时取 json，否则必须是 json/form/multipart/xml/raw，否则 `ExitLocal`：`<文件> operation <name>: body.type must be one of json, form, multipart, xml, raw`。
  - `cache.go`：
    - `const indexVersion = 2`；`cacheMeta` 加 `IndexVersion int \`json:"index_version,omitempty"\``；写 meta 的两处都写入当前版本。
    - 先读 meta，再判断 CacheOnly/TTL/304。读到旧索引且 `meta.IndexVersion != indexVersion` 时，读 `.raw` 并调用 `loadOpenAPI` 本地重建：
      - 成功：清空 `meta.ETag`（降级到 v0.2.0 后 `site refresh` 才会整份重新拉取），保留 `FetchedAt`，写入当前版本号，写回 index 和 meta，用重建结果继续后面的流程。写回失败时仍使用重建结果，并把失败作为警告返回（补全模式下警告不输出）。
      - `.raw` 缺失或重建失败：视为没有旧缓存（`haveOld=false`），同时清空内存中的 `ETag` 与 `FetchedAt`，不写任何文件。
    - 重建产生的命名警告丢弃（与 TTL 命中时一致）。
- 验证：`go test ./internal/spec/ -run 'TestBodyType|TestSimpleBodyType|TestIndexVersion' -count=1`，覆盖：
  - multipart 识别和排序（同时有 json 与 multipart 时取 json）；`openapi_test.go` 中 `multipart/form-data → form` 的期望改为 multipart，urlencoded 仍为 form；
  - simple 的非法 `body.type`；
  - B3 单测各项（TTL 内与 CacheOnly 计数 0、`fetched_at` 不变、`etag` 清空、版本为 2；TTL 过期计数 1 且无 `If-None-Match`；`.raw` 缺失的两种情况）；
  - 新写的缓存带版本号，再次读取不重建（用修改 `.raw` 的方法证明没有重建）。
- 完成标准：用例通过；除上述一处期望外，`openapi_test.go` 现有用例不改期望值。
- 依赖：无。

### T04 multipart 请求：-F 解析、组装、显示；隐藏参数拒绝

- 文件：`internal/request/multipart.go`（新）、`internal/request/open_unix.go` 与 `internal/request/open_other.go`（新，`//go:build unix` / `!unix`）、`internal/request/build.go`、`internal/request/body.go`、`internal/request/execute.go`、`internal/request/curl.go` 及测试；Unix 专用用例（FIFO、`/dev/zero`）放 `multipart_unix_test.go`，文件头写 `//go:build unix`；Windows 专用（`NUL`）放 `multipart_windows_test.go`。
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
    func TextPart(name, value string) (Part, error)     // -f 用；名字校验同 ParsePart
    func CheckFiles(parts []Part) error                 // dry-run 用：同 Assemble 的检查，并检查大小之和
    func Assemble(parts []Part) (body []byte, contentType string, err error) // 填写 Size
    ```
    - `ParsePart`：按第一个 `=` 拆出名字；值不以 `@` 开头时整段为文本值（以 `<` 开头也按文本）。以 `@` 开头时，路径可以是 `"..."`（`\"`、`\\` 转义）或不含 `;` 的裸路径，之后是 `;key=value` 参数，只接受 `type`、`filename`，`filename` 的值可加双引号。以下退出 2：名字为空、没有 `=`、未知参数、引号未闭合、路径为空、路径为 `-`（提示 `stdin is not supported; use ./-`）。
    - **校验**（`ParsePart` 与 `TextPart` 共用）：名字和 filename（含默认值）不能含控制字符、`"`、`\`；路径不能含控制字符；默认 filename 不合格时提示用 `;filename=` 指定。type 必须匹配 `^[A-Za-z0-9!#$&^_.+-]+/[A-Za-z0-9!#$&^_.+-]+$`（不带参数）。都是退出 2。
    - **内置类型表**（扩展名不区分大小写）：xlsx、xls、csv、json、txt、xml、zip、gz、pdf、png、jpg/jpeg、gif、webp、doc、docx。
    - **打开文件**（`openRegular(path) (*os.File, os.FileInfo, error)`，`CheckFiles` 与 `Assemble` 共用）：先 `os.Stat(path)`，不是普通文件返回 `ExitLocal`：`-F <name>: <path> is not a regular file`；再打开（Unix 上 `os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)`，其他平台 `os.Open`）；对句柄 `Stat`，必须仍是普通文件且与前一次 `os.SameFile`，否则同样退出 1。`/proc` 下的伪文件会被视为普通文件，只受 64 MiB 上限约束。
    - **Assemble**：用 `multipart.Writer.CreatePart` 自己写 `Content-Disposition`（名字和 filename 已保证不含 `"` 与 `\`，直接加引号）和文件部件的 `Content-Type`；写入一个超过上限即报错的缓冲区，文件内容经 `io.LimitReader(f, 剩余额度+1)` 读入，总量超过 `MaxMultipartBytes` 返回 `ExitLocal`：`multipart body exceeds 64 MiB; use curl for large uploads`。错误消息不含文件内容。
    - **CheckFiles**：对每个文件做 `openRegular` 后关闭；文本值长度与文件 `Size()` 之和超过上限时同样退出 1。
  - **Build**（`build.go`、`body.go`）：
    - `Input` 加 `Forms []string`（-F 原始值）；`Prepared` 加 `Parts []Part`。
    - 把 form 分支中 `k:=json` 的转换（TrimSpace、JSON 字符串去引号）提成一个函数，form 与 multipart 共用。
    - `buildBody` 改为返回 `(body []byte, ctype string, parts []Part, err error)`，判断顺序：
      1. `-F` 与 `-d` 或 `-f` 同时出现：退出 2 `-F cannot be combined with -d or -f`；
      2. 有 `-F`：操作存在、没有请求体时退出 2 `operation has no request body; add one with an overlay (body: multipart)`；请求体类型不是 multipart 时退出 2 `operation body is <type>, not multipart; fix it with an overlay (body: multipart)`；否则逐个 `ParsePart`；
      3. 类型为 multipart 且有 `-f`：每个字段经共用转换后调用 `TextPart`，值不解析 `@`；
      4. 类型为 multipart 且有 `-d`：交给 Build 检查 Content-Type（见下）；
      5. 类型为 multipart、没有 `-F`/`-f`/`-d` 且请求体必填：退出 1 `request body is required (use -F, -d or -f)`。
      其余分支不变。原始路径调用（`op == nil`）有 `-F` 即为 multipart。
    - Build 在合并完 `-p header.*` 与 `-H` 之后检查：有 parts 且请求头中有 `Content-Type`（不区分大小写，任何来源）退出 2；类型为 multipart、用 `-d` 且没有 Content-Type 退出 2 `multipart body with -d needs -H 'Content-Type: ...'; or use -F`。有 parts 时不设置 Content-Type 头。
    - **隐藏参数**：`collectValues` 中，`-p` 的名字（无论直接写名字还是 `in.name`）命中 `Hidden` 参数时退出 2 `parameter %q is hidden by overlay`。
  - **Execute**（`execute.go`）：
    - 写保护检查之后：dry-run 时有 parts 则 `CheckFiles`；非 dry-run 且有 parts、`Body == nil` 时 `Assemble`，把结果写回 `Prepared.Body` 和 `Prepared.Header` 的 Content-Type，之后再 `newRequest`；401 重发复用这段字节。
    - `-v`：请求头之后，每个部件一行：文件 `> part <name>: file=<打码后的 filename> type=<type> size=<n>`，文本 `> part <name>: <打码后的值>`。
  - **Curl**（`curl.go`）：签名改为 `Curl(req *http.Request, body []byte, parts []Part, env model.Env, r *model.Redactor) string`，唯一调用方是 `execute.go`。有 parts 时不输出 `Content-Type` 头和 `--data-raw`；文本部件 `--form-string <shQuote(name=打码后的值)>`；文件部件 `-F <shQuote(name=@"<打码后的路径>";type=<type>;filename="<打码后的 filename>")>`，路径中的 `\`、`"` 用 `\` 转义（filename 已保证不含这两个字符）。
- 验证：`go test ./internal/request/ -run 'TestParsePart|TestAssemble|TestMultipart|TestCurlMultipart|TestHiddenParam' -count=1`，覆盖：
  - `ParsePart` 的语法表（设计中的 4 种写法、引号转义、上面每种退出 2 的情况，含 `type=text/plain;x="a,secret.txt;y"` 和 `type=a/b,c`）；
  - `TextPart` 名字含 CR/LF 退出 2；
  - 类型推断与默认 filename；
  - 目录、FIFO、`/dev/zero`（Unix）、`NUL`（Windows）退出 1，FIFO 用例在 2 秒内结束（dry-run 与实际发送各一次）；
  - 64 MiB 上限：用 `Truncate` 造 64 MiB+1 的稀疏文件，实际发送与 dry-run 都退出 1；
  - 写保护拒绝时（退出 6）不打开文件：`-F f=@<不存在的路径>` 在受保护环境的 POST 上返回 6；
  - 401 重发两次收到的请求体相同；
  - 组合限制的每一条，含 `-p header.Content-Type`；
  - 断言原始 `Content-Disposition` 字节；
  - `-v` 不含哨兵文件内容，路径中的敏感值被打码；
  - 隐藏参数的两种 `-p` 写法；
  - 现有 `request_test.go` 中 Curl 的用例只改调用签名，期望输出不变。
- 完成标准：用例通过；`request` 只导入 `model`；`GOOS=windows go vet ./internal/request/` 通过。
- 依赖：T01（`Param.Hidden`）。

### T06 multipart 接线：-F 选项、隐藏参数选项、skill 提示

- 文件：`internal/commands/globalopts.go`、`internal/commands/dynamic.go`、`internal/commands/siterun.go`、`internal/skillgen/skillgen.go` 及测试（`internal/commands/multipart_test.go`，新；`internal/skillgen/skillgen_test.go`）。
- 动作：
  - `siteOpts` 加 `Forms []string`，绑定 `-F`（`Multipart field name=value or name=@file[;type=..][;filename=..] (repeatable)`）；`reservedOptNames` 加 `F`。
  - `siterun.go`：`in.Forms = so.Forms`。
  - `dynamic.go`：`rawCall` 判断“没有路径却给了请求参数”时算上 `-F`；`paramOptions` 跳过 `Hidden` 参数。
  - `skillgen.go`：How to call 加一条：`-F name=@path` 和 `-d @file` 会读取并发送本地文件，只用于用户明确给出的路径；`-f` 的值永远是文本。操作列表中 multipart 操作的行尾加 ` (multipart: use -F name=@path)`；隐藏参数不列入 required。
- 验证：`go test ./internal/commands/ -run 'TestMultipart' -count=1` 和 `go test ./internal/skillgen/ -count=1`，经 `commands.Run` 覆盖 B2 中除 real-env 之外的各项（操作用简单定义的 `body: {type: multipart}` 和原始路径调用）；skill 的新文字出现。
- 完成标准：用例通过；`--in-completion` 下 `-F` 出现在选项候选中。
- 依赖：T04。

### T05 overlay 接线：loadSiteIndex、overlay init、desc 标注、config check、保留名（commands 侧）

- 文件：`internal/commands/site.go`、`internal/commands/overlay.go`（新）、`internal/commands/run.go`、`internal/commands/ops.go`、`internal/commands/skill.go`、`internal/commands/config.go`、`internal/commands/app.go`、`internal/config/overlay.go`（只加模板与写入函数）及测试（`internal/commands/overlay_test.go`，新；`internal/commands/completion_test.go` 中 `managementNames` 加 `overlay`）。
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
    - 表格：方法与路径之后，若有 `name`/`summary`/`output` 改动，加一行 `OVERLAY: name, output`（按出现顺序）；被改动或新增的参数，NAME 列写成 `<name> (overlay)`；隐藏参数写成 `<name> (hidden)`；`BODY <type> (overlay)`。
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
      键用 `json.Marshal(METHOD + " " + path)`（JSON 字符串也是合法的 YAML 双引号标量）；同一 `METHOD /path` 有多个操作时（简单定义），改用操作名作键。注释经单行净化：控制字符和 U+2028/U+2029 替换为空格，按字符（rune）截断到 120。
    - **自检**（`commands/overlay.go`）：生成后严格解码，再对原始索引 `ApplyOverlay`，要求条目数等于操作数、0 改动、0 警告；不符时退出 1，不写文件。
    - `config.WriteOverlay(dir, site string, data []byte) (path string, err error)`：`overlays` 已存在时 `Lstat` 必须是目录（符号链接、Windows junction 都退出 1）；不存在时 `Mkdir(0700)`；`OpenFile(O_WRONLY|O_CREATE|O_EXCL, 0600)`；已存在时退出 1 `overlay file <path> already exists`。成功后 stdout 输出路径。
    - `--stdout` 只输出模板（同样经过自检），不写文件。
  - **app.go**：注册 `overlayCommand()`。
- 验证：`go test ./internal/commands/ -run 'TestOverlay|TestCompletion' -count=1`，经 `commands.Run` 覆盖 B1 的每一项（补全一项用 `--in-completion`，断言 stdout 不含隐藏操作、stderr 为空）。
- 完成标准：用例通过；grep 确认 `run.go`、`ops.go`、`skill.go` 已改用 `loadSiteIndex`，补全分支用 `loadSiteIndexCached`；`site.go` 中 `addSite`、`refreshSite` 仍用 `loadIndex`；分层测试通过。
- 依赖：T01、T02、T03、T06。

### T07 验收、README、发布说明草稿

- 文件：
  - 本计划：只改“进度记录”。
  - `README.md`、`README.zh-CN.md`：
    - overlay：格式、键的写法、hide 不是访问控制（隐藏参数仍可经 `-H` 或原始路径调用带上）、`overlay init`、`config check`；
    - multipart：`-F` 写法、`-f` 永远是文本、64 MiB 上限、不支持 `-F k=@-` 和 `-F k=<file`（以 `<` 开头的值按文本发送）、type 只接受 `type/subtype`、名字与 filename 不能含 `"` 和 `\`；
    - 安全提示：`-d @file` 在写保护检查之前读取且没有大小上限，与 `-F` 不同；Windows 上 UNC 路径在打开时就会发起 SMB 认证。
  - `docs/release-notes/v0.3.0.md`：
    - 新功能：overlay、`overlay init`、`-F`；
    - 兼容性变化：保留名 `overlay`；multipart 接口改为 multipart 编码（缓存本地自动重建，不联网）；multipart 操作单独 `-d` 需带 Content-Type；`-F` 不能与 `-d`、`-f`、`Content-Type` 头组合；简单定义的 `body.type` 改为枚举校验（写 `multipart` 的现在真正按 multipart 发送；写其他非法值的整个站点退出 1）；`config check` 在 overlay 有结构错误时退出 1；
    - 回滚：退回 v0.2.0 后执行 `apic site refresh <site>`（重建时已清空 ETag，会整份重新拉取），否则 TTL 内 multipart 操作仍不可用。
  - `<WP>/tmp/apic-p2b-accept/`：验收脚本、基线二进制和输出，不进仓库。
- 动作：
  1. **B1、B2**：`go test ./internal/commands/ ./internal/request/ ./internal/spec/ ./internal/config/ -run 'Overlay|Multipart|ParsePart|Assemble|Hidden|IndexVersion' -count=1`。
  2. **B2 real-env**：脚本 `curl-replay.sh` 启动一个记录 multipart 部件的小服务（tmp 下的 `main.go` 构建后运行），先用 apic 直接发送，再执行 apic `--dry-run` 输出的 curl 命令，比对两次收到的部件（名字、值、filename、type、内容 sha256）。
  3. **B3 real-env**：用一个含 multipart 操作的测试 spec，由本地服务提供；基线二进制 `site add` 建缓存；停掉服务；新二进制 `desc` 显示 `BODY multipart`；比较重建前后 meta 的 `fetched_at`。
  4. **B4**：
     - `go vet ./... && GOOS=windows go vet ./... && go test ./... -count=1 && go test ./internal/ -run TestLayering`；检查文件行数；
     - 基线与新二进制分别在 `compat/base`、`compat/new` 下跑 `compat.txt`（P3），比对输出；
     - 主机 job：`git archive HEAD` 导出到 `<WP>/tmp/apic-p2b-accept/src`，提交 `gofer job run -p my-tools-dev -a exec --runner local --sync --cwd tmp/apic-p2b-accept/src -- go test ./internal/request/ ./internal/commands/ ./internal/spec/ ./internal/config/ -count=1`。
- 验证：各项的命令和输出摘要写进“进度记录”，完整输出放在 `<WP>/tmp/apic-p2b-accept/`。
- 完成标准：B1–B4 全部通过，或明确标注 UNVERIFIED 并说明原因（P4、P5）。
- 依赖：T01–T06。

## 偏离登记

| 偏离 | 规则或设计条款 | 理由 |
|---|---|---|
| 隐藏后仍为必填的参数是结构错误 | 设计只写了“path 参数不能隐藏” | 与 D13 同理：必填参数隐藏后无法传入（`-p` 被拒绝），操作就调不通了 |
| `-p in.name` 写法命中隐藏参数同样拒绝 | 设计“隐藏参数用 `-p` 传入时退出 2” | 事实 5：带位置的写法会绕过按名字查找；不拒绝的话隐藏承诺不成立 |
| `overlay init` 的键用 JSON 字符串编码，生成后严格解码并 `ApplyOverlay` 自检，而不是由 YAML 编码器整体生成 | 设计“用 YAML 编码器生成文件内容” | 模板需要注释，编码器不便输出；自检保证 spec 文本无法注入条目，达到设计意图（安全评审确认不削弱保证） |
| `config check` 在 overlay 有结构错误时退出 1 | 设计未规定退出码 | 结构错误会让站点不可用，脚本需要能检出；写进发布说明 |
| `overlay init` 的注释净化在 `config` 内另写，不复用 `skillgen.PlainText` | 设计 S3 处理“单行净化” | 分层规则禁止领域包互相导入；规则与 `PlainText` 一致 |
| 先 `Stat` 路径，Unix 上用 `O_NONBLOCK` 打开，再对句柄 `Stat` 并 `SameFile` 比对 | 设计 §2“用 `os.Open` 打开，再对打开后的文件 Stat”及“排除 `/proc` 文件” | 事实纠正：评审探针证实 FIFO 上 `os.Open` 会阻塞；`/proc` 伪文件 Stat 为普通文件，只能靠 64 MiB 上限 |
| type 只接受不带参数的 `type/subtype` | 设计 §2“type 必须能用 `mime.ParseMediaType` 解析” | 安全评审 Q1 探针：带参数的 type 在 curl 回放时可以借 `,` 读出另一个本地文件，违反 D12 与“只有显式 `@path` 才读文件” |
| 名字与 filename 不能含 `"`、`\`；路径不能含控制字符；路径 `-` 拒绝 | 设计 §2 只规定了控制字符 | 探针：curl 对 `"` 做百分号编码、不转义 `\`，与 apic 直发不一致，违反 D12；`@-` 在 curl 中读 stdin，与非目标一致地拒绝 |
| `-f k:=json` 的非字符串值按原文（去首尾空白）发送，不压缩 | 设计 §2“其他 JSON 值以紧凑的 JSON 文本发送” | 事实纠正：v0.2.0 的 form 分支只去空白和字符串引号（`body.go:56-64`），设计要求与 form 一致 |
| 本地重建时清空 ETag | 设计 §3 未涉及 | 安全评审 Q6：保留 ETag 时，降级到 v0.2.0 后 `site refresh` 会得到 304 并沿用重建后的索引，回滚说明不成立 |

## 回滚与恢复

- 每个任务各自一个提交，恢复点写进“进度记录”；回滚某个任务用 `git revert <提交>`。
- 不涉及数据迁移。重建过的缓存旧版也能读取（多出的 meta 字段被忽略），但 multipart 操作在 v0.2.0 上不可用，需要 `site refresh`（见 T07 发布说明）。测试只用 `t.TempDir()`，不读写真实的 `~/.config/apic`。
- 实施中遇到 Semantic Amendment（例如 curl 回放显示渲染规则仍不可行），停在当前任务，回到 plan review，并请求人工批准。

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
| 安全：文件外发、部件头注入、overlay 信任边界、header 黑名单 | T02、T04、T05、T06 | 对应测试；B1、B2 |
| 兼容性与回滚 | T03、T07 | B4；发布说明 |
| 工程规范 GR104、GR107–GR110、SR2002–SR2004 | 全部任务 | 分层测试；B4 |

## 评审处理（第 1 轮）

| 评审项 | 处理 |
|---|---|
| P1 / Q2 FIFO 上 open 阻塞 | T04 `openRegular`：先 Stat、Unix 非阻塞打开、句柄 Stat 加 SameFile；偏离登记 |
| P2 / Q4 部件头转义与 curl 不一致 | 名字与 filename 禁止 `"`、`\`；断言原始头字节；real-env 用例显式给 filename；偏离登记 |
| Q1 type 参数让 curl 读另一个文件 | type 只接受 `type/subtype`；反例进 T04 测试；偏离登记 |
| P3 `k:=json` 不该压缩 | 与 form 分支共用转换；偏离登记（事实纠正） |
| P4 / Q3 `-f` 字段名注入 | `TextPart` 返回错误，与 `ParsePart` 共用校验 |
| P5 重建失败后 304 打断拉取 | 视为无缓存时清空 ETag 与 FetchedAt |
| P6 304 断言矛盾 | B3 改写：TTL 内与 CacheOnly 计数 0，过期计数 1 |
| P7 现有测试期望 | T03 改 `openapi_test.go` 一处；T05 改 `completion_test.go` |
| P8 T05 与 T06 并行 | 改为 T06 → T05，执行方式与波次表一致 |
| P9 Unix 测试 build tag | `//go:build unix`；T04 加 `GOOS=windows go vet` |
| P10 yaml 标签与 null | T01 写明标签与 null 处理，`file:` 键拒绝 |
| P11 header 黑名单误伤 | 移到 T02 的新增分支 |
| P12 / Q5 `@-` | 退出 2 |
| P13 compat 输入与改写 | P3 复制 `c1` 配置到本批目录，并 grep 确认没有 multipart |
| Q6 回滚不成立 | 重建时清空 ETag；发布说明写明降级后 `site refresh`；偏离登记 |
| Q7 发布说明缺项 | T07 补全兼容性变化与回滚步骤 |
| Q8 路径与 filename 未打码 | curl 与 `-v` 都打码 |
| 建议 S1 / Q11 desc 列出隐藏参数 | desc 标 `(hidden)`；README 写明隐藏参数仍可经 `-H` 带上 |
| 建议 S2 dry-run 不查大小 | `CheckFiles` 检查大小之和 |
| 建议 S3 多行解码错误 | 只取第一行 |
| 建议 S4 改名依赖顺序 | 整体预留；测试顺序无关 |
| 建议 S5 无请求体的提示 | 单独的消息 |
| 建议 S6 / Q10 截断与自检 | 按 rune 截断；自检加 `ApplyOverlay` 0 警告 |
| 建议 S10 路径含换行 | 路径禁止控制字符 |
| 建议 Q9 junction | `Lstat` 必须是目录 |
| 建议 Q12 `-p header.Content-Type` | 合并后检查任何来源的 Content-Type |
| 建议 Q13 补全模式写缓存 | 写回失败仍用重建结果，作为警告返回 |
| 建议 Q14 `-d @file` | 登记 deferred，README 写明差异与 UNC 提示 |
| 建议 S7、S8、S9 | `/proc` 措辞进偏离登记；发布说明补 config check；测试辅助函数加前缀 |

## 完成 Gate 与剩余工作

- 完成条件：B1–B4 全部满足，或者用户接受其中标注为 UNVERIFIED 的项。
- 完成时在“进度记录”里写一段不超过 8 行的总结。
- 剩余工作：发布 v0.3.0（外部动作 Gate）；`-d @file` 的读取时机与上限（deferred）；二期其余 deferred 项；gcli issue `my-tools-74ur`。

## 进度记录

- **2026-11-11 开工**：用户同句批准计划并发出执行请求“批准 apic 二期第二批计划 0.2（接受未复核修订）；开始实施”。
- **gofer plan**：`plan-20261111-213647-00284801`。
- **前置检查**：P1 工作树干净，HEAD `b680fcd`；P2 go1.26.0；P3 基线由 `git archive v0.2.0` 构建，`c1/{cfg,lincfg}` 复制到 `compat/{base,new}`，`.raw` 中没有 multipart；P4 curl 8.5.0 支持 `--form-string`；P5 主机 worker 在线。
- **执行方式**：
  - T01、T03 由 suga（gpt-6-sol）在主机 worktree 中完成，审查和反向验证后在主机上合并；T03 的 worktree 目录清理时报 Permission denied，留待主机清理。
  - T02、T04、T06、T05 由 sonnet 子 agent 在主 checkout 中完成，我审查后接受。
  - T05 在 T03 合并前开工：两者没有共同文件，overlay 的 `body: multipart` 不依赖 T03，属于 Operational Discovery。
- **提交**：
  - T01 `00bdf7d`（合并 `14fc178`）。反向验证：去掉 `yaml.Strict()` 后 `TestLoadOverlay` 失败。
  - T02 `e134128`。子 agent 的细化：空名字报错；`header.X` 形式的新增参数从键中拆出位置；隐藏的操作不再校验参数，也不占用名字。
  - T04 `2cc5ac1`。反向验证 3 项：阻塞打开、type 正则、隐藏参数。
  - T06 `9f51292`。
  - T05 `a36f4ff`。Corrective `96e6df6`：`config check` 遇到没有缓存的站点只提示，不退出 1，按计划只有结构错误才退出 1。
  - T03 `f6a3ca1`（合并 `9c798bc`）。反向验证：禁用重建后 `TestIndexVersion` 失败。
  - T07：Corrective `e52d05c`。Windows 主机测试发现 multipart curl 用例的期望值没有转义反斜杠，实现本身正确，只修了测试。另一处 Corrective 在发布说明的回滚步骤：新版正常拉取后会重新保存 ETag，只执行 `site refresh` 可能收到 304，所以改为先删除缓存再 refresh。
- **验收**：完整输出在 `<WP>/tmp/apic-p2b-accept/`。
  - **B1、B2 单测**：`go test ./...` 全部通过，overlay、multipart 相关用例都经 `commands.Run` 真实分发。
  - **B2 real-env**：`b2/curl-replay.sh` 发送 4 个部件，包括含 `@`、`;type=` 的文本值和含 `;`、`,`、`"` 的路径。apic 直接发送与执行 dry-run 的 curl，服务端记录的名字、filename、type 和 sha256 完全一致，结果为 `REPLAY_MATCH`。
  - **B3 real-env**：`b3/b3.sh`。v0.2.0 建的缓存是 `BODY form`；停掉 spec 服务后，新版显示 `BODY multipart`。`fetched_at` 不变，`etag` 为空，`index_version` 为 2。
  - **B4**：
    - `go vet ./...` 和 `GOOS=windows go vet ./...` 通过，`go test ./...` 和 TestLayering 通过，最大源文件 582 行。
    - `compat.sh` 的 17 条命令，把两份配置的目录路径归一后，v0.2.0 与新版输出逐字节一致。
    - Windows 主机 job `job-20261111-221515-443ff265`：request、commands、spec、config 四个包通过。
  - **真实二进制冒烟**：`overlay init` 生成的文件权限 0600、目录 0700，文件已存在时退出 1；改名、新增 header 参数、补全、`desc` 的 `(overlay)` 标注、`config check` 都符合预期。
- **文档**：README 中英文版补了 overlay、文件上传两节和安全提示；发布说明草稿在 `docs/release-notes/v0.3.0.md`。

**完成总结（COMPLETE）**：
- 主旨：二期第二批三项已交付：overlay、multipart 上传、索引格式版本与本地重建。
- 最终状态：B1–B4 全部通过，没有 UNVERIFIED 项。
- 自主决策：见上文的 3 处 Corrective（`config check` 退出码、Windows 测试期望、回滚步骤），以及 T05 提前开工。
- 提交：`00bdf7d..` 加上本记录所在的提交，共 12 个，含 2 个合并提交。
- 耗时：约 45 分钟，从 21:37 到约 22:20（+08:00）。
- 剩余：发布 v0.3.0，需要外部动作批准；清理 T03 worktree 的残留目录；`-d @file` 的读取时机与大小上限（deferred）；gcli issue `my-tools-74ur`。
