# apic

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/inhere/apic?style=flat-square)
[![GitHub tag (latest SemVer)](https://img.shields.io/github/tag/inhere/apic)](https://github.com/inhere/apic)
[![Unit-Tests](https://github.com/inhere/apic/actions/workflows/go.yml/badge.svg)](https://github.com/inhere/apic)

---

[English](./README.md) | 简体中文

`apic` 是一个在命令行里调用任意 HTTP API 的工具。它加载接口定义（OpenAPI 3.x / Swagger 2.0，或几行简单 YAML），自动生成命令，不必再给每个服务单独写 CLI。base URL、环境和认证也由它管理。

人和 AI agent 都能直接用：stdout 只输出响应体，退出码固定，生产环境的写操作有保护，密钥在所有输出里都会打码。

## 功能

- 从 OpenAPI / Swagger 的 URL 或文件注册站点，也可以用简单的 YAML 定义注册
- 两种调用方式：
  - 按操作名调用：`apic pet list-users --page 2`
  - 按原始路径调用：`apic pet -X POST /api/users -f name=tom`
- 命令名取自 `operationId`；没有 operationId 时由路径生成（`POST /back/cfg/get` → `back-cfg-get`）
- 环境（base URL）与认证分开配置，用 `apic use` 切换
- 认证类型：`none`、`bearer`、`basic`、`apikey`（请求头或 query）、`login`（先登录拿 token，token 按环境缓存，遇到 401 自动重登）
- 密钥放在 `.env`，配置里用 `${env:NAME}` 引用；`-v`、`--dry-run`、错误信息和 `config show` 里都会打码
- `--dry-run` 打印等价的 `curl` 命令，不发请求
- 写保护：
  - 标记了 `protect: true` 的环境，写操作必须加 `--yes`
  - `allow_methods` 可以把某个认证限制为只读
- 快捷命令：`apic link pet --name petctl` 生成 `petctl` 脚本（Windows 上同时生成 `.cmd`）

## 安装

```bash
go install github.com/inhere/apic/cmd/apic@latest
```

或者从源码构建：

```bash
make install   # 构建到 dist/ 并复制到 $GOPATH/bin
```

## 快速开始

```bash
# 用 OpenAPI / Swagger 文档注册站点
apic site add pet --spec https://pet.example.com/swagger/doc.json
# 文档里没有 host 时，自己给 base URL
apic site add pet --spec ./swagger.json --base-url http://127.0.0.1:8080

apic ops pet                 # 列出操作
apic desc pet create-user    # 查看参数、请求体与响应字段

apic pet list-users --page 2                    # 按操作调用
apic pet create-user -f name=tom -f age:=3      # 用字段拼 JSON 请求体
apic pet create-user -d @user.json              # 从文件读请求体（- 表示 stdin）
apic pet -X GET /api/users -p page=2            # 按原始路径调用
apic pet list-users --dry-run                   # 只打印 curl 命令
```

`-f` 的几种写法：
- `-f key=value` 传字符串；
- `-f key:=<json>` 按 JSON 类型传值；
- `-f a.b=v` 拼出嵌套对象。

参数名与全局选项重名时，用 `-p name=value` 或 `-p query.name=value` 传入。

## 配置

配置目录默认是 `~/.config/apic`，可以用 `APIC_CONFIG_DIR` 覆盖：

```
~/.config/apic/
  config.yaml          # 你的配置和站点（apic 不会改写它）
  .env                 # 密钥：KEY=VALUE；已导出的系统环境变量优先
  sites/<name>.yaml    # 一个文件一个站点，由 `apic site add` 写入
  defs/<name>.yaml     # 简单定义
  state.yaml           # 各站点当前的 env/auth、已生成的快捷命令
  cache/               # 缓存的接口定义与登录 token（权限 0600）
```

下面这个站点有两个环境、三个认证：

```yaml
sites:
  pet:
    source: {type: openapi, url: https://pet.example.com/swagger/doc.json, ttl: 24h}
    envs:
      dev:  {base_url: http://127.0.0.1:8080}
      prod: {base_url: https://pet.example.com, protect: true}
    default_env: dev
    auths:
      admin: {type: bearer, token: "${env:PET_ADMIN_TOKEN}"}
      ro:    {type: bearer, token: "${env:PET_RO_TOKEN}", allow_methods: [GET, HEAD]}
      me:
        type: login
        login:
          method: POST
          path: /api/login
          body: {username: "${env:PET_USER}", password: "${env:PET_PASS}"}
        token: {from: body, path: data.token}
        expires: {from: body, path: data.expires_in}
        apply: {header: Authorization, value: "Bearer {token}"}
    default_auth: me
```

```bash
apic use pet --env prod --auth ro        # 切换当前选择
apic pet list-users --env dev --auth me  # 只对这一次请求覆盖
apic auth status pet                     # 查看登录状态（不会输出 token）
```

### 简单定义

接口不多的服务（Jenkins、小工具服务），写一个简短的 YAML 就行，不用写 swagger：

```yaml
# defs/jenkins.yaml
operations:
  job-info:
    get: /job/{name}/api/json        # {name} 自动成为必填参数
    summary: 查看 job 信息
  job-build:
    post: /job/{name}/buildWithParameters
    query: [BRANCH, ENV]             # 可选的字符串 query 参数
  job-log:
    get: /job/{name}/{build}/consoleText
    output: raw
```

```bash
apic site add jenkins --def ~/.config/apic/defs/jenkins.yaml --base-url https://ci.example.com
apic jenkins job-build --name demo --BRANCH main
```

## 快捷命令

```bash
apic link pet --name petctl                 # petctl list-users 等于 apic --api pet list-users
apic link pet --name pet-prod --env prod --auth ro
apic link ls
apic link rm petctl
```

- **生成位置**：脚本默认写到 `defaults.link_dir`，未配置时为 `~/.local/bin`；Windows 上为 `%USERPROFILE%\bin`。
- **Windows**：会额外生成 `.cmd` 版。`cmd.exe` 会把 `%*` 重新解析一遍，所以在 Windows 上传 JSON 请求体请用 `-d @file`。

## 输出与退出码

- **stdout**：只输出响应体，其他信息都写到 stderr，所以 `apic ... | jq` 可以直接用。
- **`-i`**：输出状态行和响应头。
- **`-v`**：输出请求行和请求头，密钥会打码。

| 码 | 含义 |
|---|---|
| 0 | 2xx / 3xx（不跟随重定向） |
| 1 | 本地错误：配置、定义、参数校验 |
| 2 | 用法错误：未知选项、命令或操作 |
| 3 | 网络错误或超时 |
| 4 | HTTP 4xx |
| 5 | HTTP 5xx |
| 6 | 被写保护拒绝 |

## 开发

```bash
make test     # go vet + 单元测试
make lint     # gofmt（装了 staticcheck 时一并检查）
make build    # 生成 dist/apic
```

设计与计划：[设计文档](docs/design/apic-design.md)、[一期计划](docs/plans/2026-11-10-apic-phase1-plan.md)。

## License

[MIT](LICENSE)
