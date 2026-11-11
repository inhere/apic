<!-- template_id: design; template_version: 1.2.0 -->
# apic 二期：补全、输出筛选、skill 生成与 exec 认证 设计

> 状态：Draft 0.1 / 待人工计划批准

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-11 | Claude（inhere：“二期按推荐的排”） | 初稿：二期第一批四项——shell 补全、`--pick` 与 `-o yaml`/`-o table`、`apic skill`、exec 认证 |

> 仅语义变化递增版本；纯 identity/provenance/元数据纠正沿用原版本，并在 Git/进度记录中留痕。

## 设计工作声明

- 思考模式：`RIGOROUS`（SR1408）。
- 核心目标：让人和 agent 用 apic 时少记、少写、少翻文档。
  - 敲命令时能补全站点、操作、参数和枚举值。
  - 只取响应里需要的字段，或换成更易读的格式。
  - 一条命令生成给 agent 用的 skill。
  - token 由外部命令提供（vault、公司 SSO 工具等）的服务也能接入。
- Scope freeze：只做下面四项。二期其余内容是 oauth2、`${keyring:}`/`${exec:}`、overlay、multipart、分页、同主机重定向跟随，一期遗留的是 `--auth-for-spec` 和 `site.go` 打码整理，这些都进入二期第二批，本设计不展开。
- 暴露度：二期第一批走完整两轴独立评审，不按低暴露处理（SR1409）。
  - 外部用户可能存在：v0.1.1 于 2026-11-11 公开发布，提供 `go install` 和 release 二进制。虽然发布当天还没有已知的外部用户，但也无法证明没有。
  - exec 认证会在本机执行命令、经手凭证，属于安全边界。
- Review budget：一轮评审，两个独立评审方向，一是正确性与可实施性，二是安全与对外契约。
- 停止条件：评审没有开放的 `CORE_BLOCKING`，或由用户裁决；之后进入人工计划 Gate。

## 背景与目标

一期（v0.1.1）已经能注册站点、按操作调用、切换 env/auth、生成快捷命令。实际使用中有四处不便：

1. **补全**：只能补出管理命令和操作名，站点名、操作参数、枚举值、`--env`/`--auth` 的取值都补不出来（实测见“已确认事实”）。
2. **输出**：响应通常是一层层包起来的 JSON，只想要 `data.items[].name` 时得再接 jq。agent 环境里不一定有 jq，人看大 JSON 也费劲。
3. **给 agent 用**：要让 agent 知道某个站点能做什么，现在得手写说明，或者让它先跑 `apic ops`、`apic desc` 自己摸索。
4. **认证**：一些服务的 token 来自外部命令（`vault read`、公司 SSO CLI、`gcloud auth print-access-token` 一类），一期的 login 只能发 HTTP 登录请求，接不上这类服务。

用户可见的结果：

- `apic <TAB>` 能补出站点名；`apic pet list-users --<TAB>` 能补出参数名；`--status <TAB>` 能补出枚举值；快捷命令 `petctl <TAB>` 同样可以补全。
- `apic pet list-users --pick data.items.*.name` 只输出名字列表；`-o table` 按表格显示，`-o yaml` 按 YAML 显示。
- `apic skill pet` 生成 `SKILL.md`，可以交给 skillc 装给各个 agent。
- `type: exec` 的认证调用外部命令拿 token，token 会缓存，收到 401 时重新执行命令取新 token。

## 名词

| 名词 | 含义 |
|---|---|
| 运行时补全 | shell 补全脚本不内置候选，每次按 TAB 都调用 `apic --in-completion <已输入的词>`，由 apic 输出候选（gcli 自带机制） |
| pick 路径 | `--pick` 的取值语法，沿用 goutil `maputil` 的点号路径：`a.b`、`items.0.name`、`items.*.name` |
| skill | 一个目录，含 `SKILL.md`（frontmatter 的 `name`/`description` 加正文），供 Claude Code、codex 等 agent 按需加载 |
| exec 认证 | `type: exec`：执行一条外部命令，从它的 stdout 取 token |

其余名词沿用一期设计（[apic-design.md](apic-design.md)）。

## 范围与非目标

**范围**（仅 `inhere-tools/apic` 一个项目）：

1. shell 补全（bash、zsh、pwsh）：
   - 顶层补全站点名；
   - 操作的参数选项；
   - 参数的枚举值；
   - `--env`、`--auth`、`-o` 的取值；
   - 快捷命令的补全。
2. 输出：`--pick <路径>`；`-o` 增加 `yaml`、`table`。
3. `apic skill <site>`：生成 skill 目录。
4. 认证类型 `exec`。

