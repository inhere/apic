<!-- template_id: plan; template_version: 1.3.1 -->
# apic 二期第四批（pick 分组、分页、use --clear、拉取跟随 --env、${exec:}）实施计划

> 状态：Draft 0.1 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-12 | Claude（inhere：“按 data.{a,b},c 改 pick；批准 apic 二期第四批设计 0.3（接受未复核修订）”） | 初稿：3 个波次、7 个任务，覆盖设计 0.3 的五项 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 规划工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：交付设计 0.3 的五项并通过验收：
  1. `--pick` 分项与分组；
  2. 站点级分页；
  3. `use --clear`；
  4. 拉取定义跟随 `--env`；
  5. `${exec:}`。
- Scope freeze：只做设计 0.3 的范围；设计的非目标在实施中即使发现需要，也只记为 deferred。
- 评审预算：与设计相同，按高暴露处理（SR1409），一轮两轴独立评审：正确性与可执行性；安全与契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。
- 公开仓库约束：文档、测试数据与提交信息不写内部主机名、用户目录和账号；测试用 `example.com` 或 httptest 地址；真实值只出现在 `<WP>/tmp/` 下的验收产物与 gofer job 中。

## 目标与完成定义

**目标**：v0.4.0 的用户升级后，行为符合设计 0.3 各节与 D1–D11。

**完成定义（验收）**：

1. **B1 `--pick`**：
   - `model` 单测表覆盖设计 §1 的全部示例：
     - 单路径直接输出值；
     - `code,data.username`；
     - `data.{username,id},code` 平铺；
     - 别名；
     - `data.list.*.{id,name}` 输出对象数组；
     - `data.list.*.{id,name},data.total` 的键为 `list` 和 `total`；
     - 嵌套分组；
     - 无键名段时必须写别名；
     - 多项中缺失为 `null`；
     - 单项缺失时退出 7；
     - `*` 下元素错误为 `null`。
   - 语法错误逐条用例，都在 `ParsePick` 返回退出 2：
     - 空项、空段；
     - 别名为空或字符不合法；
     - 花括号不配对；
     - 分组不在最后一段；
     - 静态可知的重复键（含 `*` 之下的情况）；
     - `--pick` 首尾为逗号。
   - 旧投影的用例改写为新语法，不再有字面键优先的用例。
   - exec 的 `token.path` 写成分组时，沿用现有的“不是字符串”错误，退出 1。
   - 经 `commands.Run`：`--pick 'data.list.*.{id,name}' -o table` 首行按空白拆分为 `[id name]`；语法错误时退出 2，且服务端没有收到请求。
2. **B2 分页**，经 `commands.Run` 和 httptest 服务端（按页码返回、记录收到的 query）覆盖：
   - 两种响应：GitLab 式（响应本身是数组）与 DeployHub 式（`items: data.list`、`total: data.total`），都得到完整的合并数组；
   - 服务端把条数截到 2，`size_value: 5`：取齐全部数据，不在第一页停止（S2/C11 回归）；
   - 配 `total`：取够总数即停止，不多发请求；
   - 只有一页数据、未配 `total`：共请求两次，第二次得到 0 条；
   - 起始页：用户给的页码、`start: 0`、默认 1；用户给的条数参数优先于 `size_value`；
   - 操作把 `page` 声明为必填、用户未给：仍能翻页（起始页写入参数输入）；
   - 原始路径调用（`apic site /path --all-pages`）同样能翻页；
   - 第 2 页返回 500：stdout 为该页的原始响应体，stderr 有 `note: page 2 returned HTTP 500; 1 earlier pages discarded`，退出码为 5；
   - 第 2 页返回 302：退出 7；
   - 第 2 页不是 JSON、`items` 不是数组、`total` 不是整数：各退出 7；
   - 服务端忽略页码（每页相同）：退出 7，消息含 `repeats`；
   - `--max-pages 2` 而数据有 3 页：输出前 2 页，stderr 警告，退出 0；
   - 达到默认上限（测试中把上限常量设小）：退出 7，stdout 为空；
   - 用法错误退出 2，服务端没有收到请求：没配 `paginate`、方法不是 GET、页码不是整数、`--max-pages 0` 或 `abc`。用 `source.auth` 和 exec 认证写标记文件，断言认证命令没有运行；
   - dry-run 只打印一条 curl，stderr 有 note；
   - `--all-pages --pick '*.{id,name}' -o table` 输出表格；
   - `insecure` 告警只打印一次；
   - 401 后重取 token，逐页生效（第 2 页先 401、重取后 200，结果完整）；
   - 配置校验（退出 1）：缺 `page`；`size_value` 不是正整数，或配了 `size_value` 却没配 `size`；`start` 为负；`items`/`total` 含逗号、分组或 `*`；
   - 一个操作参数名为 `all-pages` 的定义：生成的选项里没有它，用 `-p` 能传入。
