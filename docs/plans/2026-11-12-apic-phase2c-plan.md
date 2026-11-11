<!-- template_id: plan; template_version: 1.3.0 -->
# apic 二期第三批（试用反馈修正）实施计划

> 状态：Draft 0.1 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-12 | Claude（inhere：“批准 apic 二期第三批设计 0.2（接受未复核修订）”） | 初稿：3 个波次、7 个任务，覆盖设计 0.2 的五项修正 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 规划工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：交付设计 0.2 的五项并通过验收：环境声明的认证、dry-run 容忍凭证变量、401 后作废 token、`--pick` 投影、简单定义 `enum`。
- Scope freeze：只做设计 0.2 的范围。设计“非目标”（`--columns`、清除保存选择的命令、变量默认值语法等）实施中即使发现需要，也只记为 deferred。
- 评审预算：与设计相同，按高暴露处理（SR1409），一轮两轴独立评审：正确性与可执行性；安全与契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。

## 目标与完成定义

**目标**：v0.3.0 的用户升级后，按设计 0.2 的“用户可见的结果”工作，行为符合设计各节与 D1–D8。

**完成定义（验收）**：

1. **B1 环境声明的认证**，经真实分发（`commands.Run`）与 httptest 服务端（记录收到的认证头）覆盖：
   - 优先级矩阵：`--auth` > `envs.<env>.auth` > 保存的认证 > `default_auth`，四个来源各一例，外加都没有时不认证；
   - S1 场景：`use dh --auth build` 后 `dh me --env factory`，服务端收到的是 factory 的 token；`use` 保存时 stderr 出现设计中的 note；
   - factory 认证 `allow_methods: [GET, HEAD]` 时，保存过 build 认证后对 factory 发 POST 仍被写保护拒绝（退出 6）；
   - link 固定 `--env factory`、不带 `--auth` 时用 factory 的认证；
   - `use` 输出 `auth: <name> (flag|env|saved|default)`；`auth ls` 的“当前”标记按 saved env 或 `default_env` 计算；显示类命令遇到写错的名字不报错；
   - `envs.<env>.auth` 指向不存在的认证时调用退出 1，消息为 `site "dh" env "factory": no auth "x"`；
   - `config check` 对写错的 `envs.*.auth` 与 `default_auth` 各给一条提示，退出码 0；
   - `apic skill` 输出各环境使用的认证，并含“显式 `--auth` 优先”的说明；注入换行的认证名被 `Sanitize`。
2. **B2 dry-run 容忍凭证变量**：
   - bearer、apikey（header 与 query）、basic、login（`login.body`）、exec（`exec.env`、`exec.command`）的凭证字段缺变量：dry-run 退出 0，stderr 每个变量一行警告，同名只一行；真实调用退出 1；
   - 同一字符串中一个已设、一个未设：已设的值在输出中显示为 `***`；
   - 非凭证字段（`apply.header`、`login.path`）缺变量：dry-run 仍退出 1；
   - `auth status <site> --env X` 缺变量退出 1；
   - exec 的 dry-run 不执行命令（命令会写标记文件，断言文件不存在）；dry-run 前后 `cache/` 目录的文件名与内容不变；
   - `source.auth` 拉取定义时缺变量仍退出 1。
3. **B3 401 作废**：
   - login 与 exec 认证，服务端始终 401：调用退出 4，之后 token 缓存文件不存在，`auth status` 显示 logged out；
   - 首次 401、重取后 200：缓存保留新 token；
   - 静态认证 401：不改动任何文件；
   - `Invalidate` 返回错误时，stderr 出现一行脱敏后的警告，退出码仍为 4（request 包单测，用替身）。
4. **B4 `--pick` 投影**：
   - 单测表：根对象投影、`*` 下投影、键序与缺键 `null`、字面键 `a,b` 优先、中间段含逗号按普通键、空键或重复键退出 7、对非对象（无 `*`）退出 7、`*` 下非对象为 `null`、投影键 `*` 与数字是字面键；
   - 现有 `pick_test.go` 用例全部不改期望；
   - 经 `commands.Run`：`--pick data.list.*.id,name -o table` 的表头为 `id  name`；含非对象元素时回退 JSON 并有 note。
