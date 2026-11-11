<!-- template_id: design; template_version: 1.2.0 -->
# apic 二期第三批：试用反馈修正（按环境认证、dry-run 缺变量、401 清缓存、--pick 投影、简单定义枚举） 设计

> 状态：Draft 0.2 / 待人工计划批准（0.2 为第 1 轮评审后的未复核修订）

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-12 | Claude（inhere：把试用问题整理成二期第三批候选，开始设计） | 初稿：五项候选（issue my-tools-qm4t）的根因与方案；`--columns` 由 `--pick` 投影覆盖，转为 deferred |
| 0.2 | 2026-11-12 | Claude | 处理第 1 轮两轴评审（阻塞 S1；纠正 C1–C6、S2–S7、S10；建议 C7–C10、S8、S9）：环境声明的认证优先于保存的选择；dry-run 只容忍凭证字段、占位显示为 `***`、警告由命令层输出；`Invalidate` 复用 `Logout`；投影的语法检查推迟到确实投影时；简单定义 enum 的校验写成新规则；有效认证函数放进 model；补全行为变化与回滚说明 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 设计工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：消除 2026-11-11 主机试用（内部 GitLab + DeployHub 两个环境）中遇到的障碍，让“一个站点、多个环境、各用各的 token”的常见配置能直接写出来，并让查详情、看表格不再别扭。
- Scope freeze：只做 issue my-tools-qm4t 列出的五项，其中第 4 项（`--columns`）由第 3 项的投影覆盖，不单独实现。
  - 不做：`${env:X:-默认值}` 语法、清除保存选择的命令、oauth2、`${keyring:}`/`${exec:}`、分页、重定向跟随（仍为 deferred）。
- 暴露度：v0.3.0 已公开发布；第 1、2 项涉及认证与密钥的解析和缓存。按高暴露处理（SR1409），走两轴独立评审。
- Review budget：一轮评审，两个方向：正确性与可实施性；安全与对外契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。
- 评审状态：第 1 轮已完成。正确性方向 PASS（0 阻塞）；安全方向 1 个阻塞项 S1（保存的认证压过环境声明的认证，凭证会发往别的环境）。0.2 已逐条修订；预算只有一轮，0.2 属于未复核修订，见“评审处理”。

## 背景与目标

试用时用简单定义接入了内部 GitLab（PAT 认证）和 DeployHub（`build` 本机、`factory` 经 gofer tun 映射，两个环境的 token 不同）。遇到的问题与根因：

| # | 现象 | 根因（代码） |
|---|---|---|
| 1 | 两个环境 token 不同，只能写 exec 脚本按 `APIC_ENV` 取值；在 exec.env 里列两个变量，则任一缺失都会退出 1 | 认证只能按站点选（`wiring.Resolve`：`--auth` > state > `default_auth`），环境无法指定自己的认证；`config.ResolveAuth` 解析所选认证的全部 `${env:}` 是合理的，问题在于没有按环境选认证的途径 |
| 1b | `--dry-run` 也因缺变量退出 1，想先看请求长什么样都不行 | `wiring.NewCall` 在 dry-run 下同样调用 `ResolveAuth`，缺变量即报错 |
| 2 | token 无效时 `auth status` 仍显示 logged in | `request.Execute` 收到 401 后 `Refresh` 一次并重发；重发仍是 401 时，新拿到的 token 已写入缓存，未清除 |
| 3 | `--pick name,default_branch` 报 segment not found，退出 7 | `model.Pick` 每段只能是键、下标或 `*` |
| 4 | `-o table` 对字段多的接口（hosts）整行铺开，难读 | `output.Table` 取全部顶层键作列；没有选列手段 |
| 5 | 简单定义的参数写不了枚举，只能再写 overlay | `spec/simple.go` 的 `simpleParam` 没有 `enum` 字段 |

用户可见的结果：

