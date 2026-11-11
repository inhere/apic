<!-- template_id: plan; template_version: 1.3.0 -->
# apic 二期第三批（试用反馈修正）实施计划

> 状态：Draft 0.2 / 待人工计划批准（0.2 为第 1 轮评审后的未复核修订）

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-12 | Claude（inhere：“批准 apic 二期第三批设计 0.2（接受未复核修订）”） | 初稿：3 个波次、7 个任务，覆盖设计 0.2 的五项修正 |
| 0.2 | 2026-11-12 | Claude | 处理第 1 轮两轴评审（阻塞 Q1；纠正 P1–P4、Q2–Q10；建议若干）：公开仓库文档去掉内部标识；写保护用例加 `--yes` 并断言服务端未收到；补 `auth status/logout --env`；缓存断言先预置缓存；容忍字段加反向用例；警告排序；enum 数字按 Go 类型转换；`use` 无认证保持 `(none)`；real-env 只复制 gitlab 配置并先 unset 主机变量；波次依赖与 grep 完成标准修正 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 规划工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：交付设计 0.2 的五项并通过验收：环境声明的认证、dry-run 容忍凭证变量、401 后作废 token、`--pick` 投影、简单定义 `enum`。
- Scope freeze：只做设计 0.2 的范围。设计“非目标”（`--columns`、清除保存选择的命令、变量默认值语法等）实施中即使发现需要，也只记为 deferred。
- 评审预算：与设计相同，按高暴露处理（SR1409），一轮两轴独立评审：正确性与可执行性；安全与契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。
- 评审状态：第 1 轮已完成。正确性方向 PASS（0 阻塞）；安全方向 1 个阻塞项 Q1（计划写入了内部主机名与用户目录，仓库公开），已在未推送的 0.1 提交上就地替换为占位（amend，未 push）。0.2 已逐条修订；预算只有一轮，0.2 属于未复核修订，见“评审处理”。
- 公开仓库约束：本仓库公开，文档、测试数据与提交信息不写内部主机名、用户目录和账号；真实值只出现在 `<WP>/tmp/` 下的验收产物与 gofer job 中。

## 目标与完成定义

**目标**：v0.3.0 的用户升级后，按设计 0.2 的“用户可见的结果”工作，行为符合设计各节与 D1–D8。

**完成定义（验收）**：

1. **B1 环境声明的认证**，经真实分发（`commands.Run`）与 httptest 服务端（记录收到的认证头）覆盖：
   - 优先级矩阵：`--auth` > `envs.<env>.auth` > 保存的认证 > `default_auth`，四个来源各一例，外加都没有时不认证；
   - S1 场景：`use dh --auth build` 后 `dh me --env factory`，服务端收到的是 factory 的 token；`use` 保存时 stderr 出现设计中的 note；
   - factory 认证 `allow_methods: [GET, HEAD]` 时，保存过 build 认证后对 factory 发 `POST ... --yes`：退出 6，消息含 `not allowed by this auth (allow_methods)`，服务端没有收到请求（`--yes` 排除了 protect 确认本身的拒绝，换回旧优先级时此用例会失败）；
   - `auth status dh --env factory`（已保存 build）显示 `auth=factory`；`auth logout dh --env factory --auth build` 只删除 `dh.factory.build` 的缓存，同目录其他身份的缓存仍在；
   - link：生成的脚本含 `--env factory`、不含 `--auth`；用同样的参数经 `commands.Run` 调用，服务端收到 factory 的 token；
   - `use` 输出 `auth: <name> (flag|env|saved|default)`，没有认证时保持 `auth: (none)`、不带后缀；`use dh --auth build` 与 `use dh --env factory --auth build` 都按保存后的环境判断并输出 note；`auth ls` 的“当前”标记按 saved env 或 `default_env` 计算；显示类命令遇到写错的名字不报错；
   - `envs.<env>.auth` 指向不存在的认证时调用退出 1，消息为 `site "dh" env "factory": no auth "x"`；
   - `config check` 对写错的 `envs.*.auth` 与 `default_auth` 各给一条提示，退出码 0；
   - `apic skill` 输出各环境使用的认证，并含“显式 `--auth` 优先”的说明；注入换行的认证名被 `Sanitize`。
