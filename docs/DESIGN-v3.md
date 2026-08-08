# entdomain 设计 v3：推翻 Scope 模型，接管 HTTP 层

> **2026-08-08 后记：模块已改名 `entapi`（ADR-0011）。** 本文是改名前的
> 过程记录，按惯例保留原名不重写；现役事实以 `DESIGN-v3-final.md` 与 ADR
> 为准。

> **状态：拷问会话裁决完毕（2026-08-07）；跨家族对抗评审完毕、评审触发的
> 复裁亦全部落定（2026-08-08，记录见 §10）；实现未开始。**
>
> 本文是 `IDEAL.md`（设计虚构）与实现之间缺的那份设计：一次 grilling 会话
> （逐分支拷问、每题附推荐、owner 逐项拍板）的净残留。方法与 DESIGN-v2 相同的
> 一条惯例照旧：**被推翻的判断留档标注，不静默删除**——包括本文作者自己被 owner
> 改判的两处（§6.4、§6.5）。
>
> 与前文的关系：`DESIGN-v2` 的五项裁决（offset 分页、矛盾即硬失败、错误映射落点、
> ScopeQuery 存留、v0 兼容策略）与五条 Accepted ADR **全部继续有效**，本文不重开。
> 其中一处需要记账的表面变化见 §4.5：ADR-0005 的门控语义原样存活，只是执法点移动。
> 术语以根目录 `CONTEXT.md` 为准，本文写作时与之同步。
>
> 对抗评审已完成（Codex GPT-5.6 攻可行性 + Gemini 3.1 Pro 攻边界，均只读、
> 证伪指令、并集合并）。11 条发现：9 条证实并已折进正文（§10.1），1 条反驳、
> 1 条部分反驳（§10.2，留档不删）。评审只攻事实断言，未重开 owner 取向裁决。

---

## 0. 论点与范围

`IDEAL.md` 把差距总结为两件事：**第 6 行的收缩**（声明成本从每字段 3–4 连调用链
降到每实体一行）与**第 7–10 行的接管**（HTTP handler、路由、OpenAPI、行级授权）。
本文裁决前三项的设计，第四项（OwnedBy）显式移出范围（§5.4）。

Scope 宪章不变：**注解只控制 HTTP 层生成，永不限制 service 层对 ent 实体能做什么。**
本文每一节都在这条线的 HTTP 一侧。

---

## 1. 注解模型：`entdomain/api`，沉默即默认

### 1.1 包身份与退场（裁决）

新注解面住**新建子包 `entdomain/api`**。`DomainField`、六个 preset builder、
四个 Scope 常量**在同一 arc 内整体删除，零共存期**。

- 调用点读作 `api.Resource()` / `api.Hidden()`——注解面是这个设计的门面；
- 零共存是 DESIGN-v2 §9.5（v0 自由破坏）的直接推论：生成器同时理解两套模型
  意味着两套语义合流，那正是最容易藏矛盾的地方；
- 已知代价：`api` 是消费者常用包名，schema 文件里可能需要 import 别名。
  schema 文件只 import 这一处，成本一次性。

### 1.2 实体级：`api.Resource()` 与操作子集（裁决）

`api.Resource()` 是实体级唯一开关：标了它，实体获得完整生成 CRUD 面；不标，
生成器对该实体不产出任何东西（今天"无 domainFields 即跳过"的行为等价延续）。

操作子集走**细粒度 `api.Resource().Except(api.OpDelete, …)`**（owner 裁决，
否决了作者推荐的粗粒度 `ReadOnly()`，见 §6.5）。操作枚举 `api.OpCreate` /
`OpPatch` / `OpDelete` / `OpGet` / `OpList` 是 `api` 包的导出常量
（原稿作 `OpUpdate`，随 §1.5 的命名裁决改为 Patch）。

随附一条从 §9.2 政策机械导出的推论：**全字段 Immutable 而未写
`Except(api.OpPatch)` → 生成期拒绝**，消息点名两个事实并给出修复行。
空 PATCH 端点不是"可以正确生成的东西"，而 Except 的存在让修复恰好是一行。

### 1.3 字段级：五词偏离词表（裁决）

字段默认**沉默**：零注解，HTTP 形状完全由 ent 事实推导。偏离词表恰好五个：

| 词 | 语义 |
|---|---|
| `api.Hidden()` | 只在 service 层存在——不进请求、响应、查询任何 HTTP 面 |
| `api.ReadOnly()` | 服务端管理——进响应，永不进 create/patch 请求 |
| `api.Searchable()` | 加入 `_q` 自由文本析取；解锁子串算子（ADR-0005） |
| `api.Filterable()` | 获得结构化过滤算子（算子集来自 ent `$field.Ops`） |
| `api.Sortable()` | 进入排序白名单 |

今天六个 preset 的完整映射（这张表就是"沉默即默认"的证明）：

| 今天的 preset | v3 表达 | 依据 |
|---|---|---|
| `DefaultField` | 沉默 | 全默认正是推导目标 |
| `IdField` | 推导 | `$.ID` 已有 |
| `CreateOnlyField` | ent `Immutable()` | 创建可给、PATCH 不存在——ent 原生语义 |
| `InputOnlyField` | ent `Sensitive()` | 永不进响应、仍可进请求——正好是 password 要的 |
| `OutputOnlyField` / `AuditLogField` | `api.ReadOnly()` | "只出不进"推不出来：`created_at` 的 Default+Immutable 与业务字段 `status` 的 `Default("draft").Immutable()` 在 ent 事实上同形，靠推导必然误伤 |
| （无对应）内部字段 | `api.Hidden()` | —— |

`WithRequired` 被砍：API 层强制必填一个 ent 可默认的字段是罕见需求，
加是增量、删是破坏，往小发（同 §9.1 的不对称论证）。#26 的 presence 规则
相应少一个输入，其余不变。

### 1.4 校验残差：不做注解，分类 `ValidationError`（裁决）

IDEAL §3.1 说数值/模式约束"保留为注解"。**该句被推翻**（留档见 §6.3）。
本会话把"为什么读不到校验器值"验到了底，两堵墙：

1. **进程边界擦除**：`entc` 经临时程序把 schema JSON 序列化传回，
   `entc/load/schema.go:56` 的 `Validators int` 只传个数（`:139`,
   `len(fd.Validators)`）——闭包不可序列化，生成器进程里 `MinLen(3)` 的 3 不存在；