5. **B5 简单定义 `enum`**：枚举生效（非法值调用退出 1，`desc` 显示枚举）；`enum: [1, true]` 按 `"1"`、`"true"` 比较；空字符串、重复、`enum: []` 使站点加载失败、退出 1。
6. **B6 工程与兼容性**：
   - `go vet ./...`、`GOOS=windows go vet ./...`、`go test ./... -count=1`、分层守卫测试通过；单个文件不超过 1000 行（GR104）；
   - Windows 主机上 `go test ./internal/... -count=1` 通过；
   - 兼容：v0.3.0 基线二进制与新二进制各用一份复制的二期第二批 compat 配置，跑同一份 dry-run 清单，输出逐字节一致（清单中如有 `use`，其 `auth:` 行的来源后缀是预期差异，单独列出）；
   - **real-env**（主机）：沙箱配置中 gitlab 站点加第二个环境 `anon`（同一 base_url，`auth: anon`），`gitlab version`（main 环境）返回版本、`gitlab version --env anon` 返回 401，`use gitlab --auth pat` 之后 `--env anon` 仍为 401（环境声明优先）；`.env` 去掉 `GITLAB_TOKEN` 的副本上 `gitlab version --dry-run` 退出 0 并警告。

## 范围、排除项与授权

**范围**：仓库 `inhere-tools/apic`（Git root）下的代码、README 和 docs。

**排除项**：

- 设计 0.2 的非目标；二期其余 deferred 项。
- 修改用户的正式配置目录 （`apic config path` 的默认位置）：不在本计划内；real-env 只用 `<WP>/tmp/apic-p2c-accept/` 下的副本。
- push、tag、发布：属于外部动作，需要另行授权。

**授权**：

- 计划批准后，还需要用户另行发出当前执行请求，才能开始实施。
- 按功能点本地提交是默认授权。
- 主机动作只有 T07 的 Windows `go test` 与 real-env 试用：
  - 源码用 `git archive HEAD` 导出到 `<WP>/tmp/apic-p2c-accept/src`，不在主机建 worktree，不动主机上的仓库；
  - real-env 用 `APIC_CONFIG_DIR=<WP>/tmp/apic-p2c-accept/cfg`（从 `<WP>/tmp/apic-try/cfg` 复制，含用户提供的 `GITLAB_TOKEN`），只发 GET 请求到内部 GitLab；
  - 除 Go 构建缓存外，只读写 `<WP>/tmp/apic-p2c-accept/`。

- `host_or_non_offline_action=REQUIRED`

## 输入与批准证据

- 设计：`docs/design/2026-11-12-apic-phase2c-design.md` Draft 0.2，已批准。
  - 批准原话（2026-11-12）：“批准 apic 二期第三批设计 0.2（接受未复核修订）”。
  - 记录批准的提交：`de6600a`。
- 设计第 1 轮评审（C1–C10、S1–S10）已并入 0.2，见设计“评审处理”。本计划按 0.2 的条款执行，不重述理由。
- 二期第二批验收产物（只读）：`<WP>/tmp/apic-p2b-accept/compat.sh` 与 `compat/`；清单 `<WP>/tmp/apic-p2-accept/compat.txt`。
- 试用沙箱：`<WP>/tmp/apic-try/cfg`。
- 跟踪 issue：`my-tools-qm4t`（父 issue `my-tools-thv4`）。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | 有效认证计算 | `wiring.Resolve`、`use.go` 的 `firstOf`、`auth.go` 的 `listAuths` | 部分 | 在 `model.Site` 加纯函数，三处改用它 | OWNER_EXTENSION | 三处各自 `firstOf`，skillgen 不能依赖 wiring | 去掉三份重复逻辑 |
| CAP-02 | 凭证字段容忍未设变量 | `config.Expand`/`ResolveAuth` | 部分 | `config/expand.go` 内部加容忍参数 | OWNER_EXTENSION | `Expand` 遇首个缺失即整串失败 | 无 |
| CAP-03 | 作废 token | `Provider.Logout` | 是 | `Invalidate` 调用 `Logout` | DIRECT_REUSE | none | 无 |
| CAP-04 | 投影 | `model.Pick` | 部分 | `model/pick.go` 原位扩展 | OWNER_EXTENSION | 每段只能是键、下标、`*` | 无 |
| CAP-05 | 简单定义枚举 | `simpleParam`、`model.Param.Enum`、调用时的枚举校验 | 部分 | `spec/simple.go` 加字段与校验 | OWNER_EXTENSION | 没有 `enum` 字段 | 无 |

