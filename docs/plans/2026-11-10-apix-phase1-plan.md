<!-- template_id: plan; template_version: 1.3.0 -->
# apix 一期（MVP）实施计划

> 状态：Draft 0.2 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-10 | Claude（inhere：“批准 apix 设计 0.2（接受未复核修订），Q1/Q2 按推荐，开始写一期计划”） | 初稿：4 个波次、10 个任务，覆盖设计 0.2“分期路线·一期”的 8 项 |
| 0.2 | 2026-11-10 | Claude | 处理合并轴评审（B1–B2 阻塞，C1–C11 纠正，S1–S4 建议）：统一入口 `commands.Run(info, args)`；A4 主机步骤固定为只写 `<WP>/tmp`；领域包只依赖 model（打码器、密钥登记表放进 model，request 自己声明 Authorizer 接口）；编排归属 `request.Execute`；`-H` 不重复注册；`--api` 后跟未配置站点时退出 2；dry-run 不触发登录；base_path 来源与覆盖；补齐验证项；TLS 并入一期；新增“偏离登记” |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 规划工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：交付 apix 一期，即设计 0.2“分期路线·一期”的第 1–8 项，并通过一期验收。
- Scope freeze：只做一期 8 项；二、三期的内容（补全、oauth2、exec、overlay、`-q`、`yaml/table`、multipart、分页、skill、MCP）一律不做，实施中发现需要也只记为 deferred。
- 一期还包括设计“安全”一节中的 TLS `insecure` 与 `ca_file`（放在 T06）。
- 评审预算：按 SR1409 属低暴露（新仓库，未发布、无用户），合并轴评审封顶一轮。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。

## 目标与完成定义

**目标**：用户能用一个 `apix` 二进制注册 Swagger 2.0/OAS3 站点和简单定义站点，按操作或按原始路径发请求，切换环境与认证，并生成快捷命令。行为符合设计 0.2 的命令形态、退出码契约、写保护和打码规则。

**完成定义（一期验收）**：

1. **A1 真实 spec**：用户提供的至少一份真实内部 Swagger 2.0 spec 能用 `site add` 注册，`ops` 和 `desc` 输出正确，`base_path` 拼接正确。如果用户有 OAS3 spec，也照此验证。
2. **A2 简单定义**：Jenkins 简单定义（`defs/jenkins.yaml`）能注册，并能按操作调用。
3. **A3 本地服务**：在 httptest 本地服务上，经真实命令分发覆盖以下行为：
   - 按操作调用和原始路径调用；
   - `-d`、`-f`、`-p`、`-H`；
   - 五种认证，包括 login 的缓存、401 重登、按 env 隔离；
   - 写保护；
   - 退出码 0–6 每一个；
   - `--dry-run` 和打码；
   - 不跟随重定向，凭证不出主机。
4. **A4 快捷命令**：
   - 在 Linux 容器里生成 sh 版快捷命令并成功调用；
   - 在 Windows 主机上经 gofer job 生成 `.cmd` 版并成功调用；
   - 覆盖与删除的保护生效。
5. **A5 工程**：
   - `go vet ./...` 和 `go test ./...` 通过；
   - 分层守卫测试通过；
   - 单个文件不超过 1000 行（GR104）。

## 范围、排除项与授权

**范围**：仓库 `inhere-tools/apix`（Git root）下的全部代码与文档。

**排除项**：

- 二、三期功能。
- 修改 gookit 各库：发现 gcli 有问题时，记到已登记的 issue `my-tools-kb4d`，不在本计划内改。
- 推送、建远端仓库（设计 Q3）、发布。

**授权**：

- 计划批准后，还需要用户另行发出当前执行请求，才能开始实施。
- 按功能点本地提交是默认授权；push 不在本计划授权内。
- A1 要读取真实 spec，A4 要在主机上执行，这些是本计划仅有的机器外或主机动作：
  - 都是只读的，或只写 `<WP>/tmp/`；
  - 不修改任何业务服务；
  - 不需要真实凭证。如果 A1 的 spec 地址需要认证，由用户把 spec 文件放到 `<WP>/tmp/apix-specs/` 下，计划不处理真实密钥。

- `host_or_non_offline_action=REQUIRED`

## 输入与批准证据

- 设计：`docs/design/apix-design.md` Draft 0.2，已批准。
  - 批准原话（2026-11-10）：“批准 apix 设计 0.2（接受未复核修订），Q1/Q2 按推荐，开始写一期计划”。
  - 记录批准的提交：`2cc8022`。
