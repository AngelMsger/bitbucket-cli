# bitbucket-cli 技术设计文档

## 1. 目标与范围

`bitbucket-cli` 是一个 Go 编写的命令行工具,让 Coding Agent(Claude Code 等)在
终端里驱动 Bitbucket 的 Pull Request review/合并工作流,并配合本地仓库做"远程
PR 状态 + 本地 codebase 读写"的闭环评审。

- **同时支持** Bitbucket Cloud(REST 2.0)与 Bitbucket Data Center / Server
  (REST 1.0,self-hosted)。
- **面向 Agent**:默认输出 JSON,错误结构化,按文件分级读取 diff
  (`pr files` → `pr diff --path`),错误信息携带可执行的下一步建议。
- **配置多来源**:CLI 参数 / 环境变量 / `.env` / 配置文件,含交互式 `init` 引导。
- **操作范围**:
  - **PR 全生命周期**:list / inbox / get / create / update / diff / commits /
    activity / threads / status / files / approve / unapprove / request-changes /
    decline / merge / fetch / checkout
  - **源码浏览**:任意 ref 下的 list / tree / get(可选行范围裁剪)
  - **仓库 / 分支 / 标签 / 提交**:repo / branch / tag / commit 的列表与查看
  - **评论**:list / add(支持 inline anchor)/ update / delete
  - **发现性命令**:workspace list / user list / user search / tag list 等,
    保证 `--workspace`、`--reviewer`、`--ref` 等每个标识符都有 CLI 内的发现路径
  - **`whoami` / `user me`** 查询当前凭据对应的用户
  - 每个写命令支持 `--dry-run` 预览将发出的请求,删除 / 合并 / 拒绝类需 `--yes`

非目标(本期不做):Pipelines(CI/CD 触发与日志读取)、Webhooks、Deploy keys、
SSH key 管理、OAuth 2.0 三方授权、Draft PR、跨 repo code search、PR tasks
(Cloud todo list)。

## 2. API Flavor 差异矩阵

CLI 用 `Flavor` 区分两类后端:

| Flavor | 说明 | REST 基址 |
|--------|------|-----------|
| `cloud` | Bitbucket Cloud(`*.bitbucket.org`) | `/2.0` |
| `datacenter` | Data Center / Server(self-hosted) | `/rest/api/1.0` |

每个操作在不同 flavor 下的端点 / 分页 / body 参数差异(`{base}` 为站点根 URL):

| 操作 | cloud | datacenter |
|------|-------|------------|
| 列仓库 | `GET /2.0/repositories/{ws}` | `GET /rest/api/1.0/projects/{key}/repos` |
| 列工作区 | `GET /2.0/workspaces` | `GET /rest/api/1.0/projects` |
| 列 PR | `GET /2.0/repositories/{ws}/{repo}/pullrequests?state=&q=` | `GET /rest/api/1.0/projects/{key}/repos/{repo}/pull-requests?state=` |
| 取 PR | `GET .../pullrequests/{id}` | `GET .../pull-requests/{id}` |
| PR diff(整) | `GET .../pullrequests/{id}/diff`(text) | `GET .../pull-requests/{id}/diff`(JSON hunks,`Accept: text/plain` 拿原文) |
| PR diff(per file) | `GET .../pullrequests/{id}/diff?path=` | `GET .../pull-requests/{id}/diff/{path}` |
| PR diffstat | `GET .../pullrequests/{id}/diffstat` | `GET .../pull-requests/{id}/changes` |
| PR 活动流 | `GET .../pullrequests/{id}/activity` | `GET .../pull-requests/{id}/activities` |
| PR 合并预检 | 派生自 `pullrequests/{id}` + `/statuses` | `GET .../pull-requests/{id}/merge` 直接返回 `{canMerge,conflicted,outcome,vetoes}` |
| CI build status | `GET /2.0/repositories/{ws}/{repo}/commit/{hash}/statuses` | `GET /rest/build-status/1.0/commits/{hash}` (注意不在 `/rest/api/1.0` 下) |
| inbox(我相关 PR) | 走 `/2.0/pullrequests/{uuid}`(author)或 workspace 内 fan-out(reviewer) | `GET /rest/api/1.0/dashboard/pull-requests?role=` |
| 列分支 | `GET .../refs/branches` | `GET .../branches` |
| 列标签 | `GET .../refs/tags` | `GET .../tags` |
| 源文件元数据 | `GET .../src/{ref}/{path}?format=meta` | `GET .../files/{path}?at={ref}` |
| 源文件 raw | `GET .../src/{ref}/{path}` | `GET .../raw/{path}?at={ref}` |
| 列用户 | `GET /2.0/workspaces/{ws}/members`(workspace 范围) | `GET /rest/api/1.0/users?filter=` |
| inline 评论 anchor | `inline:{path,from,to}` | `anchor:{path,line,lineType,fileType}` |
| 探活 | `GET /2.0/user` | `GET /rest/api/1.0/application-properties` |

