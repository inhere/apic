<!-- template_id: plan; template_version: 1.3.0 -->
# apic 二期第一批实施计划

> 状态：Draft 0.1 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-11 | Claude（inhere：“批准 apic 二期设计 0.2（接受未复核修订）”） | 初稿：3 个波次、8 个任务，覆盖设计 0.2 的四项 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 规划工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：交付设计 0.2 的四项并通过验收，四项是补全、`--pick` 与 `-o yaml|table`（含退出码 7）、`apic skill`、exec 认证。
- Scope freeze：只做设计 0.2 的范围。设计“非目标”和二期第二批的内容实施中即使发现需要，也只记为 deferred。
- 评审预算：与设计相同，按高暴露处理（SR1409），做一轮两轴独立评审，一是正确性与可执行性，二是安全与契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。

## 目标与完成定义

**目标**：v0.1.1 的用户升级后，配置不用改就能得到以下能力：

- 补全站点、操作、参数、全局选项及其取值，快捷命令也能补全；
- 用 `--pick` 取字段，用 `-o yaml|table` 换格式，数字和字段顺序保持原样；
- 用 `apic skill` 生成 skill；
- 配置 `type: exec` 认证。

行为符合设计 0.2 的各节和 D1–D16。

**完成定义（验收）**：

1. **B1 补全**：在容器里用真实 bash 加载 `--gen-completion bash` 和 `link ls --completion bash` 的输出，以下四类各至少一个用例：
   - 设计“已确认事实 2”表中的每一行都得到期望候选；
   - `--api <TAB>`、`jk<TAB>`（同时存在 jkx）、未完成的选项均不报错；
   - 快捷命令 `petctl <TAB>` 能补出操作名；
   - 补全过程中 httptest 服务端收到 0 个请求，exec 桩命令执行 0 次。
2. **B2 输出**：经真实分发覆盖以下行为：
   - `--pick` 的 `*`、空数组、缺字段填 null、下标、路径不存在；
   - 字符串与 `-o` 的组合表；
   - 大整数 `12345678901234567891` 和 `<>&` 原样保留；
   - 键顺序保持；
   - 3xx 警告、4xx/5xx 原样输出；
   - 退出码 7（非 JSON、204、路径不存在）；
   - 发送前非法路径退出 2，服务端收到 0 个请求。
3. **B3 skill**：
   - 对 A1 的真实 spec（30 个操作）和 Jenkins 简单定义分别生成 SKILL.md，frontmatter 能被 YAML 解析；
   - 文件中不出现 base URL、`${env:` 和 exec 命令；
   - 含注入文本的 summary 被净化；
   - 不带标记的文件拒绝覆盖，`--force` 才覆盖；
   - `--name ../x` 退出 2；
   - `skillc source add local` 加 `skillc install` 能装上，用 tmp 目录下的 skillc 配置，见 T08。
4. **B4 exec**：用桩命令覆盖以下行为：
   - text 和 json 两种格式；
   - 缓存与过期，无过期时间时不缓存；
   - 401 重新取 token；
   - 超时，Linux 上进程组被杀，孙进程不残留；
   - 命令失败时 stderr 只有最后一行且已打码；
   - argv 不出现在任何输出中；
   - `exec.env` 传进子进程；
   - dry-run 不执行命令；
   - `auth login/logout/status`。

   Windows 主机上经 gofer job 再验证两点：`.cmd` 桩命令能取到 token，含 `&` 的参数被拒绝。
5. **B5 工程与兼容性**：
   - `go vet ./...`、`go test ./... -count=1` 和分层守卫测试通过，单个文件不超过 1000 行（GR104）；
   - 用 v0.1.1 的验收配置（`<WP>/tmp/apic-accept/cfg`）跑一期验收中的 dry-run 用例，结果不变；
   - 名为 `pick` 的参数改走 `-p`、名为 `skill` 的站点被跳过并警告，这两点都有用例。

## 范围、排除项与授权

**范围**：仓库 `inhere-tools/apic`（Git root）下的代码、README 和 docs。

**排除项**：

