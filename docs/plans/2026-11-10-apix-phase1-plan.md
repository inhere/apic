<!-- template_id: plan; template_version: 1.3.0 -->
# apix 一期（MVP）实施计划

> 状态：Draft 0.1 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-10 | Claude（inhere：“批准 apix 设计 0.2（接受未复核修订），Q1/Q2 按推荐，开始写一期计划”） | 初稿：4 个波次、10 个任务，覆盖设计 0.2“分期路线·一期”的 8 项 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 规划工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：交付 apix 一期，即设计 0.2“分期路线·一期”的第 1–8 项，并通过一期验收。
- Scope freeze：只做一期 8 项；二、三期的内容（补全、oauth2、exec、overlay、`-q`、`yaml/table`、multipart、分页、skill、MCP）一律不做，实施中发现需要也只记为 deferred。
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
  - 都是只读的，或只写 `tmp/`；
  - 不修改任何业务服务；
  - 不需要真实凭证。如果 A1 的 spec 地址需要认证，由用户把 spec 文件放到 `tmp/apix-specs/` 下，计划不处理真实密钥。

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
3. **P3 真实 spec（A1 与 T04 都依赖它）**：用户提供至少一份 Swagger 2.0 spec 的 URL，或者把文件放到工作区 `tmp/apix-specs/`。
   - 没有提供时，T04 可以用公开的 petstore v2/v3 样例先完成。
   - 但 A1 保持未完成，完成 Gate 不得报完成。
4. **P4 解析库锁定（设计 Q4）**：T04 开工前，用一个 20 行左右的探针程序（放 `tmp/apix-probe/`），在 P3 的 spec 和 petstore v2 上调用 kin-openapi 的加载与 `openapi2conv.ToV3`。
   - 能得到全部操作、`basePath` 和参数，就锁定 kin-openapi。
   - 失败时改用 libopenapi 再试一次。仍失败就停下，按 Semantic Amendment 报回。
5. **P5 主机可用**：A4 要用到主机。gofer server 在线，本项目 worker 在线（`gofer job` 能提交）。不在线时 A4 标 UNVERIFIED_REAL_ENV，并停在完成 Gate。

## 波次与依赖

| 波次 | 任务 | 依赖 |
|---|---|---|
| W1 骨架与基础 | T01 → T02、T03 | T02、T03 都依赖 T01，可以并行 |
| W2 定义与站点 | T04 → T05 | T04 依赖 T03 和 P4；T05 依赖 T02 和 T04 |
| W3 请求链路 | T06 → T07 → T08 | T06 依赖 T03；T07 依赖 T02 和 T06；T08 依赖 T05 和 T07 |
| W4 快捷命令与验收 | T09 → T10 | T09 依赖 T08；T10 依赖全部任务 |

执行方式：按工作区约定，由 sonnet 子 agent 按任务实施，我审查 diff、核对接线并验收。每个任务完成后做一次原子提交（`feat(<pkg>): ...`），提交信息带任务号。

## 任务

所有命令都在 `inhere-tools/apix` 下运行。

### T01 仓库骨架、入口、带码错误与分层守卫

- 文件：
  - `go.mod`（module `github.com/inhere/apix`）
  - `cmd/apix/main.go`
  - `internal/commands/app.go`
  - `internal/model/errors.go`
  - `internal/layering_test.go`
  - `.gitignore`、`README.md`（只写一段简介）
- 动作：
  - `main.go`：只注入 Version/GitCommit/BuildDate，然后调用 `os.Exit(commands.NewApp(info).Run(nil))`（GR107、SR2003）。
  - `internal/model/errors.go`：定义退出码常量和带码错误。

    ```go
    const (ExitOK=0; ExitLocal=1; ExitUsage=2; ExitNetwork=3; ExitHTTP4xx=4; ExitHTTP5xx=5; ExitProtected=6)
    type CodedError struct{ C int; Err error }      // 实现 errorx.ErrorCoder（Code() int）、Error()、Unwrap()
    func Errorf(code int, format string, a ...any) error
    ```

  - `NewApp` 只创建 gcli App 并挂上管理命令，此时为空壳。
  - 分层表照搬 xhen 的写法：
    - `model` 只能导入标准库和 goutil 的 errorx；
    - `config`、`spec`、`auth`、`request`、`output`、`link` 只能导入 `model` 和它们在本计划中声明的同层依赖：`auth→config`、`request→auth`、`request→output`；
    - `commands→wiring→*`。
- 验证：`go build ./... && go test ./internal/ -run TestLayering`
- 完成标准：
  - `go run ./cmd/apix --version` 打印版本，退出 0。
  - 分层测试通过；在 `model` 里临时加一个 `config` 的导入时，测试失败（只手工验证，不提交）。