2. **B2 dry-run 容忍凭证变量**：
   - bearer、apikey（header 与 query）、basic、login（`login.body`）、exec（`exec.env`、`exec.command`）的凭证字段缺变量：dry-run 退出 0，stderr 每个变量一行警告，同名只一行，多个变量按名字排序；真实调用退出 1；
   - 同一字符串中一个已设、一个未设：已设变量的原值在 stdout 与 stderr 中都不出现（含 apikey 放在 query 的情况）；
   - 反向守卫：非凭证字段（`username`、`type`、`allow_methods`、`apply.header`、`apply.value`、`login.path`）缺变量时 dry-run 仍退出 1；变量值中出现的 `${env:..}` 不再展开（只替换一遍）；
   - 警告中的变量名去掉控制字符；
   - `auth status <site> --env X` 缺变量退出 1；
   - exec 的 dry-run 不执行命令（命令会写标记文件，断言文件不存在）；
   - 缓存不写不删：先预置同一身份的缓存（exec 带 `ttl`、login 各一份），dry-run（凭证字段含未设变量）后断言 `cache/` 下文件名、内容、mtime 都不变；
   - `source.auth` 拉取定义时缺变量仍退出 1。
3. **B3 401 作废**：
   - login 与 exec（带 `ttl`，可缓存）认证，服务端始终 401：调用退出 4，之后该身份的 token 缓存文件不存在，`auth status` 显示 logged out；预置的同站点其他 env/auth 缓存仍在；
   - 首次 401、重取后 200：缓存保留新 token；
   - 静态认证 401：不改动任何文件；
   - exec 的 `token.path` 写成投影（含逗号）时，沿用现有“不是字符串”的错误，退出 1；
   - `Invalidate` 返回错误时，stderr 出现一行脱敏后的警告，退出码仍为 4（request 包单测，用替身）。
4. **B4 `--pick` 投影**：
   - 单测表：根对象投影、`*` 下投影、键序与缺键 `null`、字面键 `a,b` 优先、中间段含逗号按普通键、空键或重复键退出 7、对非对象（无 `*`）退出 7、`*` 下非对象为 `null`、投影键 `*` 与数字是字面键；
   - 现有 `pick_test.go` 用例全部不改期望；
   - 经 `commands.Run`：`--pick data.list.*.id,name -o table` 的首行按空白拆分后为 `[id name]`；含非对象元素时回退 JSON 并有 note。
5. **B5 简单定义 `enum`**：枚举生效（非法值调用退出 1，`desc` 显示枚举）；`enum: [1, true]` 按 `"1"`、`"true"` 比较；`enum: ~` 与不写相同；空字符串、重复、`enum: []`、对象/数组/null 元素使站点加载失败、退出 1。
6. **B6 工程与兼容性**：
   - `go vet ./...`、`GOOS=windows go vet ./...`、`go test ./... -count=1`、分层守卫测试通过；单个文件不超过 1000 行（GR104）；
   - Windows 主机上 `go test ./internal/... -count=1` 通过；
   - 兼容：v0.3.0 基线二进制与新二进制各用一份复制的二期第二批 compat 配置，跑同一份 dry-run 清单，输出逐字节一致（清单中如有 `use`，其 `auth:` 行的来源后缀是预期差异，单独列出）；
   - **real-env**（主机，沙箱见 T07）：gitlab 站点加第二个环境 `anon`（同一 base_url，`auth: anon`）。预期退出码：`gitlab version`（main 环境）0；`gitlab version --env anon` 4；`use gitlab --auth pat` 之后 `gitlab version --env anon` 仍为 4（环境声明优先）；删除 `.env` 后 `gitlab version --dry-run` 0，且 stderr 恰好一行警告。证据中不记 base_url 与 token。

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
  - real-env 用 `APIC_CONFIG_DIR=<WP>/tmp/apic-p2c-accept/cfg`，只发 GET 请求到内部 GitLab；沙箱的建立与 token 处理见 T07；
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
| W2 接线 | T06 | T01–T05（T06 的端到端测试覆盖 T03 与 T05） |
| W3 验收 | T07 | T01–T06 |

**执行方式**：