- `envs.<env>.auth` 指定该环境使用的认证：`apic dh me --env factory` 自动用 factory 的 token，不用写脚本；保存过的选择也不会把一个环境的 token 发到另一个环境。
- `--dry-run` 缺凭证变量时仍打印请求，并提示哪个变量没设。
- 认证彻底失败（重取 token 后仍 401）时清掉缓存，`auth status` 显示 logged out。
- `--pick data.list.*.id,name,host -o table` 只取需要的列，按给出的顺序成表。
- 简单定义的参数可以直接写 `enum: [...]`。

## 名词

| 名词 | 含义 |
|---|---|
| 环境声明的认证 | `envs.<env>.auth`，选中该环境时使用的认证名（`--auth` 仍可覆盖） |
| 有效认证 | 按优先级算出的本次使用的认证名及其来源（flag / env / saved / site default） |
| 投影段 | `--pick` 路径的最后一段写成 `a,b,c`，从对象中取出这几个键组成新对象 |
| 未设变量 | `${env:NAME}` 在系统环境和配置目录 `.env` 中都找不到 |
| 凭证字段 | 认证中会登记为密钥的字段：`token`、`password`、`value`、`exec.command`、`exec.env`、`login.body` |

其余名词沿用一期设计（[apic-design.md](apic-design.md)）和二期前两批设计（[2026-11-11-apic-phase2-design.md](2026-11-11-apic-phase2-design.md)、[2026-11-11-apic-phase2b-design.md](2026-11-11-apic-phase2b-design.md)）。

## 范围与非目标

范围（只改 apic 仓库）：

1. 环境声明的认证 `envs.<env>.auth`，以及所有显示“当前认证”的地方。
2. `--dry-run` 下所选认证的凭证字段中的未设变量不报错，保留占位并警告。
3. 401 重发后仍 401 时作废该认证的 token。
4. `--pick` 最后一段支持投影。
5. 简单定义参数支持 `enum`。

非目标：

- `--columns`：投影已能选列和定列序，不再加选项（deferred）。
- 清除 `use` 保存的认证选择的命令：S1 修正后，保存的认证不再能压过环境声明，需求变小，deferred。
- 变量默认值语法、其他取值来源、分页等二期剩余项。
- 拉取定义用的 `source.auth`：不受环境声明的认证和 dry-run 容忍影响，保持现状。
- 不改索引格式版本：缓存的索引结构不变，`enum` 字段已存在。

## 已确认事实与规范

- 认证选择：`internal/wiring/wiring.go` `Resolve`，优先级 `--auth` > `state.yaml` > `default_auth`；state 每个站点只保存一份 env 和 auth（`config/state.go`）。显示当前认证的还有 `commands/use.go`（两处 `firstOf`）、`commands/auth.go` `listAuths`（`auth ls`，没有 `--env`）、`skillgen` 的 `(default)` 标注。link 只固定 `--env`/`--auth` 再走 `Resolve`；补全不计算认证。
- `auth login/logout/status` 接受 `--env`/`--auth`，经 `wiring.NewAuth` 严格解析认证，缺变量退出 1。
- `default_auth` 写错名字时，加载与 `config check` 都不报错，到 `Resolve` 才报 `site has no auth`。
- 写保护：`request.CheckProtect` 依据所选认证的 `allow_methods` 判断；所选认证变了，限制随之改变。
- 认证解析：`config.ResolveAuth` 只解析选中的那一个认证，凭证字段的值登记为密钥；`Expand` 碰到第一个未设变量即整串失败。
- 脱敏：`applyStatic` 把整个凭证值登记为密钥；curl 输出中 `Authorization`/`Cookie` 头整体显示为 `***`，其他头和 URL 按已登记密钥替换。
- dry-run 下 exec/login 不执行命令、不发登录请求；缓存命中则用缓存，否则用占位 token `<exec-token>`/`<login-token>`。缓存指纹由展开后的 spec 计算（exec 段、`login.body`）。
- 401 处理：`request.Execute` 只重试一次；`Provider.Refresh` 先清内存与缓存再重取。`Provider.Logout` 已实现“加锁、清内存、删缓存、文件不存在不算错”。Provider 没有 stderr，只有 Verbose。
- `model.Pick` 由 `--pick` 与 exec 的 `token.path`/`expires.path` 共用；在 `*` 之下，元素上的错误被吞成 `null`。`--pick` 在请求发出前由 `ParsePick` 解析。exec 的 `expires.path` 解析失败时静默忽略。
- `output.Table` 对对象数组按键首次出现顺序取列，遇到非对象元素放弃成表并回退 JSON；`model.Object` 保留键顺序。
- overlay 的 `enum` 没有元素校验，直接复制。调用时枚举按字符串比较（`request/build.go`）。
- `config.yaml`、`sites/*.yaml` 与简单定义均为非严格解码，旧版本会忽略新增键。
- `config check` 对明文凭证只提示、不改退出码；overlay 结构错误退出 1。
- 分层：model 无依赖；config/spec/request/output/auth/skillgen 只依赖 model；wiring 与 commands 在上层（`TestLayering` 守卫）。
- 规范：SR1403/SR1410（最小充分方案）、GR108（装配只装配）、GR109（不吞写入错误）、GR110（CLI 经真实分发测试）、SR2002（测试断言用 goutil）。

