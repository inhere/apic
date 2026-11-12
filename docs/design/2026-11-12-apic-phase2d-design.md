<!-- template_id: design; template_version: 1.2.0 -->
# apic 二期第四批：跨层取值、分页、清除选择、拉取定义跟随 --env、${exec:} 设计

> 状态：Draft 0.4 / 已批准（用户 2026-11-12：“批准 apic 二期第四批设计 0.4 与计划 0.3（接受未复核修订）；继续实施”。0.3 已于 2026-11-12 批准：“按 data.{a,b},c 改 pick；批准 apic 二期第四批设计 0.3（接受未复核修订）”。0.4 只追加 §6 与 D12，§1–§5 不变；0.2–0.4 为未复核修订；批准只授权编写实施计划，不授权实施）

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-12 | Claude（inhere：下一批做 1–4 + `${exec:}`，分页参数名在站点配置里定义） | 初稿：五项的根因与方案 |
| 0.2 | 2026-11-12 | Claude | 处理第 1 轮两轴评审（阻塞 C1/S1、S2/C11）：<br>• 预扫描同时看站点前后的 `--env`；<br>• 短页判断以第一页的实际条数为准，相邻页重复报错；<br>• 达到默认上限时退出 7；<br>• `${exec:}` 与 `${env:}` 一次扫描、结果不再展开，`${exec:` 内含 `${` 报错；<br>• 超时固定 30s，不设 `APIC_*`；<br>• 如实列出会执行命令的入口；失败消息不含命令名；dry-run 用占位 `<exec>`；<br>• 根引用规则补全，含根引用的投影在解析阶段检查；<br>• 分页的输入规则、3xx 处理、选项注册位置；<br>• `--env` 只校验显式给出的值；<br>• `use --clear` 可清已删除站点的残留；<br>• 回滚说明补“命令文本会被当作凭证发出” |
| 0.3 | 2026-11-12 | Claude（inhere：投影的 `id` 跟随上一级容易写错，按 `data.{a,b},c` 改 pick，无需兼容） | §1 改为“逗号分项从根取 + 末段 `{...}` 分组”，取消路径对象、根引用和 v0.4.0 投影；分页示例同步 |
| 0.4 | 2026-11-12 | Claude（inhere：skill 生成加 apic 自身的使用说明，含简易定义格式与示例，方便其他机器的 agent 使用 apic） | 追加 §6“apic 自身的 skill”与 D12；§1–§5 不变 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 设计工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：解决 v0.4.0 试用后剩下的五个障碍：
  1. 一次取不到不同层的字段，现有投影写法容易出错；
  2. 列表要手动翻页；
  3. 保存的选择清不掉；
  4. 拉取定义不跟随 `--env`；
  5. token 只能明文放在 `.env`。
- Scope freeze：只做 issue my-tools-ab5f 中用户确认的四项，加上 `${exec:}`。
  - 不做（deferred）：
    - `--columns` 与 overlay 默认列（由第 1 项覆盖）；
    - 读响应头（`Link`/`X-Next-Page`）翻页、响应体游标式翻页；
    - 按操作覆盖分页配置；
    - `${keyring:}`、`${env:X:-默认值}`；
    - oauth2、重定向跟随。
- 暴露度：v0.4.0 已公开发布；第 5 项会从配置执行外部命令取密钥。按高暴露处理（SR1409），走两轴独立评审。
- Review budget：一轮评审，两个方向：正确性与可实施性；安全与对外契约。
- 停止条件：评审无开放 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。
- 评审状态：第 1 轮已完成，两个方向均 FAIL。
  - 正确性方向：阻塞 C1，另有 C2–C14 必须修正。
  - 安全方向：阻塞 S1、S2，另有 S3–S10 必须修正。
  - 0.2 已逐条修订；0.3 按用户要求改写 §1。预算只有一轮，0.2、0.3 属于未复核修订，见“评审处理”。

## 背景与目标

| # | 现象 | 根因（代码） |
|---|---|---|
| 1 | `--pick` 取不到 `code` 加 `data.username` 这种不在同一层的字段；投影 `data.username,id` 中 `id` 跟随上一级，容易写错 | `model.Pick` 的投影只作用于最后一段，键都相对上一级（`model/pick.go` `projectPick`） |
| 2 | GitLab 默认每页 20 条，DeployHub 的 apps/hosts/tasks 都要翻页，只能一页一页手动调 | 每次调用只发一个请求（`commands/siterun.go` `run` → `request.Execute`） |
| 3 | `apic use` 保存的环境或认证无法清除，只能手改 `state.yaml` | `commands/use.go` 只会写入，不会删除 |
| 4 | 调用时给了 `--env`，拉取定义仍按保存的或默认的环境建 HTTP 客户端（TLS、`insecure`） | `wiring.FetchOptions` 与 `commands/site.go` `loadIndex` 只看 state 和 `default_env` |
| 5 | token 只能写进 `.env` 或系统环境变量（明文），或写一整个 exec 认证 | 值引用只有 `${env:}`（`config/expand.go`） |