- 依赖：P1、P2。

### T02 配置：目录、加载、`.env`、密钥展开、state

- 文件：`internal/config/{dir.go,load.go,dotenv.go,expand.go,state.go,check.go}` 及对应 `_test.go`
- 动作：
  - **配置目录**：`Dir()` 优先取 `$APIX_CONFIG_DIR`，否则用 `~/.config/apix`。
  - **加载**：`Load(dir) (*model.Config, error)` 读取 `config.yaml`（`version: 1`）和 `sites/*.yaml`，合并后做校验：
    - 站点名匹配 `^[a-z][a-z0-9-]*$`；
    - 站点名不是保留名（`site ops desc use auth link config help version`）；
    - 同一站点不能同时出现在 config.yaml 和 sites/ 里；
    - 简单定义的路径相对于配置目录解析；
    - 校验失败返回 `ExitLocal` 错误，信息里写明文件名和站点名。
  - **`.env`**：`ParseDotenv(text) map[string]string`，按 CAP-05 自写，键名区分大小写。查找函数 `Lookup(name)` 先查 OS 环境变量，再查 `.env` 的 map（D14）。不读当前目录的 `.env`（D13）。
  - **密钥展开**：
    - `Expand(s string, reg *Secrets) (string, error)` 展开 `${env:NAME}`。
    - 每个展开得到的值都登记到 `Secrets`（敏感值登记表，供打码器使用）。
    - 引用的变量不存在时返回 `ExitLocal`。
    - 展开推迟到实际使用时：只展开当前 env 和 auth 用到的字段，避免因为别的站点缺变量而报错。
  - **state**：`State{Selections map[site]{Env,Auth}; Links map[name]{Path,Hash,Site}; ManagedSites []string}`。用 `SaveState` 原子写入（临时文件加 rename，权限 0600）。
  - **检查**：`Check(cfg)` 返回警告列表，例如 token、password、secret、key 这类字段的值不是 `${...}` 形式。
- 验证：`go test ./internal/config/`
  - 覆盖重名站点、保留名、缺失变量、OS 环境变量优先、键名大小写、原子写入后的文件权限。
  - 文件权限只在非 Windows 上断言。
- 完成标准：上述用例全部通过；`Load` 对设计 0.2 的 config.yaml 示例解析无误（示例作为 testdata）。
- 依赖：T01。

### T03 操作模型与简单定义加载

- 文件：`internal/model/{site.go,operation.go}`、`internal/spec/{loader.go,simple.go,naming.go}` 及测试
- 动作：
  - **模型**：
    - `Operation{Name, Method, Path, Summary, Tags, Params []Param, Body *BodySpec, Output string, Responses map[string]SchemaSummary}`
    - `Param{Name, In(path|query|header), Required, Type, Enum, Desc}`
    - `BodySpec{Type(json|form|xml|raw), Required, Schema SchemaSummary}`
  - **加载接口**：`Loader` 接口与设计一致。
  - **简单定义**：`SimpleLoader` 支持两种写法：
    - 方法加路径的简写（`get: /x`）；
    - 完整写法（`params`、`body`、`query: [..]`、`output`）。
    - 路径中的 `{x}` 自动成为必填的 path 参数。
  - **命名**：`naming.go` 负责操作命令名：
    - operationId 转 kebab-case；
    - 没有 operationId 时用方法加路径生成；
    - 重名时追加 `-2`、`-3`，并返回告警列表。
- 验证：`go test ./internal/spec/ -run 'TestSimple|TestNaming'`，用设计 0.2 里的 `defs/jenkins.yaml` 示例作为 testdata。
- 完成标准：
  - Jenkins 示例解析出 4 个操作，参数、必填和 body 类型正确。
  - 命名在 `listUsers`、无 operationId、重名三种情况下都符合预期。
- 依赖：T01。

### T04 OpenAPI 加载、base_path 与操作索引缓存