2. **语言边界**：`schema/field/field.go:215` 把 MinLen 编译成
   `func(v string) error` 闭包，Go 反射读不出闭包捕获。

而做校验注解 = `MinLen(3)` 与 `api.MinLen(3)` 双重声明，可漂移，且词表膨胀。

**裁决：词表守住五词。生成的 HTTP 层把 ent 自己的 `ValidationError` 分类为 422。**
`IsValidationError` 由 ent 生成在消费者包（已验证：`basicent/ent.go:172,188`）。

两处评审修订（§10.1 C5/C3）：

- **接线不止一个 bool 谓词**：problem+json 的 `field` 成员需要
  `ValidationError.Name`，而 `IsValidationError` 只回 bool。生成侧多交一个
  extractor（形如 `func(error) (field string, ok bool)`，内部 `errors.As` 到
  消费者包的 `*ValidationError`）——runtime 仍不认识 ent，DESIGN-v2 §9.3 的
  "判定函数作参数"原则不变，只是函数从三个变四个;
- **覆盖面如实收窄**：ent 的部分校验类失败不走 `ValidationError`——如
  required edge 同时 clear+set 返回裸 `errors.New`（`edgesent/post_update.go:134`）
  → 落 500 不是 422。入档为已知残余，方向沿 errors.tmpl 的既有立场：
  宁可 500 也不冒错分类。

代价如实入档：校验发生在 `Save` 时而非绑参时；OpenAPI 不体现 `minLength` 类约束。
两者将来都是增量可补的。

### 1.5 旧 Scope 的去向

旧模型对每字段逐面枚举归属（Create/Update/Response/Query 四个布尔）；v3 是
**推导 + 减法**。四个 scope 的信息现在住在三处，映射是完整的：

| 旧 scope 表达 | v3 归宿 |
|---|---|
| ∉ ScopeResponse（进请求不进响应） | ent `Sensitive()` |
| ∉ ScopeCreate 且 ∉ ScopeUpdate（只出不进） | `api.ReadOnly()` |
| ∈ ScopeCreate 且 ∉ ScopeUpdate（创建后不可改） | ent `Immutable()` |
| 四个全无（任何 HTTP 面不可见） | `api.Hidden()` |
| 四个全有 | 沉默 |
| ScopeQuery + 维度 | `Searchable`/`Filterable`/`Sortable` 三词自含查询可达性 |
| 实体级"哪些操作存在" | `Resource().Except(op…)`（旧模型靠全体字段缺某 scope 间接表达，v3 升为实体级一等公民） |

一处**语义本质差异**要说破：旧 scope 纯 HTTP；v3 借用的 ent 事实
（`Immutable`/`Sensitive`）同时约束 service 层与日志。这是设计不是漏洞——
声明住在拥有它的层，HTTP 面跟随。由此留下的表达力缺口**故意不填**：
"service 层可改但 HTTP 不可 PATCH 且 create 可给"（旧 `∈Create ∉Update` 而无
Immutable）在 v3 无对应词。v0 等真实消费者拿出场景再加（加是增量）。

**反向速查表（效果 → 写法）**——使用者想的是"这个字段要什么效果"，查表方向
应该反过来。三面（Create/Patch/Response）8 种组合：

| # | Create | Patch | Response | v3 写法 | 典型场景 |
|---|---|---|---|---|---|
| 1 | ✓ | ✓ | ✓ | 沉默（零注解） | 普通业务字段 |
| 2 | ✓ | ✓ | ✗ | ent `Sensitive()` | 可反复重设但永不回显 |
| 3 | ✓ | ✗ | ✓ | ent `Immutable()` | 创建时定、之后不可改（slug、类型码） |
| 4 | ✓ | ✗ | ✗ | ent `Immutable().Sensitive()` | 一次性写入的秘密（注册 token） |
| 5 | ✗ | ✓ | ✓ | **无对应词（v0 留缺）** | 创建后补录字段（tracking_number 类），见下注 |
| 6 | ✗ | ✗ | ✓ | `api.ReadOnly()` | 审计字段、服务端管理字段 |
| 7 | ✗ | ✓ | ✗ | **无对应词（视为无意义）** | 可改但永不可读 |
| 8 | ✗ | ✗ | ✗ | `api.Hidden()` | password_hash |

两条特例：**主键 ID 天然是第 6 行**，零注解；查询三维度与此表正交
（任何 ✓Response 行可再加 Searchable/Filterable/Sortable）。

第 5 行注（评审修订，§10.1 A4）：原文写"罕见"，评审举证**创建后补录**是一类
真实需求（订单的 `tracking_number`：POST 时不该给、后续 PATCH 填、GET 回显）。
评审的另两个例子被反驳（`email_verified`/`cancellation_reason` 实为
ReadOnly + external 动作场景）。留缺的**裁决不变**，但理由从"罕见"修正为
"有真实需求类，当前工作法是接受该字段以 Optional 出现在 create（代价：语义上
过早可给）或 ReadOnly + external 动作端点；专用词等真实消费者的压力再加"。

命名规则说破（表列名用 Patch 的原因）：更新面只有一个、没有 PUT——
~~操作名说 Update、线格式名说 Patch~~——**该双名规则已被 owner 推翻
（2026-08-08）：命名全线用 Patch**（`api.OpPatch`、`Patch{Entity}`、
`Patch{Entity}Fn`）。推翻理由留档：双名规则守卫的 Update/Patch 区分在本框架
没有第二个成员（没有 PUT），规则本身违反"一件事一种写法"，且 k8s 语境里
Update = 整体替换、名不副实。原规则的辩护论据（ent `UpdateOne` 同族、
gRPC/AIP "Update + field mask" 惯例）记录在案——ent 的缝在两个产品之间，
不在本框架面内。
这张表应随实现进入 README/godoc——解决"逐面枚举 → v3 写法"的脑内转换负担的
是文档速查，不是 API 面。

**preset 式语法糖：不设（owner 裁决，2026-08-08）。**
反向表暴露的结构性事实是决定性论据：可表达 6 行中的 3 行（2/3/4）是
**ent 字段构建器方法，不是注解**，而 schema annotation 在 ent 里没有能力把字段
变成 Immutable/Sensitive——`api.InputOnly()` 式的糖做不成真别名，只能做成
"生成器把它当 Sensitive 对待"的第二套机制：HTTP 形状相同，但 service 层约束与
日志脱敏行为不同于真 `Sensitive()`。两种写法两种深层语义，比两种写法一种语义
更糟。对 6/8 行，已是单词，无可再省。