用户可见的结果：

- `--pick 'data.{username,id},code'` 一次取不同层的字段，每项都从根开始，不再有“跟随上一级”的隐含规则。
- 站点配置 `paginate` 写上页码、条数参数名和列表路径后，`--all-pages` 自动翻页，合并成一个数组输出。
- `apic use <site> --clear` 清除该站点保存的选择。
- `apic <site> <op> --env X`（包括带环境的快捷命令）拉取定义时也用环境 X 的设置。
- 凭证字段可写 `${exec:pass show gitlab}`，从密码管理器或系统凭据取值，不以明文落盘。

## 名词

| 名词 | 含义 |
|---|---|
| 分项 | `--pick` 中不在花括号内的逗号分隔的各项，每项从根开始 |
| 分组 | 路径最后一段的 `{...}`，组内各项相对分组前缀 |
| 分页配置 | 站点的 `paginate`：页码参数名、条数参数名、列表路径等 |
| 参考页长 | 第一页实际返回的条数，用于判断后续页是否“不足” |
| 值引用 | 配置值中的 `${env:NAME}` 和 `${exec:命令 参数...}` |
| 凭证值 | 允许出现 `${exec:}` 的字段：`token`、`password`、`value`、`exec.env` 的值、`login.body` 中的字符串 |

其余名词沿用一期设计（[apic-design.md](apic-design.md)）与二期各批设计（[第一批](2026-11-11-apic-phase2-design.md)、[第二批](2026-11-11-apic-phase2b-design.md)、[第三批](2026-11-12-apic-phase2c-design.md)）。

## 范围与非目标

范围（只改 apic 仓库）：

1. `--pick` 多字段语法：逗号分项从根取，`{...}` 分组（取代 v0.4.0 的投影）。
2. 站点级分页配置，以及 `--all-pages`、`--max-pages` 两个选项。
3. `apic use <site> --clear`。
4. 拉取定义使用本次调用的 `--env`。
5. 凭证值中的值引用 `${exec:}`。
6. `apic skill` 不带站点时生成 apic 自身的使用说明 skill（0.4 追加）。

非目标：

- 分页：
  - 只支持“页码 + 条数”式的 query 参数，不读响应头，不支持游标或 next 链接；
  - 配置只到站点级，不能按操作覆盖；
  - 不做并发翻页，不做流式输出（全部页拿齐后一次输出），不设合并结果的总量上限。
- `${exec:}`：
  - 不跨进程缓存，不经过 shell，不支持引号和转义（复杂命令请写成脚本）；
  - 不设 `APIC_*` 环境变量；
  - 不把 `auth status/logout` 改成惰性解析（见 Q2）。
- `--pick` 仍不实现 jq 语法，不支持过滤和函数。

## 已确认事实与规范

- **`--pick` 现状**
  - 在请求发出前由 `model.ParsePick` 解析，失败退出 2；求值失败退出 7（`output.postProcess`）。
  - 在 `*` 之下，元素上的求值错误会被吞成 `null`（评审探针：`data.list.*.id,id` 输出 `[null,null]`）。
  - exec 认证的 `token.path`/`expires.path` 也用 `model.Pick`。
  - 投影规则（第三批 D5）：最后一段含逗号；字面键优先；缺键为 `null`；结果是 `model.Object`，键序即书写顺序。
- **表格与选项**
  - `output.Table` 对对象数组按键首次出现的顺序取列。
  - 全局选项要在三处登记：
    - `commands/globalopts.go` 中的选项绑定；
    - 预扫描用的 `globalSpec`（是否带值）；
    - `reservedOptNames`（生成的参数选项会避开这些名字，同名参数只能用 `-p name=v` 传入）。
  - 顶层补全还有一份选项列表。
- **请求执行**
  - `request.Execute` 负责一次请求：写保护、认证、发送、401 重试一次、仍 401 时作废 token。
  - `--timeout` 的 ctx 在 `Execute` 内创建，作用于一次 `Execute`。
  - 客户端不跟随重定向；单次调用时 3xx 的退出码为 0（`output.ExitCode`）。
  - `insecure` 告警每次 `send` 都打印。
- **state**：每个站点保存一份 `{env, auth}`，另有 `links`（`config/state.go`）。`use` 先用 `siteLookup` 校验站点存在。
- **run 路径与预扫描**
  - 先 `Prescan`（只扫描站点名之前的全局选项），再 `loadSiteIndex` 生成动态命令，最后由 gcli 解析选项（`commands/run.go`、`prescan.go`）。
  - 快捷命令生成 `apic --api S --env E "$@"`（`link/script.go`）；预扫描改写后，`--env E` 在站点名之前。
  - gcli 在 app 级与站点命令上都绑定了 `g.Env`，后解析的值覆盖先解析的。
