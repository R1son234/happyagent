# Career Copilot 可复习资料库生成 Spec

## 目标

让 Career Copilot 在用户输入简历、JD、公开面经、项目资料或真实面试记录后，生成一套可以直接复习和长期维护的 Markdown 资料库，而不是只保存抽取文本、源文件副本和程序元数据。

目标输出要接近 `/Users/r1son/Desktop/projects/resumes` 中的组织方式，但复用的是“资料库组织能力”，不是复用其中任何固定行业、岗位或目录名。`ai-agent`、后端、市场营销、公司名都只能作为样例或由用户输入推断出的实例，不能写死成默认规则。

- 顶层首页：`面试资料库首页.md`
- JD 总览：`jd/JD 汇总.md`
- 公开面经总览：`experiences/面经总览.md`
- 方向资料包：如 `experiences/<方向slug>/<方向名> 面经资料包.md`
- 主题题库：如 `experiences/<方向slug>/<主题名>题库.md`
- 项目专项总览：`prepare/项目专项总览.md`
- 项目 QA：如 `prepare/<项目slug>-interview-qa.md`
- 单岗位作战页：如 `my-interviews/<公司岗位>/<公司岗位>岗作战页.md`
- 结构化 JD 画像、临阵主文档、复习计划、面经来源与 JD 关联补充

## 非目标

- 不把公开面经伪装成用户真实面试记录。
- 不把 `metadata.json`、`extracted.md` 这类程序中间产物作为主要用户入口。
- 不为了展示效果编造项目指标、公司经历、面试结果或生产事实。
- 不要求用户手动理解时间戳目录、哈希、抽取器和内部索引。
- 不把任何行业、岗位、方向或公司写死成默认目录；方向目录和题库主题必须来自用户输入、JD、简历、面经内容或可配置 taxonomy。

## 现状依据

当前工作区示例：

```text
career-workspace/experiences/experience-20260515-165353-ai-agent-2026-04-30/
  extracted.md
  metadata.json
  source.md
```

问题：

- 目录名以时间戳和低信息 slug 为主，复习时不可读。
- `extracted.md` 与 `source.md` 高度重合，用户不知道该看哪个。
- `metadata.json` 对用户无直接价值，却和主内容同级暴露。
- `ArchivePublicInterviewExperience` 把公开面经同步到 `my-interviews/市场营销/面经来源与复习清单.md`，岗位方向硬编码错误。
- `RenderWorkspaceArtifact` 只生成通用英文结构报告，没有生成面经题库、资料包、导航、作战页。
- `outputs/latest-report.md` 更像一次性分析结果，不是可复习知识库。

对照资料库示例：

```text
/Users/r1son/Desktop/projects/resumes/面试资料库首页.md
/Users/r1son/Desktop/projects/resumes/experiences/面经总览.md
/Users/r1son/Desktop/projects/resumes/experiences/ai-agent/AI Agent 面经资料包.md
/Users/r1son/Desktop/projects/resumes/experiences/ai-agent/Agent LLM 工程化高频题库.md
/Users/r1son/Desktop/projects/resumes/prepare/项目专项总览.md
/Users/r1son/Desktop/projects/resumes/my-interviews/阿里云视频生成Agent/01-临阵抗拷打主文档.md
```

这些文件的共同特征才是要复用的产品能力，具体方向名只是样例：

- 文件名表达用途和主题。
- 每个文件承担明确职责。
- 总览页负责导航，不塞满原始内容。
- 题库按主题拆分，问题和答案可直接背诵或复盘。
- 单岗位目录围绕 JD、个人项目、公开面经和复习计划组织。
- 原始来源保留，但不压过可复习内容。

## 目标工作区结构

```text
career-workspace/
  inbox/
  面试资料库首页.md
  resume/
    current.md
    versions/
  jd/
    JD 汇总.md
    <岗位或公司>-JD.md
  experiences/
    面经总览.md
    <方向slug>/
      <方向名> 面经资料包.md
      <主题名>题库.md
      <主题名>题库.md
      <来源与公司观察>.md
      sources/
  prepare/
    项目专项总览.md
    <project>-interview-qa.md
  my-interviews/
    <公司岗位>/
      00-JD结构化画像.md
      01-临阵抗拷打主文档.md
      02-复习计划.md
      03-面经来源与JD关联补充.md
      <公司岗位>岗作战页.md
  record/
    imports/
    generated/
    unclassified/
  workspace.json
  index.json
  workspace.guide.json
```

