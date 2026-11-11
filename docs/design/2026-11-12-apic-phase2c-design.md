<!-- template_id: design; template_version: 1.2.0 -->
# apic 二期第三批：试用反馈修正（按环境认证、dry-run 缺变量、401 清缓存、--pick 投影、简单定义枚举） 设计

> 状态：Draft 0.1 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-12 | Claude（inhere：把试用问题整理成二期第三批候选，开始设计） | 初稿：五项候选（issue my-tools-qm4t）的根因与方案；`--columns` 由 `--pick` 投影覆盖，转为 deferred |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 设计工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：消除 2026-11-11 主机试用（内部 GitLab + DeployHub 两个环境）中遇到的障碍，让“一个站点、多个环境、各用各的 token”的常见配置能直接写出来，并让查详情、看表格不再别扭。
- Scope freeze：只做 issue my-tools-qm4t 列出的五项，其中第 4 项（`--columns`）由第 3 项的投影覆盖，不单独实现。
  - 不做：`${env:X:-默认值}` 语法、`use --reset`、oauth2、`${keyring:}`/`${exec:}`、分页、重定向跟随（仍为 deferred）。
- 暴露度：v0.3.0 已公开发布；第 1、2 项涉及认证与密钥的解析和缓存。按高暴露处理（SR1409），走两轴独立评审。
- Review budget：一轮评审，两个方向：正确性与可实施性；安全与对外契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。

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

- `envs.<env>.auth` 指定该环境默认使用的认证：`apic dh me --env factory` 自动用 factory 的 token，不用写脚本。
- `--dry-run` 缺变量时仍打印请求，并提示哪个变量没设。
- 认证彻底失败（重取 token 后仍 401）时清掉缓存，`auth status` 显示 logged out。
- `--pick data.list.*.id,name,host -o table` 只取需要的列，按给出的顺序成表。
- 简单定义的参数可以直接写 `enum: [...]`。

## 名词

| 名词 | 含义 |
|---|---|
| 环境默认认证 | `envs.<env>.auth`，选中该环境且没有更高优先级的认证选择时使用的认证名 |
| 投影段 | `--pick` 路径的最后一段写成 `a,b,c`，从对象中取出这几个键组成新对象 |
| 未设变量 | `${env:NAME}` 在系统环境和配置目录 `.env` 中都找不到 |

其余名词沿用一期设计（[apic-design.md](apic-design.md)）和二期前两批设计（[2026-11-11-apic-phase2-design.md](2026-11-11-apic-phase2-design.md)、[2026-11-11-apic-phase2b-design.md](2026-11-11-apic-phase2b-design.md)）。

## 范围与非目标

范围（只改 apic 仓库）：

1. 环境默认认证 `envs.<env>.auth`，以及所有显示“当前认证”的地方。
2. `--dry-run` 下所选认证的未设变量不报错，保留占位并警告。
3. 401 重发后仍 401 时作废该认证的 token 缓存。
4. `--pick` 最后一段支持投影。
5. 简单定义参数支持 `enum`。

非目标：

- `--columns`：投影已能选列和定列序，不再加选项（deferred；若以后需要“取全部字段但只显示几列”再议）。
- 清除 `use` 保存的认证选择的命令（`use --reset`）：deferred，见待确认事项 Q1。
- 变量默认值语法、其他取值来源、分页等二期剩余项。
- 不改索引格式版本：缓存的索引结构不变，`enum` 字段已存在。

## 已确认事实与规范

- 认证选择：`internal/wiring/wiring.go` `Resolve`，优先级 `--auth` > `state.yaml` > `default_auth`；显示当前认证的还有 `commands/use.go`（两处 `firstOf(saved.Auth, site.DefaultAuth)`）、`commands/auth.go` `listAuths`、`skillgen` 的 `(default)` 标注。
- `default_auth` 写错名字时，加载不报错，到 `Resolve` 才报 `site has no auth`；`config check` 也不查。
- 认证解析：`config.ResolveAuth` 只解析选中的那一个认证，并把凭证字段的值登记为密钥用于脱敏。
- dry-run 下 exec/login 不执行命令、不发登录请求（`applyExec`/`applyLogin` 的 `dry` 分支），有缓存时用缓存。curl 输出中 `Authorization`/`Cookie` 头整体显示为 `***`，其他头按已登记密钥脱敏。
- 401 处理：`request.Execute` 只重试一次；`Provider.Refresh` 先清内存与缓存再重取，重取失败时缓存已清；问题只在“重取成功但重发仍 401”。
- `model.Pick` 由 `--pick` 与 exec 的 `token.path`/`expires.path` 共用。
- `output.Table` 对对象数组按键首次出现顺序取列；`model.Object` 保留键顺序。
- 简单定义用非严格解码（`yaml.Unmarshal`），旧版本会忽略未知的 `enum` 键。
- 规范：SR1403/SR1410（最小充分方案）、GR108（装配只装配）、GR109（不吞写入错误）、GR110（CLI 经真实分发测试）、SR2002（测试断言用 goutil）。

