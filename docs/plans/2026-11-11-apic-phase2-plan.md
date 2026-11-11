<!-- template_id: plan; template_version: 1.3.0 -->
# apic 二期第一批实施计划

> 状态：Draft 0.2 / 已批准（用户 2026-11-11：“批准 apic 二期计划 0.2（接受未复核修订）”；0.2 为第 1 轮评审后的未复核修订；批准不授权实施，需另行发出当前执行请求）

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-11 | Claude（inhere：“批准 apic 二期设计 0.2（接受未复核修订）”） | 初稿：3 个波次、8 个任务，覆盖设计 0.2 的四项 |
| 0.2 | 2026-11-11 | Claude | 处理第 1 轮两轴评审（阻塞 P1、P2、Q1、Q2；纠正 P3–P10、Q3–Q13；建议若干）：预扫描自己补全顶层选项；快捷命令补全改为直接放站点名；主机 job 固定配置目录；补“spec/简单定义不能注入认证”测试；警告只在 Run 输出一次、补全时不输出；只登记 `${env:}` 解析出的值；不带 pick 的输出沿用一期；先缓冲再写 stdout；exec 读写两侧都拒绝零过期、超限算失败、元字符检查改纯函数；skill 写入下沉到 skillgen；W1/W2 按文件归属串行；基线由 v0.1.1 源码构建；zsh 实测、pwsh 主机实测；skillc 用隔离 HOME |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 规划工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：交付设计 0.2 的四项并通过验收，四项是补全、`--pick` 与 `-o yaml|table`（含退出码 7）、`apic skill`、exec 认证。
- Scope freeze：只做设计 0.2 的范围。设计“非目标”和二期第二批的内容实施中即使发现需要，也只记为 deferred。
- 评审预算：与设计相同，按高暴露处理（SR1409），做一轮两轴独立评审，一是正确性与可执行性，二是安全与契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。
- 评审状态：第 1 轮已完成，两个方向共提出 4 个阻塞项，0.2 已逐条修订。预算只有一轮，所以 0.2 属于未复核修订，见“评审处理”。

## 目标与完成定义

**目标**：v0.1.1 的用户升级后，配置不用改就能得到以下能力：

- 补全站点、操作、参数、全局选项及其取值，快捷命令也能补全；
- 用 `--pick` 取字段，用 `-o yaml|table` 换格式，数字和字段顺序保持原样；
- 用 `apic skill` 生成 skill；
- 配置 `type: exec` 认证。

行为符合设计 0.2 的各节和 D1–D16。

**完成定义（验收）**：

1. **B1 补全**：
   - 在容器里用真实 bash 和 zsh 加载 `--gen-completion <shell>` 和 `link ls --completion <shell>` 的输出；pwsh 在 Windows 主机上实测（P6）。
   - 以下各类各至少一个用例：
     - 设计“已确认事实 2”表中的每一行都得到期望候选，对 stdout 的完整候选做断言；
     - `--api <TAB>`、`jk<TAB>`（同时存在 jkx）、`--e<TAB>`、`--env<TAB>` 都只输出候选，不出现 `ERROR`；
     - 快捷命令 `petctl <TAB>`、`petctl --<TAB>`、`petctl --env <TAB>`、`petctl list-users --<TAB>`；
     - 用过期缓存时也能补出操作名；
     - 补全过程中 httptest 服务端收到 0 个请求，exec 桩命令执行 0 次；
     - 配置里有名为 `skill` 的站点时，补全的 stderr 为空。
2. **B2 输出**：经真实分发覆盖以下行为：
   - `--pick` 的 `*`、空数组、缺字段填 null、下标、路径不存在；
   - 字符串与 `-o` 的组合表；
   - 大整数 `12345678901234567891` 和 `<>&` 原样保留；
   - 键顺序保持；
   - 3xx 警告、4xx/5xx 原样输出；
   - 退出码 7（非 JSON、204、路径不存在、格式化失败），每个用例都断言 stdout 为空；
   - 发送前非法路径退出 2，服务端收到 0 个请求；
   - 不带 pick 时，auto/json/raw 遇到非 JSON 和 204，输出与 v0.1.1 逐字节一致、退出 0；
   - dry-run 不受 `--pick` 和 `-o` 影响。
3. **B3 skill**：
   - 对 A1 的真实 spec（30 个操作）和 Jenkins 简单定义分别生成 SKILL.md，frontmatter 能被 YAML 解析；
   - 埋入的敏感内容一律不出现：base URL、`source` URL、`${env:` 引用名、exec 命令、login body 中的字面凭证、static token；
   - 注入文本被净化；
   - 不带标记的文件拒绝覆盖，`--force` 才覆盖；
   - 目标文件或目录是符号链接时拒绝写入；
   - `--name ../x` 退出 2；
   - 用隔离的 `HOME`（P7）执行 `skillc source add local` 加 `skillc install` 能装上，真实的 skills 目录不变。
4. **B4 exec**：
   - 用桩命令覆盖 T04 验证清单中的全部行为。
   - Windows 主机上经 gofer job 验证：
     - `.cmd` 桩命令被执行过，有标记文件为证，`auth status` 显示已缓存；
     - 含元字符的参数被拒绝，且桩命令没有执行；
     - 不带扩展名、经 PATHEXT 解析到 `.cmd` 时同样会被检查。