- 文件：`internal/spec/{openapi.go,cache.go,fetch.go}` 及测试、`internal/spec/testdata/`
- 动作：
  - **OpenAPI 加载**：`OpenAPILoader` 用 P4 锁定的库实现。
    - 遇到 2.0 先转成 3.0。
    - 产出操作列表，以及 `DefaultBasePath`：2.0 取 `basePath`，3.x 取 `servers[0]` 的路径部分。
    - 产出 `DefaultBaseURL`：由 2.0 的 host 和 schemes 拼成，或取 3.x 的 `servers[0]`。
    - `Responses` 只保留响应结构的摘要（顶层字段名和类型），供 `desc` 显示。
  - **拉取**：`fetch.go` 用 URL 或文件路径拉取 spec。
    - 支持 ETag。
    - `source.auth` 只允许 bearer、basic、apikey 三种认证；遇到 login 认证返回 `ExitLocal`。
  - **缓存**：`cache.go` 管理 `cache/specs/<site>.{raw,index.json,meta.json}`。
    - 用 `Index(site, src, opts) ([]Operation, warn []string, error)` 取索引。
    - 索引未过期时直接读缓存。
    - 已过期时先尝试刷新；刷新失败就用旧缓存，并返回一条告警。
    - 没有任何缓存时返回 `ExitLocal`，信息里提示运行 `site refresh`。
- 验证：`go test ./internal/spec/ -run 'TestOpenAPI|TestCache'`。
  - testdata 用 petstore v2 和 v3 样例，加一份带 `basePath: /api/v1` 的最小 2.0 spec。
  - P3 的真实 spec 不进仓库，只在 T10 验证。
- 完成标准：
  - 2.0 和 3.0 的样例产出相同的操作模型，`base_path` 正确。
  - 缓存过期且刷新失败时，用旧缓存并给出告警。
- 依赖：T03、P4。

### T05 管理命令：site、ops、desc、config

- 文件：
  - `internal/commands/{site.go,ops.go,config.go}`
  - `internal/wiring/wiring.go`
  - `internal/output/{format.go}`
  - 各自的测试
- 动作：
  - **`site add <name> --spec <url|file> | --def <file>`**：
    - 可选参数 `--base-url` 和 `--auth-for-spec`。
    - 写入 `sites/<name>.yaml`，并把站点名登记到 `state.ManagedSites`。
    - 没有给 `--base-url` 时，用 spec 的 `DefaultBaseURL` 生成名为 `default` 的 env。两者都没有时报错（设计 0.2）。
    - 写完立即拉取定义并建索引。
  - **`site rm`**：只删 `ManagedSites` 里登记的站点，同时清理该站点的缓存（包括 token）。写在 config.yaml 里的站点拒绝删除，并提示用户手动编辑。
  - **`site ls/show/refresh`**：`show` 输出前经过打码器。
  - **`ops <site> [--tag] [--match]`**、**`desc <site> <op>`**：都支持 `-o json`。
  - **`config path/show/check`**。
  - **输出**：`output.Format(w, body []byte, contentType, mode)` 处理 `auto`、`json`、`raw` 三种模式；`auto` 在 stdout 是终端时美化 JSON。
  - **静态命令**：照常遵守 SR2003，位置参数先声明再取值，选项绑定到命令结构体。
- 验证：经真实分发 `app.Run([]string{...})` 测试（GR110）。`go test ./internal/commands/ -run 'TestSite|TestOps|TestDesc|TestConfig'`，配置目录用 `t.TempDir()` 加 `APIX_CONFIG_DIR`。
- 完成标准：
  - add、ls、ops、desc、rm 能对 petstore 样例和 Jenkins 示例走通。
  - `site rm` 删除 config.yaml 里的站点时被拒绝，退出 1。
- 依赖：T02、T04。

### T06 请求构造、发送与打码

- 文件：`internal/request/{build.go,body.go,send.go,curl.go}`、`internal/output/redact.go` 及测试
- 动作：
  - **构造**：`Build(op *model.Operation | nil, in Input, env model.Env) (*Prepared, error)`。
    - `Input` 包含：方法、原始路径、参数 map、`-p` 列表、`-d`、`-f`、`-H`。
    - URL 的拼法按设计 0.2：操作模式是 base_url + base_path + path；原始模式是 base_url + path。
    - path 参数值用 `PathEscape` 转义。
    - 原始路径只接受以单个 `/` 开头的，否则返回 `ExitUsage`。
    - 必填与类型校验失败返回 `ExitLocal`。
  - **请求体**：
    - `-d`：`@file`、`-`（stdin）、字面量三种来源，一律先读入并缓冲。
    - `-f`：`k=v` 是字符串，`k:=json` 按 JSON 类型；点号路径合并成对象；请求体是 form 类型时编码成表单。
  - **发送**：`Send(ctx, p, auth Provider) (*Response, error)`，依次做：
    - 写保护检查（由调用方先做，见 T08）；
    - `auth.Apply`；
    - 发送，不跟随重定向；
    - 收到 401 时调用 `Refresh` 并只重发一次；
    - 网络错误返回 `ExitNetwork`。
  - **打码**：`redact.go` 提供 `Redactor`，内容来自敏感值登记表加上固定敏感头。`Redact(string) string` 应用于全部 stderr 输出和错误消息，包括 `url.Error` 里的 URL。
  - **dry-run**：`curl.go` 渲染等价的 curl 命令，先打码再输出。
