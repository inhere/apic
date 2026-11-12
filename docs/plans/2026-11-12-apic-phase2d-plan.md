<!-- template_id: plan; template_version: 1.3.1 -->
# apic 二期第四批（pick 分组、分页、use --clear、拉取跟随 --env、${exec:}）实施计划

> 状态：Draft 0.3 / 待人工计划批准（0.2 已于 2026-11-12 批准并开始实施：“批准 apic 二期第四批计划 0.2（接受未复核修订）；开始实施，使用 suga 的 oc-go/cmdc ds4.1 交替使用”。0.3 只追加 B7 与 T06d，其余不变；0.2、0.3 为未复核修订）

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-12 | Claude（inhere：“按 data.{a,b},c 改 pick；批准 apic 二期第四批设计 0.3（接受未复核修订）”） | 初稿：3 个波次、7 个任务，覆盖设计 0.3 的五项 |
| 0.2 | 2026-11-12 | Claude | 处理第 1 轮两轴评审（两个方向均 PASS；纠正 P1–P13、Q1–Q9；建议若干）：<br>• B4 改用单个 TLS 服务端，用有无 `ca_file` 区分环境；<br>• `${exec:}` 的工作目录改在 wiring 层验证；<br>• 认证不运行的断言只针对调用所用的认证，并预热定义缓存；<br>• 执行器返回 stderr 与结构化失败，复用 `lastLine` 的清洗，保留批处理检查；<br>• 失败消息格式按设计；<br>• 未闭合的 `${exec:` 报错；放错位置先检查、再展开；<br>• `${env:}` 保持非空名字；<br>• `config check` 遮蔽 `${exec:}` 命令；<br>• pick 补四条未定规则（P11）；<br>• `SizeValue` 改指针，导出 `IsPlainPath`；<br>• 同步改三个现有测试文件；<br>• `use --clear` 处理未知站点；<br>• real-env 先 dry-run、try/finally 清理、TMP 指向沙箱；<br>• 发布说明逐条对照设计的行为变化 |
| 0.3 | 2026-11-12 | Claude（inhere：skill 生成加 apic 自身的使用说明，含简易定义格式与示例） | 对应设计 0.4 §6：追加验收 B7、任务 T06d（W2，T06c 之后）；执行方式按实际记为 suga 串行（见进度记录） |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 规划工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：交付设计 0.3 的五项并通过验收：`--pick` 分项与分组、站点级分页、`use --clear`、拉取定义跟随 `--env`、`${exec:}`。
- Scope freeze：只做设计 0.3 的范围；设计的非目标在实施中即使发现需要，也只记为 deferred。
- 评审预算：与设计相同，按高暴露处理（SR1409），一轮两轴独立评审：正确性与可执行性；安全与契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。
- 评审状态：第 1 轮已完成，两个方向均 PASS（0 阻塞），必须修正项已在 0.2 处理。预算只有一轮，0.2 属于未复核修订，见“评审处理”。
- 公开仓库约束：文档、测试数据、提交信息与进度记录都不写内部主机名、用户目录和账号。测试用 `example.com` 或 httptest 地址；真实值只出现在 `<WP>/tmp/` 下的验收产物与 gofer job 中。

## 目标与完成定义

**目标**：v0.4.0 的用户升级后，行为符合设计 0.3 各节与 D1–D11，以及本计划补充的规则 R1–R4（见 T01）。

**完成定义（验收）**：

1. **B1 `--pick`**：
   - `model` 单测表覆盖设计 §1 的全部示例：
     - 单路径直接输出值；
     - `code,data.username`；
     - `data.{username,id},code` 平铺；
     - 别名；
     - `data.list.*.{id,name}` 输出对象数组；
     - `data.list.*.{id,name},data.total` 的键为 `list`、`total`；
     - 嵌套分组；
     - 无键名段时必须写别名；
     - 多项中缺失为 `null`；
     - 单项缺失时退出 7；
     - `*` 下元素错误为 `null`；
     - R1–R4 各一例。
   - 语法错误逐条用例，`ParsePick` 返回退出 2：
     - 空项、空段；
     - 别名为空或字符不合法；
     - 花括号不配对；
     - 分组不在最后一段；
     - 静态可知的重复键（含 `*` 之下）；
     - 首尾逗号。
   - 旧投影的用例改写为新语法。
   - exec 的 `token.path` 写成分组时，沿用现有的“不是字符串”错误，退出 1（T01 在 `internal/auth/exec_test.go` 中加用例）。
   - 经 `commands.Run`：
     - `--pick 'data.list.*.{id,name}' -o table` 的首行按空白拆分为 `[id name]`；
     - 语法错误时退出 2，服务端没有收到调用请求。