**分页**:Cloud 为游标分页(响应 `next` 是绝对 URL;直接跟随);Data Center 为
offset 分页(`start` / `limit`,`isLastPage` 终止)。`pagination.go` 的
`FetchPage[T]` + `CollectAll` 把两端抽象成同一回调形态。

**flavor 检测**:显式 `--flavor` / 配置 > URL 启发式(host `*.bitbucket.org`
判 cloud)> `auto` 时探 `/2.0/user` vs `/rest/api/1.0/application-properties`。

**端点差异完全收在两处**:`internal/apiclient/dialect.go`(路径 helper:
`repoPath / prPath / branchesPath / commitStatusesPath / srcPath / filesPath ...`)
与 `internal/apiclient/mapping.go`(双 flavor 原始响应 → 统一模型映射)。

## 3. 归一化数据模型

所有 API 方法返回下列与 flavor 无关的模型(`internal/apiclient/models.go`):

```
ServerInfo  { Flavor, BaseURL, Reachable }
Workspace   { Slug, Name, UUID, Type, Description, URL, CreatedAt }
Repository  { UUID, Slug, Name, Workspace, FullName, Description, Private,
              DefaultBranch, Language, Size, URL, CloneHTTPS, CloneSSH, ... }
Branch      { Name, Target, Default, LastCommit, LastUpdated }
Tag         { Name, Target, Date, Message }
Commit      { Hash, Message, Author, Date, Parents, URL }
User        { AccountID, UUID, Name, Slug, DisplayName, Email, Type }
PRRef       { Branch, Commit, Repository }
Participant { User, Role, Approved, State }
PullRequest { ID, Title, Description, State, Author, Source, Destination,
              Reviewers, Participants, Repository, URL, CommentCount,
              MergeCommit, CreatedAt, UpdatedAt, ClosedAt }
InlineAnchor{ Path, Line, From, To }
Comment     { ID, Content, Author, Inline, ParentID, PRID, CommitID,
              URL, CreatedAt, UpdatedAt }
Activity    { Kind, Actor, When, Comment, Approved, State }
Diffstat    { Path, OldPath, Status, LinesAdded, LinesRemoved, Binary }
Thread      { File, Anchor, Comments[] }                  // 按文件聚合的 inline 线程
MergeCheck  { CanMerge, Conflicted, Outcome, Vetoes }
BuildStatus { Key, Name, State, URL, Description, CommitHash, ... }
PRStatus    { PR *PullRequest, MergeCheck, Reviewers, Builds }  // pr status 聚合视图
FileEntry   { Path, Name, Type, Size, Hash, Commit }
FileContent { Path, Ref, Bytes []byte, Size, Encoding, Truncated }
```

JSON 输出字段用 snake_case;时间在 Cloud 为 RFC3339,DC 为 epoch ms(`epochToISO`
做归一);`FileContent.Bytes` 不做 base64 包装,`file get --output -` 直写 stdout。

## 4. 配置与认证

### 4.1 配置结构

```
Config   { BaseURL, Flavor, Auth, Defaults, DetectedFlavor }
AuthConfig { Scheme: pat | basic, Username }
Defaults { Format, PageSize, Timeout, MaxRetries, Workspace }
                                              # ↑ 默认 workspace,可被 --workspace 覆盖
```

### 4.2 来源与优先级

