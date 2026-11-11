<!-- template_id: design; template_version: 1.2.0 -->
# apic 二期：补全、输出筛选、skill 生成与 exec 认证 设计

> 状态：Draft 0.2 / 已批准（用户 2026-11-11：“批准 apic 二期设计 0.2（接受未复核修订）”；0.2 为第 1 轮评审后的未复核修订；批准只授权编写实施计划，不授权实施）

## 修订记录

| 版本 | 日期 | 作者 | 摘要 |
|---|---|---|---|
| 0.1 | 2026-11-11 | Claude（inhere：“二期按推荐的排”） | 初稿：二期第一批四项——shell 补全、`--pick` 与 `-o yaml`/`-o table`、`apic skill`、exec 认证 |
| 0.2 | 2026-11-11 | Claude | 处理第 1 轮两轴评审（阻塞 C1–C4、S1；纠正 C5–C11、S2–S10；建议若干）：补全根因改为共享选项不可见，补全模式下把全局选项注册到命令本地；预扫描补全模式改造；spec 只读缓存开关；JSON 保序保数字的统一解码；`--pick` 自写遍历与 `*` 语义；新增退出码 7；`-o yaml/table` 只对 2xx；exec 参数登记为敏感值、`exec.env`、缓存只在过期时间已知时写入、完整指纹、进程组终止、Windows 批处理参数限制；skill 文本净化、标记位置、名字校验；保留名与兼容性说明 |

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
- 评审状态：第 1 轮已完成，两个方向共提出 5 个阻塞项，0.2 已逐条修订。预算只有一轮，所以 0.2 属于未复核修订，修订摘要见下文“评审处理”。

## 背景与目标

一期（v0.1.1）已经能注册站点、按操作调用、切换 env/auth、生成快捷命令。实际使用中有四处不便：

1. **补全**：顶层补不出站点名，全局选项（`--env`、`-o`、`--dry-run` 等）及其取值在站点和操作层也补不出来（实测见“已确认事实”）。
2. **输出**：响应通常是一层层包起来的 JSON，只想要 `data.items[].name` 时得再接 jq。agent 环境里不一定有 jq，人看大 JSON 也费劲。
3. **给 agent 用**：要让 agent 知道某个站点能做什么，现在得手写说明，或者让它先跑 `apic ops`、`apic desc` 自己摸索。
4. **认证**：一些服务的 token 来自外部命令（`vault read`、公司 SSO CLI、`gcloud auth print-access-token` 一类），一期的 login 只能发 HTTP 登录请求，接不上这类服务。

用户可见的结果：

- `apic <TAB>` 能补出站点名；`apic pet list-users --<TAB>` 能补出参数和全局选项；`--status <TAB>` 能补出枚举值；`--env <TAB>` 能补出 env 名；快捷命令 `petctl <TAB>` 同样可以补全。
- `apic pet list-users --pick data.items.*.name` 只输出名字列表；`-o table` 按表格显示，`-o yaml` 按 YAML 显示。数字和字段顺序保持原样。
- `apic skill pet` 生成 `SKILL.md`，可以交给 skillc 装给各个 agent。
- `type: exec` 的认证调用外部命令拿 token，token 会缓存，收到 401 时重新执行命令取新 token。

## 名词

| 名词 | 含义 |
|---|---|
| 运行时补全 | shell 补全脚本不内置候选，每次按 TAB 都调用 `apic --in-completion <已输入的词>`，由 apic 输出候选（gcli 自带机制） |
| 共享选项 | gcli 的 `SharedOpts`：注册在父命令上，子命令解析参数时才并入。一期的全局选项靠它在站点和操作层生效 |
| pick 路径 | `--pick` 的取值语法：点号分段，段可以是键名、数组下标或 `*` |
| 保序解码 | 把 JSON 解成保留对象键顺序、数字保留原文的内部值，供 pick、yaml、table 共用 |
| skill | 一个目录，含 `SKILL.md`（frontmatter 的 `name`/`description` 加正文），供 Claude Code、codex 等 agent 按需加载 |
| exec 认证 | `type: exec`：执行一条外部命令，从它的 stdout 取 token |

其余名词沿用一期设计（[apic-design.md](apic-design.md)）。

## 范围与非目标

**范围**（仅 `inhere-tools/apic` 一个项目）：