2. **B2 分页**，经 `commands.Run` 和 httptest 服务端（按页码返回、记录收到的 query）覆盖。每个用例先预热定义缓存，以排除拉取定义的影响。
   - 完整取回：
     - 两种响应都得到完整的合并数组：GitLab 式（响应本身是数组），以及 DeployHub 式（`items: data.list`、`total: data.total`）；
     - 服务端把条数截到 2、`size_value: 5`：取齐全部数据，不在第一页就停止；
     - 配 `total`：取够即停止，不多发请求；
     - 只有一页数据、未配 `total`：共请求两次。
   - 起始页与参数：
     - 起始页码按三种来源各一例：用户给的值、`start: 0`、默认 1；
     - 用户给的条数参数优先于 `size_value`；
     - 操作把 `page` 声明为必填、用户未给时仍能翻页；
     - 原始路径调用（`apic site /path --all-pages`）同样能翻页。
   - 失败与截断：
     - 第 2 页返回 500：stdout 为该页原始响应体，stderr 为 `note: page 2 returned HTTP 500; 1 earlier pages discarded`，退出 5；
     - 第 1 页返回 500：没有 note，与单次调用相同；
     - 第 2 页返回 302：退出 7；
     - 第 2 页不是 JSON、`items` 不是数组、`total` 不是整数：各退出 7；
     - 页码被忽略（每页内容相同）：退出 7，消息含 `repeats`；
     - `--max-pages 2`、实际有 3 页：输出前 2 页，stderr 有警告，退出 0；
     - 达到默认上限（测试中把上限变量设小）：退出 7，stdout 为空。
   - 用法错误：以下情况退出 2，服务端没有收到调用请求，且调用所用的 exec 认证（会写标记文件）没有运行：
     - 没配 `paginate`；
     - 方法不是 GET（含小写 `-X post`）；
     - 页码或条数参数不是整数；
     - `--max-pages` 为 `0` 或 `abc`。
   - 其他行为：
     - dry-run 只打印一条 curl，stderr 有 note；
     - `--all-pages --pick '*.{id,name}' -o table` 输出表格；
     - `insecure` 告警只打印一次；
     - 第 2 页先 401、重取 token 后 200，结果完整。
   - 配置校验（退出 1）：
     - 缺 `page`；
     - `size_value` 不是正整数（含 `0`），或配了 `size_value` 却没配 `size`；
     - `start` 为负；
     - `items`/`total` 含逗号、分组或 `*`。
   - 操作参数名为 `all-pages` 或 `max-pages` 时，生成的选项中没有它，用 `-p` 能传入。
3. **B3 `use --clear`**：
   - 清除后 `state.yaml` 中没有该站点，输出与无参 `use` 一致；
   - 没有保存的选择时也成功；
   - `--clear --env x` 退出 2；
   - 站点已从配置删除、state 中有残留：删除残留并输出 `cleared`；
   - 站点不存在、也没有残留：退出 1，消息为 `site "x" does not exist`；
   - `state.links` 不变。
4. **B4 拉取跟随 `--env`**：
   - 搭建：
     - 只建一个 httptest TLS 服务端，同时提供 spec 和接口。环境 `secure` 的 `ca_file` 指向该服务端证书的 PEM；环境 `plain` 不配 `ca_file`，用它拉取会因证书不受信而失败。
     - 每个用例用新的配置目录（无缓存）。用 `ops` 或 `--dry-run` 只触发拉取，并用服务端对 spec 路径的命中计数确认确实发生了拉取。
   - 用例：
     - 默认环境为 `plain`、站点 `default_env: plain` 时，以下写法都能拉取成功：`apic dh --env secure ops`（如该命令不接受 `--env`，改为站点调用）、`apic --env secure dh me --dry-run`、`apic dh me --env secure --dry-run`、`apic --api dh --env secure me --dry-run`、`link` 生成脚本的参数形态；
     - 不带 `--env` 时拉取失败（证明区分有效）；
     - 站点前后都给 `--env` 时取最后一个，先用测试核实 gcli 的“后者覆盖”；
     - `--env bogus`（含 `apic dh --env bogus` 列操作）：退出 1，消息为 `site "dh" has no environment "bogus"`，spec 命中计数为 0；
     - state 保存的环境已被删除时，`ops` 仍成功（维持现状）。
5. **B5 `${exec:}`**：
   - config 层（替身执行函数）：
     - 凭证值（token、password、value、exec.env、login.body）都能展开；
     - 可与文字、`${env:}` 混写；
     - 一次扫描：环境变量值为 `${exec:x}` 时替身没有被调用；替身输出 `${env:Y}` 时原样保留；
     - `${env:}`（空名）保持原样，与 v0.4.0 一致；
     - 去重：同一文本只调用一次；失败不缓存，再次解析会重新调用；
     - 位置错误：非凭证字段（`exec.command`、`apply.value`、`login.apply.value`、`token.path`、`allow_methods`、`login.path`、`username`）中出现时退出 1，消息为设计文案。凭证值中同时有引用时，替身没有被调用（先检查位置）；dry-run 下同样报错；
     - 格式错误：未闭合的 `${exec:`（凭证值或其他字段）退出 1，报 `malformed ${exec:} reference`，消息不含原文，服务端没有收到请求；内容为空或含 `${` 时报错；
     - 输出检查：输出为空、多行或含控制字符时报错；末尾的 `\r\n` 被去掉，首尾空格保留；
     - dry-run 不调用替身，展开为 `<exec>`；
     - 执行函数为空时报 `${exec:} is not available`；
     - 失败消息整行断言为 `auth "pat" value: ${exec:} failed (exit 3): <清洗后的 stderr 行>`，不含命令名与参数；
     - 结果登记为密钥。
   - auth 层：
     - 现有 exec 测试全部不改期望；
     - 新增导出执行器的测试：相对路径按传入目录解析；裸名不匹配当前目录；超时（可注入短时）；4 MiB 上限；stderr 最后一行经清洗与脱敏（ANSI、控制字符被去掉，密钥被遮蔽后再截断）；
     - Windows 专用用例：`.cmd` 带 `&` 参数被拒绝（在 T07 的 Windows 主机上运行）。
   - wiring 层：`wiring.Load` 后把 `Deps.Dir` 换成另一个目录，调用 `Env.Exec` 时相对路径仍按原配置目录解析。
   - 命令层（`commands.Run`，Unix 下用 sh 脚本输出 token）：
     - 服务端收到展开后的 token；
     - `-v` 与 dry-run 的输出中不出现 token；
     - dry-run 的自定义头显示 `<exec>`，脚本没有运行；
     - 没有被选中的认证里的 `${exec:}` 不运行；
     - `site refresh` 配合 `source.auth` 的 `${exec:}` 能拉取成功；
     - `config check`：每处 `${exec:}` 输出 `runs ${exec:}`；混写值不报 plaintext；放错位置和未闭合的引用都有提示；exec 认证 `command[0]` 为 `${exec:...}` 时显示 `***`；任何输出都不含命令文本。