### 1.6 边：`api.Expand()`（裁决）

边级唯一偏离词。默认不展开；`api.Expand()` 把边选入响应（目标实体的 Summary，
一层），可带 `.JSONKey("…")`。#25 的两条规则原样带入：**永不从外键位置推导**；
**目标实体非 Resource → 生成期拒绝**。

---

## 2. 拒绝矩阵

§9.2 的政策（矛盾 → 生成失败并同时报出两个事实）扩展到新词表，逐格裁决：

| 组合 | 处置 | 不拒绝的后果 |
|---|---|---|
| `Hidden` × 其余任何偏离词 | 拒绝 | Hidden 是全量退出，再加词是自相矛盾的意图 |
| `Sensitive` × `Searchable`/`Filterable`/`Sortable` | 拒绝 | 不在响应里却可查询——匹配/排序构成侧信道 oracle，password_hash 可被逐位试出 |
| `Sensitive` × `ReadOnly` | 拒绝，消息指路 `Hidden` | 不进响应+不进请求 = Hidden 的绕路写法 |
| `ReadOnly` × ent 必填且无 Default | 拒绝 | create 请求没有它、ent 又必填——每次创建必然失败，生成期可证明的死路 |
| `Hidden` × ent 必填且无 Default，且 `OpCreate` 未 Except | 拒绝 | 同上一行的死路：生成的 create 没有该字段、ent 又必填，每次创建被 ent 校验器拒。修复行二选一：`Except(api.OpCreate)`（创建走 external，见附录 A 的 password_hash），或给字段 Default/Optional |
| `ReadOnly` × 查询三维度 | 允许 | 按 created_at 过滤/排序是正常需求 |
| `Expand` → 非 Resource | 拒绝 | 目标无 Summary，产物不编译 |
| 全 Immutable 且未 `Except(OpPatch)` | 拒绝（§1.2） | 空 PATCH 端点 |
| `_` 开头字段名进查询面 | 拒绝（§4.4） | 与保留参数命名空间冲突 |

矩阵之外全部照 #26 已裁决的 presence 规则推导。注意旧 §9.2 的
Immutable×ScopeUpdate 冲突在新模型里**消解**：PATCH 形状直接从 `MutableFields`
推导，不存在能与之矛盾的注解。

`Sensitive` 的推导本身（永不进响应，无覆写通道）是落地顺序第一片（§9）。
已验证：今天的生成器与模板对 `Sensitive` **零引用**（grep 全空），
IDEAL §4"正在泄漏"属实。

---

## 3. HTTP 层

### 3.1 拓扑：三条（裁决）

1. **生成的 handler、`API(client)`、路由注册全部落进消费者的 `ent` 包**——与
   DTO/wiring 同落点，marker 所有权、cleanup、#62 冲突检查全部现成复用；
2. **机械运行时助手**（problem+json 写出、状态码映射、op-in-value 解析器的
   共享部分、`_` 参数绑定）进 `entdomain/runtime`——`net/http` 是 stdlib，
   `TestRuntimePackageIsGeneratorFree` 的不变量不破；
3. **`entdomain/api` 保持纯 schema-time**，一个运行时符号都不放。

IDEAL §1 的 `api.Mount(mux, ent.API(client))` **被推翻**（留档见 §6.1）：
修订为 **`ent.API(client)` 返回 `http.Handler`**，挂载即
`mux.Handle("/", ent.API(client))`，另给 `.Mount(mux)` 便捷方法。

### 3.2 三步体与替换实现（裁决）

生成的 handler 永远是三步体：绑 → 调一个函数 → 写。没有 override 点。

第 2 步的函数是**替换点**，其类型**与生成的 wiring 自由函数签名逐字相同**，
默认值就是 wiring 函数本身（已验证今天的签名：
`CreateArticle(ctx, db *Client, v *ValidArticleCreateRequest) (*ArticleResponse, error)`）：

```go
ent.API(client).With(ent.CreateArticleFn(myCreate))
// myCreate: func(context.Context, *ent.Client, *ent.ValidArticleCreateRequest) (*ent.ArticleResponse, error)
```

- 替换实现 = 换函数名，零新概念；默认实现与你的实现在类型系统里是同一种东西；
- 返回 Response 而非 ent 实体：`NewArticleResponse` 对未加载边报错的陷阱
  暴露在**你的函数体里**、开发期撞到，而不是藏进框架的转换步骤运行期爆；
- List 替换点的签名随 §4.6 的 `ListRequest` v2 一起变，两者是同一个契约。

**external 门零机制**：非 CRUD 端点就是消费者直接在自己的 mux 上注册
（stdlib mux 最长匹配天然让更具体的模式赢）。框架不提供任何东西，这是设计而非缺席。

### 3.3 错误信封与状态码（裁决）

错误一律 **RFC 9457 problem+json**（`Content-Type: application/problem+json`），
带 `field` 扩展成员；成功响应**裸 DTO / Page 四字段**，不包信封：

```json
{"type":"about:blank","title":"validation failed","status":422,"detail":"title is required","field":"title"}
```

不自造信封的理由与"治理出界"同构：信封是事后改不动的线格式契约，标准把
这个决策外包给 IETF；网关/中间件/前端库对 problem+json 有现成理解。

状态码表：`201` Create · `200` Get/List/Patch · `204` Delete · `400` JSON 解析
失败与查询参数非法 · `404` ErrNotFound · `409` 唯一键冲突 · `422`
ErrValidation 与 ent ValidationError · `500` 未分类（含未装唯一键判定时的重复
键——沿用 errors.tmpl 已声明的方向：宁 500 不错答 409）。

**未知字段严格拒绝**：生成的 handler 开 `DisallowUnknownFields`。CLAUDE.md 记过
Immutable 字段出现在 PATCH body 被 `encoding/json` 静默丢弃的洞——handler 归框架
生成后这个球回到框架手里，按 ADR-0001 精神（错误绝不静默 no-op）严格拒绝。

### 3.4 URL 面（裁决）

- 路径 = **`snake(plural(Name))`**：`UserProfile → /user_profiles`。ent 全线
  snake（列名、JSON key），路径跟着走，一个系统一种命名法。无 `Path()` 覆写旋钮。