- 设计第 1 轮评审发现 A1–A2、B1–B9、S1–S5，已全部并入 0.2。本计划按 0.2 的条款执行，不重述理由。
- 跟踪 issue：`my-tools-thv4`；gcli 退出码问题：`my-tools-kb4d`。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | CLI 框架、子命令、带码退出 | gookit/gcli v3（gofer 用 v3.8.6）；`app.go:569` 识别 `errorx.ErrorCoder` | 是 | 无 | DIRECT_REUSE | none | 无 |
| CAP-02 | HTTP 客户端 | gookit/greq；net/http | 是 | 用 greq 发请求时显式关闭自动重定向（`CheckRedirect` 返回 `http.ErrUseLastResponse`） | DIRECT_REUSE | none | 无 |
| CAP-03 | OpenAPI 3.x 解析与 2.0→3.0 转换 | getkin/kin-openapi（`openapi3`、`openapi2conv`）；pb33f/libopenapi | 是（第三方依赖） | 无 | DIRECT_REUSE | none | 新增一个第三方依赖，只在 `internal/spec` 内引用；P4 用真实 spec 试解析后锁定（设计 Q4） |
| CAP-04 | YAML 读写 | goccy/go-yaml（gofer 已用） | 是 | 无 | DIRECT_REUSE | none | 无 |
| CAP-05 | `.env` 解析（不写进程环境、键名区分大小写） | goutil/envutil `Dotenv` | 否：解析入口只有 `parseAndSetEnv`，会 Setenv 并转大写（`dotenv.go:127-143`） | 在 `internal/config` 内写一个最小解析函数（支持 `KEY=VALUE`、`#` 注释、引号），约 40 行 | OWNER_EXTENSION | envutil 没有不写进程环境的公开解析入口 | 只在 `internal/config` 里，不另成包 |
| CAP-06 | 按路径从 JSON 取值（login 取 token） | goutil/maputil `GetFromAny` | 是 | 无 | DIRECT_REUSE | none | 无 |
| CAP-07 | 测试断言 | goutil/x/assert（SR2002） | 是 | 无 | DIRECT_REUSE | none | 无 |
| CAP-08 | 分层守卫测试 | xhen 的 `internal/layering_test.go`（`go list` 导入图对照表） | 照搬写法 | 换成 apix 的分层表 | DIRECT_REUSE | none | 无 |
| CAP-09 | 按 API 定义动态生成命令的通用 API CLI 本体 | Restish、invgate-cli、openapi-to-cli、climate、Speakeasy/Stainless（设计“背景与目标”） | 否 | 外部项目，无法扩展 | MINIMAL_NEW_MODULE | 现有工具缺 Swagger 2.0、login 认证、简单定义、agent 约束、Windows 快捷命令（设计 0.2 已批准） | 新仓库，唯一 owner 为 `inhere-tools/apix` |

### New module candidates

| candidate_id | capability_id | proposed_module | searched_candidates | direct_reuse_gap | thin_adapter_or_owner_extension_gap | proven_gap | unique_owner_and_lifecycle | deletion_or_merge_handling |
|---|---|---|---|---|---|---|---|---|
| NM-01 | CAP-09 | 新仓库与新 CLI `github.com/inhere/apix`（设计 D1） | Restish、invgate-cli、openapi-to-cli、climate（设计“背景与目标”） | Restish 缺 Swagger 2.0、login 认证、简单定义、agent 约束、Windows 快捷命令 | 外部项目，不能由我们扩展 | 设计 0.2 已论证并获批准 | 仓库 `inhere-tools/apix`，由 inhere 维护 | 一期验收失败且决定放弃时，删除整个仓库即可，不影响其他项目 |

### Rejected new tools

| rejected_candidate | capability_id | deletion_test_and_reason |
|---|---|---|
| 自写 OpenAPI 解析器 | CAP-03 | 删掉它也不缺能力，kin-openapi 已覆盖 |
| 通用 jsonpath 库 | CAP-06 | login 只需要点号路径，`maputil.GetFromAny` 已足够 |

## 前置检查与 fail-closed 条件

运行目录均为 `inhere-tools/apix`。

1. **P1 工作树**：`git status --short` 为空，HEAD 至少包含 `2cc8022`。否则停下，查清归属。
2. **P2 Go 版本**：容器内 `go version` ≥ 1.25（当前为 1.25.10）；`go.mod` 写 `go 1.25`。
3. **P3 真实 spec（A1 与 T04 都依赖它）**：用户提供至少一份 Swagger 2.0 spec 的 URL，或者把文件放到工作区 `<WP>/tmp/apix-specs/`。
   - 没有提供时，T04 可以用公开的 petstore v2/v3 样例先完成。
   - 但 A1 保持未完成，完成 Gate 不得报完成。
4. **P4 解析库锁定（设计 Q4）**：T04 开工前，用一个 20 行左右的探针程序（放 `<WP>/tmp/apix-probe/`），在 P3 的 spec 和 petstore v2 上调用 kin-openapi 的加载与 `openapi2conv.ToV3`。
   - 能得到全部操作、`basePath` 和参数，就锁定 kin-openapi。
   - 失败时改用 libopenapi 再试一次。仍失败就停下，按 Semantic Amendment 报回。