- 设计 0.2 的非目标，以及二期第二批的内容。
- 修改 gookit 各库。补全时 gcli 看不到共享选项这个问题，只在 gcli 立 issue，不在本计划内改。
- push、tag、发布 v0.2.0：属于外部动作，需要另行授权。

**授权**：

- 计划批准后，还需要用户另行发出当前执行请求，才能开始实施。
- 按功能点本地提交是默认授权。
- 主机动作只有 T08 的 Windows 验证。主机上只写 `<WP>/tmp/apic-p2-accept/`，不需要真实凭证，exec 用的是桩命令。

- `host_or_non_offline_action=REQUIRED`

## 输入与批准证据

- 设计：`docs/design/2026-11-11-apic-phase2-design.md` Draft 0.2，已批准。
  - 批准原话（2026-11-11）：“批准 apic 二期设计 0.2（接受未复核修订）”。
  - 记录批准的提交：`b330f3c`。
- 设计第 1 轮评审的 C1–C11、S1–S19 已并入 0.2，见设计“评审处理”。本计划按 0.2 的条款执行，不重述理由。
- 一期计划与验收产物：
  - 计划：`docs/plans/2026-11-10-apic-phase1-plan.md`；
  - 验收产物：`<WP>/tmp/apic-accept/`；
  - 真实 spec：`<WP>/tmp/apix-specs/`。
- 跟踪 issue：`my-tools-thv4`。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | 运行时 shell 补全 | gcli v3.8.6 `--in-completion` / `--gen-completion`（`completion.go`） | 是 | 补全模式下在 apic 内把全局选项注册到命令本地（设计 D1） | DIRECT_REUSE | none | 无 |
| CAP-02 | 保序、保留数字原文的 JSON 解码 | `encoding/json`（`Decoder.Token`、`UseNumber`）；goutil maputil（键顺序会丢，评审 C2/C4） | 是（stdlib） | 在 `internal/output` 写约 150 行的 token 流解码器 | OWNER_EXTENSION | maputil 和 `map[string]any` 会丢键顺序和数字精度 | 只在 `internal/output` 内 |
| CAP-03 | pick 路径遍历 | goutil `maputil.GetFromAny`（`*` 的语义不符，评审 C3） | 否 | 在 `internal/output` 自写遍历，约 80 行，沿用 maputil 的点号语法 | OWNER_EXTENSION | 空数组判为不存在、丢掉缺字段的元素 | login 仍用 maputil；两处语法一致，不统一实现 |
| CAP-04 | YAML 输出 | goccy/go-yaml（已依赖，有 `MapSlice`） | 是 | 把保序值转成 `yaml.MapSlice`，数字用字面量 | DIRECT_REUSE | none | 无 |
| CAP-05 | 表格输出 | `text/tabwriter`；gookit/cliui | 是（stdlib） | 无 | DIRECT_REUSE | none | 不引入 cliui |
| CAP-06 | 执行外部命令、进程组、超时 | `os/exec`（`CommandContext`、`WaitDelay`）、`syscall.SysProcAttr{Setpgid}` | 是（stdlib） | 无 | DIRECT_REUSE | none | Windows 不做 Job Object（D14） |
| CAP-07 | token 缓存与 401 重试 | `internal/auth` 的 tokencache 与 Refresh、`internal/request` 的 401 路径 | 是 | 按认证类型计算指纹；不写零过期时间 | OWNER_EXTENSION | none | 与 login 共用 |
| CAP-08 | SKILL.md 渲染 | `text/template`；goccy/go-yaml（frontmatter） | 是 | 新领域包 `internal/skillgen` | MINIMAL_NEW_MODULE | 没有现成包能从 `model.Index` 生成 skill；放进 `commands` 会违反 GR108，放进 `output` 会混入不相关的职责 | 见 NM-01 |

### New module candidates

| candidate_id | capability_id | proposed_module | searched_candidates | direct_reuse_gap | thin_adapter_or_owner_extension_gap | proven_gap | unique_owner_and_lifecycle | deletion_or_merge_handling |
|---|---|---|---|---|---|---|---|---|
| NM-01 | CAP-08 | `internal/skillgen` 包（设计“代码组织”） | `internal/output`、`internal/link`、`internal/commands` | 现有包都不负责生成文档类文件 | 放进 `commands` 违反 GR108；放进 `output`/`link` 职责不符 | 设计 0.2 已批准 | 仓库 `inhere-tools/apic`，只依赖 `model` | 不再需要时删除该包和 `commands/skill.go`，并从保留名中去掉 `skill` |