- **五端点**：`GET /xs`、`POST /xs`、`GET/PATCH/DELETE /xs/{id}`。
- **`DeleteBatch` 不上 HTTP 面**：批量删无标准 REST 动词，默认暴露是脚枪；
  留在 service 层，要暴露走 external 门。
- 路由用 Go 1.22+ stdlib `http.ServeMux` 方法+路径模式，不自研（IDEAL §5 既决）。

---

## 4. 查询面：op-in-value（本会话最大的方向改判）

### 4.1 线格式（owner 裁决）

HTTP 过滤参数采 **entigo 式 op-in-value**：`field=op:value`，按**第一个冒号**
切分算子与值，裸值即等值。作者原推荐（照抄 #27 的 form-tag 每算子一参数）被
owner 改判，对比过程与双方论据留档于 §6.4。

```
GET /records?title=ilike:simon&score=gt:30&status=in:active,archived&_sort=created_at:desc&_page=2
```

算子词表（源自 entigo，映射到 ent 谓词）：裸值(eq)、`ne:`、`gt:`/`ge:`/`lt:`/`le:`、
`in:`/`not_in:`（逗号分值）、`like:`/`ilike:`（Searchable 门控）、`prefix:`/`suffix:`、
`is_null:`/`not_null:`、`from:`/`to:`/`between:`（时间与范围糖）。

### 4.2 类型化结构体保留为解析结果

**#27 的 `{Entity}Filter` 结构体不删**：生成的解析器把 `score=gt:30` 解析进
`Filter.ScoreGT *int`。wiring 签名、service 层 API、替换点因此零改动。
变的只是 HTTP 绑参层——`form` tag 及其绑定语义退役（supersession 记账见 §4.5）。

### 4.3 算子表不回运行期

每个字段接受哪些前缀仍在 **codegen 期**从 `$field.Ops` ∩ 三维度标记算出，
生成为该字段的 parse switch。`funcs_filter.go` 拒绝"第二张会漂移的算子表"的
理由原样成立——表还是生成的，只是消费时机从"结构体字段存在与否"变成
"解析器接受与否"。非法算子/类型解析失败 → **400 problem+json**，`field`
扩展成员点名参数。

细则（随裁决落定）：

- **值解析规则（定稿，owner 裁决 2026-08-08，含评审 A1/A2 处置）**，
  自上而下第一条命中即生效：
  1. 裸空值（`?title=`）→ **忽略该过滤参数**（前端空输入友好）；
     显式 `?title=eq:` → 精确匹配空串;
  2. 无冒号 → `eq` 字面值（`?title=abc` ≡ `?title=eq:abc`）;
  3. 有冒号，前缀 ∈ **该字段的允许算子集** → 按算子解析;
  4. 有冒号，前缀是**全局已知算子但该字段不允许**（如 `contains:` 打在非
     Searchable 字段）→ **400**——ADR-0005 的门控保持大声，不静默吞;
  5. 有冒号，前缀**不是任何已知算子** → 整值回落 `eq` 字面值——时间
     `12:30`、URL 等朴素值直接可用（评审 A1 的痛点消解）；string 字段上
     拼错的算子静默成字面值是**已接受代价**（typed 字段由值解析失败兜出 400）;
  6. `eq:` 显式前缀 = 字面值转义舱（值恰以算子前缀开头时用：
     `eq:like:john` → 字面值 `like:john`）。
- **`between:` 是纯糖**：解析为 gte+lte 两个已有谓词，不新增谓词类型;
- **`json:` 本 arc 不做**（JSONB 与 ent JSON 字段谓词是独立深水区，defer）;
- **已知限制入档**：`in:`/`between:` 的值内含逗号无法表达——且**百分号编码
  救不了它**（评审证实，§10.1 A1）：`r.URL.Query()` 在框架解析前已把 `%2C`
  解回 `,`、`%3A` 解回 `:`，URL 编码对本语法完全透明，`eq:` 前缀是唯一转义
  通道;
- OpenAPI 代价如实入档：过滤参数 `type: string`，允许的前缀集由 codegen 写进
  该参数的 `pattern` + `description`（集合是精确已知的）；前端类型退化为 string。

### 4.4 保留参数命名空间：`_` 前缀（裁决）

op-in-value 让过滤参数变成裸字段名，`sort`/`page`/`size` 与同名列的碰撞从理论
变成现实（带 `size` 列的实体上 `?size=gt:30` 两可）。裁决：

> **`_` 前缀是框架的，裸名是 schema 的。** 恰四个保留参数
> `_sort` / `_page` / `_size` / `_q`，唯一写法，**无别名**。

- 别名方案被否：裸 `size` 都认的话，歧义并未消除，解析变成 schema 相关——
  最阴的那类上下文相关契约。重设计消除特殊情况，不加分支;
- 叫 `size`/`sort`/`q` 的字段照常可过滤，零特判;
- 冲突检查补一条：`_` 开头的字段名进查询面 → 生成期拒绝（§2 矩阵末行）;
- 响应体不受影响（`{"data","total","page","size"}` 是嵌套 JSON 键，不同空间）;
- 先例：OData 的 `$top/$skip/$orderby` 是同一手法，`_` 在 URL 里比 `$` 干净。

### 4.5 supersession 记账

**没有任何 Accepted ADR 被推翻。** 特别地，ADR-0005（子串算子需要 Searchable）
的门控语义原样存活——执法点从"参数不存在"移到"解析器拒绝并返回 400"，
门本身没动。被替换的是 #27 的 HTTP 绑定表面：`form` tag 与 `Q *string`
`form:"q"` 等绑定约定退役，`filter.tmpl` 改为生成解析器 + 保留类型化结构体。
`sort_by` + `order` 参数对退役（v0 无发布负担）。

### 4.6 排序、分页、自由文本（裁决）

已读码对比 entigo（`query.go:153-196`）与本仓 runtime 后收编：

- **`_sort=field:desc,field2`**——entigo 语法（逗号多字段、`field:dir`、裸字段名
  即 asc；`WithOrderFrom` 实录），配 entdomain 护栏：白名单只来自 `Sortable`，
  **非法字段 400 不静默跳**（entigo 的静默跳过与 ADR-0001 精神冲突，不带入），
  **PK tiebreak 永远追加在末尾**（ADR-0002）;