5. **P5 主机可用**：A4 要用到主机。gofer server 在线，本项目 worker 在线（`gofer job` 能提交）。不在线时 A4 标 UNVERIFIED_REAL_ENV，并停在完成 Gate。

## 波次与依赖

| 波次 | 任务 | 依赖 |
|---|---|---|
| W1 骨架与基础 | T01 → T02、T03 | T02、T03 都依赖 T01，可以并行 |
| W2 定义与站点 | T04 → T05 | T04 依赖 T03 和 P4；T05 依赖 T02 和 T04 |
| W3 请求链路 | T06 → T07 → T08 | T06 依赖 T02 和 T03；T07 依赖 T02 和 T06；T08 依赖 T05 和 T07 |
| W4 快捷命令与验收 | T09 → T10 | T09 依赖 T08；T10 依赖全部任务 |

执行方式：按工作区约定，由 sonnet 子 agent 按任务实施，我审查 diff、核对接线并验收。每个任务完成后做一次原子提交（`feat(<pkg>): ...`），提交信息带任务号。

## 任务

所有命令都在 `inhere-tools/apix`（Git root）下运行。文中 `<WP>/tmp/` 指工作区根目录的 `tmp/`，不是仓库内的 tmp。

### T01 仓库骨架、入口、带码错误与分层守卫

- 文件：
  - `go.mod`：module `github.com/inhere/apix`。
  - `cmd/apix/main.go`。
  - `internal/commands/{run.go,app.go}`。
  - `internal/model/errors.go`。
  - `internal/layering_test.go`。
  - `.gitignore`：含 `/tmp/`。
  - `README.md`：只写一段简介。
- 动作：
  - **入口**：`main.go` 只注入 Version/GitCommit/BuildDate，然后执行 `os.Exit(commands.Run(info, nil))`。
    - `commands.Run(info BuildInfo, args []string) int` 是唯一入口。
    - args 为 nil 时取 `os.Args[1:]`；依次做 Prescan（T08 实现，T01 先返回原样参数）、`newApp` 注册静态命令、为命中的站点注册动态命令、`app.Run(rewritten)`。
    - 所有 CLI 测试都调用 `commands.Run(info, args)`，满足 GR110。
    - 入口签名偏离 SR2003 的 `os.Exit(app.Run(nil))`，原因是要在 gcli 分发前做预扫描和动态注册。偏离已登记在“偏离登记”。
  - **带码错误**：

    ```go
    const (ExitOK=0; ExitLocal=1; ExitUsage=2; ExitNetwork=3; ExitHTTP4xx=4; ExitHTTP5xx=5; ExitProtected=6)
    type CodedError struct{ C int; Err error }   // 指针接收者：Code() int、Error()、Unwrap()
    var _ errorx.ErrorCoder = (*CodedError)(nil)
    func Errorf(code int, format string, a ...any) error
    ```

    gcli 会原样打印 err，所以错误消息在构造时必须已经打码（见 T02 的 `Redactor`）。
  - **分层表**：照搬 xhen 写法，按“偏离登记”中的边逐条列出，未登记的包或边让测试失败。
- 验证：`go build ./... && go test ./internal/ -run TestLayering`
- 完成标准：
  - `go run ./cmd/apix --version` 打印版本，退出 0。
  - 分层测试通过。在 `model` 中临时加入对 `config` 的导入时测试失败（手工验证，不提交）。
- 依赖：P1、P2。

### T02 配置、密钥登记与打码器

- 文件：
  - `internal/config/{dir.go,load.go,dotenv.go,expand.go,state.go,check.go}` 及测试。
  - `internal/model/{config.go,secrets.go}` 及测试。