3. **B3 `use --clear`**：
   - 清除后，`state.yaml` 中没有该站点，输出与无参 `use` 一致；
   - 没有保存的选择时也成功；
   - `--clear --env x` 退出 2；
   - 站点已从配置删除、state 有残留：`--clear` 删除残留，输出 `cleared`；
   - `state.links` 不变。
4. **B4 拉取跟随 `--env`**，用两个环境、各自不同 `ca_file` 的 httptest TLS 服务端提供 spec：
   - `apic dh ops`、`apic --env factory dh me`、`apic dh me --env factory`、`apic --api dh --env factory me`、`link` 生成的脚本参数：拉取时都用了对应环境的客户端（用错 CA 会 TLS 失败来区分）；
   - 站点前后都给 `--env` 时取最后一个，与 gcli 的解析一致（先用测试核实 gcli 的“后者覆盖”）；
   - `--env bogus`（含列操作 `apic dh --env bogus`）：退出 1，消息为 `site "dh" has no environment "bogus"`，没有发起拉取；
   - state 里保存的环境已被删除时：`ops` 仍成功（维持现状）。
5. **B5 `${exec:}`**：
   - config 层，用替身执行函数：
     - 凭证值（token、password、value、exec.env、login.body）中都能展开；
     - 与文字、`${env:}` 混写；
     - 一次扫描：环境变量值为 `${exec:x}` 时替身未被调用；替身输出含 `${env:Y}` 时原样保留；
     - 去重：同一文本只调用一次；失败结果不缓存，再次解析会重新调用；
     - 非凭证字段（`exec.command`、`apply.value`、`login.apply.value`、`token.path`、`allow_methods`、`login.path`、`username`）中出现时，退出 1，消息为设计文案；
     - 内容为空、含 `${` 或 `}` 时报错；
     - 输出为空、多行或含控制字符时报错；末尾的 `\r\n` 被去掉，首尾空格保留；
     - dry-run 不调用替身，展开为 `<exec>`；
     - 执行函数为空时报 `${exec:} is not available`；
     - 失败消息不含命令名与参数；
     - 结果登记为密钥。
   - auth 层：执行器的现有 exec 测试全部不改期望。新增导出执行器的测试：相对路径按配置目录解析；裸名不匹配当前目录；30s 超时可注入为短时以便测试；4 MiB 上限。
   - 经 `commands.Run`，用真实脚本（Unix 下 sh 脚本，Windows 主机下 bat/ps1）输出 token：
     - 服务端收到展开后的 token；
     - `-v` 与 dry-run 的输出中不出现 token；
     - dry-run 的自定义头显示 `<exec>`；
     - 没选中的认证里的 `${exec:}` 不运行（标记文件不存在）；
     - `site add` 用临时目录时，相对路径仍按真实配置目录解析；
     - `config check` 对每处 `${exec:}` 输出 `runs ${exec:}`，混写值不报 plaintext，放错位置的给出提示。