## 总体方案

五项彼此独立，各自落在已有的包里，不新增包、不改分层。

### 1. 环境声明的认证

```yaml
sites:
  dh:
    envs:
      build:   {base_url: "http://127.0.0.1:18090/api/v1", auth: build}
      factory: {base_url: "http://127.0.0.1:18080/api/v1", auth: factory, protect: true}
    auths:
      build:   {type: bearer, token: "${env:DH_TOKEN_BUILD}"}
      factory: {type: bearer, token: "${env:DH_TOKEN_FACTORY}", allow_methods: [GET, HEAD]}
    default_env: build
```

- 优先级：`--auth` > `envs.<所选环境>.auth` > `state.yaml` 保存的认证 > `default_auth`。
  - 环境声明排在保存的选择之前：环境声明表达“这个主机只该收到这份凭证”，保存的选择不能把别的凭证带过去，`allow_methods` 限制也不会因此失效（S1）。
  - 保存的认证只对没有声明 `auth` 的环境生效。
  - 显式 `--auth` 仍然优先，这是单次调用的明确意图。
- `model` 新增纯函数 `(*Site).EffectiveAuth(env, saved, flag string) (name, source string)`，来源取值 `flag`/`env`/`saved`/`default`/空。`wiring.Resolve`、`use`、`auth ls` 都用它，不再各自 `firstOf`。
- 显示：
  - `use` 输出 `auth: factory (env)`、`auth: x (saved)`、`auth: y (default)`。
  - `use <site> --auth x` 保存时，若当前环境声明了别的认证，stderr 提示：`note: env "factory" declares auth "factory"; the saved auth applies only to environments without one`。
  - `auth ls` 的“当前”标记按 saved env、否则 `default_env` 计算；不新增 `--env`。
  - 显示类命令（`use`、`auth ls`）遇到写错的环境或认证名不报错，与现在一致。
- skillgen 直接读 `site.Envs[*].Auth`（经 `Sanitize`），在环境列表里写出各环境使用的认证，并写明“显式 `--auth` 优先；调用时建议显式给 `--env`”。
- 校验：
  - `envs.<env>.auth` 指向不存在的认证时，`Resolve` 报 `site "dh" env "factory": no auth "x"`，退出 1。
  - `config check` 对 `envs.*.auth` 与 `default_auth` 写错名字给出提示，不改退出码（与明文凭证提示一致）。
- 缓存：token 缓存文件按 `site.env.auth` 命名；加了环境声明后，该环境改用新认证名的缓存，旧缓存文件残留但不再被使用，可用 `apic auth logout <site> --env X --auth <旧名>` 清除。

### 2. dry-run 容忍未设变量