6. **B6 工程、兼容性与 real-env**：
   - `go vet ./...`、`GOOS=windows go vet ./...`、`go test ./... -count=1`、分层守卫测试通过；单个文件不超过 1000 行（GR104）。
   - Windows 主机上 `go test ./internal/... -count=1` 通过。
   - compat：v0.4.0 基线二进制与新二进制跑同一份 dry-run 清单，输出逐字节一致；只有含旧投影写法的 `--pick` 条目是预期差异（清单中如有，单独列出）。
   - **real-env**（主机，沙箱见 T07），按以下顺序：
     1. `gitlab version --dry-run` 显示 `<exec>`，标记文件不存在；
     2. `gitlab version` 退出 0，标记文件存在；
     3. `gitlab projects --search <公开词> --per_page 5 --all-pages --max-pages 3 --pick '*.{id,path_with_namespace}' -o table` 退出 0，输出不超过 16 行（含表头）；数据多于 3 页时 stderr 有 `--max-pages` 警告；
     4. `config check` 列出 `runs ${exec:}`。

     证据中不记 base_url 与 token。

7. **B7 apic 自身的 skill**（设计 0.4 §6）：
   - 经 `commands.Run`：`apic skill --out <tmp>` 写出 `<tmp>/apic/SKILL.md`，stdout 为该路径；`--name x` 改目录名；已有文件没有标记行时不覆盖，`--force` 覆盖；`apic skill pet` 的行为与现有用例一致。
   - 配置目录不存在、或 `config.yaml` 无法解析时，`apic skill --out <tmp>` 仍成功（不读配置）。
   - 内容：frontmatter 合法（name、description）、标记行、版本号；含简易定义一节与完整示例。
   - 防漂移：`globalSpec` 的每个全局选项名、简易定义的每个字段名（反射 `spec` 中简易定义结构体的 yaml tag）、`model` 的每个退出码都在模板中出现；模板中的简易定义示例写入临时文件后能被 `spec.SimpleLoader` 加载。
   - 不泄露：在含站点、`.env` 与 token 的配置目录下生成，输出中不含这些站点名、base_url 与 token。

## 范围、排除项与授权

**范围**：仓库 `inhere-tools/apic`（Git root）下的代码、README 和 docs。

**排除项**：

- 设计 0.3 的非目标；二期其余 deferred 项。
- 修改用户的正式配置目录：不在本计划内；real-env 只用 `<WP>/tmp/apic-p2d-accept/` 下的副本。
- push、tag、发布：属于外部动作，需要另行授权。未推送的提交 `03c7470`（pick 报错提示）会被 T01 的重写取代，随本批一起发布。

**授权**：

- 计划批准后，还需要用户另行发出当前执行请求，才能开始实施。
- 按功能点本地提交是默认授权。
- 主机动作只有 T07 的 Windows `go test` 与 real-env 试用：
  - 源码用 `git archive HEAD` 导出到 `<WP>/tmp/apic-p2d-accept/src`，不在主机建 worktree，不动主机上的仓库；
  - real-env 用 `APIC_CONFIG_DIR=<WP>/tmp/apic-p2d-accept/cfg`，只发 GET；
  - job 内把 `TMP`、`TEMP`、`GOTMPDIR` 指向 `<WP>/tmp/apic-p2d-accept/tmp`；除 Go 构建与模块缓存外，只读写 `<WP>/tmp/apic-p2d-accept/`。

- `host_or_non_offline_action=REQUIRED`

## 输入与批准证据

- 设计：`docs/design/2026-11-12-apic-phase2d-design.md` Draft 0.3，已批准。
  - 批准原话（2026-11-12）：“无需兼容，现在只有我自己在使用；按 data.{a,b},c 改 pick；批准 apic 二期第四批设计 0.3（接受未复核修订）”。
  - 记录批准的提交：`14adf91`。
- 设计第 1 轮评审（C1–C23、S1–S17）已并入 0.2/0.3，报告在 `<WP>/tmp/apic-p2d-review/`。
- 计划第 1 轮评审报告：`<WP>/tmp/apic-p2d-plan-review/{correctness,security}.md`。
- 第三批验收产物（只读）：`<WP>/tmp/apic-p2c-accept/compat.sh` 与 `compat/`。
- 试用沙箱：`<WP>/tmp/apic-try/cfg`。
- 跟踪 issue：`my-tools-ab5f`（父 issue `my-tools-thv4`）。

## Capability Discovery

### Capability decisions

| capability_id | required_capability | searched_candidates | direct_reuse | thin_adapter_or_owner_extension | decision | proven_gap | duplication_and_lifecycle_risk |
|---|---|---|---|---|---|---|---|
| CAP-01 | 多字段 pick | `model.Pick`、`projectPick` | 部分 | `model/pick.go` 原位重写解析与求值 | OWNER_EXTENSION | 只有末段投影，且语义要改 | 删除 `projectPick`，无重复 |
| CAP-02 | 翻页循环 | `request.Execute`、`output.Render` | 部分 | `request` 新增 `paginate.go`，每页调用 `Execute` | OWNER_EXTENSION | 没有多次请求的循环 | 复用 Execute 的认证、401、写保护 |
| CAP-03 | 清除选择 | `config.SaveState`、`use.go` | 是 | `use.go` 加选项 | DIRECT_REUSE | none | 无 |
| CAP-04 | 拉取跟随 `--env` | `Prescan`、`loadIndex`、`FetchOptions` | 部分 | 三处加参数 | OWNER_EXTENSION | 预扫描不返回 `--env` | 无 |
| CAP-05 | `${exec:}` 执行 | `auth/exec.go` `runExec`、`lastLine`、`checkResolved`、`config.expand` | 部分 | 从 `runExec` 抽出导出的执行器；`lastLine` 拆成纯函数；config 经注入函数调用 | OWNER_EXTENSION | config 不能依赖 auth；`runExec` 绑定 Provider | 一份执行器，exec 认证与 `${exec:}` 共用 |