6. **B6 工程、兼容性与 real-env**：
   - `go vet ./...`、`GOOS=windows go vet ./...`、`go test ./... -count=1`、分层守卫测试通过；单个文件不超过 1000 行（GR104）。
   - Windows 主机上 `go test ./internal/... -count=1` 通过。
   - compat：v0.4.0 基线二进制与新二进制跑同一份 dry-run 清单。输出逐字节一致，只有一个预期差异：含旧投影写法的 `--pick` 条目（清单中如有，单独列出）。
   - **real-env**（主机，沙箱见 T07）：
     - gitlab：`gitlab projects --search <公开词> --per_page 5 --all-pages --max-pages 3 --pick '*.{id,path_with_namespace}' -o table` 退出 0，行数不超过 15，stderr 有 `--max-pages` 警告（或数据不足 3 页时没有）；
     - `${exec:}`：pat 的 `value` 改为 `${exec:powershell -NoProfile -File bin/tok.ps1}`，脚本从沙箱里的文件读 token、不回显；`gitlab version` 退出 0；
     - `gitlab version --dry-run` 显示 `<exec>`，且没有运行脚本（脚本写标记文件，断言其不存在）；
     - `config check` 列出 `runs ${exec:}`；
     - 证据中不记 base_url 与 token。

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
  - 除 Go 构建缓存外，只读写 `<WP>/tmp/apic-p2d-accept/`。

- `host_or_non_offline_action=REQUIRED`

## 输入与批准证据

- 设计：`docs/design/2026-11-12-apic-phase2d-design.md` Draft 0.3，已批准。
  - 批准原话（2026-11-12）：“无需兼容，现在只有我自己在使用；按 data.{a,b},c 改 pick；批准 apic 二期第四批设计 0.3（接受未复核修订）”。
  - 记录批准的提交：`14adf91`。
- 设计第 1 轮评审（C1–C23、S1–S17）已并入 0.2/0.3，见设计“评审处理”；报告在 `<WP>/tmp/apic-p2d-review/`。
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
| CAP-05 | `${exec:}` 执行 | `auth/exec.go` `runExec`、`config.expand` | 部分 | 从 `runExec` 抽出导出的执行器；config 经注入函数调用 | OWNER_EXTENSION | config 不能依赖 auth；`runExec` 绑定 Provider | 一份执行器，exec 认证与 `${exec:}` 共用 |

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
5. **P5 GitLab 可达**：主机对沙箱 gitlab 站点匿名 `version` 返回 401 或 200；否则 real-env 的 GitLab 项标 `UNVERIFIED_REAL_ENV`。

## 波次与依赖

| 波次 | 任务 | 依赖 |
|---|---|---|
| W1 领域 | T01 → T02 → T03；T04 → T05 | T02 用 T01 的新 `ParsePick` 校验路径；T03 用 T02 的 `model.Paginate`；T05 的注入点在 T04 的 `Env.Exec` |
| W2 接线 | T06 | T01–T05 |
| W3 验收 | T07 | T01–T06 |

**执行方式**：

- 实施方：sonnet 子 agent，在主 checkout 中**串行**执行 T01 → T02 → T03 → T04 → T05 → T06。T01、T02 同在 `model` 包，并行容易互相看到半成品。T07 由我自己做。
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