5. **B5 工程与兼容性**：
   - `go vet ./...`、`go test ./... -count=1` 和分层守卫测试通过，单个文件不超过 1000 行（GR104）；
   - 用 v0.1.1 源码构建的基线二进制和新二进制，在 `cfg` 和 `lincfg` 两个验收配置上跑同一份 dry-run 命令清单，输出一致；
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
- 主机动作只有 T08 的 Windows 验证。主机 job 有三条约束：
  - 第一步就把 `APIC_CONFIG_DIR` 设为 `%~dp0cfg`；
  - 用 `apic config path` 断言配置目录在 `<WP>/tmp/apic-p2-accept/` 之下，否则立即退出；
  - 所有读写都在该目录之内，不需要真实凭证，exec 用的是桩命令。
- skillc 验收用隔离的 `HOME`，不写真实的 skills 目录。

- `host_or_non_offline_action=REQUIRED`

## 输入与批准证据

- 设计：`docs/design/2026-11-11-apic-phase2-design.md` Draft 0.2，已批准。
  - 批准原话（2026-11-11）：“批准 apic 二期设计 0.2（接受未复核修订）”。
  - 记录批准的提交：`b330f3c`。
- 设计第 1 轮评审的 C1–C11、S1–S19 已并入 0.2，见设计“评审处理”。本计划按 0.2 的条款执行，不重述理由。
- 一期计划与验收产物：
  - 计划：`docs/plans/2026-11-10-apic-phase1-plan.md`；
  - 验收产物：`<WP>/tmp/apic-accept/`，含 `cfg` 和 `lincfg`；
  - 真实 spec：`<WP>/tmp/apix-specs/`。
- 计划第 1 轮评审：可执行性方向的 P1–P10 与 S1–S11，安全方向的 Q1–Q13 与 S1–S5，见“评审处理”。
- 跟踪 issue：`my-tools-thv4`。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | 运行时 shell 补全 | gcli v3.8.6 `--in-completion` / `--gen-completion`（`completion.go`） | 是 | 补全模式下在 apic 内把全局选项注册到命令本地（设计 D1）；预扫描负责顶层选项的补全 | DIRECT_REUSE | none | 无 |
| CAP-02 | 保序、保留数字原文的 JSON 解码 | `encoding/json`（`Decoder.Token`、`UseNumber`）；goutil maputil（键顺序会丢，设计评审 C2/C4） | 是（stdlib） | 在 `internal/model` 写约 150 行的 token 流解码器 | OWNER_EXTENSION | maputil 和 `map[string]any` 会丢键顺序和数字精度 | 只在 `model` 内，`output` 和 `auth` 共用 |
| CAP-03 | pick 路径遍历 | goutil `maputil.GetFromAny`（`*` 的语义不符，设计评审 C3） | 否 | 在 `internal/model` 自写遍历，约 80 行，沿用 maputil 的点号语法 | OWNER_EXTENSION | 空数组判为不存在、丢掉缺字段的元素 | login 仍用 maputil；两处语法一致，不统一实现 |
| CAP-04 | YAML 输出 | goccy/go-yaml（已依赖，有 `MapSlice` 和 `MarshalYAML`） | 是 | 把保序值转成 `yaml.MapSlice`，数字类型实现 `MarshalYAML`，输出字面量 | DIRECT_REUSE | none | 无 |
| CAP-05 | 表格输出 | `text/tabwriter`；gookit/cliui | 是（stdlib） | 无 | DIRECT_REUSE | none | 不引入 cliui |
| CAP-06 | 执行外部命令、进程组、超时 | `os/exec`（`CommandContext`、`WaitDelay`）、`syscall.SysProcAttr{Setpgid}` | 是（stdlib） | 无 | DIRECT_REUSE | none | Windows 不做 Job Object（D14） |
| CAP-07 | token 缓存与 401 重试 | `internal/auth` 的 tokencache 与 Refresh、`internal/request` 的 401 路径 | 是 | 按认证类型计算指纹；读写两侧都拒绝零过期时间 | OWNER_EXTENSION | none | 与 login 共用 |
| CAP-08 | SKILL.md 渲染与安全写入 | `text/template`；goccy/go-yaml（frontmatter）；`os.CreateTemp` + `os.Rename` | 是 | 新领域包 `internal/skillgen` | MINIMAL_NEW_MODULE | 没有现成包能从 `model.Index` 生成 skill；放进 `commands` 会违反 GR108，放进 `output` 会混入不相关的职责 | 见 NM-01 |

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

1. **P1 工作树**：`git status --short` 为空，HEAD 至少包含本计划的批准提交。否则停下，查清归属。
2. **P2 Go 版本**：`go.mod` 写 `go 1.26.0`。容器内用 `GOTOOLCHAIN=auto`，得到 go1.26.0。
3. **P3 验收素材与基线**：
   - 以下四项存在：`<WP>/tmp/apix-specs/big-v2.json`、`<WP>/tmp/apic-accept/jenkins-def.yaml`、`<WP>/tmp/apic-accept/cfg`、`<WP>/tmp/apic-accept/lincfg`；缺失时按一期 T10 的步骤重建。
   - 基线二进制：`git archive v0.1.1 | tar -x -C <WP>/tmp/apic-p2-accept/base` 后构建 `base/apic`。不用 worktree，因为容器里不能对主机仓库执行 `git worktree add`。