- 实施方：sonnet 子 agent，在主 checkout 中**串行**执行 T01 → T02 → T03 → T04 → T05 → T06（T01 与 T04 同在 `model` 包，T03/T05 的测试会编译 `model`，并行容易互相看到半成品）；T07 我自己做。
- 我审查每个任务的 diff（先看 `git diff --stat` 的删除行），grep 调用点确认接线，并做验收。
- 任务书写明：分层规则（model 无依赖；config/spec/request/output/auth/skillgen 只依赖 model）与 idevstd；禁止删注释和无关重排；测试不在 export 了 `GIT_DIR` 的 shell 中运行；只提交本任务的文件（提交前查暂存区）；用 `--sync` 等待，不起空等后台任务。
- 每个任务一次原子提交（`feat(<pkg>): ... (P2C-Tnn)`）；提交前跑 `go test ./internal/... -count=1`。
- 测试数据与提交信息遵守“公开仓库约束”，主机名用 `example.com` 或 httptest 地址。
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
    // 已设变量照常展开并登记。返回去重、按名字排序的未设变量名（去掉控制字符）。其他字段缺变量仍返回错误。
    func ResolveAuthDryRun(site *model.Site, name string, env *Env, sec *model.Secrets) (model.AuthSpec, []string, error)
    ```
    两者共用一个内部实现；`Expand` 拆出内部函数，容忍模式下逐个处理同一字符串里的全部引用，每个字符串只替换一遍（变量值中的 `${env:..}` 不再展开，与现有 `ReplaceAllStringFunc` 一致）。
  - `Check` 追加提示（文案固定）：`site <s>: default_auth "<x>" is not defined`；`site <s> env <e>: auth "<x>" is not defined`。站点、环境按名字排序输出。
- 验证：`go test ./internal/config/ -run 'TestResolveAuthDryRun|TestResolveAuth|TestCheckAuthNames' -count=1`，覆盖六类凭证字段各缺一个变量、同串一已设一未设（已设值已登记：`sec` 能脱敏它）、重复变量只返回一次、多个变量按名字排序、`Username`/`Type`/`AllowMethods`/`Apply.Header`/`Apply.Value`/`Login.Path` 缺变量各返回错误、变量值含 `${env:..}` 不再展开、严格版行为不变。
- 完成标准：用例通过；现有 `config_test.go` 用例不改期望。
- 依赖：T01。

### T03 401 重发后作废 token

- 文件：`internal/request/execute.go`、`internal/request/request_test.go`（`fakeAuth` 补方法与新用例）、`internal/auth/provider.go`、`internal/auth/auth_test.go` 或 `exec_test.go`。
- 动作：
  - `Authorizer` 接口加 `Invalidate() error`。
  - `Execute`：把最后的 `return c.send(client, req)` 改为先取响应；响应状态仍为 401 时调用 `c.Authorizer.Invalidate()`，出错且 `c.Stderr != nil` 时写 `warning: ` + `c.Redactor.Redact("drop cached token: " + err.Error())`；然后返回该响应。
  - `Provider.Invalidate`：类型为 login 或 exec 时 `return p.Logout()`，否则 `return nil`。不在已持锁的代码路径中调用（`Logout` 自己加锁）。
- 验证：`go test ./internal/request/ -run 'TestExecute401|TestRetry|TestInvalidate' -count=1` 与 `go test ./internal/auth/ -run TestInvalidate -count=1`，覆盖 B3 各项：重发仍 401 调用一次 Invalidate；重发 200 不调用；静态认证（`Refresh` 返回 `ErrNoRefresh`）不调用；Invalidate 出错时的警告与响应不变；login/exec（带 ttl）的缓存文件被删、预置的其他身份缓存仍在、静态认证无操作。
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
- 验证：`go test ./internal/model/ -run TestPick -count=1`，覆盖 B4 的单测表；现有用例不改期望。exec `token.path` 含逗号的用例放 `internal/auth/exec_test.go`（同一任务内）。
- 完成标准：用例通过。
- 依赖：无（与 T01 同包，串行执行）。

### T05 简单定义 `enum`

- 文件：`internal/spec/simple.go`、`internal/spec/simple_test.go`（若无则新建）。
- 动作：
  - `simpleParam` 加 `Enum []any \`yaml:"enum"\``（用 `any` 接住数字与布尔）。goccy 解码后已是 Go 类型（评审探针：`1`→uint64、`-2`→int64、`1.0`→float64、`0x10`→16），按类型转换为字符串：string 原样；整数类型用 `strconv`；float64 用 `strconv.FormatFloat(v, 'f', -1, 64)`；bool 用 `strconv.FormatBool`；其他类型（map、slice、nil 元素）报错 `... param <p>: enum values must be strings, numbers or booleans`。因此 `1.0` 得到 `"1"`、`0x10` 得到 `"16"`，README 写明“需要原样比较时请加引号”。
  - `enum` 为 nil（不写、`enum:`、`enum: ~`）视为没有枚举；非 nil 的空列表、元素为空字符串、转换后重复：`ExitLocal`：`<文件> operation <name> param <p>: enum must be a non-empty list of distinct non-empty values`。
  - 写入 `model.Param.Enum`。