`inbox/` 仍可作为用户投放入口。用户主入口是 `面试资料库首页.md` 和各类总览页，不是 `index.json`、`metadata.json` 或时间戳目录。

## 输出分层

### 1. 源资料层

保存原始文件和抽取文本，用于追溯和证据边界：

- `record/imports/` 保存导入记录。
- `experiences/<domain>/sources/` 保存公开面经原文或来源摘要。
- `resume/versions/` 保存简历版本和原始文件。
- `jd/<岗位或公司>-JD.md` 保存清理后的 JD 正文。

源资料层可以包含抽取器、MIME、原路径等程序信息，但这些信息不作为用户复习入口展示。

### 2. 结构化分析层

把源资料整理成可理解的中间文档：

- `jd/JD 汇总.md`
- `my-interviews/<公司岗位>/00-JD结构化画像.md`
- `experiences/<domain>/<方向名> 面经链接与公司观察.md`
- `my-interviews/<公司岗位>/03-面经来源与JD关联补充.md`

这一层回答：岗位要什么、公开面经问什么、候选人现有材料覆盖什么、缺口在哪里。

### 3. 可复习层

直接服务复习：

- `experiences/<domain>/*题库.md`
- `prepare/*-interview-qa.md`
- `my-interviews/<公司岗位>/01-临阵抗拷打主文档.md`
- `my-interviews/<公司岗位>/02-复习计划.md`
- `my-interviews/<公司岗位>/<公司岗位>岗作战页.md`

这一层必须使用问题、答案、追问、答题要点、项目映射和复习优先级组织内容。

### 4. 导航层

让用户知道从哪里开始：

- `面试资料库首页.md`
- `experiences/面经总览.md`
- `experiences/<domain>/<方向名> 面经资料包.md`
- `prepare/项目专项总览.md`

导航层必须只放地图、职责、入口和新增问题落点，不堆长答案。

## 生成规则

### 语义文件名

新增用户可读 Markdown 时，文件名必须由主题、用途和资料类型组成：

| 输入 | 推荐文件名 |
| --- | --- |
| 某方向公开面经 | `experiences/<方向slug>/<方向名> 面经资料包.md` |
| 某主题高频题 | `experiences/<方向slug>/<主题名>题库.md` |
| JD | `jd/<公司岗位>-JD.md` |
| 单岗位准备 | `my-interviews/<公司岗位>/<公司岗位>岗作战页.md` |
| 项目准备 | `prepare/<project>-interview-qa.md` |
| 真实面试复盘 | `my-interviews/<公司岗位>/一面面经记录-YYYY-MM-DD.md` |

时间戳目录只允许用于源资料版本和 record 过程记录，不作为用户主入口。

### Frontmatter

用户可读 Markdown 统一写 frontmatter：

```yaml
---
title: <主题名>题库
tags:
  - interview/experience
  - <方向slug>
status: active
updated: 2026-05-15
---
```

### QA 格式

题库文件使用稳定格式：

```markdown
## Q1：面试官问题

### 答题要点

- ...

### 可直接说的答案

...

### 追问与补充

- ...

### 项目映射

- happyagent：...
```

要求：

- 问题必须像面试官会问的话。
- 答案必须可口述，不写成泛泛百科。
- 项目映射必须区分已证实事实和建议补充。
- 不能为了显得强而添加未证实数字、生产规模、业务结果。

### 方向与主题拆分

方向目录和题库主题必须动态生成，不能在代码里固定为 AI Agent、后端或任何单一职业方向。

方向识别来源按优先级：

1. 用户显式指定的目标方向、岗位或公司岗位名。
2. 当前 active JD 的岗位名称、职责关键词、技能关键词。
3. 简历和项目资料中的主项目、技术栈、行业场景。
4. 面经标题、来源文件名和正文中的公司、岗位、题型信号。
5. 低置信度时使用 `general/`，并在导入记录里标注需要用户确认。