4. **P4 补全实测（D1 与设计“待确认事项”第 1 条）**：
   - 评审已用探针证实：本地注册的选项在补全时可见，Choices 也生效。
   - T06 开工前再确认两点，探针放在 `<WP>/tmp/apic-p2-probe/`：
     - 预扫描在补全模式下输出顶层选项名并返回后，gcli 不会再输出任何内容；
     - bash 会把 `--api=pe` 拆成 `--api`、`=`、`pe` 三个词，预扫描能正确处理。
   - 不成立时停下，按 Semantic Amendment 报回。
5. **P5 YAML 写法**：评审已证实可行，数字类型实现 `MarshalYAML() ([]byte, error)` 后，`12345678901234567891` 会原样输出为字面量。T01 直接采用这个做法。
6. **P6 主机可用**：
   - T08 的 Windows 验证（exec、pwsh 补全）需要 gofer server 和本项目 worker 在线。
   - 主机上有 `pwsh` 就用它，否则用 Windows PowerShell 5 做 pwsh 补全实测；两者都不能用时标 UNVERIFIED_REAL_ENV。
   - 不在线时这些项标 UNVERIFIED_REAL_ENV，停在完成 Gate。
7. **P7 skillc**：
   - 容器里 `skillc` 可用，否则用 `go build` 构建 `inhere-tools/skillc`。
   - 运行时设 `HOME=<WP>/tmp/apic-p2-accept/home`，工作目录也在 tmp 下，安装目标指向 tmp。
   - 不可用时 B3 中 skillc 那一步标 UNVERIFIED，并给出替代证据：用 YAML 解析 frontmatter，并检查 skill 目录结构。
8. **P8 zsh 片段**：容器里有 `/usr/bin/zsh`。T06 开工前，用探针确认 zsh 下快捷命令补全片段（`compdef` 函数）的写法；pwsh 片段在 T08 的主机步骤中验证。

## 波次与依赖

| 波次 | 任务 | 依赖 |
|---|---|---|
| W1 基础 | T01 输出内核 ∥（T02 → T03） | T02 和 T03 都要改 `model/config.go`、`config/load.go`，所以串行；T01 与它们没有共同文件，可以并行 |
| W2 接线 | T04 → T05 → T06 → T07 | 串行，按文件归属：T04 依赖 T01、T03；T05 依赖 T01；T06 依赖 T02、T05，也要改 T04 改过的 `site.go`；T07 依赖 T01、T02 |
| W3 验收 | T08 | T01–T07 |

**文件归属**：同一文件按波次顺序由多个任务修改，后一个任务在前一个任务合并后的 main 上开工，避免 worktree 合并冲突。

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
  - **yaml**：`ToYAML(v any) ([]byte, error)`，Object 转 `yaml.MapSlice`，数字类型实现 `MarshalYAML` 输出字面量（P5）。
  - **table**：`Table(w io.Writer, v any, tty bool) (ok bool, err error)`，`ok=false` 表示需要退回 json。规则按设计 §2，包括终端下截断和去掉控制字符。
  - **Render**：
    ```go
    type RenderOptions struct{ Mode string; Pick *model.Pick; IncludeHeaders, TTY bool }
    func Render(res *Result, opt RenderOptions, stdout, stderr io.Writer, red *model.Redactor) (int, error)
    ```
    - **dry-run**：先走 Curl 分支，与一期一致。
    - **不带 pick 且模式为 auto/json/raw**：沿用一期的 `Format` 路径。非 JSON、空 body 时原样输出，退出 0，输出逐字节不变。
    - **2xx 且带 pick 或模式为 yaml/table**：先把结果写进缓冲区，pick 和格式化都成功后才写 stdout。失败时返回 `(ExitPostProcess, err)`，错误消息为 `HTTP <code> succeeded; <原因>`，stdout 为空。
    - **3xx**：原样输出；用了 pick 或 yaml/table 时在 stderr 警告。
    - **4xx/5xx**：原样输出，不 pick，不转换格式。
    - 写 stdout 失败时返回 `(ExitLocal, err)`。
  - **pick 结果的输出形式**：按设计 §2 的组合表。
- 验证：`go test ./internal/model/ ./internal/output/ -run 'TestDecode|TestPick|TestYAML|TestTable|TestRender' -count=1`，覆盖：
  - 大整数和 `<>&` 往返后不变；
  - 键顺序保持；
  - `*` 的四种情况：数组、空数组、缺字段、对象；
  - 下标越界；
  - 组合表中的每一格；
  - 3xx、4xx 的处理；
  - 非 JSON、空 body、table/yaml 格式化失败时返回 7，且 stdout 长度为 0；
  - 不带 pick 时，非 JSON 和 204 的输出与一期一致；
  - dry-run 不受影响。
- 完成标准：上述用例全部通过；`model` 只导入标准库，`output` 只导入 `model`（分层测试）。
- 依赖：无。