1. shell 补全（bash、zsh、pwsh）：
   - 顶层补全站点名，`--api <TAB>` 也补站点名；
   - 操作的参数选项和全局选项；
   - 参数的枚举值；
   - `--env`、`--auth`、`-o` 的取值；
   - 快捷命令的补全。
2. 输出：`--pick <路径>`；`-o` 增加 `yaml`、`table`；新增退出码 7。
3. `apic skill <site>`：生成 skill 目录。
4. 认证类型 `exec`。

**非目标**：

- 不实现 jq 语法，不支持过滤表达式和函数，`--pick` 只按路径取值；键名本身带点时无法用 `--pick` 访问。
- 补全时不发起网络请求：不刷新 spec，不登录，不执行 exec 命令。
- 顶层 `apic --env <TAB>`（站点名之前）不补取值：此时还不知道是哪个站点。
- `apic skill` 不调用 skillc，也不直接写各 agent 的 skills 目录。
- exec 认证不经过 shell，不支持管道和重定向写法；需要时由用户自己写成脚本再调用。交互式登录（要输入或打开浏览器）需在 apic 之外先完成。
- Windows 上不做 Job Object 进程树终止，见 D14。
- 二期第二批的内容（见“设计工作声明”）。

## 已确认事实与规范

1. **gcli 运行时补全**（gcli v3.8.6，`gookit2/gcli/completion.go`）：
   - `--gen-completion bash|zsh|pwsh` 生成瘦脚本，按 TAB 时调用 `<bin> --in-completion <words>` 计算候选。
   - 候选来自当前节点的命令、子命令和 `node.Opts()`。
   - 选项值的候选来自 `CliOpt.Choices`。只设 Choices 不会触发取值校验，只有 struct tag `enum` 才会设置校验器（`gflag/parser.go:436-438`、`gflag/opts.go:638`）。
   - 命令的 `Config` 在 `addCommand` 时立即执行（`base.go:314`、`cmd.go:326`），因此动态注册的参数选项在补全时可见。
   - `SharedOpts` 要到解析参数时才并入子命令（`cmd.go:556,579`），补全时看不到。
2. **apic 现状实测**（v0.1.1；评审另用 openapi 站点 `pet` 复测）：

   | 输入 | 实际候选 | 期望 |
   |---|---|---|
   | `apic <TAB>` | 管理命令 | 另含站点名 |
   | `apic jk <TAB>` | `job-build job-info` | 符合 |
   | `apic jk job-build --<TAB>` | `--BRANCH --ENV --name` | 另含全局选项 |
   | `apic pet list-users --env <TAB>` | 空 | env 名 |
   | `apic --in-completion --e` | 报 `unknown option`，退出 2 | 不报错 |

3. **预扫描**（`internal/commands/prescan.go`）：会校验全局选项和 `--api` 的取值。补全时正在输入的词不完整，就会被当作错误。
4. **spec 索引**（`internal/spec/cache.go:51-58`）在缓存过期或缺失时一定会去拉取，目前没有只读缓存的选项。
5. **路径取值**：goutil 的 `maputil.GetFromAny` 有三个问题，不能直接用于 `--pick`，实测如下：
   - `*` 遇到空数组返回“不存在”；
   - 元素缺字段时会被静默丢掉；
   - 解码到 `map[string]any` 后键顺序丢失。
6. **数字与编码**（实测）：
   - `json.Unmarshal` 后再 Marshal，会把 `12345678901234567891` 改成 `12345678901234567000`，并转义 `<>&`；
   - goccy/go-yaml 会把 `1` 输出成 `1.0`，并按键名排序。
7. **配置环境**：`.env` 里的值只进 apic 自己的变量表，不写进进程环境（`internal/config/dotenv.go`）。
8. **保留名**：
   - 管理命令名在 `config.reservedNames`（`load.go:14`）和 `managementCommands`（`run.go:19`）里定义；
   - 已有站点用了保留名时，整个 `config.Load` 会失败。
9. **参数选项名**：要求至少 2 个字符（`dynamic.go:78`）。参数名与全局选项重名时，参数退回只能用 `-p` 传入（`globalopts.go:29`）。
10. **skillc** 支持本地目录作为 skill 来源：`skillc source add local <path>`，然后 `skillc install`。
11. **一期契约不变**：
    - 退出码只增不改；stdout 只输出响应体；全局选项只增不改名；
    - `${env:}` 展开的凭证值都登记为敏感值（一期设计“安全、数据、运维与回滚”）。
12. **适用规范**：
    - 结构：GR107–GR110、SR2003、SR2004。
    - 测试断言：SR2002。