高 → 低:CLI 参数 > 环境变量(`BITBUCKET_*`)> `.env` 文件 >
`~/.bitbucket/config.yaml` > 内置默认值。每层为稀疏 `Config`,非零字段覆盖低层。
每个字段记录来源,供 `config show --explain`。

环境变量映射:

| 变量 | 字段 |
|------|------|
| `BITBUCKET_SERVER` | `BaseURL` |
| `BITBUCKET_FLAVOR` | `Flavor` |
| `BITBUCKET_TOKEN` / `BITBUCKET_PERSONAL_ACCESS_TOKEN` | PAT 密钥(scheme=pat) |
| `BITBUCKET_USERNAME` | `Auth.Username` |
| `BITBUCKET_PASSWORD` | basic 密钥(DC) |
| `BITBUCKET_API_TOKEN` | basic 密钥(Cloud,与邮箱组合) |
| `BITBUCKET_DEFAULT_WORKSPACE` | `Defaults.Workspace` |
| `BITBUCKET_FORMAT` | `Defaults.Format` |
| `BITBUCKET_CONTEXT` | 当前 context 名 |

### 4.3 认证

- **pat**:`Authorization: Bearer <token>`。
  - Cloud:Workspace / Repository / Project Access Token
  - DC:HTTP Access Token
- **basic**:`Authorization: Basic base64(user:secret)`。
  - Cloud:邮箱 + API Token(id.atlassian.com 发的)或 App Password(向下兼容)
  - DC:用户名 + 密码

密钥永不写入 `config.yaml`。`config init` 写入时存入 keychain(`go-keyring`,
service `bitbucket-cli`,account `<host>:<scheme>`),失败回退
`~/.bitbucket/credentials`(文件 0600,目录 0700)。

### 4.4 init 向导

输入 base URL → 探测并确认 flavor → 选认证方式(Cloud 默认 basic,DC 默认 pat)
→ 输入凭据 → `Ping` 实时校验 → 选密钥存储方式 → 写非密字段到 `config.yaml`、
密钥入 keychain / 文件 → 打印下一步建议命令。

## 5. 命令规格

全局持久 flag:`--base-url`、`--flavor`、`--format`(json|table|ndjson)、
`--fields`、`--timeout`、`--config`、`--use-context`、`--verbose`、`--pretty`。

命令树按资源分组:`repo` `workspace` `pr` `file` `comment` `branch` `tag`
`commit` `user` `config` `auth` `doctor` `whoami` `skill` `version`。共同约定:

- **ID 解析**:`pkg/urlref` 接受 PR / 仓库 URL,自动拆出 workspace / slug /
  PR-id / commit;命令也接受 `<ws>/<repo>` 和 `<ws>/<repo>/<id>` 简写。
- **写操作**:所有创建 / 更新 / 删除 / 合并 / 拒绝 / 评论 / 分支增删都是写命令;
  每个写命令支持 `--dry-run` 预览将发出的请求,删除 / 合并 / 拒绝类需 `--yes`。
- **分页**:list 命令接受 `--limit/--all/--cursor`,输出 `{items, next, has_more}`
  信封。Cloud 的 `next` 是绝对 URL,直接续读。