### T02 spec 只读缓存与保留名 skill

- 文件：
  - `internal/spec/cache.go` 及测试；
  - `internal/config/load.go` 及测试；
  - `internal/model/config.go`：新增 `Config.Warnings []string`（`yaml:"-"`）；
  - `internal/commands/run.go`：`managementCommands` 加 `skill`；输出 Warnings。
- 动作：
  - **只读缓存**：`IndexOptions.CacheOnly bool`。为 true 时：
    - 有缓存就直接返回，过期也不刷新；
    - 没有缓存就返回 `ExitLocal`；
    - 绝不调用 Fetch；
    - simple 来源不受影响，照常从定义文件加载。
  - **保留名**：`reservedNames` 加 `skill`。
    - 站点名命中保留名时，不再让整个 Load 失败：跳过该站点，并追加一条警告 `site "skill" uses a reserved name and is skipped; rename it`（D15）。
    - `site add` 遇到保留名仍然报错退出。
  - **警告输出**：只在 `commands.Run` 里把 `Config.Warnings` 写到 stderr，每次运行只写一次；补全模式下不写（由 T06 接入判断）。管理子命令自己再加载配置时不重复输出。
- 验证：`go test ./internal/spec/ ./internal/config/ ./internal/commands/ -run 'TestIndexCacheOnly|TestReservedSiteSkipped' -count=1`，覆盖：
  - CacheOnly 遇到过期缓存时服务端收到 0 次请求；
  - 没有缓存时返回错误；
  - simple 来源照常加载；
  - 名为 `skill` 的站点被跳过，警告只输出一次，其他站点照常加载；
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
    - `command` 不能为空；`format` 只能是 text 或 json；`timeout` 必须能解析成大于 0 的时长；`ttl` 如果填了，也必须能解析成大于 0 的时长。
  - **展开与敏感值**：`ResolveAuth` 先复制 `Exec.Command` 和 `Exec.Env`，再展开。
    - 只登记 `${env:}` 解析出的值，与一期按来源打码的做法一致；
    - 字面量 argv 元素不登记，比如 `token`、`--audience`。登记了会在无关输出中被过度打码，而 argv 不出现在任何输出中，由 T04 保证。
    - 这是对设计 D13 措辞的细化，D13 写的是“展开值”。
  - **apply**：`Apply` 字段的展开方式与 `Login.Apply` 相同。
  - **配置检查**：`config.Check` 为每个 exec 认证输出一条提示 `auth <site>.<name> runs <command[0]>`。`command[0]` 是字面量，不会被打码；如果 `command[0]` 本身是 `${env:}` 引用，就显示 `***`。
- 验证：`go test ./internal/config/ -run 'TestExecSpec|TestResolveAuthExec|TestCheckExec' -count=1`，覆盖：
  - 写成 `exec.apply` 时报未知键；
  - `ttl: 0` 报错；
  - 展开后 `d.Cfg` 中的切片和映射未被修改；
  - `${env:}` 解析出的值已登记为敏感值，字面量没有登记；
  - `config check` 能显示 `command[0]`；
  - 一期的 login 配置照常加载，覆盖顶层 `apply` 与 `login.apply` 并存的情况。
- 完成标准：上述用例全部通过。
- 依赖：T02（共用文件，串行）。

### T04 exec 认证提供者

- 文件：
  - `internal/auth/{exec.go,exec_unix.go,exec_windows.go,provider.go,tokencache.go}` 及测试；
  - `internal/auth/testdata/stub/`：桩命令源码；
  - `internal/wiring/wiring.go`：`Deps.ConfigDir`；
  - `internal/commands/auth.go`：login、logout、status；
  - `internal/commands/site.go`：`site show`、`config show` 中的 argv 遮盖。