### Rejected new tools

| rejected_candidate | capability_id | deletion_test_and_reason |
|---|---|---|
| 引入 gjson 或 jq 库 | CAP-03 | 删掉它也不缺能力，自写遍历已满足设计语义；jq 语法属于非目标 |
| x/sys/windows Job Object | CAP-06 | D14：WaitDelay 已能保证 apic 不卡住 |
| 修改 gcli 让补全带上共享选项 | CAP-01 | 只立 issue；apic 侧已有解决办法 |

## 前置检查与 fail-closed 条件

运行目录均为 `inhere-tools/apic`。

1. **P1 工作树**：`git status --short` 为空，HEAD 至少包含 `b330f3c`。否则停下，查清归属。
2. **P2 Go 版本**：`go.mod` 写 `go 1.26.0`。容器内用 `GOTOOLCHAIN=auto`，得到 go1.26.0。
3. **P3 验收素材**：以下三项存在：
   - `<WP>/tmp/apix-specs/big-v2.json`（30 个操作）；
   - `<WP>/tmp/apic-accept/jenkins-def.yaml`；
   - `<WP>/tmp/apic-accept/cfg`（v0.1.1 验收配置）。

   缺失时按一期 T10 的步骤重建。
4. **P4 补全实测（D1 与“待确认事项”第 1 条）**：T06 开工前，在 `<WP>/tmp/apic-p2-probe/` 写一个探针，验证以下两点：
   - 把全局选项注册到某个操作命令本地之后，`--in-completion` 能列出这些选项及其 Choices；
   - 与同名的动态参数选项相比，本地注册的全局选项优先，同名参数改走 `-p`。

   不成立时停下，按 Semantic Amendment 报回。
5. **P5 YAML 写法（“待确认事项”第 3 条）**：T01 开工前，探针确认 `yaml.MapSlice` 能保持键顺序，且大整数能作为字面量输出。不成立时可以给数字加引号，按设计“原文无法原样表示时带引号”执行，记为 Corrective。
6. **P6 主机可用**：T08 的 Windows 验证需要 gofer server 和本项目 worker 在线。不在线时该项标 UNVERIFIED_REAL_ENV，停在完成 Gate。
7. **P7 skillc**：容器里 `skillc` 可用，否则用 `go build` 构建 `inhere-tools/skillc`。不可用时 B3 中 skillc 那一步标 UNVERIFIED，并给出替代证据：用 YAML 解析 frontmatter，并检查 skill 目录结构。

## 波次与依赖

| 波次 | 任务 | 依赖 |
|---|---|---|
| W1 基础 | T01 输出内核；T02 spec 只读缓存与保留名；T03 exec 配置模型 | 互相独立，可以并行 |
| W2 接线 | T04 exec 提供者；T05 输出接线；T06 补全；T07 skill | T04 依赖 T03；T05 依赖 T01；T06 依赖 T02、T05；T07 依赖 T01、T02 |
| W3 验收 | T08 验收、README 与工程检查 | T01–T07 |

**执行方式**：沿用一期做法，用 gofer plan 跟进，每个任务派一个 gofer worktree job。

- 实施方：简单任务（T02、T03、T05）派 `-a suga`，其余派 sonnet 子 agent 或我自己做。
- 我审查 diff、核对调用点接线，并做验收。
- 任务书要写明以下约束：
  - 分层规则 G021–G024 和 idevstd；
  - 禁止 web_fetch；
  - go.mod 和 go.sum 在允许修改的范围内；
  - 禁止删注释和无关重排。
- 每个任务完成后做一次原子提交（`feat(<pkg>): ... (P2-Tnn)`）。

## 任务

所有命令都在 `inhere-tools/apic` 下运行，并带上 `GOFLAGS=-buildvcs=false GOWORK=off GOTOOLCHAIN=auto`。文中 `<WP>/tmp/` 指工作区根目录的 `tmp/`。