- **`ListRequest` v2**：`SortBy string` → 排序项列表；`sort_by`/`order` 字段退役；
  这是 runtime 的 API 变更，随 v0 自由破坏；List 替换点签名随之变;
- **分页两家本就同模型**：`_page`/`_size`，1 起数，默认 20、上限 1000 钳制在
  唯一入口（`runtime/types.go` 现状，仅参数名加前缀）;
- **`_q` 保留 entdomain 设计**：对全部 Searchable 字段 Contains 后 OR
  （entigo 无对应物）。`Filterable` 与 `Searchable` **不合并**——结构化 AND 面
  与自由文本 OR 面是两个查询面，不是同一件事的两档松紧（本会话查证后重申，
  且合并需 supersede Accepted 的 ADR-0005）。

---

## 5. 周边接管

### 5.1 OpenAPI：落盘 + embed 服务 + 3.1（裁决）

- spec 生成到消费者 **`ent/openapi.yaml`**，随生成物 commit——IDEAL §3.2 的
  "PR diff 防线"因此成立（暴露姿态已裁决：大声打印，不设门禁）;
- 生成带 marker 的 `entdomain_openapi.go`，`go:embed` 该 yaml 并注册
  `GET /openapi.yaml`——磁盘与服务内容同源，不可能漂移;
- 版本 **OpenAPI 3.1**（JSON Schema 对齐，`*T`/nullable 映射干净；
  openapi-typescript / swagger-ui 均已支持）;
- cleanup 路径（评审修订，§10.1 C6）：~~挂在 `entdomain_openapi.go` 的存在性
  上~~——那会违反 ADR-0004（marker 所有权）：消费者删掉 yaml 里的 marker 接管
  手改后，按 `.go` 联动仍会误删手写文件。正解：**`openapi.yaml` 首行自带
  `# Code generated by entdomain extension` 注释 marker**，cleanup 对该文件名
  按 YAML 注释语法查它**自己的**首行——所有权契约（含逃生舱）与 `.go` 文件
  完全一致。

### 5.2 唯一键分类：文本匹配 + 生成期装配（裁决）

`errors.As` 到驱动错误类型的路是死的（要 import 驱动，破 stdlib-only）。裁决：

1. `runtime` 提供三个判定，**主通道是匿名接口探测、文本匹配只是兜底**
   （评审修订，§10.1 C4）：Postgres 判定先沿 `errors.As`/`Unwrap` 链探测
   `interface{ SQLState() string }`（pgx 的 `*pgconn.PgError` 与 lib/pq 的
   `*pq.Error` 都实现它，无需 import 任何驱动，stdlib-only 保持）、比对
   `"23505"`；探测不到才回落文本匹配（`SQLSTATE 23505`/`duplicate key value`）。
   MySQL 同理探测错误号/`SQLState` 字段能力，回落 `Error 1062`/`Duplicate entry`；
   SQLite 无此接口，维持 `UNIQUE constraint failed` 文本。
   动机是评审证实的一条**当前即失效**的场景：lib/pq 的 `Error()` 只输出
   `pq: ` + 本地化 Message（v1.10.9 `error.go:443`），非英语 `lc_messages` 下
   两条文本候选串都不存在——接口探测不受 locale 影响;
2. 生成的 `API(client)` 构造时按方言装配——生成代码在 `package ent` 内，
   `client.config.driver` 未导出字段包内可达（已验证：`basicent/client.go:46,73`
   在包内读写 `c.driver`），`Dialect()` 表达式成立;
3. **未知方言降级诚实**：不装判定，重复键回 500；`WithUniqueViolation`
   逃生舱保留。

残余风险：判定只在 `IsConstraintError` 为真后咨询，误报面 = 外键错误文本恰含
唯一键标记串——需要键值本身包含该字样，实践可忽略但如实入档。文本稳定性：
SQLite 消息十余年未变、PG SQLSTATE 是标准、MySQL 1062 是契约——但这确实是
**对第三方错误文本的依赖**，本方案唯一的真实成本。

### 5.3 软删除注册：ent 原生自动，spike 先行（裁决）

IDEAL 的"Mount 内自动"**被推翻**（留档见 §6.2）：注册塞进 `API(client)` 意味着
**只用 service 层的二进制（cron、worker）静默硬删**——软删除语义取决于建没建
HTTP 面，违反 Scope 宪章，失败形态是数据丢失。

原裁决：**目标是 ent 原生机制**——`SoftDeleteMixin` 自带 `Hooks()`/`Interceptors()`
声明，ent 的 `runtime.go` 在 client 构造时自动装配，零仪式；spike 验证，
失败回落显式 `RegisterSoftDelete`。

**评审后修订（§10.1 C1/C2）：ent 原生 mixin 路线在 spike 之前已被证死**，
两条独立阻断：

1. **写路径不可行**（C1）：hook 里 `SetOp` 改不了 `next` 已捕获的 DELETE 执行
   闭包；把删除重派发成 UPDATE 需要消费者的 `*Client`——框架级 mixin 无法命名
   它（ent 官方软删除示例正是消费者侧 mixin，用的是自己生成的 Client；
   `dialect/sql` 通用谓词只够 interceptor 的查询半边）;
2. **零仪式是假的**（C2）：schema 声明 hook 后，ent 把注册移进 `ent/runtime`
   并要求应用 blank-import；漏掉则 hook 列表为 nil，任何 mutation 报初始化错误
   （`basicent/ent.go:508`）。ent-native 只是把 `RegisterSoftDelete(client)`
   换成 `import _ "…/ent/runtime"`——一行仪式换了个更隐蔽的写法。

**复裁（owner，2026-08-08）：走 (a) 生成 init 注册**——框架把注册代码生成进
消费者 ent 包（生成代码可命名 `*Client` 与各实体 hook 列表），`init()` 自动
挂接：消费者零仪式、对所有进程生效。代价如实入档：IDEAL"没有 init()"的美学
承诺只对手写代码成立。spike 目标 = 验证生成 init 路径（hook 列表的挂接时机与
多 client 场景）；spike 失败回落 (b) 显式 `RegisterSoftDelete`。

### 5.4 OwnedBy：移出范围，只记边界（裁决）

行级授权与软删除同构（框架特性落在 ent 层、与 HTTP 解耦），而该模式的可行性
正等 §5.3 的 spike。模式证明之前不在未验证前提上叠设计。DESIGN-v3 只记三条
约束性边界，设计留给后续独立 arc：