## 总体方案

四项彼此独立，各自落在已有的层次上，不改统一操作模型：

| 项 | 落点 | 对外新增 |
|---|---|---|
| 补全 | `commands`：预扫描、动态命令构建；`spec`：只读缓存 | `apic link ls --completion <shell>` |
| 输出 | `output`：保序解码、pick、yaml、table | 全局选项 `--pick`；`-o yaml\|table`；退出码 7 |
| skill | 新领域包 `skillgen` + `commands/skill.go` | `apic skill <site>`；保留名 `skill` |
| exec 认证 | `auth`：exec 提供者；`model`：`AuthSpec.Exec`、`AuthSpec.Apply` | 配置 `type: exec` |

### 1. 补全

沿用 gcli 的运行时补全，apic 补齐以下几处：

- **预扫描的补全模式**：argv 里有 `--in-completion` 时启用。
  - 最后一个词是正在输入的词，不参与选项校验，也不参与站点命中判断。比如 `apic jk<TAB>`，同时存在 `jkx` 时，两个都要能补出来。
  - 预扫描出错时不报错，退回“注册全部站点占位命令”。
  - 前一个词是 `--api` 时，预扫描直接输出匹配的站点名并结束；这个选项 gcli 并不知道。
- **站点名**：没有命中站点时，把所有已配置站点注册成只有名字和描述的占位命令。占位命令不读操作索引，所以补全顶层时的开销与站点数量无关。
- **全局选项**：补全模式下，把全局选项和站点选项（`-X/-d/-f/-p`）直接注册到站点命令和每个操作命令本地，不再走共享选项。这样 `node.Opts()` 能看到它们，`--env/--auth` 的 Choices 也能生效。
  - 普通模式的注册方式保持不变。
  - 是否让 gcli 在补全时带上祖先的共享选项，另立 gcli issue，不阻塞本期。
- **候选值**：
  - **参数枚举**：`Param.Enum` 填进选项的 `Choices`。
  - **`--env`/`--auth`**：命中站点时，用该站点配置里的 env 名和 auth 名设置 Choices。
  - **`-o`**：按命令区分。站点调用和操作调用给 `auto json raw yaml table`，`ops`、`desc`、`site ls` 只给 `json`。
  - Choices 只用于补全，不改变取值校验，一期的错误行为不变。
- **只用缓存**：spec 的 `IndexOptions` 新增 `CacheOnly`。补全模式下，缓存过期也直接用，没有缓存就不给操作候选。补全时不登录，不执行 exec，所有错误都不输出。
- **快捷命令**：
  - 新增 `apic link ls --completion <shell>`，为每个已生成的快捷命令输出一段补全注册。用 `ls` 的选项而不是新子命令，是为了不占用 `link <site>` 的站点名空间。
  - 原理：快捷命令 `petctl` 本质上是 `apic --api pet --env … --auth …`，补全函数把输入改写成 `apic --in-completion --api pet <words>` 再取候选。
  - 输出前用 `link` 的名字规则重新校验每个快捷命令名，不合法的跳过，因为输出会被 `eval`。
  - 新增或删除快捷命令后，需要重新执行一次；`link` 命令成功时在 stderr 提示。

使用方式（写进 README）：

```bash
eval "$(apic --gen-completion bash)"            # apic 自身（gcli 内置）
eval "$(apic link ls --completion bash)"        # 已生成的快捷命令
```

### 2. 输出：`--pick`、`-o yaml|table` 与退出码 7

**保序解码**：pick、yaml、table 共用一个解码器。

- 用 `json.Decoder` 逐个读 token：对象保留键的出现顺序，数字保留原文（`json.Number`）。
- 输出 JSON 时数字按原文写回，不转义 HTML。
- 输出 YAML 时，对象转成 goccy 的有序映射；数字按原文写成 YAML 数字字面量。原文如果是 YAML 无法原样表示的写法，就带引号输出原文。
- 加用例固定以下行为：大整数、小数、`<>&`、键顺序。

**处理顺序**：

```
发送前：校验 --pick 路径语法，校验 -o 取值（失败退出 2，不发请求）
响应后：
  2xx ─▶ 有 --pick？─▶ 取值 ─▶ 按 -o 格式化 ─▶ stdout，退出 0
  3xx ─▶ 原样输出，退出 0；用了 --pick 或 yaml/table 时 stderr 警告“未应用”
  4xx/5xx ─▶ 原样输出，退出 4/5（不 pick、不转换格式）
```