- 动作：
  - **配置目录**：`config.Dir()` 先取 `$APIX_CONFIG_DIR`，否则用 `~/.config/apix`。
  - **加载**：`config.Load(dir) (*model.Config, error)` 读 `config.yaml` 和 `sites/*.yaml` 并合并。
    - `version` 缺省按 1 处理，其他值报错。
    - 校验站点名格式 `^[a-z][a-z0-9-]*$`。
    - 校验保留名：`site ops desc use auth link config help version`。
    - 同一站点不得同时出现在 config.yaml 与 sites/ 中。
    - 简单定义文件路径相对配置目录解析。
    - 校验失败返回 `ExitLocal`，错误信息带文件名与站点名。
  - **`.env`**：`ParseDotenv(text) map[string]string` 按 CAP-05 自写，键名区分大小写。
    - `Lookup(name)` 先查 OS 环境变量，再查 `.env`（D14）。
    - 不读当前目录的 `.env`（D13）。
  - **敏感值登记与打码**：`model.Secrets` 是线程安全的敏感值登记表。
    - `model.Redactor` 内含 `Secrets` 与固定敏感头 `Authorization`、`Proxy-Authorization`、`Cookie`、`Set-Cookie`。
    - 提供 `Redact(string) string` 与 `RedactHeaders(http.Header) http.Header`。
    - 打码器放在 model，config、auth、request、output 都只依赖 model。
  - **展开**：`config.Expand(s string, sec *model.Secrets) (string, error)` 展开 `${env:NAME}`，展开出的值登记进 `Secrets`。
    - 变量缺失返回 `ExitLocal`。
    - 只在用到时展开当前选中的 env 和 auth。
    - `config.ResolveAuth(site, authName, sec) (model.AuthSpec, error)` 返回已展开的认证配置，供 wiring 交给 auth。
  - **state**：`State{Selections, Links map[name]{Path,Hash,Site}, ManagedSites}`，`SaveState` 写临时文件再 rename，权限 0600。
  - **检查**：`Check(cfg)` 对 token、password、secret、key 类字段的明文值给出警告。
- 验证：`go test ./internal/config/ ./internal/model/`
  - 重名站点、保留名、缺失变量。
  - OS 环境变量优先，键名大小写敏感。
  - 原子写入后的权限（非 Windows 才断言）。
  - `Redact` 能替换登记值与 URL query 中的登记值。
- 完成标准：
  - 上述用例全部通过。
  - 设计 0.2 的 config.yaml 示例（testdata，不含 `version`）解析无误。
- 依赖：T01。

### T03 操作模型与简单定义加载

- 文件：`internal/model/{site.go,operation.go}`、`internal/spec/{loader.go,simple.go,naming.go}` 及测试
- 动作：
  - **模型**：
    - `Operation{Name, Method, Path, Summary, Tags, Params []Param, Body *BodySpec, Output string, Responses map[string]SchemaSummary}`。
    - `Param{Name, In(path|query|header), Required, Type, Enum, Desc}`。
    - `BodySpec{Type(json|form|xml|raw), Required, Schema SchemaSummary}`。
    - `Index{Ops []Operation, DefaultBasePath, DefaultBaseURL string}`。
  - **加载接口**：与设计一致，`Load` 返回 `*model.Index`。
  - **简单定义**：`SimpleLoader` 支持简写（`get: /x`）与完整写法（`params`、`body`、`query: [..]`、`output`）；路径中的 `{x}` 自动成为必填 path 参数。
  - **命名**：`naming.go` 把 operationId 转为 kebab-case；没有 operationId 时由方法加路径生成；重名时追加 `-2`、`-3`，并返回告警。
- 验证：`go test ./internal/spec/ -run 'TestSimple|TestNaming'`，用设计 0.2 的 `defs/jenkins.yaml` 示例作为 testdata。
- 完成标准：
  - Jenkins 示例解析出 4 个操作，参数、必填与 body 类型正确。
  - 命名在三种情况下都符合预期：`listUsers`、没有 operationId、重名。
- 依赖：T01。

### T04 OpenAPI 加载、base_path 与操作索引缓存

- 文件：`internal/spec/{openapi.go,cache.go,fetch.go}` 及测试、`internal/spec/testdata/`
- 动作：
  - **OpenAPI 解析**：`OpenAPILoader` 使用 P4 锁定的库；2.0 先转换为 3.0。
    - 产出 `Index`。
    - `DefaultBasePath`：2.0 取 `basePath`，3.x 取 `servers[0]` 的路径部分。
    - `DefaultBaseURL`：2.0 取 host 加 schemes，3.x 取 `servers[0]`。
    - `Responses` 只保留顶层字段摘要。
  - **拉取**：`fetch.go` 支持 URL 或文件，支持 ETag，使用 env 的 TLS 配置。
    - 拉取认证（`source.auth`）只允许 bearer、basic、apikey，遇到 login 返回 `ExitLocal`。
  - **缓存**：`cache.go` 管理 `cache/specs/<site>.{raw,index.json,meta.json}`，`index.json` 含 `DefaultBasePath` 与 `DefaultBaseURL`。
    - `Index(site, src, opts) (*model.Index, warn []string, error)`。
    - 未过期时读缓存。
    - 过期时先刷新，刷新失败就用旧缓存并给出告警。
    - 没有任何缓存时返回 `ExitLocal`，并提示运行 `site refresh`。
- 验证：`go test ./internal/spec/ -run 'TestOpenAPI|TestCache'`
  - 手写一对等价的最小 spec：2.0 含 `basePath: /api/v1`，3.0 含对应的 `servers`。
  - petstore v2/v3 公开样例只做冒烟测试（操作数大于 0、不报错）。
  - P3 的真实 spec 不进仓库，留到 T10 验证。