- **发现性闭环**(见 [AGENTS.md](../AGENTS.md) "Discoverability — no
  dead-end inputs"):每个 `--workspace` / `--reviewer` / `--author` / `--ref`
  类标识符都有 CLI 内的发现路径,且 `--workspace required` 类错误的
  `next_steps[0]` 必须是发现命令。

完整的命令、flag 与示例由命令树自动生成,见 [docs/cli/](cli/)(`make docs`
生成,CI 校验不漂移)—— 本节不再维护并行的命令清单。

## 6. PR 评审闭环(v0.2 核心增量)

设计目标:让 Coding Agent 用最小远程往返和最少上下文 token,在远端 PR 视图和
本地代码 checkout 之间无缝切换。

### 6.1 决策树(由 `skills/bitbucket/references/reviewing-locally.md` 强制)

```
PR URL / <ws>/<repo>/<id>
    ↓
pr inbox / pr list      ← 没有具体 PR 时,先找
    ↓
pr status               ← mergeable? conflicts? CI 绿? reviewers?
    ↓ 若可评
pr files                ← per-file diffstat,按 churn 倒序
    ↓
├─ 小 PR:  pr diff --path <path>       (单文件,一次请求)
└─ 大 PR:  pr fetch --exec + pr checkout --exec
            ↓
            本地 Read / Grep PR 范围内的文件
    ↓
pr threads              ← 已有 inline 线程,按文件聚合
    ↓
comment add --inline | --reply-to
    ↓
pr approve | pr request-changes | pr decline | pr merge
```

### 6.2 `pr status` 的并行聚合

`internal/apiclient/merge_check.go::GetPRStatus` 用 `sync.WaitGroup` 并行触发:

1. `GetPR` 拿 PR 详情(含 reviewers / state)
2. `CheckPRMerge` —— DC 直走 `/merge` 端点;Cloud 派生自 PR.state
3. `ListPRStatuses` —— Cloud 走 `/pullrequests/{id}/statuses`;DC 经
   `pr.toRef.commit` 查 `/rest/build-status/1.0/commits/{hash}`

任一子请求失败时**降级返回**(对应字段留空),不中断整体响应。返回的
`PRStatus { PR, MergeCheck, Reviewers, Builds }` 是一次调用即得的"PR 评审就绪
仪表盘",避免 Agent 三次串行 `pr get` + `/statuses` + `/merge`。

### 6.3 `pr fetch` / `pr checkout` 的双模式

默认 **print-only**:返回 JSON
```json
{ "commands": ["git fetch origin refs/pull-requests/42/from:refs/remotes/origin/pr/42"],
  "executed": false,
  "hint": "re-run with --exec to actually run these (must be inside a git checkout)." }
```

`--exec` 时:先 `git rev-parse --is-inside-work-tree` 检测 cwd 是否在 git 工作
树内,否则 `usage` 类错误 + 清晰提示;通过后用 `exec.Command("git", ...)` 顺序
执行(stderr 透传到本进程 stderr)。Bitbucket Cloud 和 DC 的 PR refspec
都是 `refs/pull-requests/<id>/from`,所以**无需 flavor 分支**。

### 6.4 `pr threads` 的客户端重排

不发新请求。`ListPRComments` 走分页拉完后,在 Go 里按 `Inline.Path` + 顶层
ParentID 重新分组成 `Thread{File, Anchor, Comments[]}`,inline 在前(按文件路径
排序),通用讨论在最后 `Thread{File: ""}` 一桶兜底。

## 7. 输出与错误模型

### 7.1 输出

`Formatter` 接口三实现:`json`(默认,面向 Agent,stdout)、`table`(人类可读)、
`ndjson`(流式大结果集)。`--fields a,b.c` 按点路径投影。list 命令输出分页信封
`{items, next, has_more}`,`--cursor` 可从上一页的 `next` 续读。

成功输出统一为 stdout 上的 JSON,例外:

- `version` 打印纯文本版本行
- `pr diff` / `pr diff --path` 打印 unified diff 文本
- `file get --output -` 写 raw 字节到 stdout
- `file get --output <path>` 写 raw 字节到磁盘
- `pr fetch` / `pr checkout` 在 `--exec` 模式下把 git 子进程的 stdout/stderr
  透传到本进程的 stderr
- `skill show` 把内嵌的 `SKILL.md` 原样打印

交互式向导(`config init`、`auth login`)的提示走 stderr;错误只走 stderr。

### 7.2 错误

错误以 JSON 写 **stderr**:

```json
{"error":{"category":"auth","code":"HTTP_Unauthorized",
  "message":"Bitbucket returned HTTP 401: ...",
  "hint":"...","next_steps":["bitbucket-cli auth login","bitbucket-cli doctor"],
  "retryable":false,"http_status":401}}
```

category:`usage config auth not_found permission conflict rate_limit network
server parse internal`。Cloud 与 DC 的错误体形态在 `extractAPIMessage` 内统一
抽取(Cloud `{"error":{"message":"..."}}`,DC `{"errors":[{"message":"..."}]}`)。

### 7.3 退出码

| 码 | category | 码 | category |
|----|----------|----|----------|
| 0 | success | 6 | not_found |
| 1 | internal | 7 | rate_limit |
| 2 | usage | 8 | network |
| 3 | config | 9 | server |
| 4 | auth | 10 | parse |
| 5 | permission | 11 | conflict |

`hints.go` 把 category 映射为 next_steps,引导 Agent 自我纠正。
**发现性约定**:任何 `--missing-X` 类报错的 `next_steps[0]` 必须是发现命令
(见 [AGENTS.md](../AGENTS.md) Discoverability section)。

## 8. Skill 大纲

`skills/bitbucket/SKILL.md`(YAML frontmatter:`name: bitbucket`、触发词、
`metadata.requires.bins: ["bitbucket-cli"]`)+ `references/`:

- `getting-started.md` — 配置 / 认证 / `doctor` / `workspace list` 发现
- `pr-workflows.md` — `pr status` → `pr files` → `pr diff --path` 决策
- `reviewing-locally.md` — 端到端"远程 + 本地"评审流程,含 `pr fetch --exec`
- `commenting.md` — inline vs general / `--reply-to`
- `reading-repos.md` — repo / branch / commit 浏览
- `files.md` — file 子树的 ref 语义和 `--range` 用法
- `errors-and-exit-codes.md` — 退出码表 + 按 category 的恢复动作

Skill 与 CLI 同步发版:每次改 Skill 或 references 都要在 SKILL.md 的
`version:` 行上 bump,`make e2e` 中的 `skill show` 断言会校验文件可读。

`skill install` 用一张 agent 路径表(`internal/app/skill.go` 的 `agentSpecs`)
描述各 Agent 的全局 / 项目 skills 目录:Claude Code 用 `~/.claude/skills`、
`./.claude/skills`;Codex 用 `~/.codex/skills`、`./.agents/skills`。无 flag
时探测目录是否存在,装入 / 移除每个命中的 Agent;`--agent` 显式指定,`--dir`
为 agent 无关的显式路径。

## 9. 测试策略

- **单元测试**:标准库 `testing`,表驱动,`t.Parallel()`。覆盖 config 优先级、
  auth 解析与文件权限、分页 offset/cursor、mapping 两 flavor 归一、output 各格式
  与 `--fields`、errors 映射、urlref。
- **HTTP 层测试**:`httptest.Server` 驱动各 Client 方法,断言路径 / 参数 / 认证头。
- **端到端**:`scripts/e2e.sh` 构建二进制 + 内置 mock Bitbucket(DC REST 1.0
  覆盖 + Cloud-shaped 路由),跑全部命令断言 stdout 输出契约与退出码,**含
  发现性断言**(如 `repo list` 缺 workspace 时 stderr 必须含 `workspace list`)。
  目前覆盖 55+ 用例。
- **只读 live 验证**:`BITBUCKET_E2E_LIVE=1 ./scripts/e2e.sh` 仅跑 `doctor` 和
  `whoami` 之类只读命令。

## 10. v0.2 关键设计点回顾

1. **`FileContent.Bytes` 不 base64** — Cloud / DC 的 raw 端点都返回原始字节,
   `file get --output -` 直写 stdout,`--range L1:L2` 在客户端用
   `bytes.Split(b, '\n')[L1-1:L2]` 完成裁剪。
2. **`pr files` 是 diffstat,不是 patch** — 默认按 `LinesAdded+LinesRemoved`
   倒序,Agent 拿到后自己决定哪些文件值得 `pr diff --path`。
3. **`pr status` 并行调度** — 用 `sync.WaitGroup` + mutex 而非引入
   `golang.org/x/sync/errgroup`,保持零外部依赖。
4. **`pr fetch` print-only 默认 + `--exec`** — Agent 一般用 print-only 把
   git 命令拼接到自己的 Bash 子调里;人用 `--exec`。
5. **`pr threads` 不发新请求** — 复用 `ListPRComments` 的全量结果在内存里
   重排,避免额外 round-trip。
6. **Cloud `pr inbox --role reviewer` 的 fan-out** — Cloud 无全局 reviewer
   索引;必须配 `--workspace`,在该 workspace 内遍历 repo + 服务端
   `q=reviewers.uuid="..."` 过滤,per-repo 错误吞掉不阻塞整体响应。