- 文件：`internal/model/pick.go`、`internal/model/pick_test.go`。
- 动作：
  - 重写 `Pick` 的内部结构，对外签名不变：`ParsePick(path string) (Pick, error)`、`(Pick) Apply(v any) (any, error)`、`(Pick) String() string`。
  - 内部结构：项列表，每项含别名、段列表，以及可选的分组（递归的项列表）。
  - 解析：
    - 按不在花括号内的逗号分项；项首 `别名=`；按点分段；末段 `{...}` 递归解析。
    - 键名推导：取别名，否则取最后一个键名段（不是 `*`、下标或分组的段）。
    - 静态检查同一对象中的重复键：可展开的分组平铺后，与同级各键一起比较；`*` 之后的分组内单独比较。
    - 所有语法错误用 `Errorf(ExitUsage, "--pick %q: <原因>", path)`。
  - 求值：
    - 只有一项且不是可展开的分组时，等同现有的单路径：缺失退出 7；`*` 之下的元素错误为 `null`。
    - 其他情况组装 `Object`（键序即书写顺序），缺失为 `nil`。
  - 删除 `projectPick` 与 `projectionHint`（`03c7470` 引入），改写原有的投影用例，期望改为新语义。
- 验证：`go test ./internal/model/ -run 'TestPick' -count=1`，覆盖 B1 的单测表。
- 完成标准：
  - B1 的 model 部分通过；
  - `grep -n projectPick internal/` 无结果；
  - exec 的 `token.path`/`expires.path` 的现有测试不改期望并通过（`go test ./internal/auth/ -count=1`）。
- 依赖：无。

### T02 分页配置：模型与加载校验

- 文件：`internal/model/config.go`、`internal/config/load.go` 及测试（`internal/config/paginate_test.go`，新）。
- 动作：
  - 模型：
    ```go
    type Paginate struct {
        Page      string `yaml:"page"`
        Size      string `yaml:"size,omitempty"`
        SizeValue int    `yaml:"size_value,omitempty"`
        Start     *int   `yaml:"start,omitempty"`
        Items     string `yaml:"items,omitempty"`
        Total     string `yaml:"total,omitempty"`
    }
    // Site 加：Paginate *Paginate `yaml:"paginate,omitempty"`
    // StartPage 返回 Start，未写时为 1。
    func (p *Paginate) StartPage() int
    ```
  - 校验：在 `validateSite` 中，错误格式为 `%s site %q: paginate: <原因>`，退出 1。
    - `items`/`total` 非空时用 `model.ParsePick` 检查是单项、无分组、无 `*` 的路径。
- 验证：`go test ./internal/config/ -run TestPaginateConfig -count=1`，覆盖 B2 的配置校验各条，以及合法配置加载后字段正确（`start: 0` 与未写可区分）。
- 完成标准：用例通过；没有 `paginate` 的现有配置测试不变。
- 依赖：T01（`ParsePick` 的新结构用于判断“单项无分组”）。

### T03 翻页循环

- 文件：`internal/request/paginate.go`（新）、`internal/request/paginate_test.go`（新）。
- 动作：新增以下接口。
  ```go
  type PageOptions struct {
      Spec     model.Paginate
      First    int  // 起始页码（调用方已确定）
      Max      int  // 上限页数，>0
      Explicit bool // 上限来自 --max-pages
  }
  type PageResult struct {
      Resp      *Response // 成功时：最后一页的状态与头，Body 为合并数组的 JSON；失败页：该页原样
      FailedAt  int       // 非 2xx 的页序号（从 1 起），0 表示没有
      Discarded int       // 失败时丢弃的前序页数
      Truncated bool      // 显式上限截断
      Pages     int
  }
  // Paginate 每页改写 URL 中 Spec.Page 参数后调用 Execute。dry-run 时只执行第一页并返回其 Response。
  // 退出 7 的情况返回 model.Errorf(ExitPostProcess, "--all-pages failed: page %d: ...")；
  // 默认上限截断返回 model.Errorf(ExitPostProcess, "--all-pages: reached the default limit of %d pages; pass --max-pages to raise or accept it")。
  func Paginate(ctx context.Context, c Call, opt PageOptions) (*PageResult, error)
  ```
  - 合并数组用 `model.EncodeJSON` 编码，元素保持 `model.Decode` 的键序。
  - 相邻页比较用编码后的字节。
  - `total` 从第一页读取：值经 `model.Decode` 后为整数（json.Number 无小数）且不小于 0。
  - `insecure` 告警只在第一页打印：第一页之后把 `Call.Stderr` 置为不打印该告警的包装，或加一个内部标志，实施时选其一并在注释中说明。
  - 默认上限常量 `DefaultMaxPages = 100` 定义为可在测试中替换的包级变量。