### New module candidates

| candidate_id | capability_id | proposed_module | searched_candidates | direct_reuse_gap | thin_adapter_or_owner_extension_gap | proven_gap | unique_owner_and_lifecycle | deletion_or_merge_handling |
|---|---|---|---|---|---|---|---|---|
| NONE | none | none | none | none | none | none | none | none |

### Rejected new tools

| rejected_candidate | capability_id | deletion_test_and_reason |
|---|---|---|
| 新包 `internal/proc` 放执行器 | CAP-05 | 删掉它不缺能力：执行器留在 auth、经 wiring 注入即可，不改分层表 |
| JMESPath / gjson 一类查询库 | CAP-01 | 删掉它不缺能力；分组语法很小，原位实现 |

## 前置检查与 fail-closed 条件

运行目录均为 `inhere-tools/apic`。

1. **P1 工作树**：`git status --short` 为空，HEAD 至少包含本计划的批准提交。否则停下，查清归属。
2. **P2 Go 版本**：容器内 `GOTOOLCHAIN=auto` 得到 go1.26.x；基线 `go test ./... -count=1` 通过。
3. **P3 基线**：`git archive v0.4.0 | tar -x -C <WP>/tmp/apic-p2d-accept/base` 后构建 `base/apic`（不用 worktree）；复制 `<WP>/tmp/apic-p2c-accept/compat/base` 到 `<WP>/tmp/apic-p2d-accept/compat/{base,new}/`。
4. **P4 主机可用**：T07 需要 gofer server 和本项目 worker 在线，否则 B6 的 Windows 与 real-env 项标 `UNVERIFIED_REAL_ENV`，停在完成 Gate。
5. **P5 GitLab 可达**：主机对沙箱 gitlab 站点匿名调用 `version`，返回 401 或 200；否则 real-env 的 GitLab 项标 `UNVERIFIED_REAL_ENV`。

## 波次与依赖

| 波次 | 任务 | 依赖 |
|---|---|---|
| W1 领域 | T01 → T02 → T03；T04 → T05 | T02 用 T01 导出的 `IsPlainPath`；T03 用 T02 的 `model.Paginate`；T05 用 T04 新增的 `model.ExecFailure` 与 `Env.Exec` |
| W2 接线 | T06（a/b/c 三次执行）→ T06d | T01–T05 |
| W3 验收 | T07 | T01–T06 |

**执行方式**：

- 实施方：sonnet 子 agent，在主 checkout 中**串行**执行 T01 → T02 → T03 → T04 → T05 → T06。T01、T02、T04 都改 `model` 包，并行容易互相看到半成品。T07 由我自己做。
- 审查：我审查每个任务的 diff（先看 `git diff --stat` 的删除行），grep 调用点确认接线，并做反向验证（改坏关键行，确认对应测试失败且不是编译失败）。
- 任务书写明以下要求：
  - 分层规则（model 无依赖；config/spec/request/output/auth/skillgen 只依赖 model）与 idevstd；
  - 禁止删注释和无关重排；
  - 测试不在 export 了 `GIT_DIR` 的 shell 中运行；
  - 只提交本任务的文件（提交前查暂存区）；
  - 用 `--sync` 等待，不起空等后台任务。
- 提交：每个任务一次原子提交（`feat(<pkg>): ... (P2D-Tnn)`），提交前跑 `go test ./internal/... -count=1`。
- 用 gofer plan 跟进，每个任务一个 todo。

## 任务

所有命令都在 `inhere-tools/apic` 下运行，并带上 `GOFLAGS=-buildvcs=false GOWORK=off GOTOOLCHAIN=auto`。

### T01 `--pick` 分项与分组

- 文件：`internal/model/pick.go`、`internal/model/pick_test.go`、`internal/auth/exec_test.go`（只加一个用例）。
- 动作：
  - 重写 `Pick` 的内部结构，对外签名不变：`ParsePick(path string) (Pick, error)`、`(Pick) Apply(v any) (any, error)`、`(Pick) String() string`。
  - 新增 `(Pick) IsPlainPath() bool`：单项、无别名、无分组、无 `*`。
  - 内部结构：项列表，每项含别名、段列表，以及可选的分组（递归的项列表）。
  - 解析：
    - 按不在花括号内的逗号分项；项首 `别名=`；按点分段；末段 `{...}` 递归解析。
    - 键名：取别名，否则取最后一个键名段（不是 `*`、下标或分组的段）。
    - 重复键静态检查：可展开的分组平铺后，与同级各键一起比较；`*` 之后的分组在组内单独比较。
    - 所有语法错误用 `Errorf(ExitUsage, "--pick %q: <原因>", path)`。
  - 求值规则（设计 §1，加上以下补充规则）：
    - **R1**：只有一项且带别名时，结果为对象 `{别名: 值}`（`uid=data.id` → `{"uid":7}`）。
    - **R2**：带别名的分组不平铺，作为嵌套对象（`u=data.{a,b}` → `{"u":{"a":…,"b":…}}`）。
    - **R3**：分组前缀不存在或不是对象时：
      - 若这是唯一一项且是可展开的分组，退出 7，与单路径缺失一致；
      - 否则组内各键为 `null`。
    - **R4**：`*` 之下：
      - 元素不是对象时，该元素为 `null`；
      - 元素是对象时，组内取不到的键为 `null`，不把整个元素置为 `null`。
    - 只有一项、不是可展开的分组、也没有别名时，等同现有的单路径：缺失退出 7，`*` 之下元素错误为 `null`。
    - 其他情况组装 `Object`（键序即书写顺序），缺失为 `nil`。
  - 删除 `projectPick` 与 `projectionHint`，改写原有的投影用例。
  - `exec_test.go` 加用例：`token.path: "{a,b}"` 沿用现有的“不是字符串”错误。