### New module candidates

| candidate_id | capability_id | proposed_module | searched_candidates | direct_reuse_gap | thin_adapter_or_owner_extension_gap | proven_gap | unique_owner_and_lifecycle | deletion_or_merge_handling |
|---|---|---|---|---|---|---|---|---|
| NONE | none | none | none | none | none | none | none | none |

### Rejected new tools

| rejected_candidate | capability_id | deletion_test_and_reason |
|---|---|---|
| `--columns` 选项 | CAP-04 | 删掉它不缺能力：投影已覆盖选列与定列序（设计 D6） |
| JMESPath / gjson 一类查询库 | CAP-04 | 删掉它不缺能力；只需最后一段投影，现有 `Pick` 原位扩展即可 |

## 前置检查与 fail-closed 条件

运行目录均为 `inhere-tools/apic`。

1. **P1 工作树**：`git status --short` 为空，HEAD 至少包含本计划的批准提交。否则停下，查清归属。
2. **P2 Go 版本**：`go.mod` 写 `go 1.26.0`，容器内 `GOTOOLCHAIN=auto` 得到 go1.26.x；基线 `go test ./... -count=1` 通过。
3. **P3 基线**：`git archive v0.3.0 | tar -x -C <WP>/tmp/apic-p2c-accept/base` 后构建 `base/apic`（不用 worktree）；把 `<WP>/tmp/apic-p2b-accept/compat/base` 复制到 `<WP>/tmp/apic-p2c-accept/compat/{base,new}/`。
4. **P4 主机可用**：T07 需要 gofer server 和本项目 worker 在线，否则 B6 的 Windows 与 real-env 项标 `UNVERIFIED_REAL_ENV`，停在完成 Gate。
5. **P5 GitLab 可达**：主机 `<内部 GitLab base_url>/projects?per_page=1`（匿名请求） 返回 200；否则 real-env 的 GitLab 项标 `UNVERIFIED_REAL_ENV`。

## 波次与依赖

| 波次 | 任务 | 依赖 |
|---|---|---|
| W1 领域 | T01 → T02；T03；T04；T05 | T02 用 T01 的 `Env.Auth`；其余互不依赖 |
| W2 接线 | T06 | T01、T02、T04 |
| W3 验收 | T07 | T01–T06 |

**执行方式**：

- 实施方：sonnet 子 agent，在主 checkout 中**串行**执行 T01 → T02 → T03 → T04 → T05 → T06（T01 与 T04 同在 `model` 包，T03/T05 的测试会编译 `model`，并行容易互相看到半成品）；T07 我自己做。
- 我审查每个任务的 diff（先看 `git diff --stat` 的删除行），grep 调用点确认接线，并做验收。
- 任务书写明：分层规则（model 无依赖；config/spec/request/output/auth/skillgen 只依赖 model）与 idevstd；禁止删注释和无关重排；测试不在 export 了 `GIT_DIR` 的 shell 中运行；只提交本任务的文件（提交前查暂存区）；用 `--sync` 等待，不起空等后台任务。
- 每个任务一次原子提交（`feat(<pkg>): ... (P2C-Tnn)`）。
- 用 gofer plan 跟进，每个任务一个 todo。

## 任务

所有命令都在 `inhere-tools/apic` 下运行，并带上 `GOFLAGS=-buildvcs=false GOWORK=off GOTOOLCHAIN=auto`。