- 验证：`go test ./internal/request/ -run TestPaginate -count=1`，用 httptest 覆盖 B2 中与循环相关的各条（截断条数、total、单页两次请求、500/302、非 JSON、items 非数组、total 非整数、重复页、显式与默认上限、401 逐页重试、dry-run 只请求一次、`insecure` 告警一次）。
- 完成标准：用例通过；`request` 只依赖 `model`。
- 依赖：T02。

### T04 `${exec:}` 展开（config 层）

- 文件：`internal/config/expand.go`、`internal/config/dotenv.go`、`internal/config/check.go`，以及测试 `internal/config/exec_ref_test.go`（新）。
- 动作：
  - `Env` 加 `Exec func(argv []string) (string, error)`，以及一个私有的去重表（带互斥锁）。
  - 正则改为一次识别两种引用：`\$\{(env|exec):([^}]*)\}`，在 `ReplaceAllStringFunc` 中分别处理，替换结果不再扫描。
    - `${exec:` 之后到第一个 `}` 之间出现 `${` 时报错（检测方式：匹配内容含 `${`）。
  - `expand` 增加参数 `allowExec bool`（只有凭证值为 true）和字段标签（用于错误消息，如 `value`、`login.body.password`）。
    - 凭证值：`Token`、`Password`、`Value`、`Exec.Env` 的值、`Login.Body` 的字符串。
    - 其余字段与 `Exec.Command` 传 false，遇到 `${exec:` 时报 `auth %q: ${exec:} is only allowed in credential values`。
  - 执行：
    - 按空白切分 → 去重表查找 → 调用 `Env.Exec`。
    - 输出去掉末尾的 `\r\n`，校验非空、无其他控制字符。
    - 成功写入去重表，失败不写；结果 `sec.Add`。
    - 失败消息：`auth %q %s: ${exec:} failed (%s)`，其中 `%s` 是执行函数返回的错误文本（不含命令）。
  - dry-run（`missing != nil`）：`${exec:...}` 替换为 `<exec>`，不调用、不登记。
  - `check.go`：
    - 每处 `${exec:}` 输出 `site S auth A <字段>: runs ${exec:}`；
    - `isReference` 改为“值中含 `${env:` 或 `${exec:` 即视为引用”；
    - 非凭证字段中出现 `${exec:` 时输出 `site S auth A <字段>: ${exec:} is only allowed in credential values`。
- 验证：`go test ./internal/config/ -run 'TestExecRef|TestExpand|TestCheck' -count=1`，覆盖 B5 的 config 部分。
- 完成标准：
  - 用例通过；
  - 第三批 dry-run 的现有测试（`dryrun_test.go`）不改期望并通过；
  - `config` 只依赖 `model`。
- 依赖：无（与 T01–T03 独立；为避免 model 半成品，排在 T03 之后串行）。

### T05 抽出执行器并注入

- 文件：`internal/auth/exec.go`、`internal/auth/exec_test.go`（只加用例）、`internal/wiring/wiring.go`。
- 动作：
  - 从 `runExec` 抽出：
    ```go
    // RunCommand 运行 argv[0]，argv[0] 按 dir 解析（相对路径）或经 PATH（裸名，拒绝当前目录）；stdin 为空设备，
    // 输出上限 4 MiB，超时 timeout。返回 stdout、退出码；失败时 error 的文本不含 argv。
    func RunCommand(ctx context.Context, argv []string, dir string, env []string, timeout time.Duration) (stdout []byte, code int, err error)
    // ExecValue 返回 config.Env.Exec 需要的函数：固定 30s 超时、父进程环境、工作目录 dir。
    // 失败时错误文本为 "exit N: <stderr 最后一行>" 或 "command not found" 等，与 execFailure 的分类一致。
    func ExecValue(dir string) func(argv []string) (string, error)
    ```
  - `runExec` 改为调用 `RunCommand`，`execFailure` 的消息与分类不变。
  - 超时常量可在测试中替换。
  - `wiring.Load`：`env.Exec = auth.ExecValue(dir)`，`dir` 为 `config.Dir()` 的结果。`site add` 换临时 `Deps.Dir` 时，不重新注入。