- 验证：`go test ./internal/model/ -run 'TestPick' -count=1`；`go test ./internal/auth/ -count=1`。
- 完成标准：
  - B1 的 model 部分与 R1–R4 用例通过；
  - `grep -rn projectPick internal/` 无结果；
  - auth 包现有测试不改期望并通过。
- 依赖：无。

### T02 分页配置：模型与加载校验

- 文件：`internal/model/config.go`、`internal/config/load.go`，以及测试 `internal/config/paginate_test.go`（新）。
- 动作：
  - 模型：
    ```go
    type Paginate struct {
        Page      string `yaml:"page"`
        Size      string `yaml:"size,omitempty"`
        SizeValue *int   `yaml:"size_value,omitempty"`
        Start     *int   `yaml:"start,omitempty"`
        Items     string `yaml:"items,omitempty"`
        Total     string `yaml:"total,omitempty"`
    }
    // Site 加：Paginate *Paginate `yaml:"paginate,omitempty"`
    func (p *Paginate) StartPage() int // Start 未写时为 1
    ```
  - 校验：在 `validateSite` 中进行，错误格式为 `%s site %q: paginate: <原因>`，退出 1。
    - `size_value` 写了就必须大于 0，并要求配了 `size`。
    - `items`/`total` 非空时，用 `ParsePick` 加 `IsPlainPath` 检查。
- 验证：`go test ./internal/config/ -run TestPaginateConfig -count=1`，覆盖 B2 的配置校验各条，以及合法配置加载后字段正确（`start: 0` 与未写可区分）。
- 完成标准：用例通过；没有 `paginate` 的现有配置测试不变。
- 依赖：T01。

### T03 翻页循环

- 文件：`internal/request/paginate.go`（新）、`internal/request/paginate_test.go`（新）、`internal/request/execute.go`（加一个字段）。
- 动作：
  - `Call` 加 `QuietInsecure bool`：为 true 时，`send` 不打印 `insecure` 告警，其他警告（如 drop cached token）照常。
  - 新增：
    ```go
    type PageOptions struct {
        Spec     model.Paginate
        First    int  // 起始页码（调用方已确定）
        Max      int  // 上限页数，>0
        Explicit bool // 上限来自 --max-pages
    }
    type PageResult struct {
        Resp      *Response // 成功：最后一页的状态与头，Body 为合并数组的 JSON；失败页：该页原样
        FailedAt  int       // 非 2xx 的页序号（从 1 起），0 表示没有
        Discarded int       // 失败时丢弃的前序页数
        Truncated bool      // 显式上限截断
        Pages     int
    }
    var DefaultMaxPages = 100 // 测试中可替换

    // Paginate 每页复制一份 Prepared、改写 URL 中 Spec.Page 参数，再调用 Execute；
    // 第一页之后设置 QuietInsecure。方法不是 GET 时返回 ExitUsage（兜底）。
    // dry-run 只执行第一页并返回其 Response。
    // 退出 7 的情况返回 model.Errorf(ExitPostProcess, "--all-pages failed: page %d: ...")；
    // 默认上限截断返回 model.Errorf(ExitPostProcess, "--all-pages: reached the default limit of %d pages; pass --max-pages to raise or accept it")。
    func Paginate(ctx context.Context, c Call, opt PageOptions) (*PageResult, error)
    ```
  - 合并数组用 `model.EncodeJSON` 编码，元素保持 `model.Decode` 的键序；相邻页比较用编码后的字节。
  - `total` 只从第一页读取：值必须是整数且不小于 0。
- 验证：`go test ./internal/request/ -run TestPaginate -count=1`，用 httptest 覆盖 B2 中与循环相关的各条（截断条数、total、单页两次请求、第 1/2 页 500、302、非 JSON、items 非数组、total 非整数、重复页、显式与默认上限、401 逐页重试、dry-run 只请求一次、`insecure` 告警一次、各页的 `Prepared.URL` 互不影响）。
- 完成标准：用例通过；`request` 只依赖 `model`。
- 依赖：T02。

### T04 `${exec:}` 展开（config 层）

- 文件：
  - `internal/model/exec.go`（新，几行）；
  - `internal/config/expand.go`、`internal/config/dotenv.go`、`internal/config/check.go`；
  - 测试 `internal/config/exec_ref_test.go`（新）。
- 动作：
  - `model` 新增：
    ```go
    // ExecFailure 由执行器返回；Code<0 表示未能启动或超时等，Reason 已清洗、脱敏，不含 argv。
    type ExecFailure struct { Code int; Reason string }
    func (e *ExecFailure) Error() string
    ```
  - `Env` 加 `Exec func(argv []string) (string, error)`，以及一个私有的去重表（带互斥锁）。
  - 引用识别：正则为 `\$\{(?:env:([^}]+)|exec:([^}]*))\}`，`${env:}` 仍要求非空名字。在一次 `ReplaceAllStringFunc` 中处理两种引用，替换结果不再扫描。
  - 解析认证时：
    1. 先检查位置：遍历所有非凭证字段（含 `Exec.Command`），出现 `${exec:` 就报 `auth %q: ${exec:} is only allowed in credential values`，退出 1。这一步在任何展开之前，dry-run 也一样。
    2. 再展开各字段。
    3. 展开后，若凭证值中仍有 `${exec:`（未闭合），报 `auth %q %s: malformed ${exec:} reference`，退出 1，不带原文。
  - 执行：
    - 内容为空或含 `${` 时报错。
    - 按空白切分 → 查去重表 → 调用 `Env.Exec`。
    - 输出去掉末尾的 `\r\n`，校验非空、无其他控制字符。
    - 成功写入去重表，失败不写；结果 `sec.Add`。
  - 失败消息：
    - 错误为 `*model.ExecFailure` 且 `Code>=0` 时：`auth %q %s: ${exec:} failed (exit %d): %s`；
    - `Code<0` 时：`auth %q %s: ${exec:} failed: %s`。
  - dry-run（`missing != nil`）：位置检查照常；`${exec:...}` 替换为 `<exec>`，不调用、不登记。
  - `check.go`：
    - 每处 `${exec:}` 输出 `site S auth A <字段>: runs ${exec:}`；
    - 非凭证字段中出现时，输出“只允许在凭证值中”的提示；未闭合时，输出 malformed 提示；
    - `isReference` 改为“值中含 `${env:` 或 `${exec:` 即视为引用”；
    - exec 认证 `command[0]` 含 `${env:` 或 `${exec:` 时显示 `***`；
    - 所有提示都不含命令文本。