**`--pick <路径>`**（全局选项，只有长名）：

- **路径语法**：点号分段，段可以是键名、非负整数下标或 `*`。顶层是数组时直接以下标或 `*` 开头，如 `0.name`、`*.id`。
- **`*` 的语义**：
  - 对数组的每个元素继续取后续路径，结果是与原数组等长的数组；
  - 元素缺少后续字段时，对应位置填 `null`，保持对齐；
  - 空数组得到 `[]`；
  - `*` 用在对象上时，依次取各个值。
- **路径不存在**：只有非通配段取不到才算，比如键不存在、下标越界、对标量继续取值。
- **字符串的输出形式**：

  | `-o` | 取到字符串 | 取到其他值 |
  |---|---|---|
  | auto / raw | 原样输出，不带引号 | JSON |
  | json | JSON 编码（带引号） | JSON |
  | yaml | YAML | YAML |
  | table | 退回 json 并在 stderr 提示 | 按 table 规则 |

**退出码 7**（新增，表示“请求已成功，本地后处理失败”）：

- 触发条件：2xx 响应，但 `--pick` 或 `-o yaml|table` 处理失败。包括响应不是 JSON、body 为空（如 204）、路径不存在。
- 此时 stdout 不输出任何内容；stderr 写明 `HTTP 201 succeeded; --pick failed: <原因>`。
- 单独占一个码，是因为退出 1 表示本地错误、请求很可能没发出。agent 可能据此重试 POST，造成重复写入；7 则明确表示请求已经生效，不要重试。
- 退出码表只新增这一项，已有含义不变。

**`-o yaml`**：用保序解码后输出 YAML。

**`-o table`**：

- **对象数组**：列名取所有对象的顶层键，按第一次出现的顺序排列。单元格中的标量原样显示，对象或数组显示成紧凑 JSON。
- **单个对象**：输出两列，键和值。
- **其他情况**（标量、空数组）：退回 json，stderr 提示。
- **终端显示**：输出到终端时，单元格截断到 60 字符，并去掉控制字符和 ANSI 序列；非终端不截断。
- 用 `text/tabwriter` 实现。

**其他规则**：

- `--dry-run` 只打印 curl，不受 `--pick` 和 `-o` 影响。
- `defaults.output`、简单定义的 `output:` 和 `-o` 的取值校验都扩展到五种，旧的三种行为不变。
- **实现位置**：`output.Render` 改为返回 `(code int, err error)`，由 siterun 统一输出错误。这样 2xx 下的后处理失败不会再被报成 `HTTP 200`。

**兼容性**：

- 站点里名为 `pick` 的参数，`--pick` 选项让给全局选项，改为只能用 `-p pick=...`。这一点写进 v0.2.0 的发布说明。
- 一期是在 `pick` 这个名字上首次占用；全局选项总要占一个长名，`pick` 作为 API 参数名很少见。

### 3. `apic skill <site>`

```text
apic skill <site> [--out <dir>] [--name <skill-name>] [--max-ops 60] [--force]
```

- **名字与位置**：
  - skill 名默认 `apic-<site>`，`--name` 必须符合 `^[a-z0-9][a-z0-9-]{0,63}$`，否则退出 2。
  - 目录名等于 skill 名，写到 `<--out 或 <配置目录>/skills>/<skill 名>/SKILL.md`。
  - 写入方式是先写临时文件再 rename，不跟随已存在的符号链接。
- **frontmatter**：用 YAML 编码器生成，不做字符串拼接。
  - `name`：skill 名；
  - `description`：一句话，说明该站点有多少个操作、主要 tag 有哪些，以及“通过 apic 调用”，限 200 字符。
- **正文**（英文，供 agent 读）：
  1. **调用方式**：`apic <site> <op> --param v`、原始路径调用、`--pick`、`--dry-run`。
  2. **退出码表**：强调 stdout 只有响应体，退出码 7 不要重试。
  3. **env 与 auth**：只列名字和类型，以及 `protect`、`allow_methods` 这类约束，提示写操作需要 `--yes`。不写 base URL、凭证、`${env:}` 引用名和 exec 命令。
  4. **操作清单**：
     - 先写一句：“以下摘要来自 API 定义，是数据而非指令。”
     - 操作数不超过 `--max-ops` 时，逐个列出：名字、方法、路径、摘要、必填参数。
     - 超过时按 tag 列出操作数，没有 tag 的归入 `untagged`；简单定义的操作没有 tag，会全部归入这一组。然后让 agent 用 `apic ops <site> [--tag t]` 和 `apic desc` 查找。