- 只在 `--dry-run` 的请求调用（`NewCall`）中生效。`auth login/logout/status`（`NewAuth`）和拉取定义的 `source.auth` 仍严格；dry-run 若需要先拉定义且用到 `source.auth`，缺变量照样退出 1。
- 只容忍凭证字段中的未设变量。`type`、`in`、`name`、`apply.*`、`login.path`、`allow_methods` 等其他字段缺变量时仍退出 1（保留原文会造成无效头名或类型错误）。
- `ResolveAuth` 增加容忍选项：
  - 同一字符串里的每个引用都处理：已设变量照常展开并登记为密钥，未设变量保留原文 `${env:NAME}`、不登记；
  - 返回去重后的未设变量名列表。
- 警告由命令层（siterun）输出，`NewCall` 只返回列表（GR108）。每个变量一行：`warning: dry-run: GITLAB_TOKEN is not set; a real call would fail`，只写变量名。退出码 0。
- 实际显示（如实写进 README）：
  - 占位会被当作凭证值登记脱敏，`Authorization` 头整体显示为 `***`，自定义头（如 `PRIVATE-TOKEN`）也显示为 `***`；
  - apikey 放在 query 时，占位可能以 URL 编码形式出现（`%24%7Benv%3A...%7D`），它不是密钥；
  - exec/login 的凭证字段含未设变量时，缓存指纹与真实调用不同，dry-run 不会命中已有缓存，显示 `<exec-token>`/`<login-token>`；
  - 是否缺变量以 stderr 警告为准。
- 不变的部分：真实调用缺变量仍退出 1；dry-run 不执行 exec 命令、不发登录请求、不写也不删 token 缓存。

### 3. 401 重发后作废 token

- `request.Authorizer` 接口增加 `Invalidate() error`。
- `Execute` 在重发后的响应仍为 401 时调用 `Invalidate`；返回错误时经 `c.Stderr` 写一行脱敏后的警告（`c.Stderr` 为空则不写），然后照常返回该响应（退出码 4 不变）。
- `Provider.Invalidate` 对 login/exec 直接调用 `Logout`（同一把锁下清内存、删缓存，文件不存在不算错）；exec 没有过期时间、本来就不缓存时只清内存；静态认证什么也不做。
- 影响（写进 release notes）：
  - 接口因权限而非 token 失效返回 401 时，刚取到的有效 token 也会被删，下次调用会多执行一次 exec 或登录。现状下每次 401 已会触发一次 `Refresh` 重取，此项只是多一次。对需要交互（SSO/MFA）或有登录频率限制的认证，要注意这一点。
  - 多个进程同时调用时，一个进程可能删掉另一个刚写入的有效 token，结果是多登录一次，不会用错 token。
- 首次请求 401、`Refresh` 返回 `ErrNoRefresh`（静态认证）的路径不变。

### 4. `--pick` 投影

- 语法：路径最后一段含逗号时视为投影段候选；`a,b,c` 按给出顺序取键组成新对象。前面可以有任意普通段和 `*`：
  - `--pick name,default_branch` → `{"name":..,"default_branch":..}`
  - `--pick data.list.*.id,name,host -o table` → 三列表格，列序即给出的顺序。
- 规则：
  - 只有最后一段可以投影；中间段含逗号仍按普通键处理（与现在相同）。
  - 兼容优先：对象恰有与整段同名的字面键（如 `a,b`）时按普通键取值，不投影。
  - 语法检查推迟到确实要投影时：`ParsePick` 不因逗号报错；要投影时若有空键或重复键，退出 7（`--pick` 失败），提示 `--pick projection "a,,b" has an empty or duplicate key`。这样以前合法的字面键路径行为不变（S6/C6）。
  - 投影键一律是字面键，`*` 和数字没有特殊含义。
  - 结果对象始终按给出的顺序包含全部键，缺少的键值为 `null`，各行列序一致。
  - 投影作用于非对象值：前面没有 `*` 时退出 7，提示 `projection needs an object; add .* before it for arrays`；在 `*` 之下，该元素得到 `null`（沿用 `*` 吞错的规则），此时 `-o table` 因含非对象元素回退为 JSON。