**非目标**：

- 不实现 jq 语法，不支持过滤表达式和函数，`--pick` 只按路径取值。
- 补全时不发起网络请求：不刷新 spec，不登录，不执行 exec 命令。
- `apic skill` 不调用 skillc，也不直接写各 agent 的 skills 目录。
- exec 认证不经过 shell，不支持管道和重定向写法；需要时由用户自己写成脚本再调用。
- 二期第二批的内容（见“设计工作声明”）。

## 已确认事实与规范

1. **gcli 运行时补全**（gcli v3.8.6，`gookit2/gcli/completion.go`）：
   - `--gen-completion bash|zsh|pwsh` 生成的是瘦脚本，按 TAB 时调用 `<bin> --in-completion <words>` 计算候选。
   - 候选来自当前已注册的命令、子命令和选项。
   - 选项值的候选来自 `CliOpt.Choices`；没有 Choices 的选项返回空，交给 shell 做文件名补全。
2. **apic 现状实测**（v0.1.1，验收配置 `tmp/apic-accept/cfg`，站点 `jk`）：

   | 输入 | 实际候选 | 期望 |
   |---|---|---|
   | `apic <TAB>` | `auth config desc help link ops site use` | 另含站点 `jk` |
   | `apic jk <TAB>` | `job-build job-info` | 符合 |
   | `apic jk job-build --<TAB>` | 空 | 参数选项 |
   | `apic jk job-build --env <TAB>` | 空 | `default` |

   - **站点名补不出来**：预扫描只在命中站点名时才注册该站点。
   - **参数选项补不出来**：参数选项在操作子命令的 `Config` 回调里注册，补全时这些选项还没有注册（推断，计划阶段核实）。
3. **预扫描**（`internal/commands/prescan.go`）已经把 `--in-completion`、`--gen-completion` 列为 gcli 内置选项。因此 `--in-completion pet ...` 中的 `pet` 会被当作第一个位置参数识别成站点。
4. **路径取值**：login 取 token 已经在用 `maputil.GetFromAny`（`internal/auth/login.go`）。它支持点号、数组下标和 `*` 通配（`goutil/maputil/get.go`）。
5. **YAML 依赖**：模块里已有 `github.com/goccy/go-yaml`；表格可以用标准库 `text/tabwriter`。因此本期不新增依赖（SR1403）。
6. **skillc** 支持本地目录作为 skill 来源：`skillc source add local <path>`，然后 `skillc install`（`inhere-tools/skillc/README.md`）。
7. **一期契约不变**：
   - 退出码契约、stdout 只输出响应体、全局选项“只增不改名”。
   - 参数选项与全局选项重名时退回 `-p`（一期设计“参数映射”）。
8. **适用规范**：
   - 结构：GR107–GR110、SR2003、SR2004。
   - 测试断言：SR2002。

## 总体方案

四项彼此独立，各自落在已有的层次上，不改统一操作模型：

| 项 | 落点 | 对外新增 |
|---|---|---|
| 补全 | `commands`：预扫描、动态命令构建 | 无新命令；`apic link completion <shell>` |
| 输出 | `output`：pick、yaml、table | 全局选项 `--pick`；`-o yaml\|table` |
| skill | 新领域包 `skillgen` + `commands/skill.go` | `apic skill <site>` |
| exec 认证 | `auth`：exec 提供者；`model`：`AuthSpec.Exec` | 配置 `type: exec` |

### 1. 补全

沿用 gcli 的运行时补全，只补 apic 自己缺的部分：

- **站点名**：预扫描识别出补全模式（argv 里有 `--in-completion`）、且还没有命中站点时，把所有已配置站点注册成只有名字和描述的占位命令。
  - 占位命令不读操作索引，因此补全顶层时的开销与站点数量无关。
- **操作与参数**：命中站点时照常注册该站点的命令，但参数选项要在补全时可见。
  - 做法一：在构建命令时直接注册选项，不放进 `Config` 回调。
  - 做法二：在 gcli 里让 `Opts()` 先完成命令初始化。
  - 计划阶段实测 gcli 行为后二选一。优先改 apic；只有改 apic 做不到时才改 gcli，并另立 issue。
- **候选值**：
  - **参数枚举**：`Param.Enum` 填进选项的 `Choices`。
  - **`--env`/`--auth`**：命中站点时，用该站点配置里的 env 名和 auth 名设置 Choices。
  - **`-o`**：候选为固定的 `auto json raw yaml table`。
  - **注意**：Choices 只用于补全。gcli 会不会因为设置了 Choices 而校验取值，计划阶段核实。如果会校验，必须保证校验结果与一期一致：枚举值仍由 apic 自己校验（失败退出 1），未知的 env 或 auth 名报错方式不变。