- 验证：`go test ./internal/config/ -run 'TestExecRef|TestExpand|TestCheck|TestDryRun' -count=1`，覆盖 B5 的 config 部分。
- 完成标准：
  - 用例通过；
  - 第三批 `dryrun_test.go` 的现有用例不改期望并通过；
  - `config` 只依赖 `model`。
- 依赖：T03（串行，避免 model 半成品；功能上不依赖）。

### T05 抽出执行器并注入

- 文件：
  - `internal/auth/exec.go`；
  - `internal/auth/exec_test.go`（只加用例）；
  - `internal/auth/exec_windows_test.go`（新，加一个用例）；
  - `internal/wiring/wiring.go`；
  - `internal/wiring/wiring_test.go`（新）。
- 动作：
  - `lastLine` 拆成纯函数 `cleanLastLine(stderr []byte, red *model.Redactor) string`，保留现有的去 ANSI、去控制字符、先脱敏再截断。原方法改为调用它。
  - 抽出：
    ```go
    // RunCommand 解析 argv[0]（相对路径按 dir，裸名经 PATH 且拒绝当前目录），执行 checkResolved 的批处理参数检查，
    // stdin 为空设备，输出上限 4 MiB，超时 timeout。成功返回 stdout；失败返回 *model.ExecFailure，
    // 其 Reason 按 execFailure 现有分类给出（command not found、timed out、exceeded 4 MiB、cleanLastLine 等），不含 argv。
    func RunCommand(ctx context.Context, argv []string, dir string, env []string, timeout time.Duration, red *model.Redactor) ([]byte, error)
    // ExecValue 返回 config.Env.Exec：固定 30s 超时（包级变量，可在测试中替换）、父进程环境、工作目录 dir。
    func ExecValue(dir string, red *model.Redactor) func(argv []string) (string, error)
    ```
  - `runExec` 改为调用 `RunCommand`，`execFailure` 的消息与分类不变。
  - `wiring.Load`：`env.Exec = auth.ExecValue(dir, redactor)`，`dir` 为 `config.Dir()` 的结果。之后换 `Deps.Dir` 不重新注入。
- 验证：
  - `go test ./internal/auth/ ./internal/wiring/ -count=1`：全部现有用例不改期望通过；新增 `TestRunCommand`、`TestExecValue`（相对路径、裸名不匹配当前目录、超时、上限、失败原因已清洗并脱敏、不含参数），以及 wiring 层“换目录后仍按原配置目录解析”。
  - `GOOS=windows go vet ./internal/auth/`；Windows 用例在 T07 的主机上运行。
- 完成标准：
  - 用例通过；
  - `grep -n "exec.CommandContext\|checkResolved(" internal/auth/*.go`（不含测试）只出现在 `RunCommand` 及其定义处；
  - 分层测试通过。
- 依赖：T04。

### T06 接线：选项、翻页分支、use --clear、预扫描 --env、README

- 文件：
  - `internal/commands/{globalopts,siterun,use,prescan,run,site,ops,overlay,skill}.go` 中受影响的调用点；
  - `internal/wiring/wiring.go`（`FetchOptions` 加 `envFlag`）；
  - 测试：
    - `internal/commands/batch4_test.go`（新）；
    - 同步修改：`internal/commands/dynamic_test.go`（`Prescan` 返回值）、`internal/commands/completion_test.go`（选项全集）、`internal/skillgen/skillgen_test.go`（pick 示例）；
  - `README.md`、`README.zh-CN.md`；
  - `internal/skillgen` 中的 pick 示例文本。