- 动作：
  - **提供者接入**：
    - `New` 的类型白名单加 `exec`；
    - `Apply`、`Refresh` 增加 exec 分支；
    - `injectToken(req, token, apply ApplySpec)`：exec 用 `spec.Apply`，login 用 `spec.Login.Apply`。
  - **执行**：`runExec(ctx) (token string, expires time.Time, err error)`，行为按设计 §4：
    - stdin 为空设备，工作目录为 `Deps.ConfigDir`；
    - `command[0]` 是相对路径时，以配置目录为基准解析；Windows 上 `LookPath` 遇到 `exec.ErrDot` 时也按这个规则处理；
    - 子进程环境为当前环境加上 `exec.env`，再加上 `APIC_SITE`、`APIC_ENV`、`APIC_AUTH`；
    - stdout 和 stderr 各限读 4 MiB，超限视为失败，退出 1；
    - 超时取 `exec.timeout` 和请求 ctx 中先到的一个，`WaitDelay=2s`；
    - Unix 上 `Setpgid`，超时后 kill 整个进程组（`exec_unix.go`）。
  - **Windows 批处理检查**：纯函数 `checkBatchArgs(resolvedPath string, args []string) error` 放在不带构建标签的 `exec.go` 里：
    - 扩展名 `.bat`/`.cmd` 不区分大小写；
    - 元字符为 `& | < > ^ % ! "` 和换行；
    - 错误只给参数序号，不给参数值。

    `exec_windows.go` 用 `LookPath` 解析出的路径调用它；非 Windows 不调用。
  - **取 token**：
    - text 格式：要求单行且不含控制字符；
    - json 格式：用 T01 的 `model.Decode` 和 `model.ParsePick` 取值，取到的 token 同样要求单行、不含控制字符；
    - expires 小于等于 0 或无法解析时，退回 ttl。
  - **缓存**：
    - 只有过期时间已知时才写入，优先用 expires，其次 ttl；
    - exec 读缓存时，过期时间为零的条目一律当作无效；
    - `fingerprint()` 按类型分支计算：exec 哈希 type、base_url、展开后的 Exec 和 Apply；login 的指纹不变。
  - **状态**：`Status()` 改为返回 `StatusInfo{LoggedIn bool; ExpiresAt time.Time; Cacheable bool}`，调用方随之更新。
  - **失败**：
    - 一律返回 `ExitLocal`，消息为 `auth <name>: exec failed (exit N): <stderr 最后一行>`；
    - stderr 最后一行先去掉 ANSI 和控制字符，截断到 200 字符，再打码；
    - 消息里不出现 argv。
  - **命令层**：
    - `auth login` 对不能缓存的 exec 提示 `not cached`；
    - `auth logout` 清缓存；
    - `auth status` 显示 `not cached (no expiry)`；
    - `site show`、`config show` 中 `command` 只显示 `command[0]` 加上 `***`。
- 验证：`go test ./internal/auth/ ./internal/commands/ ./internal/wiring/ -run 'TestExec|TestAuthExec|TestShowMasksExec|TestNoAuthInjection' -count=1`。桩命令由测试从 `testdata/stub` 编译而来：会回显 argv、读 stdin、写标记文件、派生孙进程、输出超限内容，行为按参数决定。覆盖：
  - text 和 json 两种格式，多行或含控制字符的 token 被拒绝；
  - 有 expires、有 ttl、两者都没有、expires 小于等于 0；
  - 401 后重新执行命令，计数为 2；
  - 超时：`exec.timeout` 与请求 ctx 各自先到一次；Linux 上孙进程在超时后不存在；
  - stdin 立即得到 EOF；输出超过 4 MiB 时失败，且 apic 不卡住；
  - 工作目录是配置目录；能看到 `APIC_SITE`、`APIC_ENV`、`APIC_AUTH`；`exec.env` 生效；
  - **哨兵测试**：在 argv 中放一个字面量哨兵和一个 `${env:}` 哨兵，桩命令把 argv 回显到 stderr。以下所有输出中都不出现这两个哨兵：
    - 真实请求加 `-v`；
    - dry-run 的 curl 和 `-v`；
    - 非 0 退出、超时；
    - `checkBatchArgs` 的拒绝消息；
    - `auth login`、`auth status`、`auth logout`；
    - `config show`、`site show`、`config check`。
  - dry-run 时执行计数为 0；
  - 手写一个零过期的 exec 缓存条目，它被忽略，命令照常执行；
  - 改动 base_url、command、env、apply、format 中的任一项，指纹都会变化；
  - login 指纹与一期一致，用一期写入的缓存文件做回放；
  - `checkBatchArgs` 的表驱动测试：扩展名大小写、每个元字符、换行；
  - `auth login` 的 `not cached` 提示，`logout` 和 `status`；
  - **TestNoAuthInjection**（设计“安全”一节要求）：
    - `source.auth` 指向 exec 认证时退出 1，桩命令执行 0 次；
    - `site add` 写出的站点文件只有 source 和 envs；
    - 简单定义里写 `auths:` 不会产生任何认证。
- 完成标准：
  - 上述用例全部通过；
  - `GOOS=windows go build ./...` 和 `GOOS=darwin go build ./...` 通过。
- 依赖：T01、T03。

### T05 输出接线：`--pick`、`-o` 五种取值、发送前校验

- 文件：`internal/commands/{globalopts.go,siterun.go,prescan.go}` 及测试。
- 动作：
  - **全局选项**：
    - `globalOpts` 加 `Pick string`，`bind` 注册 `--pick`，不加短名；
    - `reservedOptNames` 加 `pick`；
    - `globalSpec()` 加 `"--pick": true`。
  - **输出模式**：`outputMode` 接受 auto、json、raw、yaml、table；错误消息列出这五种。
  - **siterun**：
    - 发送前调用 `model.ParsePick`，失败退出 2，不发请求；
    - 改用 `Render(res, RenderOptions{...})`；
    - 返回的错误经 `redactErr` 处理后原样返回，不再一律报成 `HTTP %d`；只有 4xx/5xx 仍报 `HTTP %d`。
- 验证：经真实分发运行 `go test ./internal/commands/ -run 'TestPick|TestOutputModes|TestPickParamFallback' -count=1`，覆盖：
  - B2 列出的各项；
  - 名为 `pick` 的参数只能通过 `-p pick=` 传入；
  - 退出码 7 的 stderr 文案，以及 stdout 为空。
- 完成标准：上述用例全部通过；一期的 `TestExitCodes` 等测试照常通过。
- 依赖：T01。

### T06 补全