### T01 环境声明的认证：模型与有效认证函数

- 文件：`internal/model/config.go`、`internal/model/config_test.go`（新）。
- 动作：
  - `Env` 加 `Auth string \`yaml:"auth,omitempty"\``。
  - 新增：
    ```go
    // 来源取值
    const (AuthFromFlag = "flag"; AuthFromEnv = "env"; AuthFromSaved = "saved"; AuthFromDefault = "default")
    // EffectiveAuth 按 flag > envs[env].Auth > saved > DefaultAuth 返回认证名与来源；都为空时返回 ("", "")。
    // 不校验名字是否存在（显示类命令要容忍写错的名字）。env 不存在时跳过环境一级。
    func (s *Site) EffectiveAuth(env, saved, flag string) (name, source string)
    ```
- 验证：`go test ./internal/model/ -run TestEffectiveAuth -count=1`，表驱动覆盖四个来源、全空、env 不存在、环境声明为空串时落到 saved。
- 完成标准：用例通过；`model` 只导入标准库。
- 依赖：无。

### T02 凭证字段容忍未设变量；config check 名字提示

- 文件：`internal/config/expand.go`、`internal/config/check.go` 及测试。
- 动作：
  - 新增，原 `ResolveAuth` 签名与行为不变（严格）：
    ```go
    // ResolveAuthDryRun 与 ResolveAuth 相同，但凭证字段（Token、Password、Value、Exec.Command、
    // Exec.Env、Login.Body）中的未设变量保留原文 ${env:NAME}、不登记为密钥；同一字符串中的
    // 已设变量照常展开并登记。返回去重、按首次出现顺序的未设变量名。其他字段缺变量仍返回错误。
    func ResolveAuthDryRun(site *model.Site, name string, env *Env, sec *model.Secrets) (model.AuthSpec, []string, error)
    ```
    两者共用一个内部实现；`Expand` 拆出内部函数，容忍模式下逐个处理同一字符串里的全部引用。
  - `Check` 追加提示（文案固定）：`site <s>: default_auth "<x>" is not defined`；`site <s> env <e>: auth "<x>" is not defined`。站点、环境按名字排序输出。
- 验证：`go test ./internal/config/ -run 'TestResolveAuthDryRun|TestResolveAuth|TestCheckAuthNames' -count=1`，覆盖六类凭证字段各缺一个变量、同串一已设一未设（已设值已登记：`sec` 能脱敏它）、重复变量只返回一次、`Apply.Header` 缺变量返回错误、严格版行为不变。
- 完成标准：用例通过；现有 `config_test.go` 用例不改期望。
- 依赖：T01。

### T03 401 重发后作废 token

- 文件：`internal/request/execute.go`、`internal/request/request_test.go`（`fakeAuth` 补方法与新用例）、`internal/auth/provider.go`、`internal/auth/auth_test.go` 或 `exec_test.go`。
- 动作：
  - `Authorizer` 接口加 `Invalidate() error`。
  - `Execute`：把最后的 `return c.send(client, req)` 改为先取响应；响应状态仍为 401 时调用 `c.Authorizer.Invalidate()`，出错且 `c.Stderr != nil` 时写 `warning: ` + `c.Redactor.Redact("drop cached token: " + err.Error())`；然后返回该响应。
  - `Provider.Invalidate`：类型为 login 或 exec 时 `return p.Logout()`，否则 `return nil`。不在已持锁的代码路径中调用（`Logout` 自己加锁）。
- 验证：`go test ./internal/request/ -run TestRetry -count=1` 与 `go test ./internal/auth/ -run TestInvalidate -count=1`，覆盖 B3 各项：重发仍 401 调用一次 Invalidate；重发 200 不调用；静态认证（`Refresh` 返回 `ErrNoRefresh`）不调用；Invalidate 出错时的警告与响应不变；login/exec 的缓存文件被删、静态认证无操作。
- 完成标准：用例通过；`request` 只导入 `model`。
- 依赖：无。

### T04 `--pick` 投影