- 动作：
  - **选项**：
    - `globalOpts` 加 `AllPages bool`、`MaxPages string`，`bind` 中注册 `all-pages`、`max-pages`；
    - 加入 `reservedOptNames` 与 `globalSpec`（`--max-pages` 带值）；
    - 顶层补全如有手写的选项列表，一并登记（以 `grep -rn '"--pick"' internal/commands` 为准）。
  - **`siterun.run`**（在 `in.P, ... = so.Param, ...` 赋值之后、`wiring.Resolve`/`NewCall` 之前）：
    - 前提检查，失败退出 2：
      - 站点有 `Paginate`；
      - 方法转大写后为 GET（`op.Method`，原始路径调用看 `-X`）；
      - `--max-pages` 为正整数；
      - 用户给的页码与条数为整数。用户的值从 `--<名>` 选项值、`-p <名>=`、`-p query.<名>=` 中取，后者优先，与 `Build` 的覆盖顺序一致，实施时核实。
    - 用户没给时，向 `in.P` 追加 `query.<page>=<start>`；配了 `size_value` 且用户没给条数时，追加 `query.<size>=<size_value>`。
    - 翻页时调用 `request.Paginate`：
      - `Truncated` 时打印警告；
      - `FailedAt>1` 时打印 note；
      - 然后把 `Resp` 交给 `output.Render`；
      - `FailedAt>0` 时，退出码取 `output.ExitCode(status)`，为 0 时改为 7。
  - **`use.go`**：
    - 加 `--clear`，与 `--env`/`--auth` 同时给出时退出 2。
    - 站点不在配置中时：state 有残留就删除并输出 `cleared`；没有残留就报 `site %q does not exist`，退出 1。
    - 站点存在时：删除后调用 `printSelection`。
    - `links` 不动。
  - **`prescan.go`**：
    - `Prescan` 增加第四个返回值 `env string`：收集站点名之前的全局选项部分（含 `--api` 改写）与之后到 `--` 为止的 `--env X`/`--env=X`，取最后一个；
    - `PrescanCompletion` 不变；
    - 同步修改 `dynamic_test.go` 的调用。
  - **`site.go`**：
    - 改为 `loadIndex(d, name, site, refresh, envFlag)`、`loadSiteIndex(..., envFlag)`。`envFlag` 非空且站点没有该环境时，返回 `site %q has no environment %q`，退出 1，不发起拉取。
    - `run.go` 传入预扫描得到的 `env`，其余调用点传 `""`。
  - **`wiring.FetchOptions(d, name, site, envFlag)`**：选环境的规则与 `loadIndex` 相同。
  - **README**：
    - pick 一节改为分项与分组（含 R1–R4 与不兼容说明）；
    - 新增“分页”一节，含退出码与 `--max-pages` 语义；
    - `use --clear`；
    - 保留选项名 `all-pages`、`max-pages`；
    - `${exec:}` 一节：会执行的入口（含拉取定义时的 `source.auth`，以及 login.body/exec.env 中每次调用都运行），dry-run 占位，短于 4 字符不脱敏，回滚前改回 `${env:}` 及原因。
  - skillgen 的投影示例改为 `*.{id,name}`，同步修改 `skillgen_test.go`。
- 验证：`go test ./internal/commands/ ./internal/skillgen/ -count=1`，经 `commands.Run` 覆盖：
  - B1（命令部分）；
  - B2（用法错误、配置、表格、原始路径、必填 page、`-p` 传入同名参数）；
  - B3、B4；
  - B5（命令部分）。
- 完成标准：
  - 用例通过；
  - grep 确认：
    - `request.Paginate` 只在 `siterun.go` 调用；
    - `loadIndex(` 的所有调用点都传了 `envFlag`；
    - `FetchOptions(` 只有一处调用；
  - 分层测试、`go vet ./...` 通过。
- 依赖：T01–T05。

### T06d apic 自身的 skill（设计 0.4 §6）

- 文件：`internal/skillgen/self.go`、`internal/skillgen/self.md`（嵌入模板）、`internal/skillgen/self_test.go`、`internal/commands/skill.go`、`internal/commands/skill_self_test.go`、`internal/spec/simple_doc_test.go`（防漂移，只加测试）、`README.md`、`README.zh-CN.md`（“为 AI agent 生成 skill”一节）。
- 动作：
  - `skillgen.RenderSelf(version string) ([]byte, error)`：用 `embed` 的模板填入版本号，输出带 frontmatter 与 `Marker` 的完整 SKILL.md；`skillgen` 仍只依赖 `model`。
  - `skill` 命令站点参数改为可选：没有站点时不调用 `wiring.Load`，目录默认取 `config.Dir()` 下的 `skills`（取不到时要求 `--out`，退出 2），名字默认 `apic`，经 `skillgen.Write` 写入；stderr 提示 `skillc install` 的用法。带站点时行为不变。
  - 模板按设计 §6“内容”九项写；示例只用占位站点 `pet` 与 `example.com`。
  - 防漂移测试按 B7。
- 验证：`go test ./internal/skillgen/ ./internal/commands/ ./internal/spec/ -count=1`，覆盖 B7 各条。
- 完成标准：B7 通过；分层测试通过；`grep -rn` 模板不含内部主机名与用户目录。
- 依赖：T06c（同改 README 与 skillgen）。

### T07 验收、兼容性与发布说明草稿

- 文件：`docs/release-notes/v0.5.0.md`（新）、本计划的进度记录；验收产物在 `<WP>/tmp/apic-p2d-accept/`。
- 动作：
  - 跑 B6 的检查；按 P3 跑 compat，复用第三批 `compat.sh`，路径改到本批目录。
  - 主机 job 1：`git archive HEAD` 导出源码，设 `TMP/TEMP/GOTMPDIR`，跑 Windows `go test ./internal/... -count=1`（含 `exec_windows_test.go`）。
  - 主机 job 2，real-env。整个脚本包在 `try/finally` 中，finally 删除 `secret/` 与标记文件；不在容器打印任何内容。
    - 搭建：
      - 由脚本从 `<WP>/tmp/apic-try/cfg/config.yaml` 取出 gitlab 站点，写入 `cfg/config.yaml`，加上 `paginate: {page: page, size: per_page}`，并把 pat 的 `value` 改为 `${exec:powershell -NoProfile -ExecutionPolicy Bypass -File bin/tok.ps1}`。job 命令行里不出现 base_url。
      - 复制 `defs/gitlab.yaml`。
      - `bin/tok.ps1` 读 `cfg/secret/token` 并写标记文件；token 由脚本从 `<WP>/tmp/apic-try/cfg/.env` 过滤写入，不回显。
    - 开头执行 `Remove-Item env:GITLAB_TOKEN`。
    - 禁止 `-v`、`config show`，禁止输出 token 文件与回显。
    - 按 B6 real-env 的 1–4 顺序执行，记录退出码、stderr 行数、输出行数。
  - 写发布说明草稿，逐条对照设计“行为变化”与“回滚”两节：
    - 新功能；
    - pick：语法改为分项与分组、语法错误改为请求前退出 2、字面键优先取消、含 `. , = { }` 的键无法访问、R1–R4；
    - 保留选项名；
    - 分页的退出码（非 2xx、3xx 为 7、默认上限为 7、显式上限为 0）；
    - `--env`：拉取改用该环境的 TLS，写错时列操作和 `--help` 也退出 1；
    - `${exec:}`：凭证值中的文本会执行命令；非凭证字段与未闭合的引用报错；login.body/exec.env 中的引用每次调用都运行；
    - `config check` 的变化；
    - 新增的 stderr 行；
    - 回滚：回退前改回 `${env:}`，原因是 v0.4 会把命令文本当作凭证发出、进访问日志、check 也不告警；
    - compat 差异。
  - 进度记录只写 job id、退出码、行数和 `<WP>` 占位路径；写完成总结；评论 issue `my-tools-ab5f`。