- **拉取定义**
  - `loadIndex`/`FetchOptions` 按 state、`default_env` 选环境：用它判断 `insecure` 并建客户端。
  - 环境不存在时静默用零值环境。
  - `source.auth` 只允许 bearer/basic/apikey，没有 token 缓存，以严格模式解析。
  - 调用方：站点调用（含 `--dry-run`、`--help`、列操作）、`ops`、`desc`、`skill`、`overlay init`、`site add/refresh`。补全只读缓存，不建认证。
- **exec 认证的执行器**（`auth/exec.go` `runExec`）
  - 命令解析：裸名经 PATH 且拒绝当前目录；相对路径按配置目录；Windows 先补全 PATHEXT，再检查批处理参数。
  - 运行：stdin 为空设备；输出上限 4 MiB；`WaitDelay` 已设；默认超时 30s。
  - 失败消息不含 argv（包括 `command[0]`）。
- **值引用的展开**
  - `config.Expand` 只认 `${env:}`，正则 `[^}]+` 单次替换。
  - `config.ResolveAuth(site, name, env, sec)` 没有 ctx，在 `wiring.NewCall` 中、`Execute` 之前调用；只解析所选的一个认证，凭证字段的值登记为密钥。
  - login 与 exec 的缓存指纹包含展开后的值：login 为 base_url 加 Login（含 body），exec 包含 exec.env（`auth/tokencache.go`、`auth/exec.go`），读缓存在解析之后。
- **脱敏与检查**
  - curl 输出中只有 `Authorization`、`Cookie` 头整体显示 `***`，其他头和 URL 只遮已登记的密钥（`request/curl.go`）。
  - 少于 4 个字符的密钥不参与替换（`model/secrets.go`）。
  - `config check` 的 `isReference` 只把整串 `${...}` 当作引用。
- **分层**：config/auth/request/output/spec 只依赖 model；wiring 与 commands 在上层（`internal/layering_test.go`）。
- **规范**：GR107–GR110、SR2003。

## 总体方案

五项彼此独立，不新增包、不改分层。

### 1. `--pick` 多字段：逗号分项从根取，`{...}` 分组

v0.4.0 的投影（`data.username,id` 中的 `id` 跟随上一级）容易写错，本批取消，改为下面的统一语法。用户确认无需兼容（目前只有用户本人使用）。

```bash
apic dh me --pick data.username                        # "u"（单个路径，直接输出值，不变）
apic dh me --pick 'code,data.username'                 # {"code":0,"username":"u"}
apic dh me --pick 'data.{username,id},code'            # {"username":"u","id":7,"code":0}
apic dh me --pick 'uid=data.id,code'                   # {"uid":7,"code":0}
apic dh apps --pick 'data.list.*.{id,name}' -o table   # 对象数组，两列
apic dh apps --pick 'data.list.*.{id,name},data.total' # {"list":[{…},…],"total":42}
```

**语法**

- 顶层按不在花括号内的逗号分项。每项从根开始，写作 `路径` 或 `别名=路径`。
  - 别名在项首，用第一个 `=` 切分，只允许 `[A-Za-z0-9_-]+`。
- 路径按点分段，每段是键、下标或 `*`，与现在相同。
- 路径的最后一段可以是分组 `{项,项,...}`。组内各项是相对分组前缀的路径，语法与顶层项相同：可带别名，也可以再嵌套分组。
  - 分组只能作为最后一段，`data.{a,b}.x` 报错。

**结果**

- 只有一项且不是可展开的分组时，直接输出该值，与现在的单路径相同。例如 `data.list.*.{id,name}` 输出对象数组。
- 其他情况结果是对象，键序即书写顺序：
  - **键名**：取别名；没有别名时，取路径中最后一个“键名段”（不是 `*`、下标或分组的段）。例如 `data.username` → `username`，`data.list.*.{id,name}` → `list`，`0.name` → `name`。没有键名段时（如 `*`、`0`）必须写别名。
  - **可展开的分组**（路径以分组结尾，且路径中没有 `*`）：组内各键平铺进所在对象。例如 `data.{username,id},code` 得到三个键，而不是嵌套的 `data`。
  - **`*` 之后的分组**：对每个元素取一个对象，结果是对象数组。
- **缺失**：
  - 只有一项时，缺失仍按现状退出 7。
  - 多项或分组中，缺失的路径为 `null`；`*` 之下元素上的错误也为 `null`，与现在一致。

**校验**

- 全部在 `ParsePick` 中完成，请求前报错，退出 2：
  - 空项、空段，别名或路径为空，别名字符不合法；
  - 花括号不配对，分组不在最后一段；
  - 同一对象中键名重复（静态可知），需要无别名的键时；
  - `--pick` 以逗号开头或结尾。
- 不再有“字面键优先”和推迟到求值时的语法检查（第三批 D5 的兼容规则一并取消）。
- 键名本身含 `.`、`,`、`=`、`{`、`}` 时无法用 `--pick` 访问。

**其他**