- 文件：`internal/model/pick.go`、`internal/model/pick_test.go`。
- 动作：
  - `ParsePick` 不变（逗号不报错）。`applyPick` 在处理**最后一段**且该段含逗号时：
    1. 当前值是 `Object` 且有与整段同名的键：按普通键取值；
    2. 否则按逗号拆分；有空键或重复键：返回 `ExitPostProcess` 错误 `--pick projection %q has an empty or duplicate key`；
    3. 当前值是 `Object`：按给出顺序构造新 `Object`，`Keys` 含全部键，缺少的值为 `nil`；
    4. 当前值不是 `Object`：返回 `ExitPostProcess` 错误 `--pick projection %q needs an object; add .* before it for arrays`（在 `*` 之下该错误会按现有规则被吞成 `nil`）。
  - 中间段含逗号：保持现有普通键处理。
- 验证：`go test ./internal/model/ -run TestPick -count=1`，覆盖 B4 的单测表；现有用例不改期望。
- 完成标准：用例通过。
- 依赖：无（与 T01 同包，串行执行）。

### T05 简单定义 `enum`

- 文件：`internal/spec/simple.go`、`internal/spec/simple_test.go`（若无则新建）。
- 动作：
  - `simpleParam` 加 `Enum []any \`yaml:"enum"\``（用 `any` 接住数字与布尔），转换为字符串：字符串原样，数字用 YAML 原文形式（整数不带小数点），布尔为 `true`/`false`；其他类型（对象、数组、null）报错。
  - 校验：`enum` 键存在但为空列表、元素为空字符串、元素重复：`ExitLocal`：`<文件> operation <name> param <p>: enum must be a non-empty list of distinct non-empty values`。
  - 写入 `model.Param.Enum`。
- 验证：`go test ./internal/spec/ -run TestSimpleEnum -count=1`，覆盖：字符串枚举、`[1, true]` 得到 `["1","true"]`、`[]`、`[""]`、重复、对象元素各报错、未写 `enum` 时 `Enum` 为 nil。
- 完成标准：用例通过；`spec` 只导入 `model`。
- 依赖：无。

### T06 接线：选择、dry-run 警告、显示、skill、README

- 文件：`internal/wiring/wiring.go`、`internal/commands/siterun.go`、`internal/commands/use.go`、`internal/commands/auth.go`、`internal/skillgen/skillgen.go`、README.md、README.zh-CN.md 及测试（`internal/commands/envauth_test.go`，新；`internal/skillgen/skillgen_test.go`；`internal/commands/pick_test.go` 追加投影的端到端用例；`internal/wiring` 没有测试文件，选择逻辑经 commands 端到端覆盖）。
- 动作：
  - **Resolve**：`Selection` 加 `AuthSource string`；`Auth, AuthSource = site.EffectiveAuth(sel.Env, saved.Auth, authFlag)`；认证名不存在时：来源为 env 报 `site %q env %q: no auth %q`，其他来源沿用 `site %q has no auth %q`；均为 `ExitLocal`。
  - **NewCall**：签名改为 `NewCall(...) (*request.Call, []string, error)`，第二个返回值为 dry-run 下的未设变量名（非 dry-run 恒为 nil）；`fl.DryRun` 时 `newProvider` 改用 `config.ResolveAuthDryRun`。`NewAuth` 与 `FetchOptions` 的 `source.auth` 不变（严格）。
  - **siterun.go**：拿到未设列表后，在执行前向 stderr 每个写一行 `warning: dry-run: <NAME> is not set; a real call would fail`。
  - **use.go**：`printSelection` 的 auth 行写 `auth: <name> (<source>)`，无认证时保持 `auth: ` 原样；两处 `firstOf` 改用 `EffectiveAuth(env, saved.Auth, "")`；`use --auth x` 保存后，若当前环境（saved env 或 `default_env`）声明了不同的认证，stderr 写设计中的 note。
  - **auth.go**：`listAuths` 的当前标记改用 `EffectiveAuth(firstOf(saved.Env, site.DefaultEnv), saved.Auth, "")`。
  - **skillgen**：环境列表每行追加 `; auth <Sanitize(name)>`（有声明时）；认证一节加一句 `An explicit --auth overrides the environment's auth; pass --env explicitly.`；How to call 加投影示例 `--pick data.items.*.id,name`。
  - **README**（中英文）：`envs.<env>.auth` 与优先级；dry-run 缺变量的表现（`***`、URL 编码、`<exec-token>`、以警告为准）；401 作废；投影；简单定义 `enum`。