### T01 输出内核：保序解码、pick、yaml、table、Render 与退出码 7

- 文件：
  - `internal/model/{jsonv.go,pick.go}` 及测试：保序解码与 pick 遍历，只用标准库；
  - `internal/model/errors.go`：`ExitPostProcess = 7`；
  - `internal/output/{yaml.go,table.go,exit.go,format.go}` 及测试。
- 动作：
  - **保序解码**（`model`）：
    ```go
    type Object struct{ Keys []string; Vals map[string]any }
    func Decode(body []byte) (any, error)          // Object / []any / json.Number / string / bool / nil
    func EncodeJSON(w io.Writer, v any, indent bool) error  // 数字按原文；SetEscapeHTML(false)；键按 Keys 顺序
    ```
  - **pick**（`model`）：语法和语义按设计 §2。
    ```go
    type Pick struct{ segs []string }
    func ParsePick(path string) (Pick, error)      // 语法错误 → ExitUsage
    func (p Pick) Apply(v any) (any, error)        // 路径不存在 → ExitPostProcess，消息含失败段
    ```
  - **yaml**：`ToYAML(v any) ([]byte, error)`，Object 转 `yaml.MapSlice`；数字用字面量，无法原样表示时带引号输出原文（P5）。
  - **table**：`Table(w io.Writer, v any, tty bool) (ok bool, err error)`，`ok=false` 表示需要退回 json。规则按设计 §2，包括终端下截断和去掉控制字符。
  - **Render**：
    ```go
    type RenderOptions struct{ Mode string; Pick *Pick; IncludeHeaders, TTY bool }
    func Render(res *Result, opt RenderOptions, stdout, stderr io.Writer, red *model.Redactor) (int, error)
    ```
    - 2xx：依次 pick、格式化；失败时返回 `(ExitPostProcess, err)`，错误消息为 `HTTP <code> succeeded; <原因>`，stdout 不输出任何内容。
    - 3xx：原样输出；用了 pick 或 yaml/table 时在 stderr 警告。
    - 4xx/5xx：原样输出，不 pick，不转换格式。
    - 写 stdout 失败时返回 `(ExitLocal, err)`。
  - **pick 结果的输出形式**：按设计 §2 的组合表。
- 验证：`go test ./internal/model/ ./internal/output/ -run 'TestDecode|TestPick|TestYAML|TestTable|TestRender' -count=1`，覆盖：
  - 大整数和 `<>&` 往返后不变；
  - 键顺序保持；
  - `*` 的四种情况：数组、空数组、缺字段、对象；
  - 下标越界；
  - 组合表中的每一格；
  - 3xx、4xx 的处理；
  - 非 JSON、空 body 时返回 7。
- 完成标准：上述用例全部通过；`model` 只导入标准库，`output` 只导入 `model`（分层测试）。
- 依赖：P5。

### T02 spec 只读缓存与保留名 skill

- 文件：
  - `internal/spec/cache.go` 及测试；
  - `internal/config/load.go` 及测试；
  - `internal/model/config.go`：新增 `Config.Warnings []string`（`yaml:"-"`）；
  - `internal/commands/run.go`：`managementCommands` 加 `skill`；
  - `internal/wiring/wiring.go`：把 `Config.Warnings` 输出到 stderr。
- 动作：
  - **只读缓存**：`IndexOptions.CacheOnly bool`。为 true 时：
    - 有缓存就直接返回，过期也不刷新；
    - 没有缓存就返回 `ExitLocal`；
    - 绝不调用 Fetch。
  - **保留名**：`reservedNames` 加 `skill`。
    - 站点名命中保留名时，不再让整个 Load 失败：跳过该站点，并追加一条警告 `site "skill" uses a reserved name and is skipped; rename it`（D15）。
    - `site add` 遇到保留名仍然报错退出。
- 验证：`go test ./internal/spec/ ./internal/config/ -run 'TestIndexCacheOnly|TestReservedSiteSkipped' -count=1`，覆盖：
  - CacheOnly 遇到过期缓存时服务端收到 0 次请求；
  - 没有缓存时返回错误；
  - 名为 `skill` 的站点被跳过并产生警告，其他站点照常加载；
  - `site add skill` 退出 1。