- 验证：`go test ./internal/request/ ./internal/output/`，用 httptest 服务覆盖：
  - `//evil` 和 `http://x` 形式的路径被拒绝；
  - 302 原样返回、不跟随；
  - 401 只重试一次，stdin 读入的请求体能重放；
  - query 里的 apikey 在网络错误信息中被打码；
  - curl 输出中没有任何明文密钥。
- 完成标准：上述用例全部通过。
- 依赖：T03。

### T07 认证：五种类型、token 缓存、auth 与 use 命令

- 文件：
  - `internal/auth/{provider.go,static.go,login.go,tokencache.go}` 及测试
  - `internal/commands/{auth.go,use.go}` 及测试
- 动作：
  - **构造**：`auth.New(spec model.AuthSpec, env model.Env, cfg *config.Config, sec *config.Secrets, cacheDir string) (Provider, error)`。
  - **静态类型**（none、bearer、basic、apikey）：
    - apikey 支持放在请求头或 query 里，用参数 `in` 和 `name` 指定。
    - 注入的值都登记为敏感值。
    - `Refresh` 返回 `ErrNoRefresh`。
  - **login**：
    - 用 `login` 段里的配置发登录请求（登录请求不受写保护限制）。
    - 按 `token.from/path` 和 `expires` 从响应里取 token 和有效期。
    - 按 `apply` 把 token 注入请求；token 登记为敏感值。
    - 登录失败时按网络错误、4xx、5xx 分别映射为退出码 3、4、5。
  - **token 缓存**：存在 `cache/tokens/<site>.<env>.<auth>.json`，内容 `{token, expires_at, fingerprint}`。
    - fingerprint 是 base_url 加上 login 段原文（展开前）的 sha256。
    - fingerprint 不一致就作废缓存；文件以 0600 原子写入。
  - **`use <site> [--env] [--auth]`**：写 state。选择优先级：单次选项 > state > `default_*`。
  - **`auth ls/login/logout/status <site>`**：`status` 输出前打码。
- 验证：`go test ./internal/auth/ ./internal/commands/ -run 'TestAuth|TestUse|TestLogin'`，用 httptest 覆盖：
  - token 过期后重登；
  - 切换 env 后不复用另一环境的 token；
  - fingerprint 变化后重新登录；
  - 登录请求返回 4xx 时退出 4。
- 完成标准：上述用例全部通过。
- 依赖：T02、T06。

### T08 动态站点命令、预扫描、写保护与退出码接线

- 文件：
  - `internal/commands/{dynamic.go,prescan.go,globalopts.go}` 及测试
  - `internal/request/protect.go`
- 动作：
  - **预扫描**：`Prescan(args []string, globals GlobalSpec) (site string, rewritten []string, err error)`。
    - 先跳过全局选项及它们带的值，并校验其中有没有未知选项，有则返回 `ExitUsage`。
    - 把 `--api X` 或 `--api=X` 摘出来，改写成位置参数 X。
    - 候选名不是已配置的站点时，不做改写，交给 gcli 处理。
  - **全局选项**：`--env --auth -o -i -v --dry-run --yes --timeout -H` 同时注册在 `app.Flags()` 和 `<site>` 的 SharedOpts 上，绑定同一变量。
  - **站点命令**：`<site>` 命令声明可选位置参数 `path`，并定义 `-X -d -f -H -p`。
    - path 的值不以 `/` 开头时，报“未知操作 <x>”并返回 `ExitUsage`。
    - 每个操作是 `<site>` 的子命令，不声明位置参数。
    - 参数选项在运行时注册。遇到冲突（全局选项、gcli 内置选项、同名但位置不同的参数、非法选项名）时不生成 `--<name>`，回退到 `-p`。
  - **写保护**：`protect.go` 中 `CheckProtect(method, env, authSpec, yes bool, tty bool) error`。
    - 方法先转大写再比较。
    - 只有 stdin 和 stderr 都是终端、且没有设置 `APIX_NO_PROMPT` 时才交互确认。
    - 拒绝时返回 `ExitProtected`。
    - 在 `auth.Apply` 之前调用，`--dry-run` 时跳过。
  - **输出与退出码**：
    - 2xx、3xx 退出 0，4xx 退出 4，5xx 退出 5。
    - 响应体一律写到 stdout。
    - `-i` 把状态行和响应头写到 stderr，`-v` 把请求行和请求头写到 stderr，都经过打码器。
  - **`NewApp`**：先调用 `Prescan`，再只为这一个站点注册命令，然后执行 `app.Run(rewritten)`。