- **只用缓存**：补全模式下，操作索引只读缓存，过期也直接用，不刷新；没有缓存就不给操作候选。不登录，不执行 exec。错误一律不输出，按 TAB 时不应在终端上打出报错。
- **快捷命令**：新增 `apic link completion <shell>`，为每个已生成的快捷命令输出一段补全注册。
  - 原理：快捷命令 `petctl` 本质上是 `apic --api pet --env … --auth …`。注册的补全函数把输入改写成 `apic --in-completion --api pet <words>` 再取候选。
  - 预扫描已经会把 `--api pet` 改写成位置参数，这一步不用改。
  - 输出内容只依赖 state.yaml 里的快捷命令记录。新增或删除快捷命令后，需要重新执行一次（`link` 命令成功时在 stderr 提示）。

使用方式（写进 README）：

```bash
eval "$(apic --gen-completion bash)"       # apic 自身（gcli 内置）
eval "$(apic link completion bash)"        # 已生成的快捷命令
```

### 2. 输出：`--pick` 与 `-o yaml|table`

处理顺序：拿到响应 → 2xx 时应用 `--pick` → 按 `-o` 格式化 → 写 stdout。

- **`--pick <路径>`**（全局选项，只有长名）：
  - 只对 2xx 响应生效。4xx/5xx 的响应体原样输出，退出码照旧，不能让筛选把错误信息吞掉。
  - 响应体必须是 JSON，否则报错退出 1（`--pick needs a JSON response`）。
  - 路径不存在时报错退出 1，stderr 给出路径；stdout 不输出任何内容。
  - 取到字符串时原样输出、不带引号（相当于 `jq -r`），方便在 shell 里直接用；其他类型按 JSON 输出。
  - `*` 取到的是数组，比如 `data.items.*.name` 得到名字数组。
  - 不加短名的原因：一期路线写的是 `-q`，但 `q` 是很常见的搜索参数名。按一期规则，全局选项占用一个名字后，同名参数只能改用 `-p q=...`，会让已有站点的用法变差。所以改为长名 `--pick`，记为 D2。
- **`-o yaml`**：先把响应体按 JSON 解析，再输出 YAML；不是 JSON 时原样输出，并在 stderr 给一行提示。
- **`-o table`**：
  - **对象数组**：列名取所有对象的顶层键，按第一次出现的顺序排列。单元格中的标量原样显示，对象或数组显示成紧凑 JSON；终端下单元格截断到 60 字符，非终端不截断。
  - **单个对象**：输出两列，键和值。
  - **其他情况**（标量、空数组、非 JSON）：退回 `json` 格式的输出，并在 stderr 提示。
  - 用 `text/tabwriter` 实现，不新增依赖。
- `--pick` 和 `-o` 可以组合使用：`--pick data.items -o table`。`--dry-run` 只打印 curl，不受这两个选项影响。
- **兼容性**：`defaults.output` 和 `-o` 的取值校验扩展到五种，旧的三种行为不变。

### 3. `apic skill <site>`

```text
apic skill <site> [--out <dir>] [--name <skill-name>] [--max-ops 60] [--force]
```

- **输出位置**：默认写到 `<配置目录>/skills/apic-<site>/SKILL.md`，`--out` 可以指定父目录。完成后在 stderr 提示 skillc 的接入方式：`skillc source add local <配置目录>/skills`，然后 `skillc install apic-<site>`。
- **frontmatter**：
  - `name` 默认 `apic-<site>`；
  - `description` 是一句话，说明该站点有多少个操作、主要 tag 有哪些，以及“通过 apic 调用”。
  - 这些都由操作索引生成，不需要新增模型字段。
- **正文**（英文，供 agent 读）：
  1. 怎么调用：`apic <site> <op> --param v`、原始路径调用、`--pick`、`--dry-run`。
  2. 退出码表，强调 stdout 只有响应体。
  3. env 与 auth：只列名字和类型，以及 `protect`、`allow_methods` 这类约束，提示写操作需要 `--yes`。不写 base URL，不写任何凭证或 `${env:}` 引用名。
  4. 操作清单：
     - 操作数不超过 `--max-ops` 时，逐个列出：名字、方法、路径、摘要、必填参数。
     - 超过时只按 tag 列出操作数，并让 agent 先用 `apic ops <site> --tag <t>` 和 `apic desc <site> <op>` 查找。
- **覆盖规则**：生成的文件第一行写标记 `<!-- generated by apic skill -->`。
  - 目标文件已存在且带标记时，直接覆盖。
  - 不带标记时（用户手改过并去掉了标记，或者是别人的文件），拒绝覆盖并退出 1，加 `--force` 才覆盖。