- 完成标准：上述用例全部通过。
- 依赖：无。

### T03 exec 配置模型、校验与展开

- 文件：
  - `internal/model/config.go`：`ExecSpec`、`AuthSpec.Exec`、`AuthSpec.Apply`；
  - `internal/config/{load.go,expand.go,check.go}` 及测试。
- 动作：
  - **模型**：
    ```go
    type ExecSpec struct {
        Command []string          `yaml:"command"`
        Env     map[string]string `yaml:"env,omitempty"`
        Timeout string            `yaml:"timeout,omitempty"`
        Format  string            `yaml:"format,omitempty"`   // text | json
        Token   ValueFrom         `yaml:"token,omitempty"`
        Expires ValueFrom         `yaml:"expires,omitempty"`
        TTL     string            `yaml:"ttl,omitempty"`
    }
    ```
    `AuthSpec` 新增 `Exec ExecSpec yaml:"exec,omitempty"` 和 `Apply ApplySpec yaml:"apply,omitempty"`。
  - **加载校验**：
    - `type: exec` 时，`exec` 节做未知键校验，未知键报错退出 1，错误消息含键名；
    - `command` 不能为空，`format` 只能是 text 或 json，`timeout`、`ttl` 必须能解析成时长。
  - **展开**：`ResolveAuth` 先复制 `Exec.Command` 和 `Exec.Env`，再展开。展开结果全部登记为敏感值（D13）。`Apply` 字段的展开方式与 `Login.Apply` 相同。
  - **配置检查**：`config.Check` 为每个 exec 认证输出一条提示 `auth <site>.<name> runs <command[0]>`。
- 验证：`go test ./internal/config/ -run 'TestExecSpec|TestResolveAuthExec|TestCheckExec' -count=1`，覆盖：
  - 写成 `exec.apply` 时报未知键；
  - 展开后 `d.Cfg` 中的切片未被修改；
  - argv 和 env 的值都已登记为敏感值；
  - 一期的 login 配置照常加载，覆盖顶层 `apply` 与 `login.apply` 并存的情况。
- 完成标准：上述用例全部通过。
- 依赖：无。

### T04 exec 认证提供者

- 文件：
  - `internal/auth/{exec.go,exec_unix.go,exec_windows.go,provider.go,tokencache.go}` 及测试；
  - `internal/wiring/wiring.go`：`Deps.ConfigDir`；
  - `internal/commands/auth.go`：login、status；
  - `internal/commands/site.go`：`site show`、`config show` 中的 argv 打码。
- 动作：
  - **提供者接入**：
    - `New` 的类型白名单加 `exec`；
    - `Apply`、`Refresh` 增加 exec 分支；
    - `injectToken(req, token, apply ApplySpec)`：exec 用 `spec.Apply`，login 用 `spec.Login.Apply`。
  - **执行**：`runExec(ctx) (token string, expires time.Time, err error)`，行为按设计 §4：
    - stdin 为空设备，工作目录为 `Deps.ConfigDir`；
    - 子进程环境为当前环境加上 `exec.env`，再加上 `APIC_SITE`、`APIC_ENV`、`APIC_AUTH`；
    - stdout 和 stderr 各限读 4 MiB；
    - 超时取 `exec.timeout` 和请求 ctx 中先到的一个，`WaitDelay=2s`；
    - Unix 上 `Setpgid`，超时后 kill 整个进程组（`exec_unix.go`）；
    - Windows 上目标是 `.bat`/`.cmd` 时检查元字符（`exec_windows.go`）。
  - **取 token**：text 要求单行且不含控制字符；json 用 T01 的 `model.Decode` 和 `model.ParsePick` 取值。
  - **缓存**：
    - 只有过期时间已知时才写入，优先用 expires，其次 ttl；
    - `fingerprint()` 按类型分支计算：exec 哈希 type、base_url、展开后的 Exec 和 Apply；login 的指纹不变；
    - `Status()` 对不能缓存的 exec 返回专门的状态。
  - **失败**：
    - 一律返回 `ExitLocal`，消息为 `auth <name>: exec failed (exit N): <stderr 最后一行，已打码>`；
    - 消息里不出现 argv。
  - **命令层**：
    - `auth login` 对不能缓存的 exec 提示 `not cached`；
    - `auth status` 显示 `not cached (no expiry)`；
    - `site show`、`config show` 中 `command` 只显示 `command[0]` 加上 `***`。