- 验证：
  - `go test ./internal/auth/ -count=1`：全部现有用例不改期望通过，新增 `TestRunCommand`、`TestExecValue`（相对路径、裸名不匹配当前目录、超时、上限、失败文本不含参数）。
  - `GOOS=windows go vet ./internal/auth/`。
- 完成标准：
  - 用例通过；
  - `grep -n "exec.CommandContext" internal/auth/` 只剩 `RunCommand` 一处；
  - 分层测试通过。
- 依赖：T04。

### T06 接线：选项、翻页分支、use --clear、预扫描 --env、README

- 文件：
  - `internal/commands/{globalopts,siterun,use,prescan,run,site,ops,overlay,skill}.go` 中受影响的调用点；
  - `internal/wiring/wiring.go`（`FetchOptions` 加 `envFlag`）；
  - 测试：`internal/commands/batch4_test.go`（新）；
  - `README.md`、`README.zh-CN.md`；
  - `internal/skillgen` 中的 pick 示例文本。
- 动作：
  - `globalOpts` 加 `AllPages bool`、`MaxPages string`，在 `bind` 中注册 `all-pages`、`max-pages`；加入 `reservedOptNames` 与 `globalSpec`（`--max-pages` 带值）。顶层补全如有手写的选项列表，一并登记（以 `grep -rn '"--pick"' internal/commands` 为准）。
  - `siterun.run`：
    - 在 `wiring.Resolve`/`NewCall` 之前做翻页的前提检查（退出 2）：站点有 `Paginate`；方法为 GET（`op.Method`，原始路径调用看 `-X`）；`--max-pages` 为正整数。
    - 从 `--<page>` 选项值、`-p <page>=`、`-p query.<page>=` 中取用户给的页码与条数，检查是否为整数。
    - 用户没给时，向 `in.P` 追加 `query.<page>=<start>` 与 `query.<size>=<size_value>`。
    - 翻页时调用 `request.Paginate`，按结果打印 note 或警告，再把 `Resp` 交给 `output.Render`。
    - `FailedAt>0` 时，退出码取 `output.ExitCode(status)`，为 0 时改为 7。
  - `use.go`：
    - 加 `--clear`，与 `--env`/`--auth` 同时给出时退出 2。
    - 先查 state：站点不在配置中但 state 有残留时，删除并输出 `cleared`；否则删除后调用 `printSelection`。
    - `links` 不动。
  - `prescan.go`：
    - `Prescan` 增加返回值 `env string`：收集站点名之前的全局选项部分（含 `--api` 改写）与之后到 `--` 为止的 `--env X`/`--env=X`，取最后一个。
    - `PrescanCompletion` 不变。
  - `site.go`：`loadIndex(d, name, site, refresh, envFlag)`、`loadSiteIndex(..., envFlag)`。`envFlag` 非空且站点没有该环境时，返回 `site %q has no environment %q`，退出 1，不发起拉取。
    - `run.go` 传入预扫描得到的 `env`，其余调用点传 `""`。
  - `wiring.FetchOptions(d, name, site, envFlag)`：选环境的规则与 `loadIndex` 相同。
  - README：
    - pick 一节改为分项与分组（含不兼容说明）；
    - 新增“分页”一节；
    - `use --clear`；
    - `${exec:}` 一节，写明会执行的入口、dry-run 占位、短于 4 字符不脱敏、回滚前改回 `${env:}`。
  - skillgen 的投影示例改为 `*.{id,name}`。