- **数据来源**：只读取操作索引和站点配置，不发请求；索引按一期规则加载，过期时会刷新。

### 4. exec 认证

```yaml
auths:
  sso:
    type: exec
    exec:
      command: [corp-sso, token, --audience, pet]   # argv，不经过 shell
      timeout: 30s                                    # 默认 30s
      format: text                                    # text（默认）| json
      token: {path: data.token}                       # 仅 json
      expires: {path: expires_in}                     # 仅 json，可选；单位秒
      ttl: 30m                                        # 没有 expires 时的缓存时长；不填则不缓存
    apply: {header: Authorization, value: "Bearer {token}"}   # 与 login 相同，默认 Bearer
```

- **模型**：
  - `AuthSpec` 增加 `Exec ExecSpec`，并新增顶层 `apply`，供 exec 使用。
  - login 仍用 `login.apply`。为保证一期配置继续可用，不搬动它，两种位置都接受。
  - 复用 `ValueFrom` 和 `ApplySpec` 两个类型。
- **执行**：
  - 用 `os/exec` 按 argv 执行，不经过 shell。
  - 工作目录为配置目录。
  - 继承当前进程的环境变量，另外附加 `APIC_SITE`、`APIC_ENV`、`APIC_AUTH`。
  - 超时则杀掉进程。
- **取 token**：
  - `text` 格式：取 stdout 去掉首尾空白后的内容，结果为空视为失败。
  - `json` 格式：按路径取 `token` 和 `expires`。
  - 取到的 token 和应用后的头部值登记为敏感值，在所有输出里打码。
- **缓存**：复用 login 的 token 缓存，即同样的 `cache/` 目录、0600 权限、按 site.env.auth 区分。
  - 指纹包含 command 和 format，配置一改旧缓存就自动失效。
  - 过期时间优先用 `expires`，其次用 `ttl`；两者都没有时不缓存，每次请求都执行一次命令。
- **401**：走与 login 相同的 `Refresh` 路径，清缓存、重新执行命令、重发一次请求。只重试一次。
- **失败**：命令退出码非 0、超时或取不到 token 时，按本地错误退出 1。
  - stderr 只输出命令退出码和命令 stderr 的最后一行，经打码后再输出。
  - 命令自己的 stderr 不直接透传。
- **`--dry-run`**：有有效缓存就用缓存的 token（输出时打码）；没有就用占位符 `<exec-token>`，不执行命令。这与 login 的 dry-run 行为一致。
- **`auth login/logout/status`**：
  - `login`：执行一次命令并写入缓存；
  - `logout`：清缓存；
  - `status`：显示是否有缓存以及过期时间。
- **`${env:}`**：`command` 的每个元素都可以写 `${env:}`。展开后的值不登记为敏感值，因为命令行参数通常不是凭证；如果某个参数确实是凭证，应改为通过环境变量传给命令。
- **配置来源**：exec 只能出现在用户自己的配置里（`config.yaml`、`sites/*.yaml`）。spec 和简单定义都不能定义认证，加载器保持现状，不读取其中的认证配置。

## 架构

### 代码组织

沿用一期结构（参照 gofer `cmd/gofer/main.go` + `internal/commands/`，suga `internal/wiring` + 分层守卫测试，SR2004），只新增一个领域包：

```
internal/commands/prescan.go     # 补全模式识别；占位站点命令
internal/commands/dynamic.go     # 参数选项可见于补全；Choices
internal/commands/link.go        # link completion
internal/commands/skill.go       # apic skill（只做参数解析与调用）
internal/output/pick.go          # --pick
internal/output/yaml.go, table.go
internal/skillgen/               # 新：由 model.Index + 站点配置渲染 SKILL.md；只依赖 model
internal/auth/exec.go            # exec 提供者
internal/model/config.go         # AuthSpec.Exec、ExecSpec、AuthSpec.Apply
internal/config/expand.go        # exec 字段的 ${env:} 展开（不登记为敏感值）
internal/link/                   # 生成补全注册片段
```

依赖方向不变：commands → wiring → 领域包 → model。`skillgen` 加入分层守卫测试的领域包列表。

## 关键流程

### 补全

```
shell TAB ─▶ apic --in-completion <words>
   预扫描：补全模式？
     ├─ 未命中站点 ─▶ 注册所有站点的占位命令（不读索引）
     └─ 命中站点   ─▶ 只读缓存索引 ─▶ 注册站点命令（参数选项立即注册，含 Choices）
   gcli resolveCompletion ─▶ stdout 输出候选；任何错误都静默
```