1. 落点**必须**是 ent 扩展点（interceptor/privacy）——DESIGN-v2 §0.2 已证
   wiring 层过滤可被一行 `db.X.Query()` 绕开，是建议不是保证;
2. "当前用户"契约走 `runtime` 的 stdlib context 取值函数，不自研 Context 类型;
3. 永不生成进 wiring/handler。

---

## 6. 被推翻的判断（留档，不静默删除）

### 6.1 IDEAL §1：`api.Mount(mux, ent.API(client))`

被 §3.1 推翻。`entdomain/api` 是 schema-time 包，`Mount` 是 run-time 调用，
同包两头跨违反本仓最承重的"按何时运行切包"原则。修订为 `ent.API(client)`
返回 `http.Handler`。门面损失一个 `api.` 前缀，换包边界零例外。

### 6.2 IDEAL §2.3/§4："唯一键分类 init、软删除注册等仪式 → 0（Mount 内自动）"

唯一键一半成立（§5.2 按方言在 `API(client)` 构造时装配）；**软删除一半被推翻**
（§5.3）：cron/worker 静默硬删是数据丢失级陷阱。正确的"零仪式"落点是 ent 的
runtime 装配机制，不是 Mount。

### 6.3 IDEAL §3.1："数值/模式类约束保留为注解"

被 §1.4 推翻：校验注解 = 与 ent 校验器双重声明，可漂移；正解是分类 ent 自己
生成的 `ValidationError`。IDEAL 该句改写为"保留为 ent 校验器，HTTP 层分类
其错误"。

### 6.4 作者推荐：HTTP 过滤参数照抄 form-tag 契约（owner 改判）

作者论据：类型链完整到 OpenAPI/前端（`ScoreGT *int` → `type: integer`）、
算子门控结构化（参数不存在 vs 运行期 400）、零转义语法。owner 反问触发重查：
作者对 entigo 格式"ISO8601 冒号无法切分"的批评**是错的**（entigo 按第一个冒号
切分，值分隔用逗号）——修正后 entigo 的真实代价收窄为：OpenAPI 型别退化、
值内逗号限制、字面值需 `eq:` 转义。owner 权衡后选 op-in-value（紧凑、单参数
单字段、entigo 生态延续），作者的类型链论据作为已知代价入档（§4.3）。
**决定性事实**：#27 的类型化结构体可保留为解析结果（§4.2），这使改判的
波及面从"推翻 #27"缩小到"只换绑参语法"——改判因此可行。

### 6.5 作者推荐：操作子集用粗粒度 `Resource().ReadOnly()`（owner 改判）

作者按"往小发"推荐只做只读粗档。owner 选细粒度 `Except(op...)`——操作枚举
常量进入公开 API 的代价被接受，换取任意子集的表达力。§1.2 的空 PATCH 拒绝
推论正是 Except 存在才成立的。

---

## 7. 与今天的差距（IDEAL §4 表的更新版）

| | 今天 | v3 裁决 | 性质 |
|---|---|---|---|
| 中间层生成（DTO/filter/wiring/errors） | ✅ | 保留；filter 绑参语法换 op-in-value | `[已存在]` + 改造 |
| 泛型运行时 0 entgo 依赖 | ✅ | 保留；新增 http 助手仍 stdlib-only | `[已存在]` + 扩展 |
| 声明成本 | 每字段 3–4 连链 | `api.Resource()` 一行 + 五词偏离 | **推翻 Scope 模型** |
| 属性推导 | 仅 presence | Sensitive/Immutable/Default 全面推导 + 拒绝矩阵 | 扩展 |
| HTTP handler/路由 | 无 | 生成三步体 + 替换实现 + external | **新增** |
| OpenAPI | 无 | 落盘 + embed 服务 + 3.1 | **新增** |
| 唯一键仪式 | 手写 init | 文本匹配 + 生成期按方言装配 | **新增** |
| 软删除仪式 | 手写 Register | ent 原生自动（spike 先行） | **新增（带验证门）** |
| 行级授权 | 无 | 移出范围，三条边界入档 | 愿景保留 |

---

## 8. 残余风险（对抗评审的攻击面索引）

> 评审后状态：1/4/6 已被评审命中并在正文修订闭合（指向见各条）；其余仍开放。

1. ~~方言文本匹配依赖第三方错误文本稳定性~~——**评审证实并已修订**（§5.2：
   主通道改接口探测，lib/pq 非英语 locale 场景闭合；C4）。残余：接口探测
   覆盖不到的驱动仍靠文本兜底;
2. **`eq:` 转义舱的完备性**（§4.3）——评审攻了 URL 编码维度（A1，已入档）；
   `in:`/`between:` 值内逗号仍是不可表达残余;
3. **`client.config.driver` 包内访问**依赖 ent 生成形状不变（§5.2 已验证
   v0.14.4，升级 ent 需复验）;
4. ~~`IsValidationError` 字段名提取~~——**评审证实并已修订**（§1.4：生成侧
   extractor；C5）。残余：非 `ValidationError` 形态的校验失败落 500（C3，
   已接受）;
5. **新生成符号的保留名**：`API`、`Mount`、各 `XxxFn` 需并入 #62 的实体名
   冲突检查; `_` 前缀字段拒绝（§4.4）是新增检查项;
6. ~~`openapi.yaml` 的 cleanup 特例~~——**评审证实并已修订**（§5.1：yaml
   自带注释 marker，ADR-0004 一致；C6）;
7. **op-in-value 解析器的模糊面**：重复同名参数（`?score=gt:30&score=lt:50`）
   语义未裁决——AND 合并还是 400，实现时定并入档;
8. 校验 422 发生在 Save 时而非绑参时；OpenAPI 不体现数值约束（§1.4，已接受）。

## 9. 落地顺序与未裁决事项

**第一片（IDEAL §4 原裁决带入）**：`Sensitive()` 从响应消失——"从 ent 推导"
最小的可证伪切片，纯收窄，只可能修复一个正在泄漏的消费者。做完它，属性推导
从主张变成已落地模式。

**第二片建议**（本文新增，实现期可调）：软删除 ent 原生 spike（§5.3）——它
同时是 OwnedBy arc 的前置验证。

**未裁决（§9.6 体例）**：

- 挂载前缀下 OpenAPI `servers`/paths 对齐、spec `info.title/version` 来源
  ——实现期定;