## 总体方案

五项彼此独立，各自落在已有的包里，不新增包、不改分层。

### 1. 环境默认认证

```yaml
sites:
  dh:
    envs:
      build:   {base_url: "http://127.0.0.1:18090/api/v1", auth: build}
      factory: {base_url: "http://127.0.0.1:18080/api/v1", auth: factory, protect: true}
    auths:
      build:   {type: bearer, token: "${env:DH_TOKEN_BUILD}"}
      factory: {type: bearer, token: "${env:DH_TOKEN_FACTORY}"}
    default_env: build
```

- 优先级：`--auth` > `state.yaml` 中保存的认证 > `envs.<所选环境>.auth` > `default_auth`。
  - state 排在环境默认之前：`apic use dh --auth x` 是用户的明确选择，应当保持。代价是保存过认证后，切换环境不会再换认证；`use` 的输出会显示认证来源，见下。
- 新增一个函数计算“有效认证及其来源”，`Resolve`、`use`、`auth ls`、`auth status`、`skillgen` 都用它，不再各自 `firstOf`。
  - `use` 显示：`auth: factory (env default)`、`auth: x (saved)`、`auth: y (site default)`。
  - `skillgen` 在环境列表里写出各环境的默认认证。
- 校验：
  - `envs.<env>.auth` 指向不存在的认证时，`Resolve` 报 `site "dh" env "factory": no auth "x"`，退出 1，与现有 `default_auth` 写错的行为一致。
  - `config check` 对 `envs.*.auth` 与 `default_auth` 写错名字都给出提示（只提示，不改退出码，与现有 `config check` 的其他提示一致）。

### 2. dry-run 容忍未设变量

- 只在 `--dry-run` 时生效，且只影响所选认证的解析。
- `ResolveAuth` 增加一个“容忍未设”选项：未设变量保留原文 `${env:NAME}`，不登记为密钥，返回未设变量名列表。
- `NewCall` 在 dry-run 时启用该选项，并向 stderr 每个变量写一行：`warning: dry-run: GITLAB_TOKEN is not set; a real call would fail`。退出码仍为 0。
- 不改变的部分：真实调用缺变量仍退出 1；dry-run 仍不执行 exec 命令、不发登录请求；已设变量照常登记为密钥并脱敏。
- 占位原文出现在 curl 输出中（例如 `-H 'PRIVATE-TOKEN: ${env:GITLAB_TOKEN}'`）：它不是密钥，而且正好告诉用户真实调用时这里会放什么。

### 3. 401 重发后作废缓存

- `request.Authorizer` 接口增加 `Invalidate() error`。
- `Execute` 在重发后的响应仍为 401 时调用 `Invalidate`，然后照常返回该响应（退出码 4 不变）。
- `Provider.Invalidate`：login/exec 清内存 token 并删除缓存文件（文件不存在不算错误）；静态认证什么也不做。删除失败时向 stderr 写一行警告，不改退出码（GR109：best-effort 且有记录）。
- 首次请求 401、`Refresh` 返回 `ErrNoRefresh`（静态认证）的路径不变。

### 4. `--pick` 投影

- 语法：路径最后一段含逗号时为投影段，`a,b,c` 按给出顺序取键组成新对象。前面可以有任意普通段和 `*`：
  - `--pick name,default_branch` → `{"name":..,"default_branch":..}`
  - `--pick data.list.*.id,name,host -o table` → 三列表格，列序即给出的顺序。
- 规则：
  - 投影段只能是最后一段；中间段含逗号仍按普通键处理（与现在相同）。
  - 每个键非空且不重复，否则退出 2（`--pick` 语法错误，与现有空段错误一致）。
  - 对象缺少的键取 `null`（与 `*` 下缺字段填 `null` 一致），不报错。
  - 兼容：对象本身恰有名为 `a,b` 的键时按普通键取值，不做投影。
  - 投影作用于数组（前面没有 `*`）或标量时，退出 7，提示 `projection needs an object; add .* before it for arrays`。
- exec 的 `token.path`/`expires.path` 也用 `Pick`：投影得到对象，不是字符串/数字，沿用现有“token 不是字符串”的错误，不额外处理。
- README 与 `apic skill` 生成的提示补一行投影写法。

### 5. 简单定义 `enum`

```yaml
operations:
  mrs:
    get: /projects/{id}/merge_requests
    params:
      - {name: state, in: query, enum: [opened, closed, merged, all]}
```