- 结果对象不是对象数组时，`-o table` 按现有规则回退为 JSON 并提示。例如 `data.list.*.{id,name},data.total`；需要表格时只取列表。
- exec 认证的 `token.path`/`expires.path` 用同一解析器：多项或分组的结果是对象，会被现有的“token 必须是字符串”校验拒绝，不另加限制。
- 实现：`model.Pick` 改为“项列表 + 可递归分组”的结构，`Apply` 按项求值并组装 `model.Object`；`projectPick` 删除。

### 2. 分页

```yaml
sites:
  gitlab:
    paginate:
      page: page          # 页码参数名（必填）
      size: per_page      # 每页条数参数名（可选）
      size_value: 100     # 用户没给条数参数时自动带上的值（可选，需要 size）
      start: 1            # 首页页码，默认 1，允许 0
      items: ""           # 列表在响应中的路径；不写或空表示响应本身就是数组
  dh:
    paginate: {page: page, size: size, size_value: 100, items: data.list, total: data.total}
```

**配置校验**（加载时，错误退出 1）：

- `page` 必填。
- `size_value` 必须为正整数，且要求配了 `size`。
- `start` 必须为非负整数。模型中用指针区分“未写”和 `0`。
- `items`、`total` 若非空，必须是只含键和下标的普通 pick 路径（不允许多项、分组和 `*`）。

**选项**

- `--all-pages` 开启翻页；不加时行为完全不变。
- `--max-pages N` 设上限，同时隐含 `--all-pages`；`N` 必须是正整数，否则退出 2。默认上限 100。
- 两个选项都登记到选项绑定、`globalSpec`（`--max-pages` 带值）、`reservedOptNames` 和顶层补全列表。

**前提检查**：在解析认证之前进行，避免先执行 `${exec:}` 再报用法错误。以下情况退出 2：

- 站点没配 `paginate`；
- 方法不是 GET；
- 用户给的页码或条数参数不是整数。

**翻页流程**

- 起始页码：取用户给的页码参数，否则取 `start`。
- 条数：取用户给的条数参数，否则取 `size_value`，都没有就不带。
- 构建请求前，把起始页码和条数写进参数输入，这样操作把 `page` 声明为必填时也能通过参数校验。
- 页码与条数参数不要求操作声明过，直接写入 URL query；原始路径调用同样适用。
- `request` 新增 `Paginate`：每页改写 URL 中的页码（覆盖同名参数），调用一次 `Execute`。401 重试与作废逐页生效，`--timeout` 也按页生效。
- `insecure` 告警只在第一页打印。

**每页的处理**

1. 网络或认证错误：直接返回该错误。
2. 非 2xx（含 3xx）：停止，按普通响应原样输出该页，前面各页丢弃。
   - stderr 提示 `note: page N returned HTTP 302; M earlier pages discarded`。
   - 退出码按状态码映射；映射结果为 0 时（3xx）改为 7。
3. 2xx 但响应体为空或不是 JSON：退出 7，提示 `--all-pages failed: page N: response is not JSON`。
4. 按 `items` 取列表：取不到或不是数组时退出 7，提示 `--all-pages failed: page N: items "data.list" is not an array`。
5. 配了 `total` 时，从第一页读取总数：必须是非负整数，否则退出 7；后续页不再读取。
6. 相邻两页的列表内容完全相同（多于 0 条）时退出 7，提示 `page N repeats page N-1; check paginate.page`。这用来发现页码参数名配错、服务端忽略页码的情况。
7. 追加列表，然后判断是否结束：
   - 本页 0 条：结束。
   - 配了 `total`：已取条数不少于总数时结束。此时不做下面的短页判断。
   - 没配 `total`：参考页长是第一页的实际条数，后续某页少于它时结束。
     - 不用请求的条数作参考：服务端常把条数限制在上限内（GitLab `per_page` 最多 100），请求值偏大时，第一页就会被误判为最后一页。
     - 代价：只有一页数据时，会多发一次请求（得到 0 条）。
   - 达到上限、但还不能判断已经结束时：
     - 上限来自显式的 `--max-pages`：输出已取到的结果，stderr 警告 `warning: stopped after N pages (--max-pages); more may exist`，退出 0。
     - 上限是默认的 100 页：不输出，退出 7，提示 `--all-pages: reached the default limit of 100 pages; pass --max-pages to raise or accept it`。这样脚本不会把截断的列表当成完整结果。

**输出**

- 合并后的数组作为响应体，`--pick` 和 `-o` 作用在这个数组上，例如 `--all-pages --pick '*.{id,name}' -o table`。路径相对合并后的数组，不再带 `data.list` 前缀。
- 状态码与响应头取最后一页。`-i` 打印最后一页的头，其中 `Content-Length` 与合并后的 body 不一致。
- dry-run：只打印第一页的 curl，stderr 提示 `note: dry-run: --all-pages would request further pages`。
- `-v`：每页的请求行照常打印。

**资源**

- 全部页留在内存中。
- 总耗时最多为页数乘以 `--timeout`。
- 不可缓存的 `type: exec` 认证（未设 ttl、输出不带过期时间）每页都会运行一次命令。

### 3. 清除保存的选择