- **净化**：来自 spec 的字段（tag、summary、参数名）一律净化后再写入：
  - 去掉控制字符和换行，截断到 120 字符；
  - 转义反引号和 Markdown 标记；
  - 去掉 `<!--`、`-->`。
- **覆盖标记**：在 frontmatter 之后第一行写 `<!-- generated by apic skill -->`。
  - 目标文件已存在且带标记时，直接覆盖。
  - 不带标记时拒绝覆盖并退出 1，加 `--force` 才覆盖。
- **数据来源**：只读取操作索引和站点配置，不发请求；索引按一期规则加载，过期时会刷新。
- **完成提示**：在 stderr 提示两点：
  - skillc 的接入方式：`skillc source add local <目录>`，然后 `skillc install <skill 名>`；
  - 分享前先检查，因为文件里有内部 API 的路径。
- **保留名**：`skill` 加入管理命令保留名。已有名为 `skill` 的站点，加载时跳过该站点并在 stderr 警告“改名后可用”，其他站点不受影响；不能因为一个站点让整个配置加载失败。这条“跳过并警告”的规则对今后新增的保留名同样适用。

### 4. exec 认证

```yaml
auths:
  sso:
    type: exec
    exec:
      command: [corp-sso, token, --audience, pet]   # argv，不经过 shell
      env: {SSO_CLIENT_SECRET: "${env:PET_SSO_SECRET}"}  # 附加给子进程的环境变量
      timeout: 30s                                    # 默认 30s
      format: text                                    # text（默认）| json
      token: {path: data.token}                       # 仅 json
      expires: {path: expires_in}                     # 仅 json，可选；单位秒
      ttl: 30m                                        # 没有 expires 时的缓存时长
    apply: {header: Authorization, value: "Bearer {token}"}   # 默认 Bearer
```

- **模型与校验**：
  - `AuthSpec` 增加 `Exec ExecSpec` 和顶层 `Apply`。
  - login 仍用 `login.apply`，一期配置不改就能继续用。
  - 复用 `ValueFrom` 和 `ApplySpec` 两个类型。
  - `exec` 节做未知键校验，比如误写成 `exec.apply` 时报错，而不是静默退回默认头。
- **展开与敏感值**：
  - `command` 的每个元素和 `env` 的每个值都允许 `${env:}`，展开结果一律登记为敏感值，保持一期契约。
  - 展开前先复制 `command` 切片和 `env` 映射，不改动已加载的配置。
  - 凭证应通过 `exec.env` 传入。`.env` 里的值不在进程环境中，不经 `exec.env` 子进程就拿不到。
- **执行**：
  - 用 `os/exec` 按 argv 执行，不经过 shell；stdin 固定为空设备。
  - 工作目录为配置目录，为此 `auth.Deps` 增加 `ConfigDir`。
  - 子进程环境是当前进程的环境，加上 `exec.env`，再加上 `APIC_SITE`、`APIC_ENV`、`APIC_AUTH`。
  - stdout 和 stderr 各限读 4 MiB。
- **超时**：
  - 超时时间取 `exec.timeout`，同时受请求 ctx 约束，先到先停。
  - Linux 和 macOS 上，子进程放进独立进程组，超时就杀掉整个进程组。
  - 所有平台都设置 `Cmd.WaitDelay`（2 秒），防止孙进程占着管道导致一直等下去。
- **Windows 批处理**：解析出的可执行文件是 `.bat` 或 `.cmd` 时，参数里不允许出现 cmd 元字符（`& | < > ^ % ! "` 和换行），出现就报错退出 1，不执行。原因是 cmd.exe 会重新解释这些参数。
- **取 token**：
  - `text` 格式：stdout 去掉首尾空白后必须是单行，不能含控制字符，否则视为失败。
  - `json` 格式：按 pick 路径语法取 `token` 和 `expires`。
  - token 和应用后的头部值都登记为敏感值。
- **缓存**：复用 login 的 token 缓存，即同样的 `cache/` 目录、0600 权限（Windows 上该权限位不生效，文档注明）、按 site.env.auth 区分。
  - **过期时间**：取得到 `expires` 就用它；取不到就用 `ttl`；两者都没有就不缓存。
  - **不写永不过期的缓存**：exec 写缓存时过期时间必须是确定的，不写零值。
  - **指纹**：包含 type、base_url、展开后的整个 `exec` 节和 `apply`，任一变化旧缓存即失效。指纹按认证类型分开计算，login 的指纹不变。