主题拆分规则：

- 每个方向下的主题由输入内容聚类生成，例如“系统设计”“数据库”“项目深挖”“行为面试”“算法题”“产品案例”“销售策略”“设计作品集”等。
- 如果已有 `workspace.guide.json` 或用户配置了 taxonomy，优先复用配置主题。
- 如果同一方向下已有题库，新增内容优先合并到最相近题库，避免每次导入都生成一堆碎文件。
- 主题不足以判断时，写入 `record/unclassified/` 并在导入记录中说明需要用户确认，不把问题硬塞到错误题库。

AI Agent、后端、市场营销等只允许出现在测试样例、demo 数据或由用户输入推断出的具体实例里，不能作为默认分支写进业务逻辑。

## 代码影响范围

### `internal/career/workspace.go`

- 保留 `inbox/` 作为入口。
- 初始化时创建首页和总览页的空骨架，避免用户进入空目录。
- 增加用户可读 Markdown 的写入路径工具，区分 `user_material` 和 `internal_record`。

### `internal/career/workspace_item.go`

- `AddMaterialFromFile` 保存源资料后，不再只返回 `extracted.md`。
- 写入或更新对应的用户可读文档。
- `WorkspaceItem.Path` 对用户资料类型优先指向可读 Markdown；metadata 中保留 extracted/source 路径。

### `internal/career/workspace_archive.go`

- 移除 `市场营销` 硬编码。
- `ArchivePublicInterviewExperience` 改为：
  - 保存公开面经源文到 `experiences/<domain>/sources/`。
  - 更新 `experiences/面经总览.md`。
  - 更新 `experiences/<domain>/<资料包>.md`。
  - 按主题更新题库。
  - 如输入中有具体公司岗位，更新 `my-interviews/<公司岗位>/03-面经来源与JD关联补充.md`。
  - 写入 `record/imports/`，记录源文、生成文件和需要确认的点。

### `internal/career/artifacts.go`

- 新增 `review-library` 或 `interview-library` export kind，用于生成完整资料库闭环。
- 现有 `jd-match`、`resume-review`、`project-pitch`、`interview-review` 保留，但输出位置改成对应业务目录或 `outputs/` 的 latest 报告，不替代资料库。

### `internal/career/prompts.go`

- 增加资料库生成 prompt：
  - 明确读取已归档的 JD、简历、项目资料和面经源文。
  - 要求生成首页、总览、题库、作战页、复习计划。
  - 强制区分事实、推断和待确认。
  - 要求每份文档职责单一、文件名可读。

### `internal/career/command_handler.go`

- 增加命令：
  - `/library`：根据当前工作区生成或刷新复习资料库。
  - `/export review-library`：等价生成完整资料库。
- 自动归档文件后，如果用户明确要求分析或准备，触发资料库生成，而不只生成 `latest-report.md`。

### `README.md`

- 更新 Career Copilot 使用说明：
  - `inbox/` 是投放入口。
  - `面试资料库首页.md` 是复习入口。
  - `record/` 是过程记录。
  - `metadata.json` 和 `extracted.md` 是追溯资料，不是主要阅读对象。

## 验证方式

- 单元测试：
  - 新工作区初始化后存在 `面试资料库首页.md`、`experiences/面经总览.md`、`prepare/项目专项总览.md`。
  - 导入带有明确方向的公开面经后，生成 `experiences/<方向slug>/<方向名> 面经资料包.md` 和至少一个主题题库。
  - 不同方向输入会生成不同方向目录，例如产品、后端、AI Agent、运营等都走同一套动态规则。
  - 公开面经不会写成真实面试记录。
  - `市场营销`、`ai-agent`、`backend`、任何公司名或岗位名都不作为硬编码默认目录出现。
  - `WorkspaceItem.Path` 对用户可读材料指向语义 Markdown。
- 集成测试：
  - 用至少两组不同方向夹具跑导入和资料库生成，确认目录和题库来自输入语义，而不是固定规则。
  - 检查生成文件名、导航链接、QA 格式、JD 画像、复习计划和作战页。