- exec 的 `token.path`/`expires.path` 也用 `Pick`：投影得到对象，`token` 沿用现有“不是字符串”的错误；`expires` 沿用现有静默忽略。
- README 与 `apic skill` 生成的提示补一行投影写法。

### 5. 简单定义 `enum`

```yaml
operations:
  mrs:
    get: /projects/{id}/merge_requests
    params:
      - {name: state, in: query, enum: [opened, closed, merged, all]}
```

- `simpleParam` 增加 `Enum`，转成字符串后写入 `model.Param.Enum`。调用时的枚举校验、`desc`、补全都已支持，无需改动。
- 简单定义新增的校验（overlay 不变）：
  - YAML 中的数字、布尔值按字符串保存和比较（如 `enum: [1, true]` 对应 `"1"`、`"true"`），与调用时按字符串比较一致；
  - 元素不能为空字符串，不能重复；`enum: []` 视为错误；
  - 违反时该站点加载失败，退出 1；
  - 不按 `type` 校验元素。
- `query: [...]` 简写不支持枚举；要枚举就写进 `params`。
- `desc`、`ops` 原样显示枚举值。

## 架构

不新增包，依赖方向不变。

| 项 | 改动位置 |
|---|---|
| 1 | `model.Env` 加 `Auth`，`model.Site.EffectiveAuth`；`wiring.Resolve` 使用；`commands/use.go`、`commands/auth.go`；`skillgen` 读 `Envs[*].Auth`；`config/check.go` 增加名字提示 |
| 2 | `config/expand.go`（容忍选项与未设列表）；`wiring.NewCall` 传入 dry-run 并返回列表；`commands/siterun.go` 输出警告 |
| 3 | `request/execute.go`（接口与调用、警告）；`auth/provider.go`（`Invalidate` 调 `Logout`）；测试替身补方法 |
| 4 | `model/pick.go`；README；`skillgen` 提示文本 |
| 5 | `spec/simple.go` |

## 关键流程

### 选择认证

```text
--auth 给出? ──是──> 用它（flag）
      │否
envs[所选环境].auth? ──是──> 用它（env）
      │否
state 保存了? ──是──> 用它（saved）
      │否
default_auth? ──是──> 用它（default）
      │否
不认证
```

### 401

```text
发送 → 401 → Refresh（清缓存，重取）→ 重发 → 仍 401 → Invalidate（Logout）→ 返回 401（退出 4）
                  └─ 重取失败 → 返回错误（缓存已在 Refresh 中清除）
```

## 安全、数据、运维与回滚

- **凭证隔离**：环境声明的认证优先于保存的选择，保存的选择不会把凭证带到声明了别的认证的环境；只有显式 `--auth` 能覆盖。
- **密钥**：dry-run 保留的 `${env:NAME}` 原文不是密钥；已设变量的登记和脱敏不变。真实调用的失败行为不变。
- **缓存**：第 3 项只会多删缓存，不会多写；dry-run 不写不删缓存。
- **配置兼容**：`envs.<env>.auth` 与简单定义的 `enum` 都是新增字段；没写这些字段的现有配置行为不变。
- **行为变化**（逐条写进 release notes，并对照 README 的退出码表）：
  - dry-run 时凭证字段缺变量：以前退出 1，现在退出 0 并警告。要检查变量是否齐全，改用 `apic auth status <site> --env X`（严格解析，缺变量退出 1）或真实调用。
  - `--pick` 最后一段含逗号且对象没有该字面键：以前退出 7（not found），现在按投影返回。
  - 重发后仍 401 会删除该认证的 token 缓存。
  - `use` 输出的 `auth:` 行多了来源后缀，解析该行的脚本需调整。