- 验证：B1–B6 全部有证据（测试名或 job 输出路径）。
- 完成标准：
  - B6 各项通过，或按 P4/P5 披露 `UNVERIFIED_REAL_ENV`；
  - 设计每条行为变化在发布说明中都有对应条目（逐条核对）。
- 依赖：T01–T06。

## 回滚与恢复

- 每个任务各自一个提交，恢复点写进“进度记录”；回滚某个任务用 `git revert <提交>`。T06 依赖全部领域任务，回滚领域任务前先回滚 T06。
- 不涉及数据迁移和缓存格式变化。测试只用 `t.TempDir()`，不读写真实的 `~/.config/apic`。
- 实施中遇到 Semantic Amendment（例如分组的键名规则或翻页的停止条件需要再调整），停在当前任务，回到 plan review，并请求人工批准。

## 人工 Gate

1. 计划批准：本文档。
2. 当前执行请求：计划批准后，由用户另行发出。
3. 外部动作：push、tag 和发布 v0.5.0 不在本计划内，完成后另行批准。

## 可追溯性

| 设计条款与验收 | 任务 | 验证 |
|---|---|---|
| §1 分项与分组；D1、D2；R1–R4 | T01、T06 | T01、T06 测试；B1 |
| §2 分页配置、前提检查、循环、停止条件、输出；D3–D6、D11 | T02、T03、T06 | T02、T03、T06 测试；B2、B6 real-env |
| §3 `use --clear`；D7 | T06 | B3 |
| §4 拉取跟随 `--env`；D8 | T06 | B4 |
| §5 `${exec:}`：一次扫描、白名单、执行器、dry-run、去重、check；D9、D10 | T04、T05、T06 | T04、T05、T06 测试；B5、B6 real-env |
| 安全：注入、未闭合引用、密钥登记、不执行的入口、失败消息清洗 | T04、T05、T06 | B5 |
| 行为变化与回滚 | T06（README）、T07 | 发布说明；B6 compat |
| §6 apic 自身的 skill；D12 | T06d | B7 |
| 工程规范 GR104、GR108–GR110、SR2002、SR2003 | 全部任务 | 分层测试；B6 |

## 评审处理（第 1 轮）

报告：`<WP>/tmp/apic-p2d-plan-review/{correctness,security}.md`。两个方向均 PASS。

| 评审项 | 处理 |
|---|---|
| P1 B4 两个 TLS 服务端证书相同 | 改为单服务端，用有无 `ca_file` 区分；每例新配置目录，用 spec 命中计数 |
| P2、Q6 `site add` 不会触发 `${exec:}` | 改为 wiring 层测试，加上 `site refresh` + `source.auth` |
| P3、Q4 用法错误时 `source.auth` 已经运行 | 预热缓存，只断言调用所用的认证没有运行；README 写明拉取时会运行 |
| P4、Q2、Q3 执行器丢失 stderr、清洗与批处理检查 | `RunCommand` 返回结构化失败，`cleanLastLine` 纯函数带 redactor，`checkResolved` 留在 `RunCommand` 内并加 Windows 用例 |
| P5、Q5 失败消息格式 | `model.ExecFailure`，由 config 按设计格式化，整行断言 |
| P6 config 无法判断普通路径 | T01 导出 `IsPlainPath` |
| P7 `size_value: 0` | 改为 `*int`，写了就必须大于 0 |
| P8、Q10 `${env:}` 空名 | env 分支保持 `[^}]+` |
| P9、Q9 放错位置在执行之后才报 | 先检查位置，再展开；dry-run 同样 |
| P10 check 显示 `${exec:}` 命令 | `command[0]` 含引用时显示 `***`，所有提示不含命令文本 |
| P11 pick 未定规则 | 补 R1–R4（实现层面的细化，不改变设计的用户结果，在计划批准时一并确认） |
| P12 三个现有测试文件、`token.path` 用例归属 | T06 同步修改三文件；`token.path` 用例归 T01 |
| P13、Q14 `use --clear` 未知站点 | 没有残留时退出 1 |
| Q1 未闭合的 `${exec:` | 报 malformed，不带原文；check 也报；用例断言没有发出请求 |
| Q7 real-env 步骤与清理 | 先 dry-run；try/finally；TMP 指向沙箱；配置由脚本生成；`-ExecutionPolicy Bypass` |
| Q8 发布说明与 README 漏项 | T07 逐条对照设计列出；README 加保留选项名等 |
| 建议 P14–P17、Q11–Q13、Q16、Q17 | 翻页参数在 `in.P` 赋值之后追加；每页复制 Prepared；`QuietInsecure` 字段；第一页失败无 note；方法转大写并在 Paginate 内兜底；补条数非整数与 `max-pages` 参数名用例；进度记录只记 id 与计数 |
| 建议 Q15 明文前缀加引用的告警 | 不采纳：前缀（如 `Bearer `）通常不是密钥，维持“含引用即视为引用” |

## 完成 Gate 与剩余工作

- 完成条件：T01–T07 全部完成，B1–B6 有证据；完成总结写入进度记录；issue 评论。
- 剩余工作：
  - 发布 v0.5.0（外部动作，需授权）；
  - 正式配置的 dh 站点可加 `paginate`（用户配置，不在本计划内）；
  - deferred 项留在 issue。

## 进度记录

（实施开始后填写。）