- `apic use <site> --clear` 删除 `state.yaml` 中该站点的选择（环境与认证都删），写回 state 后，按清除后的状态打印，与无参 `use` 一致。
- 与 `--env`/`--auth` 同时给出时，报用法错误，退出 2。
- 幂等：站点没有保存的选择时同样成功。
- 站点已从配置中删除、但 state 里还有残留时，`--clear` 照样删除残留并打印 `cleared`，不经 `siteLookup`。其他 `use` 用法仍要求站点存在。
- 不影响 `state.links`。快捷命令自带 `--env`/`--auth`，也不受影响。

### 4. 拉取定义使用本次 `--env`

- `loadIndex`、`loadSiteIndex` 与 `wiring.FetchOptions` 增加参数 `envFlag`：
  - 环境按 `envFlag` > 保存的 > `default_env` 选，`insecure` 告警也按这个环境判断；
  - 其余调用点（`ops`、`desc`、`skill`、`overlay init`、`site add/refresh`）传空串，行为不变。
- `Prescan` 收集站点名之前（全局选项部分，含 `--api` 改写后的位置）和之后（到 `--` 为止）的所有 `--env X`/`--env=X`，取最后一个，与 gcli 后者覆盖前者一致；实施时先用测试核实这一点。
  - 极端情况下（例如某个选项的值本身就是字面量 `--env`），预扫描结果可能与 gcli 的解析不同，只影响拉取定义时用哪个环境的 TLS 设置，不另做校验。
- 只校验 `envFlag`：显式给出的环境不存在时，`loadIndex` 报 `site %q has no environment %q`，退出 1，这时还没有发起拉取。
  - 保存的或默认的环境不存在时维持现状，静默用零值环境，避免 `ops`、`desc` 等从成功变成失败。
- `source.auth` 的选择不变（第三批非目标），本项只改变用哪个环境的 TLS 设置建客户端。

### 5. 值引用 `${exec:}`

```yaml
auths:
  pat:
    type: apikey
    in: header
    name: PRIVATE-TOKEN
    value: ${exec:pass show gitlab/token}
  ad:
    type: login
    login:
      body: {username: me, password: "${exec:powershell -NoProfile -File bin/secret.ps1 ad}"}
```

**语法与展开**

- 写法：`${exec:命令 参数...}`，内容按空白切分，不支持引号和转义。
  - 内容为空，或含 `${`、`}`，都会报错，退出 1。所以 `${exec:tool ${env:X}}` 这种嵌套不允许。
- 同一串可与文字、`${env:}` 混写，如 `"Bearer ${exec:get-token}"`。
- 一个正则一次扫描，同时识别 `${env:...}` 与 `${exec:...}`，替换结果不再展开：
  - 环境变量值里的 `${exec:...}` 不会被执行；
  - 命令输出里的 `${env:...}` 也不会被展开。
  - 测试覆盖这两种情况。

**允许的位置**

- 只能出现在凭证值中：`AuthSpec.Token`、`Password`、`Value`、`exec.env` 的值、`login.body` 中的字符串。
- 不能出现在其他字段：`exec.command`、`apply.value`、`login.apply.value`、`*.token.path`、`allow_methods`、`login.path` 等。
  - 解析认证时报错，退出 1：`auth "pat": ${exec:} is only allowed in credential values`。
  - `config check` 也会静态报出放错的位置。

**执行**

- 复用 exec 认证的执行器：命令解析、空 stdin、4 MiB 上限、`WaitDelay`、Windows 批处理检查都相同。
- 超时固定 30s，与 `--timeout` 无关。
- 工作目录为真实的配置目录：在 `wiring.Load` 时绑定，`site add` 把 `Deps.Dir` 换成临时目录也不影响。
- 环境为父进程环境，不加 `APIC_*`。

**输出**

- 取 stdout，去掉末尾的 `\r`、`\n`，不做 TrimSpace，因为密码首尾可能有空格。
- 结果为空、含多行或含其他控制字符时报错。只输出第一行的场景请包一层脚本。
- 结果登记为密钥。少于 4 个字符的结果不会被脱敏，写进 README。

**失败**

- 退出 1，消息为 `auth "pat" value: ${exec:} failed (exit 1): <stderr 最后一行>`。
- 与 exec 认证一致，消息不含命令名和参数，整条经脱敏。stderr 最后一行仍可能含敏感内容，与 exec 认证的现状相同。

**同一进程内去重**

- 同一引用文本只执行一次：没有 `APIC_*`，结果与站点无关，例如拉取定义和调用都用到同一认证。
- 失败不缓存。
- 不跨进程缓存。

**何时执行**：只解析所选认证的凭证值，没被选中的认证里的 `${exec:}` 不执行。会执行的入口有：

- 站点调用（真实调用；dry-run 不执行认证中的引用，见下）；
- `auth login`、`auth logout`、`auth status`（都会严格解析所选认证）；
- 需要拉取定义（缓存缺失或过期）且配了 `source.auth` 时，`ops`、`desc`、`skill`、`overlay init`、`site add/refresh` 以及站点调用（含 dry-run）都会以严格模式解析 `source.auth`。