- `simpleParam` 增加 `Enum []string`，原样写入 `model.Param.Enum`。调用时的枚举校验、`desc`、补全都已支持 `Enum`，无需改动。
- 校验与 overlay 的 `enum` 相同：元素非空、不重复；违反时该站点加载失败，退出 1。
- `query: [...]` 简写不支持枚举；要枚举就写进 `params`。

## 架构

不新增包，依赖方向不变（model 无依赖；config/spec/request/output/auth/skillgen 只依赖 model；wiring 与 commands 在上层）。

| 项 | 改动位置 |
|---|---|
| 1 | `model.Env` 加 `Auth`；`wiring` 新增有效认证函数并在 `Resolve` 使用；`commands/use.go`、`commands/auth.go`、`skillgen` 改用它；`config/check.go` 增加名字校验 |
| 2 | `config/expand.go`（容忍选项）；`wiring.NewCall` 传入 dry-run 并写警告 |
| 3 | `request/execute.go`（接口与调用）；`auth/provider.go`、`auth/tokencache.go`（`Invalidate`） |
| 4 | `model/pick.go`；README；`skillgen` 提示文本 |
| 5 | `spec/simple.go` |

skillgen 只拿到 `model.Site`，环境默认认证从 `site.Envs[*].Auth` 直接读取，不依赖 wiring。

## 关键流程

### 选择认证

```text
--auth 给出? ──是──> 用它
      │否
state 保存了? ──是──> 用它（来源 saved）
      │否
envs[所选环境].auth? ──是──> 用它（来源 env default）
      │否
default_auth? ──是──> 用它（来源 site default）
      │否
不认证
```

### 401

```text
发送 → 401 → Refresh（清缓存，重取）→ 重发 → 仍 401 → Invalidate（清缓存）→ 返回 401（退出 4）
                  └─ 重取失败 → 返回错误（缓存已在 Refresh 中清除）
```

## 安全、数据、运维与回滚

- **密钥**：dry-run 保留的 `${env:NAME}` 原文不是密钥；已设变量的登记和脱敏不变。真实调用的失败行为不变。
- **缓存**：第 3 项只会多删缓存，不会多写；最坏情况是下次多执行一次 exec 或登录。
- **配置兼容**：`envs.<env>.auth` 与简单定义的 `enum` 都是新增字段。
  - 旧版本忽略 `envs.<env>.auth`（`config.yaml` 与 `sites/*.yaml` 均为非严格解码）：回退后这些环境改用 `default_auth`，若站点没有 `default_auth` 就不认证，请求会 401。回退前应先补回 `default_auth` 或在调用时加 `--auth`。
  - 旧版本忽略简单定义的 `enum`：只是少了校验。
- **行为变化**：
  - 第 2 项：以前 dry-run 缺变量退出 1，现在退出 0 并警告。依赖该退出码做“变量是否齐全”检查的脚本需改用真实调用或 `config check`。
  - 第 4 项：以前含逗号的最后一段若对象没有该字面键会报错退出 7，现在按投影返回。
- **回滚**：不改缓存格式，退回 v0.3.x 无需清缓存；只需按上面撤掉或补齐配置。

## 决策

| ID | 决策 | 理由 |
|---|---|---|
| D1 | 用 `envs.<env>.auth` 解决按环境用 token，不做 exec.env 的懒解析 | 声明式、无需脚本；所选认证全部解析是正确的失败语义，问题在缺少按环境选认证的途径 |
| D2 | 优先级 `--auth` > state > 环境默认 > `default_auth` | 保持用户明确保存的选择；`use` 显示来源以减少困惑 |
| D3 | dry-run 只对认证解析容忍未设变量，保留原文并警告 | 让用户先看请求；真实调用的失败语义不变 |
| D4 | 重发仍 401 才作废缓存 | 最小改动，正好覆盖观察到的问题；首次 401 已由 Refresh 处理 |
| D5 | 投影只在最后一段，缺键为 `null` | 一个规则同时解决多字段和选列；与 `*` 的缺字段语义一致 |
| D6 | 不做 `--columns` | 投影已覆盖；SR1403 不加第二个机制 |
| D7 | 简单定义的 `enum` 只在 `params` 中支持 | `query` 简写保持字符串列表，避免两种写法混用 |

## 待确认事项

- Q1（非阻塞）：保存过认证后切换环境不会换认证（D2）。若实际使用中常踩到，可补 `use <site> --auth -`（清除保存的认证）；本批先不做，列为 deferred。
- Q2（非阻塞）：`config check` 对认证名写错只提示、不改退出码，与现有行为一致；若希望作为结构错误退出 1，需另行决定（会改变 `config check` 的退出码约定）。

## 结论与人工计划 Gate

本设计确定五项修正的行为与落点，第 4 项由投影覆盖。评审后等待用户批准设计；批准只授权编写实施计划，不授权实施。