- 文件：
  - `internal/commands/{prescan.go,run.go,dynamic.go,globalopts.go,link.go,site.go}` 及测试；
  - `internal/link/completion.go` 及测试。
- 动作：
  - **预扫描补全模式**：`Prescan` 识别 `--in-completion`，规则按设计 §1：
    - 最后一个词不参与校验，也不参与站点命中判断；
    - **顶层选项由预扫描自己补全**：最后一个词以 `-` 开头且前面没有位置参数时，预扫描直接输出匹配的全局选项名，返回 0，不再交给 gcli；
    - 最后一个词正好是某个取值型全局选项的全名时，同样处理。这样 gcli 解析 app 级选项时的报错就不会出现；
    - 前一个词是 `--api` 时，直接输出匹配的站点名，返回 0；
    - bash 会把 `--api=pe` 拆成 `--api`、`=`、`pe`，单独的 `=` 要能正确处理（P4）；
    - 预扫描出错时退回注册占位命令。
  - **注册**：在 `run.go` 中：
    - 补全模式下没有命中站点时，注册所有站点的占位命令，只有名字和描述；
    - 命中站点时，`loadIndex`（`site.go`）用 `CacheOnly` 读索引，所有错误都静默；
    - 补全模式下不输出 `Config.Warnings`。
  - **本地注册选项**：在 `dynamic.go` 中，补全模式下给站点命令和每个操作命令注册本地的全局选项和站点选项：
    - `--env`、`--auth` 的 Choices 取该站点的 env 名和 auth 名；
    - `-o` 的 Choices 按命令区分；
    - 参数选项的 Choices 取 `Param.Enum`。
    - 非补全模式的注册方式不变。
  - **快捷命令**：
    - `link ls --completion <bash|zsh|pwsh>`：调用 `link.CompletionScript(shell string, recs Records) (string, error)`；
    - **嵌入内容**：每条快捷命令只嵌入它的名字和站点。两者都要通过 `link.Names(site, name, "", "")` 校验，不合法的跳过；Windows 上名字去掉 `.cmd` 后缀。
    - **嵌入的命令名**：只用固定的 `apic`，不嵌入二进制路径。
    - **补全函数的改写**：把用户输入改写成 `apic --in-completion <site> <words>`，即把站点名作为第一个位置参数，用户输入的词全部排在它后面。不再使用 `--api`，也不带 `--env/--auth`：state 里本来就没有这两项，补全候选也不依赖它们。
    - `link` 和 `link rm` 成功时，在 stderr 提示重新生成补全。
  - **已知限制**（写进 README）：gcli 下钻时遇到选项值就停止，所以 `apic pet --env dev list-users --<TAB>` 只会给站点层的选项。这一点并入 gcli issue。
  - **gcli issue**：在工作区 tracker 立一个 issue，内容是“gcli 补全不包含祖先的共享选项；遇到选项值就停止下钻”，只立 issue。
- 验证：`go test ./internal/commands/ ./internal/link/ -run 'TestCompletion|TestPrescanCompletion|TestLinkCompletion' -count=1`，覆盖：
  - 设计“已确认事实 2”的表，对 stdout 的完整候选做断言；
  - `--e`、`--env`、`--api pe`、`--api=pe` 都不输出 `ERROR`，stderr 为空；
  - `jk` 与 `jkx` 同时存在；
  - 快捷命令改写后的四个用例（见 B1）；
  - httptest 服务端收到 0 个请求，exec 桩命令执行 0 次；
  - 配置里有名为 `skill` 的站点时，补全的 stderr 为空；
  - state 里有恶意的 site 或 name 值时，bash、zsh、pwsh 三种输出都跳过该条。

  bash、zsh、pwsh 的真实环境验证放在 T08 做。
- 完成标准：上述用例全部通过；非补全模式下一期的 `TestDynamic`、`TestPrescan` 照常通过。
- 依赖：T02、T04（`site.go`）、T05、P4、P8。

### T07 `apic skill`

- 文件：
  - `internal/skillgen/{skillgen.go,sanitize.go,write.go}` 及测试；
  - `internal/commands/{skill.go,app.go}` 及测试：注册命令；
  - `internal/layering_test.go`：加上 `internal/skillgen`，只依赖 `internal/model`；`wiring` 和 `commands` 可以导入它。
- 动作：
  - **接口**：
    ```go
    type Input struct{ Site string; Index *model.Index; Cfg *model.Site; Name string; MaxOps int }
    func Render(in Input) ([]byte, error)          // 完整 SKILL.md；frontmatter 由 goccy/go-yaml 编码
    func Sanitize(s string, max int) string        // 去控制字符和换行、转义反引号与 Markdown 标记、去 <!-- -->、截断
    func Write(parent, name string, data []byte, force bool) (path string, err error)
    const Marker = "<!-- generated by apic skill -->"
    ```
  - **Write**：
    - name 不符合 `^[a-z0-9][a-z0-9-]{0,63}$` 时返回 `ExitUsage`；
    - `<parent>/<name>` 目录或目标文件是符号链接时拒绝；
    - 目标已存在时，只有 frontmatter 之后的第一行正好是 Marker 才视为可覆盖，否则不加 force 就返回 `ExitLocal`；
    - 先写临时文件，再 rename。
  - **净化范围**：tag、summary、参数名、操作路径、操作名（简单定义的操作名来自 YAML 键，同样不可信）。
  - **正文**：内容按设计 §3 生成，正文使用英文。
  - **命令**：`skill <site> [--out] [--name] [--max-ops 60] [--force]`，只做参数解析，调用 Render 和 Write，然后在 stderr 输出两条提示：skillc 接入方式，以及“分享前检查”。