- 验证：经真实分发 `go test ./internal/commands/ -run 'TestDynamic|TestPrescan|TestExitCodes|TestProtect'`，覆盖：
  - 全局选项写在站点名前、站点名后、操作名后三种位置；
  - `--api` 的两种写法；
  - `--bogus` 退出 2；
  - `pet list-userz` 退出 2 且不发请求；
  - 退出码 0–6 每个都有用例；
  - prod 环境上的 POST 不带 `--yes`、在非终端下退出 6，且服务端没有收到 login 请求。
- 完成标准：上述用例全部通过；`apix pet -X POST /api/users -f name=tom` 能对 httptest 服务成功调用。
- 依赖：T05、T07。

### T09 快捷命令 link

- 文件：`internal/link/{link.go,script.go}`、`internal/commands/link.go` 及测试
- 动作：
  - **`link <site> --name <n> [--env] [--auth] [--dir]`**：
    - 校验 site、name、env、auth 都匹配 `^[a-z0-9][a-z0-9-]*$`。
    - 拒绝预带 `--yes`。
    - 生成脚本：非 Windows 生成 sh 版（0755）；Windows 生成 `.cmd` 版和 sh 版。
    - 在 state 里记录路径和 sha256。
    - 目标文件已存在、但与 state 记录不一致时拒绝覆盖。
    - 输出里提示 `.cmd` 版不适合直接传 JSON 参数（用 `-d @file`）。
  - **`link ls`**：列出 state 里记录的快捷命令。
  - **`link rm <name>`**：只有文件哈希与记录一致时才删除。
- 验证：`go test ./internal/link/ ./internal/commands/ -run TestLink`，覆盖：
  - 名字带 `../` 时被拒绝；
  - 覆盖他人文件被拒绝；
  - 文件被改过之后 rm 被拒绝；
  - 生成的 sh 脚本经 `sh <script> --help` 能实际执行（仅 Linux）。
- 完成标准：上述用例全部通过。
- 依赖：T08。

### T10 一期验收

- 文件：
  - `docs/plans/2026-11-10-apix-phase1-plan.md`（只改“进度记录”一节）
  - `README.md`：补用法、配置示例和 `.cmd` 限制说明
  - 工作区 `tmp/apix-accept/`：存放验收脚本与输出，不进仓库
- 动作：
  1. **A1**：用 P3 的真实 spec 执行 `site add`、`ops`、`desc`，再用 `--dry-run` 核对 URL（含 base_path）。只有在用户另行同意时，才真实调用一个只读的 GET 接口。
  2. **A2**：注册 Jenkins 简单定义，对 httptest 模拟的 Jenkins 执行 `job-info` 和 `job-build`。如果用户提供了可用的 Jenkins，并同意用他的凭证，再加一次真实只读调用。
  3. **A3**：汇总 T05–T08 的测试，执行 `go test ./... -count=1`。
  4. **A4**：
     - 容器内：生成 sh 版快捷命令并成功调用。
     - 主机上：在 Windows 构建 apix，生成 `.cmd` 版快捷命令，经它调用 httptest 等价的本地服务（或 `--dry-run`），并核对覆盖和删除保护。
     - 主机步骤用这条命令执行：`gofer job run -p my-tools-dev -a exec --runner local --sync --cwd inhere-tools/apix -- <命令>`。产物只写到 `tmp/`。
  5. **A5**：执行 `go vet ./... && go test ./... -count=1 && go test ./internal/ -run TestLayering`，并用 `wc -l` 检查文件行数。
- 验证：每项验收的命令和输出摘要写进“进度记录”，长输出放到 `tmp/apix-accept/`。
- 完成标准：A1–A5 全部通过，或明确标注 UNVERIFIED，并写明原因（P3、P5 缺失）。
- 依赖：T01–T09。

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
| 工程规范 GR104、GR107、GR108、GR110、SR2002–SR2004 | T01、T05、T08 | 分层测试；A5 |

## 完成 Gate 与剩余工作

- 完成条件：A1–A5 全部满足，或者用户接受其中标注为 UNVERIFIED 的项。
- 完成时在“进度记录”里写一段不超过 8 行的总结。
- 关闭 issue `my-tools-thv4`，或者改为跟踪二期。
- 剩余工作：二、三期见设计“分期路线”；gcli 的退出码问题见 issue `my-tools-kb4d`。

## 进度记录

（实施时填写：任务状态、实际路径、验证命令与结果、提交号。）