- 验证：`go test ./internal/spec/ -run TestSimpleEnum -count=1`，覆盖：字符串枚举、`[1, true]` 得到 `["1","true"]`、`[1.0, 0x10]` 得到 `["1","16"]`、`enum: ~` 为 nil、`[]`、`[""]`、重复（含 `[1, "1"]`）、map 与 null 元素各报错、未写 `enum` 时 `Enum` 为 nil。
- 完成标准：用例通过；`spec` 只导入 `model`。
- 依赖：无。

### T06 接线：选择、dry-run 警告、显示、skill、README

- 文件：`internal/wiring/wiring.go`、`internal/commands/siterun.go`、`internal/commands/use.go`、`internal/commands/auth.go`、`internal/skillgen/skillgen.go`、README.md、README.zh-CN.md 及测试（`internal/commands/envauth_test.go`，新；`internal/skillgen/skillgen_test.go`；`internal/commands/pick_test.go` 追加投影的端到端用例；`internal/wiring` 没有测试文件，选择逻辑经 commands 端到端覆盖）。
- 动作：
  - **Resolve**：`Selection` 加 `AuthSource string`；`Auth, AuthSource = site.EffectiveAuth(sel.Env, saved.Auth, authFlag)`；认证名不存在时：来源为 env 报 `site %q env %q: no auth %q`，其他来源沿用 `site %q has no auth %q`；均为 `ExitLocal`。
  - **NewCall**：签名改为 `NewCall(...) (*request.Call, []string, error)`，第二个返回值为 dry-run 下的未设变量名（非 dry-run 恒为 nil）；`fl.DryRun` 时 `newProvider` 改用 `config.ResolveAuthDryRun`。`NewAuth` 与 `FetchOptions` 的 `source.auth` 不变（严格）。
  - **siterun.go**：拿到未设列表后，在执行前向 stderr 每个写一行 `warning: dry-run: <NAME> is not set; a real call would fail`。
  - **use.go**：`printSelection` 的 auth 行写 `auth: <name> (<source>)`，没有认证时保持现有的 `auth: (none)`、不带后缀；`firstOf(saved.Auth, …)` 与 `firstOf(next.Auth, …)` 两处改用 `EffectiveAuth(env, saved.Auth/next.Auth, "")`；`use --auth x` 保存后，按保存后的环境 `firstOf(next.Env, site.DefaultEnv)` 判断，若该环境声明了不同的认证，stderr 写设计中的 note（同时给 `--env` 与 `--auth` 时同样判断）。
  - **auth.go**：`listAuths` 的当前标记改用 `EffectiveAuth(firstOf(saved.Env, site.DefaultEnv), saved.Auth, "")`，不再直接读 `DefaultAuth`。
  - **skillgen**：环境列表每行追加 `; auth <Sanitize(name)>`（有声明时）；认证一节加一句 `An explicit --auth overrides the environment's auth; pass --env explicitly.`；How to call 加投影示例 `--pick data.items.*.id,name`。
  - **README**（中英文）：`envs.<env>.auth` 与优先级，并建议多环境站点给每个环境都写 `auth`（没写的环境会用保存的认证）；加环境声明后旧缓存残留与 `auth logout --env X --auth <旧名>` 清除；dry-run 缺变量的表现（`***`、URL 编码、`<exec-token>`、以警告为准）；401 作废；投影；简单定义 `enum`（数字按类型转换，需原样比较时加引号）。
- 验证：`go test ./internal/commands/ -run 'TestEnvAuth|TestDryRunMissing|TestPickProjection|TestInvalidateE2E|TestSimpleEnumE2E' -count=1` 与 `go test ./internal/skillgen/ -count=1`，经 `commands.Run` 覆盖 B1、B2、B3（login/exec 端到端）、B4 的端到端项和 B5。
- 完成标准：用例通过；`go test ./internal/... -count=1` 通过；`NewCall` 只有 siterun 一个调用方；`grep -rnE 'firstOf\((saved|next)\.Auth|\.DefaultAuth|firstNonEmpty\(authFlag' internal/commands internal/wiring` 无结果。
- 依赖：T01、T02、T03、T04、T05。

### T07 验收、兼容性与发布说明草稿