- **401**：与 login 走同一条 `Refresh` 路径，清缓存、重新执行命令、重发一次请求，只重试一次。
- **失败**：命令退出码非 0、超时、取不到 token 或违反 Windows 参数限制时，一律按本地错误退出 1。
  - stderr 只输出认证名、命令退出码，以及命令 stderr 的最后一行（经打码）。
  - argv 在任何输出中都不出现，包括 `-v`、错误消息、`auth status`、`config show` 和 `site show`；这些地方 `command` 只显示 `command[0]` 加上 `***`。
- **`--dry-run`**：有有效缓存就用缓存的 token（输出时打码）；没有就用占位符 `<exec-token>`，不执行命令，与 login 一致。
- **`auth login/logout/status`**：
  - `login`：执行一次命令。能缓存时写入缓存；不能缓存时只验证能取到 token，并提示“未缓存，每次请求都会执行命令”。
  - `logout`：清缓存。
  - `status`：显示缓存状态和过期时间，不能缓存的 exec 显示 `not cached (no expiry)`。
- **接入点**：
  - `auth.New` 的类型白名单、`Apply` 的分支、`Refresh` 的类型判断、`injectToken` 读哪个 apply、按类型计算指纹；
  - `commands/auth.go` 里 status 的类型判断；
  - `wiring` 给 `Deps.ConfigDir` 赋值。

## 架构

### 代码组织

沿用一期结构（参照 gofer `cmd/gofer/main.go` + `internal/commands/`，suga `internal/wiring` + 分层守卫测试，SR2004），只新增一个领域包：

```
internal/commands/prescan.go     # 补全模式：cur 不校验、占位站点、--api 候选
internal/commands/dynamic.go     # 补全模式下全局/站点选项本地注册；Choices
internal/commands/globalopts.go  # --pick：reservedOptNames、globalSpec、bind；-o 按命令的 Choices
internal/commands/run.go         # managementCommands 加 skill
internal/commands/siterun.go     # 发送前校验；Render 返回 (code, err)；退出码 7
internal/commands/link.go        # link ls --completion
internal/commands/skill.go       # apic skill（只做参数解析与调用）
internal/commands/auth.go        # status 支持 exec
internal/output/jsonv.go         # 保序解码 / JSON 写回 / 转 YAML 值
internal/output/pick.go, yaml.go, table.go
internal/output/exit.go          # Render 签名；ExitPostProcess = 7（常量放 model）
internal/model/                  # ExitPostProcess；AuthSpec.Exec、ExecSpec、AuthSpec.Apply
internal/spec/cache.go           # IndexOptions.CacheOnly
internal/skillgen/               # 新：由 model.Index + 站点配置渲染 SKILL.md；只依赖 model
internal/auth/exec.go            # exec 提供者；进程组终止分平台文件（exec_unix.go / exec_windows.go）
internal/auth/provider.go, tokencache.go   # 类型分支、按类型计算指纹、Deps.ConfigDir
internal/config/expand.go        # exec.command / exec.env 展开并登记为敏感值
internal/config/load.go          # 保留名 skill；保留名冲突时跳过站点并警告；exec 未知键校验
internal/link/                   # 生成补全注册片段（名字重新校验）
```

依赖方向不变：commands → wiring → 领域包 → model。`skillgen` 加入分层守卫测试的领域包列表。

## 关键流程

### 补全

```
shell TAB ─▶ apic --in-completion <words>
   预扫描（补全模式；最后一个词不校验、不命中）：
     ├─ 前一个词是 --api      ─▶ 直接输出匹配的站点名
     ├─ 未命中站点或预扫描出错 ─▶ 注册所有站点的占位命令（不读索引）
     └─ 命中站点              ─▶ 只读缓存索引 ─▶ 注册站点命令
                                  （全局/站点选项本地注册，参数选项带 Choices）
   gcli resolveCompletion ─▶ stdout 输出候选；任何错误都静默
```

### 一次带 `--pick` 的请求

见第 2 节“处理顺序”。

### exec 认证