- 验证：`go test ./internal/skillgen/ ./internal/commands/ -run 'TestSkill' -count=1`，覆盖：
  - frontmatter 能被 YAML 解析，且 name 和 description 正确；
  - 以下字段中埋入注入文本时都被净化：summary、tag、操作路径、简单定义的操作名。注入文本包括 `---`、换行、`<!--`、反引号和“run … --yes”；
  - 文件中不出现以下埋入内容：base URL、source URL、`${env:` 引用名、exec 的 command、login body 中的字面凭证、static token；
  - 超过 `--max-ops` 时按 tag 分组，含 `untagged`；
  - 覆盖：正文里引用了 Marker 但第一行不是 Marker 的文件被拒绝；`--force` 可以覆盖；
  - 目标文件是符号链接、`<name>` 目录是符号链接两种情况都被拒绝；
  - `--name ../x` 退出 2。
- 完成标准：上述用例全部通过；分层测试通过。
- 依赖：T01、T02。

### T08 验收、README 与工程检查

- 文件：
  - 本计划：只改“进度记录”。
  - `README.md`、`README.zh-CN.md`：
    - 补全，含已知限制；
    - `--pick`/`-o` 与退出码 7；
    - `apic skill`；
    - exec 认证：配置文件等同可执行代码；Windows 上 0600 权限不生效；`command[0]` 用绝对路径，或以配置目录为基准的相对路径。
  - `docs/release-notes/v0.2.0.md`：发布说明草稿，写明三点兼容性变化：`pick` 参数、保留名 `skill`、退出码 7。
  - `<WP>/tmp/apic-p2-accept/`：验收脚本、基线二进制和输出，不进仓库。
- 动作：
  1. **B1**：
     - 准备一个缓存已过期的配置；
     - 用脚本 `completion.sh`（bash）和 `completion.zsh`（zsh）分别：
       - 加载两段补全输出，即 `--gen-completion` 和 `link ls --completion`；
       - 模拟按 TAB：bash 设置 `COMP_WORDS` 和 `COMP_CWORD` 后调用补全函数，zsh 用 `zpty` 或直接调用补全函数，断言候选；
     - 同时启动计数用的 httptest 服务和 exec 桩命令，断言两者的计数都为 0；
     - pwsh 并入第 4 步的主机 job。
  2. **B2**：`go test ./internal/model/ ./internal/output/ ./internal/commands/ -run 'TestPick|TestRender|TestOutputModes' -count=1`。
  3. **B3**：
     - 用 big-v2.json 和 Jenkins 定义生成 skill，用 grep 确认文件中没有埋入的敏感内容；
     - 在 `HOME=<WP>/tmp/apic-p2-accept/home` 下执行 `skillc source add local` 和 `skillc install`；
     - 断言容器中真实的 `~/.claude/skills` 和 `~/.config/skillc` 都没有变化（P7）。
  4. **B4 与 pwsh**：
     - 在容器内运行 T04 的测试。
     - 在容器里构建 `GOOS=windows` 的 `apic.exe`，并准备 `tok.cmd` 桩命令，它会写一个标记文件后输出 token。
     - 提交主机 job：`gofer job run -p my-tools-dev -a exec --runner local --sync --cwd tmp/apic-p2-accept -- cmd /c accept-exec.cmd`。`accept-exec.cmd` 依次：
       1. `set APIC_CONFIG_DIR=%~dp0cfg`，执行 `apic config path` 并断言结果在 `tmp\apic-p2-accept` 之下，不符合就退出；
       2. 配置 exec 认证，`command` 用 `tok.cmd` 的绝对路径，并设置 `ttl`；
       3. 执行 `auth login`，断言标记文件存在、`auth status` 显示已缓存；
       4. 执行 `--dry-run`，断言请求头显示为 `***`；
       5. 参数含 `&` 时退出 1，且标记文件计数没有增加；
       6. 再测一次不带扩展名、经 PATHEXT 解析到 `.cmd` 的情况，同样被检查；
       7. 用 `pwsh` 加载补全，没有就用 `powershell` 加载 `--gen-completion pwsh` 和 `link ls --completion pwsh`，调用补全函数并断言候选；
       8. 输出各步的退出码。
     - 主机上只读写 `<WP>/tmp/apic-p2-accept/`。
  5. **B5**：
     - `go vet ./... && go test ./... -count=1 && go test ./internal/ -run TestLayering`；
     - 检查文件行数；
     - 用基线二进制（P3）和新二进制，在 `cfg` 和 `lincfg` 上跑同一份 dry-run 命令清单，对比输出。清单写在 `<WP>/tmp/apic-p2-accept/compat.txt`；
     - 跑兼容性用例。