- 验证：`go test ./internal/auth/ ./internal/commands/ -run 'TestExec|TestAuthStatusExec|TestShowMasksExec' -count=1`。桩命令由测试编译，写在 `testdata` 下的小 Go 程序里，覆盖：
  - text 和 json 两种格式；
  - 有 expires、有 ttl、两者都没有；
  - 401 后重新执行命令，计数为 2；
  - 超时，Linux 上桩命令派生的孙进程在超时后不存在；
  - stderr 中的 token 被打码，argv 不出现在任何输出中；
  - `exec.env` 生效；
  - dry-run 时执行计数为 0；
  - base_url 改变后指纹变化；
  - login 指纹与一期一致，用一期写入的缓存文件做回放。
- 完成标准：
  - 上述用例全部通过；
  - `GOOS=windows go build ./...` 和 `GOOS=darwin go build ./...` 通过。
- 依赖：T03；T01（`model.Decode`、`model.ParsePick`）。

### T05 输出接线：`--pick`、`-o` 五种取值、发送前校验

- 文件：`internal/commands/{globalopts.go,siterun.go,prescan.go}` 及测试。
- 动作：
  - **全局选项**：
    - `globalOpts` 加 `Pick string`，`bind` 注册 `--pick`，不加短名；
    - `reservedOptNames` 加 `pick`；
    - `globalSpec()` 加 `"--pick": true`。
  - **输出模式**：`outputMode` 接受 auto、json、raw、yaml、table；错误消息列出这五种。
  - **siterun**：
    - 发送前调用 `ParsePick`，失败退出 2，不发请求；
    - 改用 `Render(res, RenderOptions{...})`；
    - 返回的错误经 `redactErr` 处理后原样返回，不再一律报成 `HTTP %d`；只有 4xx/5xx 仍报 `HTTP %d`。
- 验证：经真实分发运行 `go test ./internal/commands/ -run 'TestPick|TestOutputModes|TestPickParamFallback' -count=1`，覆盖：
  - B2 列出的各项；
  - 名为 `pick` 的参数只能通过 `-p pick=` 传入；
  - 退出码 7 的 stderr 文案。
- 完成标准：上述用例全部通过；一期的 `TestExitCodes` 等测试照常通过。
- 依赖：T01。

### T06 补全

- 文件：
  - `internal/commands/{prescan.go,run.go,dynamic.go,globalopts.go,link.go}` 及测试；
  - `internal/link/completion.go` 及测试。
- 动作：
  - **预扫描补全模式**：`Prescan` 识别 `--in-completion`，规则按设计 §1：
    - 最后一个词不参与校验，也不参与站点命中判断；
    - 预扫描出错时退回注册占位命令；
    - 前一个词是 `--api` 时，直接输出匹配的站点名，返回 0。
  - **注册**：在 `run.go` 中：
    - 补全模式下没有命中站点时，注册所有站点的占位命令，只有名字和描述；
    - 命中站点时，用 `CacheOnly` 读索引，所有错误都静默。
  - **本地注册选项**：在 `dynamic.go` 中，补全模式下给站点命令和每个操作命令注册本地的全局选项和站点选项（P4 已验证）：
    - `--env`、`--auth` 的 Choices 取该站点的 env 名和 auth 名；
    - `-o` 的 Choices 按命令区分；
    - 参数选项的 Choices 取 `Param.Enum`。
    - 非补全模式的注册方式不变。
  - **快捷命令**：
    - `link ls --completion <bash|zsh|pwsh>`：调用 `link.CompletionScript(shell string, recs Records) (string, error)`；
    - 输出前对每条记录重新做 `link.Names` 校验，不合法的跳过；Windows 上名字去掉 `.cmd` 后缀；
    - 补全函数的工作方式：把输入改写成 `apic --in-completion --api <site> [--env e] [--auth a] <words>` 取候选；
    - `link` 和 `link rm` 成功时，在 stderr 提示重新生成补全。
  - **gcli issue**：在工作区 tracker 立一个 issue，内容是“gcli 补全不包含祖先的共享选项”，只立 issue。