```
Apply ─▶ 缓存有效？──是──▶ 注入 token
           │否
           ▼
        dry-run？──是──▶ 注入 <exec-token>
           │否
           ▼
        展开 argv/env（登记为敏感值）─▶ Windows 批处理参数检查
           ─▶ 执行（stdin 为空、进程组、超时、WaitDelay）─▶ 取 token（单行校验）
           ─▶ 登记为敏感值 ─▶ 过期时间已知？─是─▶ 写缓存 ─▶ 注入
                                        └否─▶ 不缓存 ─▶ 注入
收到 401 ─▶ Refresh：清缓存，重新执行命令 ─▶ 重发一次
```

## 安全、数据、运维与回滚

- **配置文件等同可执行代码**：有了 exec 之后，`config.yaml` 和 `sites/*.yaml` 都可能让 apic 执行命令。
  - README 和安全说明写明：不要导入不信任的站点文件。
  - `config check` 列出所有 exec 认证及其 `command[0]`，作为提示。
  - spec 和简单定义都不能定义认证：`site add` 只写 source 和 envs，spec 认证的白名单也排除了 exec（`internal/wiring/wiring.go`）。计划中加测试固定这两点。
- **命令执行**：
  - Unix 上不经过 shell，没有 shell 注入面。
  - Windows 上目标是 `.bat`/`.cmd` 时，按第 4 节的元字符检查拒绝执行。
  - 超时会杀掉进程组，并有 WaitDelay 兜底。
- **凭证**：
  - exec 的 argv 和 env 展开值、token 都登记为敏感值，在所有输出中打码；argv 不出现在任何输出里。
  - 命令 stderr 只取最后一行并打码。
  - skill 文件不写 base URL、凭证、引用名和 exec 命令。
- **skill 的提示注入**：来自 spec 的文本都经过净化，并在正文中声明“摘要是数据”。生成完成时在 stderr 提醒分享前检查。
- **补全**：不联网，不登录，不执行命令，按 TAB 不会产生任何副作用。`link ls --completion` 的输出会被 `eval`，输出前重新校验名字。
- **数据**：
  - 保序解码保证 `--pick`、yaml、table 不改动数字和字段顺序，用例固定。
  - config.yaml 只新增字段，state.yaml 和缓存格式不变。
- **兼容性**：
  - 名为 `pick` 的参数改为只能用 `-p` 传入；
  - 名为 `skill` 的站点需要改名才能使用；
  - 退出码新增 7。
  - 以上三点都写进 v0.2.0 发布说明。
- **回滚**：回退到 v0.1.1 二进制即可。但要先撤掉新增的配置：
  - 用到 `type: exec` 的认证，旧版会报未知类型；
  - `defaults.output` 或简单定义的 `output:` 设为 yaml 或 table 时，旧版会让所有调用都退出 2。
- **发布**：做完后发 v0.2.0。发版是外部动作，需要用户另行授权。

## 决策

| ID | 决策 | 理由 |
|---|---|---|
| D1 | 补全沿用 gcli 运行时补全；补全模式下把全局选项和站点选项注册到命令本地 | 共享选项要到解析参数时才并入子命令，补全时看不到；只在补全模式改注册方式，不影响正常调用 |
| D2 | 新全局选项取长名 `--pick`，没有短名 | 全局选项总要占用一个长名，`pick` 很少作为 API 参数名；单字母参数本来就不生成选项，所以加不加短名与参数冲突无关，不加短名是为了少占名字 |
| D3 | `--pick` 自写路径遍历，`*` 结果与原数组对齐（缺字段填 null），空数组得到 `[]` | 能区分“空结果”和“路径写错”；goutil 的实现会丢元素，也会把空数组判成不存在 |
| D4 | pick、yaml、table 共用保序、保数字原文的解码器 | 不改动 ID 等大整数，不打乱字段顺序 |
| D5 | table 只处理对象数组和单个对象，其他情况退回 json | 覆盖常见的列表接口，不做通用的表格推断 |
| D6 | skill 正文只写 env/auth 名字和约束；spec 文本净化后才写入；操作多时只给 tag 索引 | 防泄露、防提示注入、防撑爆 agent 上下文 |
| D7 | `apic skill` 不调用 skillc | 不形成跨工具的运行时依赖；只给出接入提示 |
| D8 | exec 按 argv 执行，不经过 shell；复用 login 的缓存与 401 重试 | Unix 上没有注入面；重用已验证的缓存和重试路径 |
| D9 | exec 新增顶层 `apply`，login 的 `login.apply` 保持不变 | 不破坏一期配置 |
| D10 | 补全模式只读缓存，所有错误静默 | 按 TAB 时不能等网络，也不能在终端上打出报错 |
| D11 | 新增退出码 7：2xx 已成功但本地后处理失败 | 不让 agent 误判请求失败而重复写入；只增不改，符合退出码契约 |
| D12 | `--pick` 和 `-o yaml/table` 只作用于 2xx；3xx、4xx、5xx 原样输出 | 保持一期的错误输出契约 |
| D13 | exec 的 argv 和 env 展开值都登记为敏感值，argv 不出现在任何输出中，凭证经 `exec.env` 传给子进程 | 保持一期“`${env:}` 展开的凭证值都打码”的契约；`.env` 的值不在进程环境里 |
| D14 | 进程树终止：Unix 按进程组；Windows 只靠 WaitDelay 兜底，不做 Job Object | Job Object 需要额外依赖（x/sys/windows），收益有限；先保证 apic 自身不会卡住 |
| D15 | 新增保留名与已有站点冲突时，跳过该站点并警告，不让整个配置加载失败 | 升级后其他站点照常可用；适用于今后新增的所有保留名 |
| D16 | 快捷命令补全用 `link ls --completion`，不新增 `link completion` 子命令 | `link` 的位置参数是站点名，新增子命令会占掉 `completion` 这个站点名 |