- 回归测试：
  - `go test ./internal/career ./cmd/happyagent`
  - 必要时补 `go test ./...`。

## 验收标准

- 用户打开 `career-workspace/面试资料库首页.md`，能直接进入 JD、面经、项目和具体岗位准备材料。
- 任意职业方向都能生成可读资料库；未识别方向进入 `general/` 或待确认流程，而不是落入某个写死方向。
- `experiences/` 下不再只看到时间戳目录和 `extracted.md`。
- 同一公开面经可以追溯源文，但复习入口是资料包和题库。
- 题库内容按主题拆分，文件之间职责清楚，不能只是同一段文字复制到不同文件。
- 单岗位目录能回答：JD 要什么、我怎么匹配、公开面经问什么、临阵先背什么、哪些点需要补证据。
- 程序元数据仍保留给系统使用，但不干扰用户复习路径。

## 风险与处理

- 风险：自动主题分类错误。处理：低置信度内容进入 `record/unclassified/`，导入记录写明原因，用户可再次指定方向生成。
- 风险：LLM 生成题库时编造事实。处理：prompt 和测试要求区分事实、推断、待确认；项目映射只引用已读材料。
- 风险：一次生成过多文件导致质量参差。处理：按导航、JD、面经题库、项目 QA、岗位作战页的固定合同生成，并用测试验证关键文件存在和格式。
- 风险：旧工作区已经有时间戳目录。处理：保留源目录作为证据层，新增语义 Markdown 作为用户入口，不删除用户已有资料。

---

# Log 复盘后的修正 Spec

## 背景

2026-05-15 的最新执行日志 `logs/20260515-193437.944/log.md` 证明第一轮实现只改善了“有入口文件和骨架文件”，没有真正达到“用户可以拿来复习”的目标。

实际生成结果包括：

```text
career-workspace/experiences/domain-69140cef/
  市场营销 / 新媒体运营实习生 面经资料包.md
  项目深挖题库.md
  行为面试题库.md
  业务与策略题库.md
  市场营销 / 新媒体运营实习生 面经链接与公司观察.md
```

主要问题：

- `项目深挖题库.md` 只是模板话术，没有把面经问题、JD 要求、简历经历融合成可复习 QA。
- 模型尝试用 `file_write` 写入更完整文件，但工具未获批准，出现 `approval required for dangerous tool "file_write"`，最终只能把分析写进 final answer，再由 app 层保存到 `outputs/latest-report.md`。
- `workspace.guide.json` 和 prompt summary 仍注入 `Use "市场营销" as the neutral demo topic...`，污染通用产品目标。
- `index.json` 的 `WorkspaceItem.Path` 仍指向 `extracted.md`，用户入口和系统索引入口不一致。
- 方向 slug 生成成 `domain-69140cef`，不可读，不适合长期资料库维护。
- 首页、总览、资料包和题库没有形成“JD -> 简历证据 -> 面经问题 -> 项目回答 -> 复习计划”的闭环。

## 修正目标

把资料库生成从“确定性骨架”升级为“应用层可写入的深度资料生成”：

- app 层负责写入用户可读资料库文件，不依赖模型直接调用 `file_write`。
- 模型负责产出结构化内容，app 层解析、校验并写入业务目录。
- 每次用户说“记录并分析”“生成面试准备材料”“帮我复习”时，除了 `outputs/latest-report.md`，还要同步生成或刷新业务资料库文件。
- `WorkspaceItem.Path` 对用户资料优先指向语义 Markdown，metadata 继续保留 `extracted/source/original`。
- 去掉 demo topic 对运行 prompt 的污染。
- 方向 slug 和岗位目录必须可读、稳定、可维护。

## 非目标

- 不要求第一步接入联网检索。
- 不要求一次性支持所有 Obsidian 高级语法。
- 不删除旧 `extracted.md`、`source.*`、`metadata.json`，它们仍是证据层。
- 不允许模型为了填充资料库编造未出现在简历、JD、面经里的事实、指标或经历。

## 当前根因

### 1. 写入责任放错层

当前 prompt 允许模型调用 `file_write`，但运行时未批准该工具。结果是：