- 验证：`go test ./internal/commands/ -run 'TestEnvAuth|TestDryRunMissing|TestPickProjection|TestInvalidateE2E|TestSimpleEnumE2E' -count=1` 与 `go test ./internal/skillgen/ -count=1`，经 `commands.Run` 覆盖 B1、B2、B3（login/exec 端到端）、B4 的端到端项和 B5。
- 完成标准：用例通过；`NewCall` 只有 siterun 一个调用方；`grep -n 'firstOf(saved.Auth' internal/commands` 无结果。
- 依赖：T01、T02、T03、T04、T05。

### T07 验收、兼容性与发布说明草稿

- 文件：`docs/release-notes/v0.4.0.md`（新）、本计划的进度记录；验收产物在 `<WP>/tmp/apic-p2c-accept/`。
- 动作：
  - 跑 B6 的检查；按 P3 跑 compat（复用二期第二批 `compat.sh` 的写法，路径改到本批目录）；
  - 主机 job：`git archive HEAD` 导出源码，跑 Windows `go test`；复制沙箱配置、加 `anon` 环境，跑 B6 的 real-env 命令；
  - 写发布说明草稿：新功能、行为变化（设计“行为变化”四条，对照 README 退出码表）、回滚（先删 `envs.*.auth` 或显式 `--auth`）；
  - 进度记录写完成总结；评论 issue `my-tools-qm4t`。
- 验证：B1–B6 全部有证据（测试名或 job 输出路径）。
- 完成标准：B6 各项通过或按 P4/P5 披露 `UNVERIFIED_REAL_ENV`；发布说明经 `idevstd` 无需验证（非受治理文档），由我对照设计核对条目齐全。
- 依赖：T01–T06。

## 回滚与恢复

- 每个任务各自一个提交，恢复点写进“进度记录”；回滚某个任务用 `git revert <提交>`。
- 不涉及数据迁移和缓存格式变化。测试只用 `t.TempDir()`，不读写真实的 `~/.config/apic`。
- 实施中遇到 Semantic Amendment（例如优先级需要再调整、某类凭证字段无法容忍），停在当前任务，回到 plan review，并请求人工批准。

## 人工 Gate

1. 计划批准：本文档。
2. 当前执行请求：计划批准后，由用户另行发出。
3. 外部动作：push、tag 和发布 v0.4.0 不在本计划内，完成后另行批准。

## 可追溯性

| 设计条款与验收 | 任务 | 验证 |
|---|---|---|
| §1 环境声明的认证、优先级、显示、校验；D1、D2、D8 | T01、T02、T06 | T01、T02、T06 测试；B1、B6 real-env |
| §2 dry-run 容忍凭证字段、警告由命令层输出；D3 | T02、T06 | T02、T06 测试；B2、B6 real-env |
| §3 401 作废；D4 | T03、T06 | T03、T06 测试；B3 |
| §4 投影；D5、D6 | T04、T06 | T04、T06 测试；B4 |
| §5 简单定义 enum；D7 | T05、T06 | T05、T06 测试；B5 |
| 安全：凭证隔离、密钥登记、缓存不写不删 | T02、T03、T06 | B1、B2、B3 |
| 行为变化与回滚 | T07 | 发布说明；B6 compat |
| 工程规范 GR104、GR108–GR110、SR2002 | 全部任务 | 分层测试；B6 |

## 完成 Gate 与剩余工作

- 完成条件：T01–T07 全部完成，B1–B6 有证据；完成总结写入进度记录；issue 评论。
- 剩余工作：发布 v0.4.0（外部动作，需授权）；deferred 项留在 issue。

## 进度记录

（实施时填写）