### 一次带 `--pick` 的请求

```
响应 ─▶ 2xx？──否──▶ 原样输出，退出码 4/5
          │是
          ▼
       有 --pick？──是──▶ 解析 JSON ─▶ 按路径取值（失败退出 1）
          │                                  │
          ▼                                  ▼
       按 -o 格式化（auto/json/raw/yaml/table）─▶ stdout
```

### exec 认证

```
Apply ─▶ 缓存有效？──是──▶ 注入 token
           │否
           ▼
        dry-run？──是──▶ 注入 <exec-token>
           │否
           ▼
        执行 argv（超时）─▶ 取 token ─▶ 登记为敏感值 ─▶ 写缓存 ─▶ 注入
收到 401 ─▶ Refresh：清缓存，重新执行命令 ─▶ 重发一次
```

## 安全、数据、运维与回滚

- **exec 执行命令**：
  - 命令只来自用户自己的配置文件。apic 一期就不会把 spec 内容写进 auths，二期保持这一点，并在计划中加测试固定下来：spec 或简单定义里出现认证字段时，加载器必须忽略。
  - 不经过 shell，没有注入面。
  - 超时会杀掉子进程。
- **凭证**：
  - exec 取到的 token 只写进 0600 缓存，所有输出都打码。
  - 命令 stderr 只取最后一行并打码，以免命令打印的 token 泄露。
  - skill 文件不写 base URL、凭证和 `${env:}` 引用名。
- **补全**：不联网，不登录，不执行命令，按 TAB 不会产生任何副作用。补全会读取缓存的 spec 索引，与 `apic ops` 读到的数据相同。
- **数据格式**：
  - config.yaml 只新增字段，一期配置不改就能继续用。
  - state.yaml 和缓存格式不变。
- **回滚**：四项都是新增功能，可以逐项撤回；回退到 v0.1.1 二进制即可回滚，配置里新增的 exec 认证在旧版本上会报“unknown type”。
- **发布**：做完后发 v0.2.0。发版是外部动作，需要用户另行授权。

## 决策

| ID | 决策 | 理由 |
|---|---|---|
| D1 | 补全沿用 gcli 运行时补全，apic 只补站点占位、参数选项可见、Choices 和快捷命令 | gcli 已有 `--in-completion` 与三种 shell 的脚本；静态脚本无法覆盖“每个站点各自一套操作”的动态结构 |
| D2 | 一期路线里的 `-q` 改为长选项 `--pick`，没有短名 | `q` 是常见的搜索参数名；全局选项占用一个名字后，同名参数就只能写成 `-p q=`。另外 `-q` 容易被误认为 jq |
| D3 | `--pick` 只对 2xx 生效，路径不存在退出 1 | 不吞错误响应；agent 能区分“取到空值”和“路径写错了” |
| D4 | `--pick` 用 goutil maputil 路径语法，不做 jq | 已在 login 中使用，语法一致；SR1403 先复用 |
| D5 | table 只处理对象数组和单个对象，其他情况退回 json | 覆盖常见的列表接口，不做通用的表格推断 |
| D6 | skill 正文只写 env/auth 名字和约束，不写 base URL 与凭证；操作多时只给 tag 索引 | skill 可能被分享或提交到仓库；大 spec 全量列出会撑爆 agent 上下文 |
| D7 | `apic skill` 不调用 skillc | 不形成跨工具的运行时依赖；只给出接入提示 |
| D8 | exec 按 argv 执行，不经过 shell；复用 login 的缓存与 401 重试 | 没有注入面；重用已验证的缓存和重试路径 |
| D9 | exec 新增顶层 `apply`，login 的 `login.apply` 保持不变 | 不破坏一期配置；exec 没有 login 节，不应把 apply 写在 login 下 |
| D10 | 补全模式只读缓存，所有错误静默 | 按 TAB 时不能等网络，也不能在终端上打出报错 |

## 待确认事项

无阻塞项。以下在计划阶段先核实，再写进任务：

- 参数选项在补全时不可见的确切原因（`Config` 是否懒执行），以及修复放在 apic 还是 gcli（D1）。
- 设置 `Choices` 后 gcli 是否会校验取值，以及如何保证与一期的错误行为一致。
- zsh 和 pwsh 下快捷命令补全片段的写法（bash 的写法已明确）。

## 结论与人工计划 Gate

本设计确定二期第一批四项的范围、对外接口（`--pick`、`-o yaml|table`、`apic skill`、`apic link completion`、`type: exec`）和安全边界。评审完成后等待人工批准；批准只授权编写二期第一批实施计划，不授权实施。