- 完成标准：
  - 等价的 2.0 和 3.0 最小 spec 产出相同的 `Index`，包括 `DefaultBasePath`。
  - 过期且刷新失败时使用旧缓存并给出告警。
- 依赖：T03、P4。

### T05 管理命令：site、ops、desc、config

- 文件：`internal/commands/{site.go,ops.go,config.go}`、`internal/wiring/wiring.go`、`internal/output/format.go` 及各自测试
- 动作：
  - **`site add <name> --spec <url|file> | --def <file>`**：
    - 可选 `--base-url`、`--auth-for-spec`。
    - 站点名已在 config.yaml 或 sites/ 中存在时报错，退出 1。
    - 写入 `sites/<name>.yaml`，并登记到 `ManagedSites`。
    - 没有给 `--base-url` 时，用 `DefaultBaseURL` 生成 `default` env；两者都没有时报错。
    - 写完立即建立索引。
  - **`site rm`**：只删除 `ManagedSites` 中的站点，同时清理该站点的缓存（含 token）。写在 config.yaml 里的站点拒绝删除，退出 1，并提示手动编辑。
  - **`site ls/show/refresh`**、**`ops <site> [--tag] [--match]`**、**`desc <site> <op>`**（支持 `-o json`）、**`config path/show/check`**。`site show` 与 `config show` 的输出都经过 `Redactor`。
  - **输出**：`output.Format(w, body []byte, contentType, mode)` 支持 `auto/json/raw`。
  - **wiring**：只负责构造依赖（GR108）。
  - **SR2003**：静态命令照常遵守，位置参数先声明再取值，选项绑定到命令结构体。
- 验证：经真实分发 `commands.Run(info, args)`（GR110）运行 `go test ./internal/commands/ -run 'TestSite|TestOps|TestDesc|TestConfig'`，配置目录用 `t.TempDir()` 并设置 `APIX_CONFIG_DIR`。
- 完成标准：
  - 对等价的最小 spec 与 Jenkins 示例走通 add、ls、ops、desc、rm。
  - 删除 config.yaml 中的站点被拒绝，退出 1。
  - 重复 add 退出 1。
  - `site show` 和 `config show` 中不出现登记的密钥值。
- 依赖：T02、T04。

### T06 请求构造与执行

- 文件：`internal/request/{build.go,body.go,execute.go,curl.go,protect.go,tls.go}` 及测试
- 动作：
  - **构造请求**：`Build(op *model.Operation /*原始模式为 nil*/, in Input, env model.Env, basePath string) (*Prepared, error)`
    - `Input` 包含：
      - 方法，统一转大写；
      - 原始路径；
      - 参数 map；
      - `-p` 列表，支持 `name=v` 和 `query.name=v`、`path.name=v`、`header.name=v`；
      - `-d`、`-f`、`-H`。
    - URL 组成：
      - 操作模式为 `base_url + basePath + path`，其中 `basePath` 按 env.BasePath、Index.DefaultBasePath、空的顺序取第一个非空值，由调用方传入；
      - 原始模式为 `base_url + path`。
    - path 参数值经过 `PathEscape`。
    - 原始路径必须以单个 `/` 开头，否则返回 `ExitUsage`。
    - 必填参数或类型校验失败返回 `ExitLocal`。
  - **请求体**：`-d` 支持 `@file`、`-`（stdin）和字面量，一律先读入缓冲；`-f` 支持 `k=v`、`k:=json`，按点号路径合并为对象，form 类型时编码为表单。
  - **写保护**：`CheckProtect(method, env, authSpec, yes, tty bool) error`。
    - 方法转大写后再判断。
    - `allow_methods` 不允许、或 protect 环境上写操作缺少 `--yes` 时，返回 `ExitProtected`。
    - 只有 stdin 与 stderr 都是终端、且没有设置 `APIX_NO_PROMPT` 时才交互确认。
  - **鉴权接口**：由消费方声明，`type Authorizer interface{ Apply(ctx, req *http.Request, dry bool) error; Refresh(ctx) error }`；`model.ErrNoRefresh` 定义在 model。request 不导入 auth。
  - **执行**：一次请求的编排归属 `Execute(ctx, c Call) (*Response, error)`（C2）。`Call` 包含 `Prepared`、`Authorizer`、`Redactor`、`DryRun`、`Yes`、`TTY`、`Timeout`、`Verbose io.Writer`。依次执行：
    1. `CheckProtect`，dry-run 时跳过；
    2. `Apply`，dry-run 时传 `dry=true`；
    3. dry-run 时只渲染 curl 并返回；
    4. 发送：不跟随重定向，带超时和 TLS 配置；
    5. 遇到 401 时 `Refresh` 并重发一次；
    6. 网络错误或超时返回 `ExitNetwork`，错误消息经过 `Redact`。
  - **TLS**：`tls.go` 按 env 的 `insecure`（每次请求在 stderr 警告）和 `ca_file` 构造 `http.Client`。认证模块的 login 也使用这个 client，由 wiring 注入。
  - **curl**：`curl.go` 先打码再渲染。