- 文件：`docs/release-notes/v0.4.0.md`（新）、本计划的进度记录；验收产物在 `<WP>/tmp/apic-p2c-accept/`。
- 动作：
  - 跑 B6 的检查；按 P3 跑 compat（复用二期第二批 `compat.sh` 的写法，路径改到本批目录）；
  - 主机 job：`git archive HEAD` 导出源码，跑 Windows `go test`；
  - real-env 沙箱（主机 job 内完成，不在容器打印任何内容）：
    - 只建 `<WP>/tmp/apic-p2c-accept/cfg/`：`config.yaml` 只含 gitlab 站点（从 `<WP>/tmp/apic-try/cfg/config.yaml` 取出该站点，加 `anon` 环境）、`defs/gitlab.yaml`、以及只含 `GITLAB_TOKEN` 一行的 `.env`（脚本过滤写入，不回显）；不复制 `bin/`、`cache/`、`overlays/` 和 dh 站点；
    - job 开头 `Remove-Item env:GITLAB_TOKEN`，确保变量只来自 `.env`；
    - 禁止 `-v`、`config show`、输出 `.env`、列环境变量和脚本回显；token 不出现在 job 命令行；
    - 按 B6 real-env 的顺序执行并记录退出码与 stderr 行数；最后删除 `.env` 跑 dry-run 项，job 结束时 `.env` 已不存在；
  - 写发布说明草稿：新功能、行为变化（设计“行为变化”四条，对照 README 退出码表）、新增的 stderr 行（dry-run 警告、作废失败警告、`use` note）、`config check` 新提示、`apic skill` 文本变化、回滚（先删 `envs.*.auth` 或显式 `--auth`）；compat 若有差异逐条列出；
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

## 评审处理（第 1 轮）

报告：`<WP>/tmp/apic-p2c-plan-review/{correctness,security}.md`。

| 评审项 | 处理 |
|---|---|
| Q1 阻塞：公开仓库写入内部主机名与用户目录 | 未推送的 0.1 提交就地替换为占位（amend）；加“公开仓库约束”，测试数据用 `example.com` |
| Q2 写保护用例测不出回归 | 加 `--yes`，断言 allow_methods 消息且服务端未收到 |
| Q3 未覆盖 `auth status/logout --env` | B1 加两条 |
| Q4、P3 grep 完成标准漏项 | 改为覆盖 `next.Auth`、`.DefaultAuth`、`firstNonEmpty(authFlag` |
| Q5、P2、P5 `use` 无认证输出与 note 判断 | 保持 `auth: (none)`；按保存后的环境判断，覆盖同时给 `--env --auth` |
| Q6 警告顺序不稳定 | 按名字排序 |
| Q7 容忍字段缺反向守卫 | B2 与 T02 加非凭证字段反向用例与“只替换一遍” |
| Q8 `***` 断言无效 | 改为断言已设变量原值不出现（含 query） |
| Q9 缓存断言前提 | B2 先预置缓存再比对 mtime；B3 预置其他身份缓存；exec 用 ttl |
| Q10、P10 real-env 沙箱与 token | T07 只复制 gitlab 配置与单行 `.env`，先 unset 主机变量，禁回显，结束删 `.env`，写明预期退出码 |
| P1 YAML 数字原文不可得 | 按 Go 类型转换，写明 `1.0`→`"1"`、`0x10`→`"16"`；README 提示加引号 |
| P4 波次依赖 | W2 依赖 T01–T05 |
| P6 测试命令 | T03 用 `TestExecute401\|TestRetry\|TestInvalidate`；提交前跑 `./internal/...` |
| P7 `enum: ~` 与元素类型 | nil 视为未写；固定报错文案 |
| P8 表头宽度 | 断言首行拆分为 `[id name]` |
| P9 link 测法 | 断言脚本参数并经 `commands.Run` 调用 |
| P11 旧缓存残留 | README 写明清除方法 |
| P12、建议 S1 exec `token.path` 投影 | T04 加用例 |
| 建议 S2 发布说明 | T07 补 stderr 新行、`config check`、skill 文本、compat 差异 |
| 建议 S3 变量名控制字符 | T02 去掉控制字符 |
| 建议 S4 `FetchOptions` 忽略 `--env` | 现有行为，deferred；README 建议每个环境都写 `auth` |
| 建议 S5 证据不记 base_url | B6 real-env 写明 |

## 完成 Gate 与剩余工作

- 完成条件：T01–T07 全部完成，B1–B6 有证据；完成总结写入进度记录；issue 评论。
- 剩余工作：发布 v0.4.0（外部动作，需授权）；deferred 项留在 issue。

## 进度记录

（实施时填写）