- 软删除实体的 HTTP 语义（restore 端点、`include_deleted`）——defer，
  与 OwnedBy 同理;
- `json:` 算子（§4.3）——defer;
- `in:`/`between:` 值内逗号——入档为已知限制，暂不解;
- 重复同名过滤参数语义（§8.7）——实现时定;
- 迁移说明写到哪（README Known limitations vs MIGRATION.md）——沿
  DESIGN-v2 §9.6 继续悬置，#23 收口时定;
- ~~preset 式语法糖~~——**已裁决：不设**（owner，2026-08-08，裁决与结构性
  论据见 §1.5）;
- ~~空过滤值语义~~——**已裁决**（owner，2026-08-08）：**裸空值 `?title=` →
  忽略该过滤参数；显式 `?title=eq:` → 精确匹配空串**。同时确认裸值规则：
  无冒号的值（`?title=abc`）即 `eq:abc`（§4.1 既有规则）;
- ~~`Expand` × 目标实体 `Except`~~——**已裁决**（owner，2026-08-08）：**允许并
  在文档言明**——`Except` 只管该实体自己的端点，"只能经父实体读到的引用数据"
  是合法形态；字段连嵌入都不该出现时用字段级 `Hidden`/`Sensitive`。

---

## 10. 对抗评审记录（2026-08-08）

一轮、两席、只读、证伪指令、并集合并（不投票；两席发现集不相交是常态）。
席位 1：**Codex GPT-5.6**（effort high）攻可行性；席位 2：**Gemini 3.1 Pro**
（agy，plan 模式）攻边界与前提。brief 含 read-first 清单与裁决围栏
（owner 取向裁决不得重开）。每条单席发现由 orchestrator 回源码独立核实。

### 10.1 证实并已折进正文（9 条）

| # | 席位 | 发现 | 落点 |
|---|---|---|---|
| C1 | Codex | 框架级 mixin 写 hook 不可行（`SetOp` 改不了已捕获闭包；重派发需消费者 `*Client`） | §5.3 路线修订 |
| C2 | Codex | ent-native hook 需 blank-import `ent/runtime`，零仪式不成立（`ent.go:508`） | §5.3 路线修订 |
| C3 | Codex | required edge clear+set 返回裸 error → 500 非 422（`post_update.go:134`） | §1.4 残余入档 |
| C4 | Codex | lib/pq `Error()` 无 SQLSTATE、Message 随 locale 变——文本匹配当前即失效 | §5.2 改接口探测主通道 |
| C5 | Codex | bool 谓词传不出 `ValidationError.Name`，`field` 成员填不上 | §1.4 增 extractor |
| C6 | Codex | yaml 清理挂 `.go` 存在性违反 ADR-0004 | §5.1 yaml 自带 marker |
| A1 | agy | `r.URL.Query()` 先行解码，`%2C`/`%3A` 当不了转义通道 | §4.3 限制加深 |
| A2 | agy | `?title=` 空值语义未定义 | §9 待裁决（附建议） |
| A3 | agy | `Expand` × 目标 `Except(OpGet)` 语义未言明 | §9 待裁决（附建议） |

### 10.2 反驳与部分反驳（留档不删）

- **A5（反驳）**："ent `MultiError` 破坏单 `field` 契约"——核实：生成的
  `check()` 逐字段先败先返（`widget_create.go:120-123`），全代码库零
  `MultiError`；`Save` 一次只报一个字段，单数 `field` 恰好够。残余内核
  （逐字段修复的 UX）是 REST 常见取舍，非缺陷;
- **A4（部分反驳）**：第 5 行"罕见"论——`email_verified`/`cancellation_reason`
  实为 ReadOnly + external 场景，不构成反例；`tracking_number` 类成立，
  已按 §1.5 注修订措辞，留缺裁决不变。

### 10.3 评审触发的复裁（已全部裁决，owner，2026-08-08）

1. **软删除注册路线**（§5.3）：ent 原生 mixin 前提被 C1/C2 证死 →
   **改走生成 init 注册**，spike 验证之，失败回落显式 Register;
2. **空过滤值**（A2）→ 裸空忽略、`eq:` 匹配空串（§4.3 值解析规则第 1 条）;
3. **Expand × 目标 Except**（A3）→ 允许并文档言明（§9）;
4. **eq 回落边界**（由 2 的裁决引出的子叉）→ 混合规则：未知前缀回落字面值、
   已知但被禁的算子 400（§4.3 值解析规则第 4/5 条）——ADR-0005 门控不因
   回落规则而变静默。

## 11. 定稿后增补（2026-08-08，owner 裁决）

两项在定稿与评审之后追加的裁决，均已同步进 `DESIGN-v3-final.md`：

1. **模块改名 `entdomain` → `entapi`**（ADR-0011，已执行）。动机：v3 删光了
   全部 domain 命名的符号后，模块名成了唯一残留的误导——"domain"（DDD 语境
   的业务层）恰是本框架明确拒绝承载的东西。名字入 ent 扩展命名族谱
   （entgql/entproto/entoas）；`entrest` 因同赛道活跃项目占用而避开。
   marker 换新并**永久**识别 legacy marker（一次字符串比较的代价，换掉整类
   "改名后旧生成物变孤儿"的迁移事故）。GitHub 仓库同步改名，旧 URL 重定向。
2. **路由清单（Route Manifest）**。provenance：gin/echo 接入评估暴露出唯一
   真实耦合点——生成的 handler 用 `r.PathValue` 取路径参数，依赖 stdlib mux
   的匹配填充，第三方路由器逐路由注册时取空。裁决：`API(client)` 的注册底座
   改为一份导出的数据清单（`Routes() []entapi.Route{Method, Path, Handler}`，
   stdlib 模式语法），适配器在消费者侧把自家参数经 `r.SetPathValue`（Go
   1.22+）注入后调 Handler。整树挂载仍是默认；清单是纯数据导出，不是行为
   扩展点。形状见 `DESIGN-v3-final.md` §2.5。