```text
tool error: approval required for dangerous tool "file_write"; approved tools:
```

因此，模型生成的深度分析没有落到 `experiences/`、`prepare/`、`my-interviews/`，只进入 final answer 和 `outputs/latest-report.md`。

修正：深度资料库写入必须由 Career Copilot app 层完成，不能依赖模型工具权限。

### 2. 资料库生成器没有读全上下文

第一轮确定性 `library.go` 只基于单个公开面经内容生成题库。它没有同时读取：

- active JD
- current resume
- prepare/project material
- public interview experience
- latest model analysis

因此无法生成岗位作战页、项目映射和临阵主文档。

### 3. 索引仍然面向系统中间产物

`WorkspaceItem.Path` 仍是：

```text
experiences/experience-.../extracted.md
```

这会让模型和用户继续围绕抽取文本工作，而不是围绕资料包、题库和作战页工作。

### 4. demo topic 被注入运行上下文

`WorkspaceGuide.PromptSummary()` 会在运行 prompt 里输出：

```text
Use "市场营销" as the neutral demo topic in generated examples, specs, templates, and tests.
```

这条规则只适合测试和 demo，不适合真实用户运行时。

### 5. slug 策略不可读

中文方向名经过现有 slug 逻辑后可能退化成 hash，例如：

```text
domain-69140cef
```

这不符合“可复习资料库”的长期维护要求。

## 目标行为

用户把简历、JD、公开面经放进 `inbox/` 并输入：

```text
我把简历、JD、面经放到 inbox 里了，你帮我记录并分析下
```

系统应完成：

1. 自动归档源资料。
2. 读取已归档的简历、JD、面经。
3. 生成 `outputs/latest-report.md` 作为本次运行报告。
4. 同步写入或刷新以下用户可读资料库文件：

```text
面试资料库首页.md
jd/JD 汇总.md
jd/<岗位名>-JD.md
experiences/面经总览.md
experiences/<方向slug>/<方向名> 面经资料包.md
experiences/<方向slug>/<具体主题>题库.md
experiences/<方向slug>/<方向名> 面经链接与公司观察.md
prepare/项目专项总览.md
prepare/<项目slug>-interview-qa.md
my-interviews/<公司岗位>/00-JD结构化画像.md
my-interviews/<公司岗位>/01-临阵抗拷打主文档.md
my-interviews/<公司岗位>/02-复习计划.md
my-interviews/<公司岗位>/03-面经来源与JD关联补充.md
my-interviews/<公司岗位>/<公司岗位>岗作战页.md
```

如果资料不足，例如没有项目材料，则对应文档必须明确写：

- 已证实材料是什么。
- 缺什么证据。
- 用户需要补充什么。
- 不能把待确认内容写成事实。

## 内容质量合同

### 题库文件

题库不能只按“项目深挖 / 行为面试 / 业务策略”套模板，必须从面经里抽出具体问题。

格式：

```markdown
## Q1：给你 5 分钟看一个账号，你会如何判断问题并提出优化建议？

### 面试官想考

- 是否理解账号定位、内容质量、更新节奏、数据指标和转化链路。

### 可直接说的答案

...

### 结合简历的项目映射

- 抖音个人 IP 账号运营：已证实有账号定位、选题、标题、封面、周更、粉丝 8.3W。
- Wonderlab：已证实有多平台投放、KOL 合作、数据复盘。

### 可能追问

- 你当时看哪些数据？
- 如果播放高但转粉低，你怎么判断问题？

### 证据边界

- 已证实：...
- 待确认：...
```

### 单岗位作战页

必须回答：

- 这个岗位要什么。
- 简历中最强匹配证据是什么。
- 面经中最可能问什么。
- 候选人最该先背哪 3 个答案。
- 哪些材料需要补证据。
- 面试前 30 分钟看哪几页。

### 项目 QA

必须围绕简历真实经历生成，而不是只写“待确认”。

示例主题：

- Wonderlab KOL 投放复盘
- 抖音个人 IP 账号增长
- 活动策划负责人经历

每个项目 QA 至少包含：

- 项目一句话。
- STAR 回答。
- 数据与证据。
- 可能追问。
- 风险点和不能夸大的地方。