- 验证：`go test ./internal/request/`，使用 httptest 与假的 Authorizer，覆盖：
  - `//evil` 与 `http://x` 被拒绝；
  - 302 原样返回；
  - 401 只重试一次，且 stdin 请求体能重放；
  - query 中的 apikey 在网络错误消息里被打码；
  - curl 不含明文；
  - env.BasePath 覆盖索引里的 basePath；
  - `-p query.id=1` 与同名的 path 参数各自生效；
  - 小写的 `post` 也会触发写保护；
  - `allow_methods` 拒绝时返回 6；
  - dry-run 不经过写保护，且服务端收到 0 次请求；
  - `--timeout` 超时返回 3；
  - `insecure` 时 stderr 有警告；
  - `ca_file` 能信任自签证书（httptest TLS）。
- 完成标准：上述用例全部通过。
- 依赖：T02、T03。

### T07 认证：五种类型、token 缓存、auth 与 use 命令

- 文件：`internal/auth/{provider.go,static.go,login.go,tokencache.go}` 及测试、`internal/commands/{auth.go,use.go}` 及测试
- 动作：
  - **构造**：`auth.New(spec model.AuthSpec /*已展开*/, env model.Env, deps Deps) (*Provider, error)`。
    - `Deps{Client *http.Client, Secrets *model.Secrets, Redactor *model.Redactor, CacheDir string, Verbose io.Writer}`。
    - auth 只依赖 model，不导入 config 与 request；`*Provider` 在结构上满足 `request.Authorizer`。
  - **静态认证**（none、bearer、basic、apikey）：apikey 支持放在请求头或 query 中，通过 `in` 和 `name` 指定。注入的值登记为敏感值。`Refresh` 返回 `ErrNoRefresh`。
  - **login**：
    - 用注入的 client 发出登录请求：不跟随重定向，不受写保护限制。
    - 按 `token.from/path` 与 `expires` 取值，按 `apply` 注入请求，token 登记为敏感值。
    - `-v` 输出 login 请求时经过 `Redactor`。
    - 失败时网络错误、4xx、5xx 分别映射为退出码 3、4、5。
    - dry-run 时不发登录请求：有缓存 token 就使用缓存，否则注入占位符 `<login-token>`。
  - **token 缓存**：路径为 `cache/tokens/<site>.<env>.<auth>.json`，内容 `{token, expires_at, fingerprint}`。
    - `fingerprint` 是 base_url 加 login 段原文的 sha256。
    - 指纹不一致时作废缓存。
    - 写入时 0600，原子写入。
  - **`use <site> [--env] [--auth]`**：写入 state。选择优先级：单次选项 > state > `default_*`。
  - **`auth ls/login/logout/status <site>`**：输出经过打码。
- 验证：`go test ./internal/auth/ ./internal/commands/ -run 'TestAuth|TestUse|TestLogin'`，使用 httptest，覆盖：
  - bearer、basic、apikey（header 与 query）各自的注入效果；
  - token 过期后重新登录；
  - 切换 env 后不复用 token；
  - 指纹变化后重新登录；
  - 登录返回 4xx 时退出 4；
  - dry-run 且没有缓存时服务端收到 0 次登录请求；
  - `-v` 输出中不含密码和 token。
- 完成标准：上述用例全部通过。
- 依赖：T02、T06（复用 `Authorizer` 接口形状与 TLS client）。

### T08 动态站点命令、预扫描与退出码接线

- 文件：`internal/commands/{dynamic.go,prescan.go,globalopts.go,siterun.go}` 及测试、`internal/output/exit.go`
- 动作：
  - **预扫描**：`Prescan(args []string, globals GlobalSpec, sites []string) (site string, rewritten []string, err error)`
    - 跳过全局选项及其值。
    - 遇到未知全局选项时返回 `ExitUsage`。
    - 把 `--api X` 或 `--api=X` 改写成位置参数 X；X 不在 `sites` 中时返回 `ExitUsage`（C5）。
    - 位置参数形式的候选名不是站点时，不做改写，交给 gcli。
  - **全局选项**：`--env --auth -o -i -v --dry-run --yes --timeout -H`，同时注册在 `app.Flags()` 和 `<site>` 的 SharedOpts 上，绑定同一变量。
  - **站点命令**：
    - `<site>` 声明可选位置参数 `path`。
    - 站点自有选项只有 `-X -d -f -p`；`-H` 已属于全局选项，不重复注册。
    - path 不以 `/` 开头时报“未知操作 <x>”，返回 `ExitUsage`。
    - 每个操作是 `<site>` 的子命令，不声明位置参数。
    - 参数选项在运行时注册；与全局选项、gcli 内置选项冲突，或同名但位置不同的参数，回退为 `-p`。
  - **执行流程**：`siterun.go` 只做顺序调用，不含业务判断：
    1. 调用 `wiring.NewCall(site, op, selection, flags)` 组装 `request.Call`；wiring 内部做 `config.ResolveAuth`、`auth.New`，并确定 basePath。
    2. 调用 `request.Execute`。
    3. 调用 `output.Render(resp, mode, stdout, stderr, redactor) int`。
  - **输出与退出码**：`output.Render` 把 2xx/3xx 映射为 0、4xx 为 4、5xx 为 5。
    - 响应体写到 stdout。
    - `-i` 输出的状态行和响应头（含 Set-Cookie）经过 `RedactHeaders`。
    - `-v` 输出的请求行和请求头经过打码。
    - 错误一律以 `*model.CodedError` 返回，由 gcli 按 Code 退出。