- **回滚到 v0.3.x**：
  - 旧版本忽略 `envs.<env>.auth`，所有环境改用保存的选择或 `default_auth`，可能把一个环境的 token 发往另一个环境。回退前先删掉 `envs.*.auth`，或调用时对每个环境显式加 `--auth`。
  - 旧版本忽略简单定义的 `enum`，只是少了校验。
  - 不改缓存格式，退回无需清缓存；按环境认证新增的缓存文件在旧版本中不会被使用。

## 决策

| ID | 决策 | 理由 |
|---|---|---|
| D1 | 用 `envs.<env>.auth` 解决按环境用 token，不做 exec.env 的懒解析 | 声明式、无需脚本；所选认证全部解析是正确的失败语义 |
| D2 | 优先级 `--auth` > 环境声明 > 保存的选择 > `default_auth` | 凭证按环境隔离，`allow_methods` 不被绕过（S1）；显式 flag 仍可覆盖 |
| D3 | dry-run 只对凭证字段容忍未设变量，警告由命令层输出 | 让用户先看请求；其他字段保留原文会产生无效请求；装配层不写输出 |
| D4 | 重发仍 401 才作废，复用 `Logout` | 最小改动，正好覆盖观察到的问题 |
| D5 | 投影只在最后一段，语法检查推迟到确实投影时，缺键为 `null` | 一个规则同时解决多字段和选列；以前合法的字面键路径不受影响 |
| D6 | 不做 `--columns` | 投影已覆盖；SR1403 不加第二个机制 |
| D7 | 简单定义的 `enum` 只在 `params` 中支持，校验是简单定义新增规则 | `query` 简写保持字符串列表；不改 overlay 的现有行为 |
| D8 | 有效认证函数放在 model | wiring、commands 共用；skillgen 只依赖 model，直接读字段 |

## 评审处理

第 1 轮两个方向（正确性与可实施性；安全与对外契约），全部在 0.2 处理：

| ID | 级别 | 处理 |
|---|---|---|
| S1 | 阻塞 | 优先级改为环境声明在保存的选择之前（D2）；`use --auth` 被环境声明遮蔽时提示；回滚说明补跨环境风险 |
| S2 | 纠正 | 检查变量是否齐全改为用 `auth status` |
| S3、C1、C9 | 纠正/建议 | 如实写明占位显示为 `***`、query 中可能 URL 编码、dry-run 不命中缓存；dry-run 不写不删缓存 |
| S4、C2、C7 | 纠正/建议 | 只容忍凭证字段；同串多引用逐个处理；警告只写变量名并去重、由 siterun 输出；`NewAuth` 与 `source.auth` 仍严格 |
| S5、C4 | 纠正 | `Invalidate` 复用 `Logout`；警告经 `c.Stderr` 输出；写明权限型 401 与多进程竞争的影响 |
| S6、C6 | 纠正 | 投影语法检查推迟到确实投影时，错误退出 7；`*` 下非对象为 `null`；键序固定；投影键为字面键 |
| S7、C5 | 纠正 | 有效认证函数放 model；`auth ls` 用 saved env 或 `default_env`，不加 `--env`；显示类命令不因写错名字报错 |
| C3、S9 | 纠正/建议 | 简单定义 enum 的校验写成新规则；数字/布尔按字符串；`enum: []` 为错误；不按 type 校验 |
| S8 | 建议 | skill 文本经 `Sanitize`，写明显式 `--auth` 优先 |
| S10、C8 | 纠正/建议 | 补缓存文件名变化、回滚的跨环境风险、Q2 表述、release notes 行为变化清单（含 `use` 输出后缀） |
| C10 | 建议 | 由 S1 的优先级修正覆盖，不再需要单独警告 |

## 待确认事项

- Q1（非阻塞）：`config check` 对认证名写错只提示、不改退出码，与明文凭证提示一致；若希望作为结构错误退出 1，需另行决定。

## 结论与人工计划 Gate

本设计确定五项修正的行为与落点，第 4 项由投影覆盖。第 1 轮评审的阻塞项 S1 已在 0.2 修订，属于未复核修订。等待用户批准设计；批准只授权编写实施计划，不授权实施。