### JD 画像

必须拆出：

- 岗位目标。
- 核心职责。
- 硬性要求。
- 加分项。
- 关键词。
- 简历匹配证据。
- 缺口和补证据建议。

## 实现方案

### 1. 新增结构化资料库生成合同

新增类型：

```go
type ReviewLibraryDocumentSet struct {
  Home string
  JDIndex string
  ExperienceIndex string
  PrepareIndex string
  RolePackage RolePackage
  ExperiencePackage ExperiencePackage
  QuestionBanks []QuestionBank
  ProjectQAs []ProjectQA
}
```

或等价结构。重点是模型输出或 app 层生成结果必须是结构化对象，而不是一段 final answer。

### 2. 新增 `BuildReviewLibraryPrompt`

输入：

- current resume path/content
- active JD path/content
- public experience path/content
- prepare/project material
- target role label
- output file contract

输出要求：

- 模型返回 JSON，不直接写文件。
- JSON 字段对应每个目标文件的 title、path、content、evidence_paths。
- content 必须是完整 Markdown。
- 必须声明 evidence boundary。

### 3. App 层解析并写入业务目录

新增流程：

```text
model final JSON -> ParseReviewLibraryDocumentSet -> Validate -> WriteReviewLibraryDocuments
```

写入使用 `Workspace.writeWorkspaceText`，不走模型 `file_write` 工具，因此不受工具审批影响。

如果 JSON 解析失败，使用类似 `BuildReportRepairPrompt` 的 repair prompt 修复。

### 4. 自然语言触发资料库生成

在 `handleNaturalLanguageInput` 或分析完成后的保存逻辑中，如果满足任一条件：

- 用户说“记录并分析”
- 用户说“面试准备”
- 用户说“复习”
- 用户说“整理资料”
- 自动归档中包含 resume + jd + experiences

则在保存 `latest-report.md` 后同步调用 review-library 生成。

### 5. 去掉运行 prompt 的 demo topic

修改：

- `WorkspaceGuide.PromptSummary()` 不再输出 demo topic。
- `WorkspaceGuide.Validate()` 不再强制 `demo_topic` 必填。
- demo topic 仅保留在测试夹具或示例文档，不进入真实运行 prompt。

### 6. 可读 slug 策略

新增 `ReadableSlug(label string) string`：

- 英文和数字保留。
- 已知中文岗位/方向可用轻量词典转英文：
  - 市场营销 -> marketing
  - 新媒体运营 -> new-media-operations
  - 产品经理 -> product-manager
  - 后端 -> backend
  - AI Agent -> ai-agent
- 未知中文不直接 hash 为主路径，优先用 `role-<short-fingerprint>` 并在目录内标题保留中文。
- 文件名允许中文，目录 slug 要稳定可读。

本次样例应生成类似：

```text
experiences/marketing-new-media-operations/
my-interviews/市场营销新媒体运营实习生/
```

而不是：

```text
experiences/domain-69140cef/
```

### 7. 更新 WorkspaceItem.Path 语义

对用户资料类 item：

- `WorkspaceItem.Path` 指向用户可读主文档。
- `WorkspaceItem.Metadata.Source` 指向 `extracted.md`。
- `WorkspaceItem.Metadata.Original` 指向原文件副本。

例如公开面经：

```json
{
  "type": "experiences",
  "path": "experiences/marketing-new-media-operations/市场营销新媒体运营实习生 面经资料包.md",
  "metadata": {
    "source": "experiences/experience-.../extracted.md",
    "original": "experiences/experience-.../source.md"
  }
}
```

如果担心兼容性，可以新增 `DisplayPath` 字段；但用户入口和 prompt 中的 `stored_path` 必须优先使用可读文档。

### 8. Prompt 指向资料库入口

`BuildInteractivePromptWithAutoSavedAndGuide` 中的 auto saved assets 应包含：

- `stored_path`: 用户可读主文档。
- `source_path`: extracted/source 证据路径。
- `material_role`: resume/jd/experiences/prepare/my-interviews。

模型应该优先阅读用户可读主文档；需要证据细节时再读 source。