- 验证：
  - `go test ./internal/commands/ ./internal/link/ -run 'TestCompletion|TestPrescanCompletion|TestLinkCompletion' -count=1`，覆盖：
    - 设计“已确认事实 2”的表；
    - `--api` 的候选；
    - `jk` 与 `jkx` 同时存在；
    - 输入不完整的选项时退出 0 且没有 stderr 输出；
    - httptest 服务端收到 0 个请求；
    - 非法的快捷命令名被跳过。
  - real-env：真实 bash 中的验证放在 T08 做。
- 完成标准：上述用例全部通过；非补全模式下一期的 `TestDynamic`、`TestPrescan` 照常通过。
- 依赖：T02、T05、P4。

### T07 `apic skill`

- 文件：
  - `internal/skillgen/{skillgen.go,sanitize.go}` 及测试；
  - `internal/commands/skill.go` 及测试；
  - `internal/layering_test.go`：加上 `internal/skillgen`，只依赖 `internal/model`；`wiring` 和 `commands` 可以导入它。
- 动作：
  - **接口**：
    ```go
    type Input struct{ Site string; Index *model.Index; Cfg *model.Site; Name string; MaxOps int }
    func Render(in Input) ([]byte, error)          // 完整 SKILL.md；frontmatter 由 goccy/go-yaml 编码
    func Sanitize(s string, max int) string        // 去控制字符和换行、转义反引号与 Markdown 标记、去 <!-- -->、截断
    const Marker = "<!-- generated by apic skill -->"
    ```
  - **正文**：内容按设计 §3 生成，正文使用英文。
  - **命令**：`skill <site> [--out] [--name] [--max-ops 60] [--force]`：
    - name 不符合 `^[a-z0-9][a-z0-9-]{0,63}$` 时退出 2；
    - 先写临时文件再 rename；目标是符号链接时拒绝写入；
    - 目标已存在但不带 Marker 时，不加 `--force` 就退出 1；
    - 完成后在 stderr 提示 skillc 接入方式和“分享前检查”。
- 验证：`go test ./internal/skillgen/ ./internal/commands/ -run 'TestSkill' -count=1`，覆盖：
  - frontmatter 能被 YAML 解析，且 name 和 description 正确；
  - summary 中含 `---`、换行、`<!--`、反引号和“run … --yes”这类注入文本时，被净化；
  - 文件中不出现 base URL、`${env:` 和 exec 的 command；
  - 超过 `--max-ops` 时按 tag 分组，含 `untagged`；
  - 覆盖与 `--force`；
  - `--name ../x` 退出 2。
- 完成标准：上述用例全部通过；分层测试通过。
- 依赖：T01、T02。

### T08 验收、README 与工程检查

- 文件：
  - 本计划：只改“进度记录”。
  - `README.md`、`README.zh-CN.md`：补全、`--pick`/`-o`、退出码 7、`apic skill`、exec 认证，以及安全提示“配置文件等同可执行代码”。
  - `<WP>/tmp/apic-p2-accept/`：验收脚本和输出，不进仓库。
- 动作：
  1. **B1**：脚本 `completion.sh` 依次执行：
     - 加载 `apic --gen-completion bash` 和 `apic link ls --completion bash` 的输出；
     - 设置 `COMP_WORDS` 和 `COMP_CWORD`，调用补全函数，断言 `COMPREPLY` 的内容；
     - 同时启动一个计数用的 httptest 服务和 exec 桩命令，断言两者的计数都为 0。
  2. **B2**：`go test ./internal/output/ ./internal/commands/ -run 'TestPick|TestRender|TestOutputModes' -count=1`。
  3. **B3**：
     - 用 big-v2.json 和 Jenkins 定义生成 skill，用 grep 确认文件中没有 base URL 和 `${env:`；
     - 在 tmp 目录下的 skillc 配置中执行 `skillc source add local` 和 `skillc install`（P7）。
  4. **B4**：
     - 在容器内运行 T04 的测试；
     - 在主机上：在容器里构建 `GOOS=windows` 的 apic.exe，以及 `tok.cmd` 桩命令，然后提交 job：`gofer job run -p my-tools-dev -a exec --runner local --sync --cwd tmp/apic-p2-accept -- cmd /c accept-exec.cmd`。`accept-exec.cmd` 依次验证：
       - `.cmd` 桩命令能取到 token，用 `--dry-run -v` 查看打码后的请求头；
       - 含 `&` 的参数会被拒绝，退出 1；
       - 输出各步的退出码。
     - 主机上只读写 `<WP>/tmp/apic-p2-accept/`。
  5. **B5**：
     - `go vet ./... && go test ./... -count=1 && go test ./internal/ -run TestLayering`；
     - 检查文件行数；
     - 用 v0.1.1 的验收配置跑一期的 dry-run 用例，对比输出；
     - 跑兼容性用例。