不执行的入口：

- 补全（只读缓存）；
- `site show`、`auth ls`、`config check`、`link` 创建（都不解析认证）。

**与缓存的关系**

- login 认证的 `login.body`、exec 认证的 `exec.env` 中有 `${exec:}` 时，命令在每次调用时都会先运行：校验缓存的指纹需要展开后的值，即使 token 缓存有效也是如此。
- 命令输出每次不同时（OTP、轮换 token），指纹每次都变，缓存永远不命中，每次都重新登录。
- 想让命令结果随 token 一起缓存，应把命令写成 `type: exec` 认证本身的 `command` 并设 `ttl`，不要放进 `exec.env`。

**dry-run**

- 认证中的 `${exec:}` 不执行，替换为占位 `<exec>`，不登记、不告警。
- curl 中，Authorization 头显示 `***`；自定义头或 query 显示 `<exec>`，不暴露命令文本。
- 拉取定义不属于 dry-run 的容忍范围，见上面的入口列表。

**`config check`**

- 每处 `${exec:}` 输出 `site S auth A <字段>: runs ${exec:}`，不显示命令文本，与失败消息的策略一致。
- 含 `${exec:` 或 `${env:` 的混写值不再报“plaintext credential”，`isReference` 改为“值中含引用即可”。

**`site show`**：维持现有的整串遮蔽（`***`）。

**实现**

- `config.Env` 增加执行函数字段，由 `wiring.Load` 填入 auth 包导出的执行器（绑定真实的配置目录），config 不新增依赖。
- 字段为空时，引用报 `${exec:} is not available`。
- 执行器从 `runExec` 中抽出，exec 认证改为调用它，行为与失败消息不变。

### 6. apic 自身的 skill（0.4 追加）

**背景**：`apic skill <site>` 只描述某个站点的操作。另一台机器上的 agent 拿到它，仍不知道 apic 怎么装配置、怎么加站点、简易定义怎么写、全局选项和退出码是什么。

**命令**

- `apic skill` 不带站点名时，生成 apic 自身的 skill：`<out>/<name>/SKILL.md`，默认名 `apic`，默认目录与站点 skill 相同（`<config dir>/skills`）。
- 沿用 `--out`、`--name`、`--force` 以及标记行与原子写入规则（`skillgen.Write`）；`--max-ops` 不适用，给出时忽略。
- 站点参数改为可选；带站点名时行为不变。
- 不读取站点配置、`.env`、state 和定义缓存：配置目录不存在或配置有错时也能生成。

**内容**（英文，静态模板随二进制发布，生成时只填入版本号）

1. 用途与触发：什么时候用 apic（调用 HTTP API、查看操作、生成站点 skill）。
2. 配置目录与文件：`APIC_CONFIG_DIR`、`config.yaml`、`sites/<name>.yaml`、`.env`、`state.yaml`、overlay、缓存；只描述布局，不列本机内容。
3. 加站点：`site add` 的 OpenAPI URL、文件与简易定义三种来源。
4. **简易定义格式**：`operations` 下每个操作的字段（`get/post/put/patch/delete/head`、`summary`、`output`、`query`、`params` 的 `name/in/required/type/desc/enum`、`body.type`），路径参数 `{id}` 的写法，并附一个完整示例（占位站点 `pet`）。
5. 站点配置：`envs`（`base_url`、`protect`、`insecure`、`ca_file`、`auth`）、`auths` 的各类型（bearer、basic、apikey、login、exec）、`${env:}` 与 `${exec:}`、`paginate`。
6. 调用：`apic <site> <op>`、生成的参数选项、`-p`、`-d`/`-f`/`-F`、原始路径 `-X`、`ops`、`desc`、`use`（含 `--clear`）、快捷命令。
7. 输出：`-o`、`--pick` 分项与分组、`--all-pages`/`--max-pages`、`--dry-run`、`-v`、`-i`。
8. 退出码表。
9. 生成站点 skill：`apic skill <site>`。

**约束**

- 内容不含任何主机名、URL、凭证或本机站点名，生成结果可以直接复制到其他机器。
- 防漂移：测试断言模板覆盖 `globalSpec` 中的全部全局选项、简易定义的全部字段名（取自 `spec` 的结构体 tag）以及全部退出码；模板中的简易定义示例要能被 `spec` 加载。新增选项或字段而未更新模板时测试失败。
- 实现放在 `skillgen`（模板用 `embed`），`commands/skill.go` 只做分支；不新增包，依赖方向不变。

## 架构

不新增包，依赖方向不变。