## 影响文件

- `internal/career/guide.go`
  - 移除运行 prompt 中 demo topic 输出。
  - 放宽 `demo_topic` 校验。
- `internal/career/prompts.go`
  - 新增 review library JSON 生成 prompt。
  - 修改 interactive prompt 中 stored path 语义。
- `internal/career/report.go` 或新增 `internal/career/review_library_report.go`
  - 定义结构化资料库输出 schema 和解析/repair。
- `internal/career/library.go`
  - 从骨架生成升级为结构化资料写入器。
  - 合并 JD、简历、面经、项目资料。
- `internal/career/workspace_item.go`
  - 支持用户可读主文档路径和 evidence path 分离。
- `internal/career/workspace_output.go`
  - 在分析报告保存后触发资料库文件写入。
- `internal/career/command_intent.go`
  - 自然语言触发 review-library 生成。
- `internal/career/command_handler.go`
  - `/library` 使用深度生成流程，而不是只刷新骨架。
- `internal/career/ui.go`
  - 命令帮助和欢迎语列出 `/library`。
  - 如果 `/help` 白名单里有旧命令列表，加入 `/library`。
- `README.md`
  - 更新复习入口、证据层和资料库生成说明。
- `internal/career/*_test.go`
  - 更新和新增测试。

## 验证计划

### 单元测试

新增或更新：

- `TestWorkspaceGuidePromptSummaryDoesNotInjectDemoTopic`
  - 确认真实运行 prompt 不出现 `Use "市场营销" as the neutral demo topic`。
- `TestReviewLibraryWritesDeepDocumentsFromResumeJDExperience`
  - 输入简历、JD、面经，断言生成：
    - JD 画像
    - 岗位作战页
    - 具体题库
    - 项目 QA
  - 断言题库中包含面经具体问题，例如“给你 5 分钟看一个账号”。
  - 断言项目 QA 中包含简历真实项目，例如 Wonderlab 或个人账号运营。
- `TestReviewLibraryDoesNotRequireModelFileWrite`
  - 模拟模型返回 JSON，app 层写文件，不调用 `file_write`。
- `TestReadableSlugForChineseRole`
  - `市场营销 / 新媒体运营实习生` 生成可读 slug，不生成 `domain-...`。
- `TestWorkspaceIndexPrefersReadableMaterialPath`
  - index 中用户入口不是 `extracted.md`。

### 集成测试

用真实样例：

```text
career-workspace/inbox/岗位JD_市场营销新媒体运营实习生.md
career-workspace/inbox/知页简历_市场营销实习生.docx
career-workspace/inbox/面经_市场营销新媒体运营实习生.md
```

跑一次自然语言：

```text
我把简历、JD、面经放到 inbox 里了，你帮我记录并分析下
```

验收：

- `outputs/latest-report.md` 存在。
- `my-interviews/市场营销新媒体运营实习生/01-临阵抗拷打主文档.md` 存在。
- `experiences/marketing-new-media-operations/*题库.md` 存在，且不是模板空话。
- `prepare/*interview-qa.md` 存在，且引用简历项目证据。
- log 中不出现 `approval required for dangerous tool "file_write"`。
- prompt 中不出现 demo topic 注入。

### 回归测试

```bash
env GOCACHE=/Users/r1son/Desktop/projects/happyagent/.gocache go test ./internal/career ./cmd/happyagent
env GOCACHE=/Users/r1son/Desktop/projects/happyagent/.gocache go test ./...
```

## 验收标准

- 用户打开 `面试资料库首页.md` 后，能一路进入岗位作战页、题库和项目 QA。
- 用户不需要理解 `extracted.md/source.md/metadata.json` 才能复习。
- 题库每个问题来自面经或 JD，不是泛泛模板。
- 答案能结合简历真实经历，但不会编造未证实事实。
- 模型没有 `file_write` 权限时，资料库文件仍能写入。
- 方向目录可读稳定，不出现无意义 `domain-<hash>` 作为主要路径。
- `index.json` 和 prompt 中的用户入口优先指向可读资料库文件。
- 运行 prompt 不再把任何 demo topic 当真实用户方向。