- 验证：各项的命令和输出摘要写进“进度记录”，完整输出放在 `<WP>/tmp/apic-p2-accept/`。
- 完成标准：B1–B5 全部通过，或明确标注 UNVERIFIED 并说明原因（P6、P7 缺失）。
- 依赖：T01–T07。

## 偏离登记

| 偏离 | 规则或设计条款 | 理由 |
|---|---|---|
| 补全模式下全局选项注册在命令本地，不走 SharedOpts | 一期设计“全局选项同时注册在 app 和 `<site>` 的 SharedOpts 上” | 设计 0.2 D1 已批准；只影响补全模式 |
| 保序解码与 pick 遍历放在 `model`（`jsonv.go`、`pick.go`），不放在 `output` | 设计“代码组织”中的 `internal/output/{jsonv,pick}.go` | `auth` 的 exec json 格式也要用；领域包之间不能互相依赖，只能共用 `model`。`output` 只负责调用和格式化 |

分层表：在一期的基础上加入 `internal/skillgen`，只能导入 `internal/model`；`internal/wiring` 和 `internal/commands` 可以导入它。

## 回滚与恢复

- 每个任务各自一个提交。回滚某个任务就 `git revert <该任务提交>`；恢复点是每个任务的提交号，写进“进度记录”。
- 不涉及数据迁移。测试只用 `t.TempDir()`，绝不读写真实的 `~/.config/apic`。
- 实施中遇到 Semantic Amendment（例如 P4 不成立）时，停在当前任务，回到 plan review，并请求人工批准。

## 人工 Gate

1. 计划批准：本文档。
2. 当前执行请求：计划批准后，由用户另行发出。
3. 外部动作：push、tag 和发布 v0.2.0 不在本计划内，完成后另行批准。

## 可追溯性

| 设计条款与验收 | 任务 | 验证 |
|---|---|---|
| §1 补全；D1、D10、D16 | T02、T06、T08 | T06 测试；B1 |
| §2 保序解码、pick、yaml、table；D3、D4、D5、D12 | T01、T05 | T01、T05 测试；B2 |
| 退出码 7；D11 | T01、T05 | T01、T05 测试；B2 |
| 兼容性：`pick` 参数、保留名 `skill`；D2、D15 | T02、T05 | T02、T05 测试；B5 |
| §3 skill；D6、D7 | T07、T08 | T07 测试；B3 |
| §4 exec；D8、D9、D13、D14 | T03、T04、T08 | T03、T04 测试；B4 |
| 安全一节：配置文件等同可执行代码、打码、净化 | T03、T04、T07、T08 | T03、T04、T07 测试；README |
| 工程规范 GR104、GR107、GR108、GR110、SR2002–SR2004 | 全部任务 | 分层测试；B5 |

## 完成 Gate 与剩余工作

- 完成条件：B1–B5 全部满足，或者用户接受其中标注为 UNVERIFIED 的项。
- 完成时在“进度记录”里写一段不超过 8 行的总结。
- 剩余工作：
  - 二期第二批：oauth2、`${keyring:}`/`${exec:}`、overlay、multipart、分页、同主机重定向跟随、`--auth-for-spec`、`site.go` 打码整理；
  - gcli 共享选项补全的 issue；
  - 发布 v0.2.0，需外部动作 Gate。

## 进度记录

（实施开始后填写）