- 验证：`go test ./internal/commands/ -run 'TestBatch4' -count=1`，经 `commands.Run` 覆盖 B1（命令部分）、B2（用法错误、配置、表格、原始路径、必填 page、`-p` 传入同名参数）、B3、B4、B5（命令部分）。
- 完成标准：
  - 用例通过；
  - grep 确认：`request.Paginate` 只在 `siterun.go` 调用；`loadIndex(` 的所有调用点都传了 `envFlag`；`FetchOptions(` 只有一处调用；
  - 分层测试通过；
  - `go vet ./...` 通过。
- 依赖：T01–T05。

### T07 验收、兼容性与发布说明草稿

- 文件：`docs/release-notes/v0.5.0.md`（新）、本计划的进度记录；验收产物在 `<WP>/tmp/apic-p2d-accept/`。
- 动作：
  - 跑 B6 的检查；按 P3 跑 compat，复用第三批 `compat.sh`，路径改到本批目录。
  - 主机 job：`git archive HEAD` 导出源码，跑 Windows `go test`。
  - real-env 沙箱（主机 job 内完成，不在容器打印任何内容）：
    - 建 `<WP>/tmp/apic-p2d-accept/cfg/`：`config.yaml` 只含 gitlab 站点，加上 `paginate: {page: page, size: per_page}`；`defs/gitlab.yaml`；
    - `bin/tok.ps1` 从 `cfg/secret/token` 读 token，并写标记文件；token 由脚本从 `<WP>/tmp/apic-try/cfg/.env` 过滤写入 `cfg/secret/token`，不回显；
    - job 开头 `Remove-Item env:GITLAB_TOKEN`；禁止 `-v`、`config show`、输出 token 文件与脚本回显；
    - 按 B6 real-env 的顺序执行，记录退出码、stderr 行数与表格行数；结束时删除 `secret/`。
  - 写发布说明草稿：
    - 新功能；
    - 不兼容变化：pick 语法、保留选项名、`--env` 校验、`${exec:}` 文本会执行命令、`config check` 变化；
    - 新增的 stderr 行；
    - 回滚（回退前改回 `${env:}`，删除 `paginate` 不必要）；
    - compat 差异。
  - 进度记录写完成总结；评论 issue `my-tools-ab5f`。
- 验证：B1–B6 全部有证据（测试名或 job 输出路径）。
- 完成标准：B6 各项通过，或按 P4/P5 披露 `UNVERIFIED_REAL_ENV`；我对照设计核对发布说明条目齐全。
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
| §1 分项与分组；D1、D2 | T01、T06 | T01、T06 测试；B1 |
| §2 分页配置、前提检查、循环、停止条件、输出；D3–D6、D11 | T02、T03、T06 | T02、T03、T06 测试；B2、B6 real-env |
| §3 `use --clear`；D7 | T06 | B3 |
| §4 拉取跟随 `--env`；D8 | T06 | B4 |
| §5 `${exec:}`：一次扫描、白名单、执行器、dry-run、去重、check；D9、D10 | T04、T05、T06 | T04、T05、T06 测试；B5、B6 real-env |
| 安全：注入、密钥登记、不执行的入口、失败消息 | T04、T05、T06 | B5 |
| 行为变化与回滚 | T06（README）、T07 | 发布说明；B6 compat |
| 工程规范 GR104、GR108–GR110、SR2002、SR2003 | 全部任务 | 分层测试；B6 |

## 完成 Gate 与剩余工作

- 完成条件：T01–T07 全部完成，B1–B6 有证据；完成总结写入进度记录；issue 评论。
- 剩余工作：发布 v0.5.0（外部动作，需授权）；正式配置的 dh 站点可加 `paginate`（用户配置，不在本计划内）；deferred 项留在 issue。

## 进度记录

（实施开始后填写。）