## 评审处理（第 1 轮）

| 评审项 | 处理 |
|---|---|
| C1 根因错误；全局选项和 `--env` 的 Choices 不生效 | 改写已确认事实 1–2、§1、D1；顶层 `--env` 补全写进非目标 |
| C2 重新编码改坏数字 | 新增保序解码（D4）及用例 |
| C3 `*` 语义与 D3 冲突 | 自写遍历（D3） |
| C4 列顺序 | 保序解码 |
| C5 / S12 `--pick` 与参数撞名，D2 理由不成立 | 改写 D2；在兼容性中说明 `pick` 参数改走 `-p` |
| C6 / S10 pick 的错误出口和输出规则 | Render 返回 `(code, err)`；新增退出码 7（D11）；pick × `-o` 组合表；3xx 的处理；发送前校验路径语法 |
| C7 预扫描 | 补全模式规则 |
| C8 只读缓存 | 新增 `IndexOptions.CacheOnly` |
| C9 保留名 | 新增保留名 `skill`（D15）；快捷命令补全改用 `link ls --completion`（D16） |
| C10 / S2 缓存语义 | 只在过期时间已知时写缓存；`auth login` 和 `status` 的说明 |
| C11 / S3 接入点与指纹 | 列出接入点；指纹包含完整 exec 节和 apply |
| S1 argv 中的凭证 | D13；新增 `exec.env` |
| S4 超时与 stdin | 进程组终止、WaitDelay、stdin 为空设备、退出码统一为 1 |
| S5 Windows 批处理 | 元字符检查；措辞改为 Unix 无注入面 |
| S6 配置文件的信任级别 | 安全节写明；`config check` 提示 |
| S7 / S8 / S9 skill | 净化 spec 文本；frontmatter 由编码器生成；覆盖标记移到 frontmatter 之后；名字校验；原子写入 |
| S11 / S13 / S14 / S15 / S16 / S17 / S18 / S19，以及正确性方向的建议 S1–S3 | 均已采纳，见各节 |

## 待确认事项

无阻塞项。以下在计划阶段先核实，再写进任务：

- 补全模式下本地注册全局选项后，与动态参数选项重名时的优先级。按一期规则，参数退回 `-p`，需要实测确认。
- zsh 和 pwsh 下快捷命令补全片段的写法（bash 的写法已明确）。
- goccy/go-yaml 有序映射与数字字面量的写法（用例先行）。

## 结论与人工计划 Gate

本设计确定二期第一批四项的范围、对外接口和安全边界：

- **对外接口**：`--pick`、`-o yaml|table`、退出码 7、`apic skill`、`apic link ls --completion`、`type: exec`。
- **兼容性变化**：`pick` 参数改用 `-p` 传入；`skill` 成为保留名。

第 1 轮两轴评审的阻塞项都已在 0.2 修订，但修订本身未经复核。可以选择以下任一方式批准：

- 点名接受未复核修订并批准 0.2；
- 只批准最后评审过的 0.1；
- 先要求再评审一轮。

批准只授权编写二期第一批实施计划，不授权实施。