| 项 | 改动位置 |
|---|---|
| 1 | `model/pick.go`（解析与求值）；README；`skillgen` 示例一条 |
| 2 | `model.Site.Paginate`（`model/config.go`）；`config/load.go` 校验；`request` 新增 `paginate.go`；`commands/globalopts.go`（选项、`globalSpec`、保留名）与顶层补全；`commands/siterun.go` 选择单页或翻页 |
| 3 | `commands/use.go` |
| 4 | `commands/prescan.go`（返回最后一个 `--env`）；`commands/site.go` `loadIndex`/`loadSiteIndex`；`wiring.FetchOptions`；`commands/run.go` 调用点 |
| 5 | `config/expand.go`（一次扫描两种引用、字段白名单、dry-run 占位、去重）；`config/dotenv.go` `Env` 加执行函数；`config/check.go`；`auth/exec.go` 抽出执行器；`wiring.Load` 注入 |
| 6 | `skillgen` 新增 `self.go` 与嵌入模板 `self.md`；`commands/skill.go` 站点参数可选 |

## 关键流程

### 分页

```text
siterun: --all-pages / --max-pages? ──否──> Execute 一次（不变）
              │是
前提检查（paginate、GET、整数）──失败──> 退出 2（认证尚未解析）
              │
page = 用户值 或 start；size = 用户值 或 size_value（写入参数输入）
loop:
  写入 page → Execute ──错误──> 返回
       │非 2xx ──> 原样输出该页 + note；退出码映射为 0 时改 7
       │2xx
  非 JSON / items 非数组 / total 非整数 / 与上页相同 ──> 退出 7
  追加
  0 条 ──> 结束
  配了 total：已取 >= total ──> 结束
  未配 total：本页 < 第一页条数 ──> 结束
  达到上限：显式 --max-pages ──> 警告，输出；默认上限 ──> 退出 7
  page++
合并数组 → output.Render（--pick、-o）
```

### 值引用展开

```text
ResolveAuth(所选认证)
  一次扫描：${env:X} → 查变量；${exec:...} → （凭证值？否则退出 1）→ 去重执行 → 登记密钥
  替换结果不再扫描
dry-run（ResolveAuthDryRun）：${exec:...} → <exec>，不执行
```

## 安全、数据、运维与回滚

- **执行命令的信任边界**：`${exec:}` 与 exec 认证一样，执行的是用户配置目录里写明的命令，不扩大信任面。
  - 不经过 shell，参数不做展开；一次扫描，不做二次展开，避免由环境变量注入命令。
  - 会执行的入口如上文所列，`config check` 会标出每一处。
  - 补全、`site show`、`auth ls` 不执行。
- **密钥**
  - 命令输出登记为密钥，失败消息不含命令文本。
  - 结果不以明文落盘。login/exec 的缓存指纹含其摘要，这与 `${env:}` 相同。
- **分页**
  - 只允许 GET。每页都经过写保护和认证。
  - 默认上限 100 页，达到时退出 7，避免无提示截断。
  - 相邻页重复时报错，避免页码配错时得到重复数据。
- **行为变化**（写进 release notes）：
  - `--pick`（不兼容，用户确认无需兼容）：逗号分隔的各项改为都从根取。v0.4.0 的 `data.items.*.id,name` 要改写为 `data.items.*.{id,name}`；带逗号的字面键优先规则取消；多字段写法的语法错误改为请求前退出 2。
  - 选项：`all-pages`、`max-pages` 成为保留选项名，同名的操作参数改为只能用 `-p` 传入。
  - `--env`：
    - 给出 `--env` 时（包括快捷命令），拉取定义改用该环境的 TLS 设置。
    - `--env` 写错时，列操作和 `--help` 也会退出 1，并且在拉取之前就报错。
  - `${exec:}`：
    - 凭证值中的 `${exec:...}` 以前被当作字面值发出，现在会执行命令。
    - 放在非凭证字段的 `${exec:...}` 以前是字面值，现在报错。
  - `config check`：混写引用的值不再报 plaintext，并新增 `runs ${exec:}` 提示。
- **回滚到 v0.4.x**
  - 旧版本忽略 `paginate`。
  - 旧版本会把 `${exec:...}` 的命令文本当作凭证发往服务端：apikey 放在 query 时还会进入访问日志和代理日志，login 会把它当密码提交。v0.4 的 `config check` 也不告警。**回退前必须改回 `${env:}`**。
  - state、缓存格式不变。

## 决策