- 验证：经真实分发运行 `go test ./internal/commands/ -run 'TestDynamic|TestPrescan|TestExitCodes|TestProtect'`，覆盖：
  - 全局选项放在三种位置都生效；
  - `--api` 两种写法；
  - `--api bogus` 退出 2；
  - `--bogus` 退出 2；
  - `pet list-userz` 退出 2 且服务端没有收到请求；
  - 退出码 0–6 每个都有用例；
  - 在非终端下对 prod 发 POST 且没有 `--yes`，退出 6，服务端收到 0 次登录请求；
  - `-i` 输出中 Set-Cookie 被打码。
- 完成标准：
  - 上述用例全部通过。
  - `apix pet -X POST /api/users -f name=tom` 对 httptest 服务调用成功。
- 依赖：T05、T07。

### T09 快捷命令 link

- 文件：`internal/link/{link.go,script.go}`、`internal/commands/link.go` 及测试
- 动作：
  - **`link <site> --name <n> [--env] [--auth] [--dir]`**：
    - site、name、env、auth 都必须匹配 `^[a-z0-9][a-z0-9-]*$`。
    - 拒绝把 `--yes` 写进脚本。
    - 生成的脚本：
      - 非 Windows 生成 sh 版，权限 0755；
      - Windows 同时生成 `.cmd` 和 sh 版。
    - 在 state 中记录路径和 sha256。
    - 目标文件已存在且与记录不一致时拒绝覆盖。
    - 输出中提示：`.cmd` 版传 JSON 时用 `-d @file`。
  - **`link ls`**；**`link rm <name>`**：只有文件哈希与记录一致时才删除。
- 验证：`go test ./internal/link/ ./internal/commands/ -run TestLink`，覆盖：
  - `../` 被拒绝；
  - 覆盖他人文件被拒绝；
  - 文件被改动后 rm 被拒绝；
  - 透传（仅 Linux）：在测试 PATH 最前面放一个桩脚本 `apix`（把 argv 逐行写入文件），执行生成的 sh 脚本加 `list-users --x "a b"`，断言预带选项与 `"$@"` 都原样透传。
- 完成标准：上述用例全部通过。
- 依赖：T08。

### T10 一期验收

- 文件：
  - `docs/plans/2026-11-10-apix-phase1-plan.md`：只改“进度记录”。
  - `README.md`：用法、配置示例、`.cmd` 的限制。
  - `<WP>/tmp/apix-accept/`：验收脚本、配置目录和输出，不进仓库。
- 动作：
  1. **A1 真实 spec**：
     - 设 `APIX_CONFIG_DIR=<WP>/tmp/apix-accept/cfg`。
     - 用 P3 的真实 spec 依次运行 `site add`、`ops`、`desc`。
     - 用 `--dry-run` 核对拼出的 URL（含 base_path）。dry-run 不发请求，也不登录。
     - 真实调用只读 GET，需要用户另行同意。
  2. **A2 Jenkins**：
     - 注册 Jenkins 简单定义，对 httptest 模拟的 Jenkins 执行 `job-info` 和 `job-build`。
     - 只有用户提供了 Jenkins 并同意使用其凭证时，才增加一次真实只读调用。
  3. **A3 汇总测试**：`go test ./... -count=1`。
  4. **A4 快捷命令**：
     - **容器内**：用 `<WP>/tmp/apix-accept/` 下的配置目录和 link 目录，生成 sh 版并调用一次。
     - **主机**：
       - 先在容器里构建：`GOOS=windows GOARCH=amd64 go build -o <WP>/tmp/apix-accept/bin/apix.exe ./cmd/apix`。
       - 再提交主机 job：`gofer job run -p my-tools-dev -a exec --runner local --sync --cwd tmp/apix-accept -- cmd /c accept-win.cmd`。
       - `accept-win.cmd` 由本任务写在 `<WP>/tmp/apix-accept/`，它会：
         - 设置 `APIX_CONFIG_DIR=%CD%\cfg`，把 `%CD%\bin` 放到 PATH 最前；
         - 运行 `apix link pet --name petx --dir %CD%\links`，用 `links\petx.cmd list-users --dry-run` 调用；
         - 验证覆盖保护和删除保护；
         - 输出各步退出码。
       - 主机上的所有读写都在 `<WP>/tmp/apix-accept/` 之内。
  5. **A5 工程检查**：`go vet ./... && go test ./... -count=1 && go test ./internal/ -run TestLayering`，并检查文件行数。