- 验证：各项的命令和输出摘要写进“进度记录”，完整输出放在 `<WP>/tmp/apic-p2-accept/`。
- 完成标准：B1–B5 全部通过，或明确标注 UNVERIFIED 并说明原因（P6、P7 缺失）。
- 依赖：T01–T07。

## 偏离登记

| 偏离 | 规则或设计条款 | 理由 |
|---|---|---|
| 补全模式下全局选项注册在命令本地，不走 SharedOpts | 一期设计“全局选项同时注册在 app 和 `<site>` 的 SharedOpts 上” | 设计 0.2 D1 已批准；只影响补全模式 |
| 保序解码与 pick 遍历放在 `model`（`jsonv.go`、`pick.go`），不放在 `output` | 设计“代码组织”中的 `internal/output/{jsonv,pick}.go` | `auth` 的 exec json 格式也要用；领域包之间不能互相依赖，只能共用 `model`。`output` 只负责调用和格式化 |
| 快捷命令补全改写成 `apic --in-completion <site> <words>`，不再用 `--api`，也不带 `--env/--auth` | 设计 §1“改写成 `apic --in-completion --api pet <words>`” | 评审 P2 实测：预扫描会把站点名插到所有前导选项之后，用 `--api` 会补出错误候选；state 里也没有 env/auth。补全结果与设计目标一致 |
| 只登记 `${env:}` 解析出的值，字面量 argv 不登记 | 设计 D13“argv 和 env 展开值都登记为敏感值” | 细化 D13 的措辞：字面量不是凭证，登记后会在无关输出中被过度打码；argv 不出现在任何输出中，由 T04 的哨兵测试保证 |
| 顶层选项的补全由预扫描输出，不交给 gcli | 设计 §1“沿用 gcli 的运行时补全” | 评审 P1 实测：gcli 解析 app 级选项时出错会把错误打到 stdout |

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
| 兼容性：`pick` 参数、保留名 `skill`、不带 pick 的输出不变；D2、D15 | T01、T02、T05 | T01、T02、T05 测试；B5 |
| §3 skill；D6、D7 | T07、T08 | T07 测试；B3 |
| §4 exec；D8、D9、D13、D14 | T03、T04、T08 | T03、T04 测试；B4 |
| 安全一节：spec 和简单定义不能注入认证 | T04 | `TestNoAuthInjection` |
| 安全一节：配置文件等同可执行代码、打码、净化 | T03、T04、T07、T08 | T03、T04、T07 测试；README |
| 工程规范 GR104、GR107、GR108、GR110、SR2002–SR2004 | 全部任务 | 分层测试；B5 |

## 评审处理（第 1 轮）

| 评审项 | 处理 |
|---|---|
| P1 顶层选项补全报错到 stdout | T06 由预扫描补全顶层选项；断言完整候选；新增偏离登记 |
| P2 快捷命令补全改写错误 / Q11 eval 校验 | 改写成 `<site> <words>`，只嵌入名字和站点并校验；新增偏离登记；新增四个用例 |
| Q1 主机 job 配置目录 | 授权一节与 T08 固定 `APIC_CONFIG_DIR` 并断言 |
| Q2 / P5 不能注入认证的测试 | T04 `TestNoAuthInjection` |
| P3 `.cmd` 是否执行过 | B4 用标记文件、`auth login/status` 证明 |
| P4 / Q3 警告重复、补全时输出 | T02 只在 Run 输出一次；T06 补全时不输出 |
| P6 zsh/pwsh | P8 探针；B1 zsh 实测；pwsh 主机实测 |
| P7 写入逻辑分层 | `skillgen.Write`；T07 加入 `app.go` |
| P8 并行改同一文件 | W1 中 T02→T03；W2 串行；T06 加入 `site.go` |
| P9 / Q5 / Q6 / Q7 / Q8 exec 测试缺项 | T04 验证清单补齐；`checkBatchArgs` 改纯函数；超限算失败；读写两侧都拒绝零过期 |
| P10 基线 | P3 用 `git archive v0.1.1` 构建 |
| Q4 / 正确性 S2 过度打码 | 只登记 `${env:}` 解析出的值；新增偏离登记 |
| Q9 / Q10 输出兼容与 stdout 为空 | T01 不带 pick 沿用一期路径；先缓冲再写 |
| Q12 skill 测试 | T07 补符号链接、净化范围、埋入敏感内容 |
| Q13 / 正确性 S4 skillc 隔离 | P7 与 B3 用隔离的 HOME |
| 安全建议 S1–S5、正确性建议 S1、S3、S5–S11 | 均已采纳，见各任务 |

## 完成 Gate 与剩余工作

- 完成条件：B1–B5 全部满足，或者用户接受其中标注为 UNVERIFIED 的项。
- 完成时在“进度记录”里写一段不超过 8 行的总结。
- 剩余工作：
  - 二期第二批：oauth2、`${keyring:}`/`${exec:}`、overlay、multipart、分页、同主机重定向跟随、`--auth-for-spec`、`site.go` 打码整理；
  - gcli 补全的 issue；
  - 发布 v0.2.0，需外部动作 Gate。

## 进度记录

（实施开始后填写）