| ID | 决策 | 理由 |
|---|---|---|
| D1 | `--pick` 多字段统一为“逗号分项从根取 + 末段 `{...}` 分组”，取消 v0.4.0 投影，不兼容 | 用户 0.3 确认：投影的“跟随上一级”容易写错；目前只有用户本人使用 |
| D2 | 键名默认取最后一个键名段，可展开的分组平铺；多项中缺失为 `null`；语法和重复键在请求前退出 2 | 表格友好；`*` 下错误会被吞，只能在解析阶段检查 |
| D3 | 分页参数名、列表路径由站点配置定义，只支持页码 + 条数 | 用户确认；覆盖 GitLab 与 DeployHub |
| D4 | 翻页显式开启；显式上限截断时警告并输出，默认上限截断时退出 7 | 不改变现有调用；脚本不会拿到无提示的截断结果 |
| D5 | 某页失败即整体失败，输出该页，不输出部分结果（显式 `--max-pages` 例外） | 脚本拿到的要么是完整列表，要么是明确的错误 |
| D6 | 短页以第一页实际条数为参考；配了 total 时以 total 为准；相邻页重复报错 | 服务端会限制条数（S2/C11）；页码配错时不静默重复 |
| D7 | `use --clear` 同时清环境与认证，可清已删除站点的残留 | 最小命令面 |
| D8 | 拉取定义跟随本次 `--env`（预扫描站点前后），只校验显式的 `--env` | 快捷命令也生效（C1/S1）；不让 stale state 引起新错误 |
| D9 | `${exec:}` 只在凭证值、只解析所选认证、一次扫描、不跨进程缓存、超时固定 30s、不设 `APIC_*` | 防注入；解析时没有调用上下文，最小实现 |
| D10 | `${exec:}` 复用 exec 认证的执行器，经注入函数接入 config；失败消息不含命令文本 | 不重复实现进程管理；与 exec 认证的披露策略一致 |
| D11 | 选项名用 `--all-pages`/`--max-pages` | `all` 更可能与操作参数撞名 |
| D12 | `apic skill` 不带站点名时生成 apic 自身的 skill：静态英文模板，不读配置，用测试防漂移 | 用户 0.4 要求；可移植到其他机器；模板与代码同仓发布 |

## 评审处理

第 1 轮两个方向（正确性与可实施性 C；安全与对外契约 S），全部在 0.2 处理：

| ID | 级别 | 处理 |
|---|---|---|
| C1、S1 | 阻塞 | 预扫描收集站点前后的 `--env`，取最后一个（§4、D8） |
| S2、C11 | 阻塞 | 短页以第一页实际条数为参考，配了 total 时以 total 为准（§2、D6） |
| C2、S5 | 纠正 | 超时固定 30s、不设 `APIC_*`；去重键为引用文本（同一进程内结果与站点无关），失败不缓存 |
| C3、S6 | 纠正 | 一次扫描两种引用，结果不再展开；`${exec:` 内含 `${` 报错；补两条注入测试 |
| C4、S3 | 纠正 | 如实列出会执行命令的入口，含 dry-run 时的拉取定义与 `auth status/logout` |
| C5、S4 | 纠正 | 写明 login.body/exec.env 中的引用每次调用都运行、指纹含摘要、“不以明文落盘”；status/logout 惰性解析列为 Q2 |
| C6、C7、S7 | 纠正 | 失败消息不含命令名；dry-run 用占位 `<exec>`；`config check` 不显示命令文本 |
| C8、C9 | 纠正 | 0.2 补全根引用规则；0.3 按用户要求整体改为分项 + 分组语法，根引用取消，校验全部在解析阶段（退出 2） |
| C10、S9 | 纠正 | 只校验显式的 `--env`；写进行为变化 |
| C12 | 纠正 | 非 2xx（含 3xx）页停止，映射为 0 时退出 7 |
| C13 | 纠正 | 选项登记到绑定、`globalSpec`、保留名与补全 |
| C14、S8 | 纠正 | 输入规则（整数、`--max-pages`、total、非 JSON、`items` 空串、`start` 指针）；默认上限截断退出 7 |
| S10 | 纠正 | 回滚节写明命令文本会作为凭证发出 |
| S11、S12 | 建议 | 白名单写精确；`config check` 静态报放错位置；`isReference` 支持混写 |
| S13、C20 | 建议 | 只去 `\r\n` 的理由；空内容、多行报错；短于 4 字符不脱敏写进 README |
| S14、C16 | 建议 | 写明按页超时、内存、不可缓存 exec 每页运行；`insecure` 只在第一页告警；`-i` 的 Content-Length |
| S15 | 建议 | 相邻页重复报错 |
| S16 | 建议 | `source.auth` 无 token 缓存，§4 改为只影响 TLS |
| S17、C21 | 建议 | `--clear` 不影响 links，可清已删除站点的残留 |
| C15、C17 | 建议 | 前提检查在解析认证之前；起始页码与条数写入参数输入，通过必填校验 |
| C18、C19 | 建议 | 去重键说明；执行器绑定真实配置目录 |
| C22 | 建议 | 架构表补 `loadSiteIndex` |
| C23 | 建议 | 键名含 `=` 等字符无法访问，写进 §1 校验 |

## 待确认事项

- Q1（非阻塞）：分页的默认上限 100 页是否合适；如有需要，以后加 `paginate.max_pages` 配置。
- Q2（非阻塞）：`auth status`/`auth logout` 会执行所选认证中的 `${exec:}`（可能弹出密码管理器的解锁提示）。本批维持现状；如希望这两个命令不执行，需另改为惰性解析。

## 结论与人工计划 Gate

本设计确定五项的行为与落点。第 1 轮两轴评审的阻塞项（C1/S1、S2/C11）及其余必须修正项已在 0.2 修订，0.3 按用户要求改写 §1，均为未复核修订。用户已于 2026-11-12 批准 0.3（接受未复核修订）；批准只授权编写实施计划，不授权实施。