- 验证：各项的命令和输出摘要写进“进度记录”，完整输出放在 `<WP>/tmp/apix-accept/`。
- 完成标准：A1–A5 全部通过，或明确标注 UNVERIFIED 并说明原因（P3、P5 缺失）。
- 依赖：T01–T09。

## 偏离登记

| 偏离 | 规则或设计条款 | 理由 |
|---|---|---|
| 入口用 `os.Exit(commands.Run(info, nil))`，不用 `os.Exit(app.Run(nil))` | SR2003 | 必须在 gcli 分发前做预扫描和动态注册，同时要让测试走同一入口（GR110） |
| 动态操作的选项在运行时注册，不绑定命令结构体 | SR2003，设计 D15 | 设计已批准 |
| `commands` 除 `wiring` 外，还直接调用 `request.Execute`、`output.Render` 和 `config`、`spec` 的查询函数 | 设计“代码组织”中 commands→wiring→* | 与 gofer 一致：命令层调用领域函数，wiring 只负责构造；领域包之间仍然没有互相依赖 |

分层表（T01 测试固定）：
- `model` 只导入标准库与 goutil/errorx。
- `config`、`spec`、`auth`、`request`、`output`、`link` 只导入 `model`，彼此之间没有依赖。
- `wiring` 可以导入以上所有包。
- `commands` 可以导入 `wiring` 与以上所有包。

## 回滚与恢复

- 每个任务各自一个提交。回滚某个任务就 `git revert <该任务提交>`，或者回到上一个任务完成时的提交，再重做本任务。
- 恢复点：每个任务的提交号写进“进度记录”。
- 不涉及数据迁移和外部服务状态。测试只用 `t.TempDir()` 作为配置目录，绝不读写真实的 `~/.config/apix`。
- 实施中遇到 Semantic Amendment（例如 gcli 做不到设计约定的行为）时，停在当前任务，回到 plan review，并请求人工批准。

## 人工 Gate

1. 计划批准：本文档。
2. 当前执行请求：计划批准后，由用户另行发出。
3. A1、A2 中真实服务调用与真实凭证的使用：逐项征得用户同意。默认只用 `--dry-run` 和 httptest。
4. 不包含 push 或发布。以后要做时另行审批。

## 可追溯性

| 验收与设计条款 | 任务 | 验证 |
|---|---|---|
| 一期第 1 项：配置目录、`.env`、state、`config` 命令；D3、D13、D14 | T02、T05 | T02、T05 测试 |
| 一期第 2 项：站点管理、两种来源、缓存；D2、D5、D8、Q4 | T03、T04、T05 | T03–T05 测试；A1、A2 |
| 一期第 3 项：`ops`、`desc` | T05 | T05 测试；A1 |
| 一期第 4 项：两种调用、参数映射、请求体；D6、D7、D15 | T06、T08 | T06、T08 测试 |
| 一期第 5 项：env、auth、`use`；D4、D9 | T07 | T07 测试 |
| 一期第 6 项：输出、`-i`/`-v`/dry-run、退出码、打码；D10 | T05、T06、T08 | T06、T08 测试；A3 |
| 一期第 7 项：写保护；D11 | T08 | T08 测试 |
| 一期第 8 项：快捷命令；D12 | T09 | T09 测试；A4 |
| 设计“安全”：凭证不出主机、按来源打码、TLS、token 隔离 | T02、T06、T07、T08 | T06–T08 测试 |
| 工程规范 GR104、GR107、GR108、GR110、SR2002–SR2004（偏离见“偏离登记”） | T01、T05、T08 | 分层测试；A5 |

## 完成 Gate 与剩余工作

- 完成条件：A1–A5 全部满足，或者用户接受其中标注为 UNVERIFIED 的项。
- 完成时在“进度记录”里写一段不超过 8 行的总结。
- 关闭 issue `my-tools-thv4`，或者改为跟踪二期。
- 剩余工作：二、三期见设计“分期路线”；gcli 的退出码问题见 issue `my-tools-kb4d`。

## 进度记录

（实施时填写：任务状态、实际路径、验证命令与结果、提交号。）