3. **竞对对比评审的增补**（独立上下文 Opus 对比 lrstanley/entrest，报告存
   `docs/COMPARISON-entrest.md`）。入档四组：① §4.4 边界从三条扩到六条
   （非请求路径旁路契约、privacy×软删除 spike 共存场景、privacy 下
   `ListPage.total` 待验证）；② **实现前必决**：事务边界（替换点/wiring
   收 `*Client` 不收 `*Tx`，跨实体事务无法纳入生成步）；`With` 组合语义
   同日由 owner 裁决为 Functional Options（变参≡链式、last-wins、nil 拒绝，
   见 final §2.2），不再悬置；
   ③ §2.2 说破"替换实现不是中间件"——横切走 `http.Handler` 洋葱组合或
   `Routes()` 逐路由包裹，身份经 context 传递；④ 偷师清单七项进 §5
   backlog，其中 ErrorHandler 观测钩子（第 3 步、已分类结果、观测/替换
   两档、全局+op）形态定稿。

## 附录 A：样例——账号系统的 User

这个样例把 v3 的每个机制踩一遍，包括它**故意不管**的部分。

### A.1 全部声明

```go
// ent/schema/user.go
type User struct{ ent.Schema }

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{entdomain.SoftDeleteMixin{}}          // §5.3：client 构造时自动装配
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Annotations(api.Searchable(), api.Sortable()),
		field.String("email").Unique().
			Annotations(api.Filterable()),                   // Unique → 重复 409 是推导的
		field.String("password_hash").
			Annotations(api.Hidden()),                       // HTTP 面上不存在这个字段
		field.Enum("status").Values("active", "suspended").
			Default("active").
			Annotations(api.Filterable()),
		field.Time("created_at").Default(time.Now).Immutable().
			Annotations(api.ReadOnly(), api.Sortable()),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).
			Annotations(api.ReadOnly()),
	}
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		api.Resource().Except(api.OpCreate),                 // 注册不是 CRUD，见 A.3
	}
}
```

沉默但成立的（零注解，全是 ent 事实）：`name`/`email` 非 Optional → 必填；
`status` 有 Default → 请求里 `*string` 可省；`created_at` Immutable → PATCH 面
无此字段；ID 类型、404/409 分类、分页钳制。

**若删掉 `Except(api.OpCreate)`**：`password_hash` 是 Hidden + ent 必填无
Default，命中 §2 矩阵的拒绝行——生成期失败并给出修复行，而不是生成一个
"每次创建都被 ent 校验器拒"的死端点。本样例正是那一行的动机。

### A.2 推导出的 HTTP 面

端点三组：`GET /users`、`GET/PATCH/DELETE /users/{id}`（`POST` 被 Except；
`DELETE` 被软删除 hook 改写，handler 不知情——Scope 宪章）。

| 字段 | PATCH | Response | 查询 |
|---|---|---|---|
| `name` | ✅ | ✅ | `_q` 析取、`_sort=name` |
| `email` | ✅ | ✅ | `?email=a@b.c` 裸值 eq；`?email=ilike:…` → **400**（未标 Searchable，ADR-0005 门控） |
| `password_hash` | 不存在 | 不存在 | 不存在 |
| `status` | ✅ | ✅ | `?status=in:active,suspended` |
| `created_at` | 不存在 | ✅ | `_sort=created_at:desc` |

```console
$ curl 'localhost:8080/users?status=suspended&_q=simon&_sort=created_at:desc,name&_page=1'
{"data":[…],"total":3,"page":1,"size":20}

$ curl -XPATCH localhost:8080/users/9f1c… -d '{"password_hash":"x"}'
{"type":"about:blank","title":"bad request","status":400,
 "detail":"unknown field \"password_hash\"","field":"password_hash"}   # §3.3 严格拒绝
```

### A.3 框架故意不管的部分

注册的请求体是 `{"email","password"}`，不是生成的 CreateRequest 形状，中间有
一步哈希——业务逻辑，走 **external 门**：

```go
// register.go —— 普通函数，框架看不见它
func Register(db *ent.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Email, Password string }
		json.NewDecoder(r.Body).Decode(&req)
		hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		u, err := db.User.Create().
			SetEmail(req.Email).SetName(req.Email).
			SetPasswordHash(string(hash)).Save(r.Context())  // service 层不受任何注解限制
		// 错误映射复用 ent.ErrorMap，响应复用 ent.NewUserResponse(u)
		_ = u; _ = err
	}
}

// main.go —— 全部接线
mux := http.NewServeMux()
ent.API(client).Mount(mux)                                   // 生成的端点 + /openapi.yaml
mux.HandleFunc("POST /register", Register(client))           // external：整个动词归你
mux.HandleFunc("POST /login", Login(client))                 // login 从头到尾不是 CRUD
```

梯度在此可见：`Register` 里生成物照用（`SetPasswordHash` 是 ent 的，
`NewUserResponse`/`ErrorMap` 是生成的），只是没用生成的 handler 与 wiring——
"偏离 = 少调一个函数"。

只改一步而不换请求形状时才用**替换实现**（§3.2）。2026-08-08 增补：service 接入
与横切的完整写法收进 `DESIGN-v3-final.md` 附录，要点两条——

1. **用 service 替换实现 = 方法值**：`UserService` 普通结构体注入依赖，
   `Patch` 方法与替换点同签名，`With(ent.PatchUserFn(svc.Patch))` 直接塞——
   receiver 被方法值捕获，IoC 就是闭包捕获，框架对 service 形状零规定；
2. **横切（认证/header/审计）= `http.Handler` 洋葱**，跑在三步体之前、
   401 短路不进绑参，身份经 `entapi.WithActor`（runtime context 契约，
   §4.4 边界 2）传给替换点与 ent privacy；按路由选择性横切用 `Routes()`
   清单按 `Method`/`Path` 选切点。切面栈是 main.go 里的显式包裹，
   无注解拦截器链。

```go
svc := &UserService{mailer: mailer}
api := ent.API(client).With(ent.PatchUserFn(svc.Patch))  // 替换实现：方法值即闭包
h := withAuth(withHeaderCheck(api))                       // 横切：洋葱，外层先跑
mux.Handle("/api/", http.StripPrefix("/api", h))
```

### A.4 样例的裁决价值

- `password_hash` 逼出了 §2 矩阵的 Hidden×必填×OpCreate 拒绝行;
- `Except(OpCreate)` 展示了操作子集的真实动机：**"创建"不是 CRUD 的实体**
  （注册、下单、开票……）是常态而非例外;
- 三扇门各就各位：中间件在 Mount 外、替换实现替换整单元、external 整动词自持——
  没有一处发生在生成类型内部（#29 的比例论证闭环）。
