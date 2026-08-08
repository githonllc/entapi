# entapi v3 定稿：沉默即默认的注解模型 + 生成的 HTTP 层

> **状态：定稿（2026-08-08），实现未开始。**
> **第三轮 grilling（2026-08-08）追加 13 条 owner 裁决**，逐条就地标注
> "（owner 裁决，2026-08-08）"，覆盖：事务边界、主键查询面、Summary 与列表行
> 形状、runtime 导出符号、请求硬化、并发写边界、重复过滤参数、值解析失败、
> OpenAPI `servers`/`info`、HTTP 层无开关、`Except` 的三层语义与 create 一族
> 例外。
>
> 本文只陈述最终设计。裁决过程、被推翻的判断、两轮跨家族评审与第三轮 grilling
> 记录在 `DESIGN-v3.md`（§6/§10/§12/§13）；术语表在根目录 `CONTEXT.md`；
> 约束性决策的单条记录在 `docs/adr/0006`–`0013`。冲突时以 ADR 为准。
>
> Scope 宪章不变：**注解只控制 HTTP 层生成，永不限制 service 层对 ent
> 实体能做什么。**

---

## 1. 注解模型

### 1.1 包与开关

注解住 **`entapi/api`** 子包（纯 schema-time，不含任何运行时符号）。
旧模型（`DomainField`、六个 preset、四个 Scope）整体删除，零共存期。

- **`api.Resource()`**——实体级唯一开关：标了它获得完整生成 CRUD 面，
  不标则该实体零产出。
- **`api.Resource().Except(api.OpCreate, …)`**——细粒度操作子集，
  枚举常量 `OpCreate / OpPatch / OpDelete / OpGet / OpList`。

**`Except` 关掉什么（owner 裁决，2026-08-08）**——三层分开处置：

| 层 | 处置 |
|---|---|
| HTTP 端点 + 路由 + OpenAPI path | **关** |
| 定制点类型 `{Op}{Entity}Fn` | **关**（不生成） |
| wiring 函数 `{Op}{Entity}` + `{Entity}{Op}Request`/`Valid…` | **留**（照常生成） |

第三行是 **Scope 宪章的机械推论**，不是权衡：「注解只控制 HTTP 层生成，永不
限制 service 层对 ent 实体能做什么」——`Except` 是注解，因此无权拿走
`CreateUser`。典型受益者是 external 端点：一个"多做一步、其余照旧"的自定义
动词（先算点什么、再建实体），最自然的写法是复用 `Valid{Entity}CreateRequest`
与 `Create{Entity}`；若一并拿走，external 端点就得手写整套校验，`Except` 便从
"关一个端点"变成了惩罚。（附录的 `POST /register` **不是**这一档——它落在
下面的 create 一族例外里，理由见那里。）
第二行的理由是 **fails-open**：若 `CreateUserFn` 仍然生成，
`With(ent.CreateUserFn(myCreate))` 会**编译通过却永不被调用**——静默失效的
替换点，正是 §1.4 那条"错位注解静默失效"要防的形态。不生成它，同一行代码
变成**编译错误**。
**代价**：`ent` 包里会有"HTTP 面上看不见"的 `CreateUser`/`UserCreateRequest`。
靠生成的 godoc 一句话消解（"端点被 Except，函数保留供 service 层与 external
端点使用"），不靠改设计。

**create 一族的例外：判据是"create 面是否可用"，不是哪个词**（owner 裁决，
2026-08-08）。上表第三行若无条件套用，会在一种 schema 下生成 100% 失败的导出
函数：附录样例的 `password_hash` 是 ent 必填（无 `Optional`/无 `Default`）
且被 `api.Hidden()` 挡在所有 HTTP 面外，于是 `UserCreateRequest` 里没有它、
`Apply` 永不 `SetPasswordHash`、`CreateUser` 每次都在 `Save` 处失败。
判据因此上移一层：

> **create 一族（请求 + `Valid…` + wiring + 端点 + 定制点类型）的生成，取决于
> ent 全部"必填且无 Default"的字段能否都被 create 请求覆盖。**
>
> - **覆盖不全 + 未 `Except(OpCreate)`** → 生成期拒绝（§1.4 那一行）;
> - **覆盖不全 + `Except(OpCreate)`** → **create 一族整体不生成**。作者已明说
>   这个实体不从 HTTP 建，而 wiring 在这个 schema 下不可能正确;
> - **覆盖齐全 + `Except(OpCreate)`** → 端点与定制点不生成，wiring 与请求
>   照常生成（上表第三行原样）。

由此 `Hidden` 与 `ReadOnly` 归一——两者只是"把字段挡在请求外"的两种方式，
`ReadOnly` 那行此前的无条件拒绝放宽为同样条件。
**代价**：`Except(OpCreate)` 因此有两种后果，取决于该实体的必填字段能否被
覆盖。可接受，因为差异**从 schema 直接读得出**（有没有被挡住的必填无默认
字段），不是隐藏状态；生成的 godoc 会写明是哪个字段导致 create 一族缺席。

**这条例外的边界，明写以防被读宽**（owner 裁决，2026-08-08）：**只有"可证明
必然失败"才配整族不生成**。对称情形 PATCH 字段集为空 + `Except(OpPatch)`
落在另一边——`PatchUser` 与空的 `{Entity}PatchRequest` **照常生成**：空 patch
是"无用但不坏"（`Apply` 什么都不做），而"无用"不足以推翻 Scope 宪章；
create 那边是"缺一个 ent 必填无默认字段，`Save` 每次都失败"。
**待验证（实现空 PATCH wiring 前必须先跑）**：ent 的 `UpdateOne` 在零 setter
时是否仍发 SQL、`UpdateDefault` 字段（如 `updated_at`）是否照样被写——本文
未核实，不得当作已知事实写进生成的 godoc。

### 1.2 字段级：沉默 + 五词偏离

字段默认**沉默**：零注解，HTTP 形状完全由 ent 事实推导
（Optional / Default / Nillable / Immutable / Sensitive / 类型）。
偏离词表恰好五个：

| 词 | 语义 |
|---|---|
| `api.Hidden()` | 只在 service 层存在——不进请求、响应、查询任何 HTTP 面 |
| `api.ReadOnly()` | 服务端管理——进响应，永不进 create/patch 请求 |
| `api.Searchable()` | 加入 `_q` 自由文本析取；解锁子串算子类（ADR-0005） |
| `api.Filterable()` | 获得结构化过滤算子（索引友好类，算子集来自 ent `$field.Ops`） |
| `api.Sortable()` | 进入排序白名单 |

边级唯一偏离词：**`api.Expand()`**（可带 `.JSONKey("…")`）——把边选入响应
（目标实体 Summary，一层）。永不从外键位置推导；目标非 Resource 即拒绝。
目标实体 `Except` 掉自己的读端点不影响嵌入：`Except` 只管该实体自己的端点，
"只能经父实体读到的引用数据"是合法形态。

**Summary 的字段集与列表的行形状（owner 裁决，2026-08-08）**：

- **`{Entity}Summary` = 该实体的响应字段全集减去所有边**——v3 语汇下即主键 +
  全部非 `Hidden`、非 ent `Sensitive()` 的标量字段（`ReadOnly` 照进）。
  **"哪些字段算简介"的收窄显式 defer**：schema 里没有任何事实说"这个字段是
  简版的那个"，猜就是发明信息；收窄需要一个新注解，那是另一件事;
- **`GET /xs` 列表的每一行是 `{Entity}Response`，边照常 Expand**——同一资源的
  列表与详情**返回同形**是 REST 消费者最强的期待，让列表悄悄少几个键属于
  "能编译、能跑、每个前端都要踩一次"的惊喜。成本已核实且比直觉小：ent 的
  eager-load 是**整页批量**的（`loadAuthor` 收集整页 FK 后发一条
  `user.IDIn(ids...)`，`edges/edgesent/post_query.go:447-460`），所以带边的
  列表是 **O(边数) 条额外查询/页**，不是 N+1，且不随页大小恶化。
  **残留代价**：to-many 边在列表上放大返回体积（行数 × 子对象数），
  且**没有关闭旋钮**——`Expand` 是一次声明，管详情也管列表。

**不设 preset 式语法糖**：常见组合已是零注解；Immutable/Sensitive 是 ent
字段构建器方法，注解做不成它们的真别名。

数值/模式校验**不做注解**（ent 在 codegen 前擦除校验器值）：ent 校验器照写，
HTTP 层分类 `ent.ValidationError` → 422。

### 1.3 反向速查表（效果 → 写法）

| # | Create | Patch | Response | 写法 | 典型场景 |
|---|---|---|---|---|---|
| 1 | ✓ | ✓ | ✓ | 沉默 | 普通业务字段 |
| 2 | ✓ | ✓ | ✗ | ent `Sensitive()` | 可反复重设但永不回显 |
| 3 | ✓ | ✗ | ✓ | ent `Immutable()` | 创建时定、之后不可改 |
| 4 | ✓ | ✗ | ✗ | ent `Immutable().Sensitive()` | 一次性写入的秘密 |
| 5 | ✗ | ✓ | ✓ | 无对应词（留缺） | 创建后补录；用 Optional-in-create 或 ReadOnly+external 代偿 |
| 6 | ✗ | ✗ | ✓ | `api.ReadOnly()` | 审计/服务端管理字段 |
| 7 | ✗ | ✓ | ✗ | 无对应词（无意义） | —— |
| 8 | ✗ | ✗ | ✗ | `api.Hidden()` | password_hash |

主键 ID 天然是第 6 行，零注解。查询三维度与此表正交。

**主键的查询面（owner 裁决，2026-08-08）**：主键**天然进 `Filterable` +
`Sortable` 白名单，零注解；永不 `Searchable`**；`id` 字段上写任何偏离词 →
生成期拒绝。这是"沉默即默认"最纯的一例——主键是全 schema 唯一 ent 事实完备到
不需要任何声明的字段（必有索引、必可比较、必在响应里、必非秘密）。由此
`?id=in:a,b,c`（批量回填）与 `_sort=id` 直接可用，后者与 §3.2 的 tiebreak
推广天然合流（PK 出现在排序列表任一位置即不再追加）。`Searchable` 的排除是
硬的：`_q` 是 Contains 析取，对 UUID/int 做子串扫描既无索引也无语义
（ADR-0005 的门控前提是"子串算子类"）。
**机制注记**：ent 的 `NewType` 把用户声明的 `id` 赋给 `typ.ID` 而**不放进
`typ.Fields`**（`entc/gen/type.go:262-273`），未声明时合成的 `typ.ID` 连
`Annotations` 都是 nil——所以偏离词在 id 上要么无处可写、要么静默失效，拒绝
是唯一不 fails-open 的处置。
**代价，如实入档**：这个默认**不可关闭**——没有反向词（引入会破"恰五个偏离
词"）。判断是不构成攻击面：`GET /xs/{id}` 本就逐个可取，`in:` 只省往返，
且 `_size` 上限 1000 已封顶。

命名**全线用 Patch**：`OpPatch`、`Patch{Entity}`、`Patch{Entity}Fn`、
`{Entity}PatchRequest`、HTTP `PATCH`——一个概念一个名，没有要背的换算规则。
只有部分更新，没有 PUT（也因此没有需要"Update"来区分的第二个操作）。
ent 构建器仍叫 `UpdateOne`——那条缝在两个产品之间，不在本框架的面内。

### 1.4 拒绝矩阵（矛盾 → 生成失败并报出两个事实）

| 组合 | 处置 |
|---|---|
| `Hidden` × 其余任何偏离词 | 拒绝 |
| `Sensitive` × `Searchable`/`Filterable`/`Sortable` | 拒绝（侧信道 oracle） |
| `Sensitive` × `ReadOnly` | 拒绝，消息指路 `Hidden` |
| ent 必填且无 Default 的字段被挡在 create 请求外（`Hidden` 或 `ReadOnly`），且 `OpCreate` 未 Except | 拒绝（创建必然失败）；报出两个事实：哪个字段必填、被哪个词挡住。修复：`Except(OpCreate)`、或给 Default/Optional、或去掉该词 |
| PATCH 字段集为空且未 `Except(OpPatch)`——全字段 Immutable，或可变字段全被 `ReadOnly`/`Hidden` 排除 | 拒绝（空 PATCH 端点；全 Immutable 是特例，不是全部） |
| 字段级偏离词标在 edge 上（或 `Expand` 标在字段上） | 拒绝（错位注解无读取路径，静默失效即 fails-open） |
| 任何偏离词标在 `id` 字段上 | 拒绝（同上：`typ.ID` 不在 `typ.Fields` 里；主键的查询面是天然的，见 §1.3） |
| `Searchable`/`Filterable`/`Sortable` × `OpList` 被 Except | 拒绝（列表端点不存在，查询标记无可达面） |
| `Expand` → 非 Resource | 拒绝 |
| `_` 开头字段名进查询面 | 拒绝（保留命名空间） |
| `ReadOnly` × 查询三维度 | **允许** |

---

## 2. HTTP 层

### 2.1 拓扑

1. 生成的 handler、`API(client)`、路由注册落进消费者的 **`ent` 包**
   （与 DTO/wiring 同落点，marker/cleanup/保留名检查复用）;
2. 机械运行时助手（problem+json 写出、状态码映射、解析器共享部分）进
   **`entapi/runtime`**——`net/http` 是 stdlib，stdlib-only 不变量不破。
   其中两组符号**导出**（owner 裁决，2026-08-08；定稿附录此前引用它们却无定义）：

   ```go
   // 生成 handler 第 3 步用的同一个写出器，导出后消费者 middleware 与框架
   // 错误体同形（样例里 401 的写法即靠它）。
   func WriteProblem(w http.ResponseWriter, status int, title string, err error)
   // body: {"type","title","status","detail"}；type 缺省 "about:blank"，
   // detail 取 err.Error()；`field` 扩展成员的精确形态留给实现。

   // 身份契约（§4.4 边界 2 的现货形态）：
   func WithActor(ctx context.Context, actor any) context.Context
   func ActorFrom(ctx context.Context) (any, bool)
   ```

   **`any` 的代价如实入档**：消费者每次读都要类型断言，且 actor 的具体类型不由
   框架约束。这是 context 值的固有形态（stdlib 同型），换来的是 OwnedBy 落地
   之前不冻结身份类型;
3. **HTTP 层无条件生成，没有开关**（owner 裁决，2026-08-08）——凡标了
   `api.Resource()` 的实体，handler / 路由 / `openapi.yaml` 一并产出。
   沿 #29 的方向：那次删掉 `WithBaseService`/`WithBaseHandler` 两个开关，正是
   因为"开关 × 模板"的组合面比它省下的东西贵；而且开关会把 §1.4 拒绝矩阵劈成
   两套——矩阵里已有三行依赖端点存在性（`× OpList 被 Except`、
   `× OpCreate 未 Except`），整体可关意味着同一份 schema 在两种配置下合法性
   不同。**四条代价如实入档**：只用 DTO+wiring 的现有消费者升级后，`ent` 包
   凭空长出 `API`/`Mount`/`Routes`/每实体一个 `{Entity}Fn`；`ent/` 目录凭空
   出现 `openapi.yaml` 并进 git；`ent` 包从此传递依赖 `net/http`（stdlib，
   不破 runtime 的 stdlib-only 不变量，但纯 gRPC/CLI 消费者是白拿的公开面）；
   保留名冲突面变大（`API`/`Mount`/`Routes`/`XxxFn` 须并入 #62 的实体名检查
   ——那是**检查**不是开关）。缓解手段是既有的：`openapi.yaml` 带 marker，
   删 marker 即接管（ADR-0004）;
4. 入口：**`ent.API(client)` 返回 `*API`（实现 `http.Handler`）**——
   静态类型必须是具体类型，否则 `.With`/`.Mount`/`.Routes` 不可编译；
   `mux.Handle("/", ent.API(client))` 或便捷方法 `.Mount(mux)`。

### 2.2 三段式 handler与自定义实现

生成的 handler 永远三步：绑 → 调一个函数 → 写。**没有 override 点。**
第 2 步是定制点，**类型与生成的 wiring 函数签名逐字相同**，默认值就是
wiring 函数本身：

```go
ent.API(client).With(ent.CreateArticleFn(myCreate))
// myCreate: func(ctx, *ent.Client, *ent.ValidArticleCreateRequest) (*ent.ArticleResponse, error)
```

`With` 的组合语义 = Go **Functional Options**（owner 裁决，2026-08-08，
ADR-0012）：
变参且链式等价（`With(a,b).With(c)` ≡ `With(a,b,c)`）；重复设置同一定制点
**后写覆盖**（last-wins，支撑"先默认后条件覆写"的惯用组装）；唯一例外：
**nil 函数值构造期拒绝，形态是 panic**——链式签名没有错误通道，程序员错误
按 stdlib 惯例（`http.Handle` 对 nil handler 同型）panic；nil 替换函数
不可能是有意的，静默落回默认是被 ADR-0001 判死的那类沉默。

**自定义端点（external）零机制**：非 CRUD 端点由消费者直接注册在自己的 mux 上
（stdlib mux 最长匹配）。框架永不生成 Service 骨架。

**自定义实现不是中间件，横切另有通道。** 自定义实现是逐操作的整单元替换；要对**所有**
端点统一加身份认证、header 检查、审计、租户注入这类横切关注，用 Go 的标准
AOP 形态——`http.Handler` 洋葱组合：整树包裹 `auth(ent.API(client))`，或经
`Routes()` 清单按 `Method`/`Path` 元数据**选择性逐路由包裹**。身份经
`context.Context` 向内传递（middleware 写入 → 定制点/wiring/ent privacy 读取），
context 就是这里的 IoC 容器。框架不提供注解式拦截器链——显式组合优于容器
魔法，且三段式 handler内部没有注入点是规则不是缺口（第 3 步的 ErrorHandler 观测钩子
见 §5 backlog）。service 接入同理零机制：定制点签名里没有 receiver 位置，
依赖注入靠闭包捕获——先构造你的 service，再用匿名函数包成定制点类型。

### 2.3 错误与状态码

错误一律 **RFC 9457 problem+json**（带 `field` 扩展成员）；成功响应裸
DTO / `Page` 四字段（`{"data","total","page","size"}`），不包信封。

`201` Create · `200` Get/List/Patch · `204` Delete · `400` JSON/查询参数非法 ·
`404` not-found · `409` 唯一键冲突 · **`413` 请求体超限** ·
**`415` 媒体类型不匹配** · `422` 校验失败 · `500` 未分类
（含未装唯一键判定时的重复键——宁 500 不错答 409）。

**请求硬化（owner 裁决，2026-08-08）**：生成的 handler 在绑参前做两件事，
**两者都零旋钮**——

1. `r.Body = http.MaxBytesReader(w, r.Body, 1<<20)`，超限 → **413**。
   Go 的 `http.Server` 只限制 header 不限制 body，无上限时一个大 body 就是
   零成本 DoS，而 `DisallowUnknownFields` 拦不住（解析器边读边分配）;
2. `POST`/`PATCH` 的媒体类型必须是 `application/json`（允许 `; charset=…`
   之类参数；缺失也算不匹配）→ **415**。`GET`/`DELETE` 不检查。

与 `DisallowUnknownFields` 同向：请求面已经选了"严格且显式报错"，body 无上限
是这条线上唯一 fails-open 的洞。**不给旋钮的理由**：`With` 目前的参数是
单一族（逐操作替换函数，签名与 wiring 逐字相同），混入全局配置项会让它从
一个族变成两个族，而单一族正是它编译期保证的来源。
**代价，如实入档**：1 MiB 是硬顶，本版无出路——middleware 里包一个更大的
`MaxBytesReader` 无效（内层更小者仍赢）。真有消费者撞上，那才是 backlog 里
开旋钮的触发条件，不是现在猜；超 1 MiB 的多半是文件上传，而上传本就走
external 端点。

生成的 handler 开 **`DisallowUnknownFields`**：未知/Immutable 字段出现在
body → 400 点名字段，绝不静默丢弃（ADR-0001 精神）。已知残余：字段名来自
解析 `encoding/json` 的错误文本（`json: unknown field %q`，非契约格式）——
实现必须容忍提取失败，降级为不带 `field` 成员的 400，绝不因提取失败改判
其他状态码。

**并发写：本版无乐观锁，丢失更新是已知边界**（owner 裁决，2026-08-08）。
两个客户端并发 `PATCH` 同一行的**同一字段**时，后到者无条件覆盖，前者的写
静默丢失——生成的 wiring 是 `UpdateOne(id).Set…().Save(ctx)`，无版本谓词。
受害面确实比 PUT 语义窄：本框架只有部分更新，改不同字段的两个 PATCH 互不
影响。**不做的理由**：乐观锁需要一个版本列，而框架没有合法途径知道哪一列是它
——按名猜 `version`/`updated_at` 正是 #18 退役的那类约定推导，要做就得加第六个
偏离词，而"恰好五个"是 ADR-0006 的骨架；ETag 则需要先定稳定的序列化输入，
而 `Expand` 让响应形状随 schema 变，那是一整块设计。**消费者今天的出路**：
自加版本列，用定制点整单元替换那一步（`With(ent.PatchUserFn(…))`），在里面写
`Where(user.Version(v))` 并对零行受影响返 409——这正是"没有 override 点、
只有整单元替换"设计的兑现处。条目见 §5 显式 defer。

错误分类接线（生成进消费者包，runtime 不认识 ent）：三个判定函数
`IsNotFound` / `IsConstraintError` / `IsValidationError` + 一个字段名
extractor `func(error) (field string, ok bool)`。已知残余：非
`ValidationError` 形态的校验失败（如 required edge clear+set）落 500。

### 2.4 URL 面

- 路径 = **`snake(plural(Name))`**（`UserProfile → /user_profiles`），
  无 Path 覆写旋钮;
- 五端点：`GET /xs` · `POST /xs` · `GET/PATCH/DELETE /xs/{id}`;
- `DeleteBatch` 不上 HTTP 面（service 层专用）;
- 路由用 Go 1.22+ stdlib `http.ServeMux` 方法+路径模式，不自研。

### 2.5 路由清单（owner 裁决，2026-08-08）

`API(client)` 的路由注册以一份**数据形态**的清单为底座，并把它导出：

```go
// runtime（stdlib-only）：
type Route struct {
	Method  string        // "GET" / "POST" / "PATCH" / "DELETE"
	Path    string        // stdlib 模式语法："/users/{id}"
	Handler http.Handler  // 三段式 handler，路径参数经 r.PathValue 读取
}
// 生成：
func (a *API) Routes() []entapi.Route
```

`Mount`/`http.Handler` 整树挂载仍是默认路径（内部就是遍历这份清单注册进
自己的 ServeMux）。清单的存在解决第三方路由器的逐路由原生集成，适配器
契约含两件事：**模式语法转换**（stdlib `{id}` ↔ gin/echo `:id`，两家不认
`{id}`）与**参数值注入**（经 `r.SetPathValue`，Go 1.22+ 标准方法）后调
`Handler`——每框架 ~10 行、消费者侧手写，**框架自身零路由器依赖**。
已知差异如实入档：编码路径段（如 `%2F`）的解码语义各路由器与 stdlib
不一致（评审举证 gin 默认对含 `%2F` 的段 404，v3 §12 X1，实现适配器时
复验），适配器不桥接这个差异。
清单是纯数据导出，不是行为扩展点：自定义实现仍走 `With(...)`，端点增删仍走
`Except`/external，清单只回答"有哪些路由"。

### 2.6 事务边界：签名不动，事务归调用者（owner 裁决，2026-08-08）

定制点与 wiring 一律收 `*ent.Client`，**不设 `*Tx` 变体、不走 tx-from-context**。
跨实体事务用 ent 自己的既有出路：`tx.Client()` 返回的 client 绑定当前事务
（ent 生成物 `tx.go`：`Client returns a Client that binds to current transaction`，
其 `config.driver` 就是 `txDriver`），所以生成的那一步照常可以纳入外部事务：

```go
tx, _ := db.Tx(ctx)
resp, err := ent.PatchUser(ctx, tx.Client(), id, v)   // 生成的这一步在事务里
_, err = tx.Client().AuditLog.Create()./* … */.Save(ctx)
tx.Commit()
```

三条理由：`*Tx` 变体会让每个定制点长出双胞胎签名，而"定制点签名与 wiring 逐字
相同"是整个替换机制的编译期保证；tx-from-context 让"这次调用在不在事务里"变成
不可见的隐式状态，与 ADR-0001 的"沉默即缺陷"反向；事务边界天然属于调用者——
谁开谁提交，框架自己开反而把跨实体逻辑锁死在单实体粒度。

**残留代价，如实入档**：`ent.API(client)` 持有根 client，HTTP 路径上的定制点
拿到的 `db` 参数**不是** tx-bound 的。定制点内要事务，须自己 `db.Tx(ctx)` 后用
`tx.Client()` 重新调生成的 wiring 函数。**框架永不生成事务边界**——这是规则，
不是缺口。

---

## 3. 查询面

### 3.1 op-in-value 线格式

过滤参数 = 裸字段名，值 = `op:value`，按**第一个冒号**切分：

```
GET /records?title=ilike:simon&score=gt:30&status=in:active,archived&_sort=created_at:desc,name&_page=2
```

算子词表：裸值(eq)、`ne:`、`gt:`/`ge:`/`lt:`/`le:`、`in:`/`not_in:`（逗号分值）、
`like:`/`ilike:`/`prefix:`/`suffix:`（子串类，Searchable 门控）、
`is_null:`/`not_null:`、`from:`/`to:`/`between:`（gte+lte 糖）。
`json:` 不在本版。

**值解析规则**（自上而下第一条命中即生效）：

1. 裸空值（`?title=`）→ 忽略该过滤参数；显式 `?title=eq:` → 精确匹配空串;
2. 无冒号 → `eq` 字面值;
3. 前缀 ∈ 该字段允许算子集 → 按算子解析;
4. 前缀是全局已知算子但该字段不允许 → **400**（违规显式报 400 不静默，ADR-0005）;
5. 前缀不是任何已知算子 → 整值回落 `eq` 字面值（`12:30`、URL 直接可用）;
6. `eq:` 显式前缀 = 字面值转义写法。

**值转不出目标类型 = 400，点名字段与值**（owner 裁决，2026-08-08；ADR-0013）。
上面六条规则只判**前缀**——值本身的解析失败此前只在 ADR-0007 的 Consequences
里带过一句（"typed fields still fail value-parse to 400"），规则表与正文均未
规定，闭集成员（enum/uuid）完全未覆盖。本条把它提升为规范并扩展到闭集：
`?score=gt:abc`（int）、
`?status=eq:banana`（非合法 enum 成员）、`?created_at=from:2026/08/08`
（非 RFC 3339）、`?id=in:not-a-uuid` 一律 400。
**不沿用 entigo 的静默跳过**（`tag_filter.go:154-165` 写的是
`if …; err == nil { qb.And(…) }`，解析失败就不加该谓词）——静默跳过是
fails-open 的教科书形态：过滤条件消失意味着**返回的行比调用者以为的多**，
在带授权的部署里那是数据泄漏的直接路径，而调用者收到的是 200 和一页看起来
正常的数据。这与本设计已定的三条同则（非法 `_sort` 字段 400、不允许的算子
前缀 400、`_q` 打在零 Searchable 实体 400）。enum 尤其要 400：合法成员集在
codegen 期完全已知，生成一次 `switch` 判定，零运行期成本。
**与下条的关系**：重复 `eq` 的"恒空"是**一致规则的自然后果**（每个谓词都成立，
只是交集为空）；这里不同——**谓词凭空消失**不是任何规则的后果，是解析器吞掉了
输入。**代价**：依赖"传坏值等于不过滤"的 entigo 前端升级后从 200 变 400，
属行为破坏，须与下条的 `IN`→`AND` 分道写在同一处迁移说明里。

**重复同名过滤参数 = AND 合并**（owner 裁决，2026-08-08；v3 §8.7 原标"实现时
定"；ADR-0013）。每次出现都是一个独立谓词，全部 AND 到一起，于是
`?score=gt:30&score=le:50` 这种**半开区间**可表达——`between:`（gte+lte 闭区间）
覆盖不到它。不选 OR 的理由：OR 已有头等拼写（`?status=in:a,b`），重复参数再
承担一次 OR 就是同一语义两条路，而 `_` 命名空间那条裁决的骨架正是"唯一写法、
无别名"。
**与 entigo 传统分道，如实入档**：entigo 对多值参数生成 `IN (?)`
（`tag_filter.go:136-139`），即重复同名 = OR——那是**没有 `in:` 算子**年代的
形态。因此 `?status=active&status=archived` 在 entigo 下是 `IN('active','archived')`，
在 entapi 下是 `status='active' AND status='archived'` ⇒ **恒空结果集**，
不报错不告警。这是从 entigo 迁移时最可能踩的一脚，迁移说明必须点名。
特判"多个 `eq` → 400"被否决：那是往一致规则上打补丁，正是"加 if 分支"
而非"消除特殊情况"。

每字段的允许算子集在 **codegen 期**从 `$field.Ops` ∩ 三维度标记算出，生成为
parse switch——没有运行期算子表。解析结果写入保留的类型化
`{Entity}Filter` 结构体，wiring/自定义实现/service 层 API 不变。

已知限制：`in:`/`between:` 值内逗号不可表达（百分号编码在 `r.URL.Query()`
处已被解开，救不了）；OpenAPI 过滤参数为 `type: string` + `pattern` +
description 枚举前缀集。时间值线格式为 **RFC 3339**（与 `encoding/json`
的 `time.Time` 一致）——值内无逗号，不触上述限制。

### 3.2 保留参数：`_` 前缀命名空间

**`_` 开头 = 框架的，裸名 = schema 的。** 恰四个保留参数，唯一写法、无别名：

- **`_sort=field:desc,field2`**——逗号多字段，裸字段名即 asc；白名单只来自
  `Sortable`；非法字段 **400**（不静默跳）；PK tiebreak 追加在末尾，
  **除非 PK 已出现在排序列表任一位置**（ADR-0002 批准约束的列表化推广：
  永不 `ORDER BY id, …, id`）;
- **`_page` / `_size`**——1 起数，默认 20，上限 1000，钳制在唯一入口。
  查询串里**显式** `_size=0` 或负值 → **400**：`0` 的线值预留给未来的
  count-only 语义（arbitrate 两席一致，2026-08-08，v3 §12.5——先钳制后改义
  会破坏已依赖钳制的消费者，pre-release 预留是免费的）；Go 层 `ListRequest`
  的修复语义不变（`Limit()` 永不返回 0 的不变量不动，presence 在 HTTP
  解析层区分）;
- **`_q`**——对全部 Searchable 字段 Contains 后 OR 的自由文本析取；
  打在零 Searchable 字段的实体上 → **400**（与 `_sort` 非法字段同则，
  不静默吞）。

叫 `size`/`sort`/`q` 的字段照常过滤。`ListRequest` v2：排序项列表化，
`sort_by`/`order` 字段退役。

---

## 4. 周边接管

### 4.1 OpenAPI

- 生成到消费者 **`ent/openapi.yaml`**，随生成物 commit（PR diff 即暴露面
  审查）；首行自带 `# Code generated by entapi extension` 注释 marker，
  cleanup 按该文件自己的 marker 决定可删性（ADR-0004 一致，删 marker 即接管）。
  改名前的 legacy marker（`Code generated by entdomain extension`）**永久**
  被 cleanup 识别（ADR-0011，已实现于 `cleanup.go`），旧生成物不会变孤儿;
- 生成 `entapi_openapi.go`：`go:embed` 该 yaml 并注册 `GET /openapi.yaml`
  ——磁盘与服务内容同源;
- 版本 **OpenAPI 3.1**;
- **`servers` 一律不生成**，spec 路径保持无前缀（`/users`）（owner 裁决，
  2026-08-08）。挂载前缀是**部署期事实**——`http.StripPrefix` 发生在消费者的
  `main.go`，而 yaml 在 `go generate` 时就写死了；同一份生成物可以同时挂在
  `/api/v1` 和裸根上。生成一个必然可能错的 `servers` 比不生成更糟；
  3.1 缺省 `servers` 即 `[{"url":"/"}]`，客户端按 spec 所在源解析相对路径，
  这是最不会说谎的语义。**代价**：需要前缀出现在 spec 里的消费者（把 yaml
  直接喂 Postman / 前端 codegen）本版无出路，只能删 marker 接管该文件
  （ADR-0004 逃生舱）;
- **`info.title` / `info.version` 走 entc.go 的生成期选项**，即
  `ExtensionConfig` 新增两个字段（`OpenAPITitle` / `OpenAPIVersion`），
  缺省 `"<ent 包名> API"` 与 `"0.0.0"`。`ExtensionConfig` 已是本仓唯一的
  生成期配置入口（今天只有 `EntAPIPackage`），这是**沿已有的缝**而不是开新缝；
  关键是它不碰 `With` 家族——`With` 的参数保持"逐操作替换函数"这一个族
  （与 §2.3 拒绝 body 上限旋钮同一条理由）。`version` 不从 git tag 猜：
  codegen 期读工作树状态会立刻破掉"clean checkout 跑完测试 `git status`
  干净"这条既有不变量。

### 4.2 唯一键分类：接口探测 + 文本兜底

`runtime` 判定函数（stdlib-only）：**主通道**沿 `errors.As`/`Unwrap` 链探测
`interface{ SQLState() string }`（pgx v5 与 lib/pq **≥ v1.10.6** 实现；
更旧的 lib/pq 无此方法，落文本兜底）比对 `23505`；MySQL 探测
错误号能力；探测不到回落文本匹配（`duplicate key value`、`Error 1062`/
`Duplicate entry`、`UNIQUE constraint failed`）。生成的 `API(client)` 构造时
按 `client.driver.Dialect()` 装配；未知方言不装（重复键回 500）；
`WithUniqueViolation` 逃生舱保留。

### 4.3 软删除：生成 init 注册（spike 门控）

框架把注册代码生成进消费者 ent 包，`init()` 自动挂接——消费者零仪式，
对**所有**持 client 的进程生效（HTTP、cron、测试一视同仁）。**机制约束**
（第二轮评审证实，v3 §12 X2/X3）：`init()` 运行时尚无任何 `*Client` 实例，
且 hooks/inters 是每实例状态（`newConfig` 每次新建）——init 能挂的不是
client，而是 **mutation 期被咨询的间接层**（如 mixin 声明的 hook 槽，
经 `m.Client()` 在 mutation 时取回类型化 client）。spike 验证四个场景：
**挂接点存在性**（init 填充、mutation 期消费的间接层是否成立）、挂接时机、
多 client、**与 ent privacy 共存**（deny-by-default 的 privacy 规则不得
拒掉软删除 hook 自己的读写——竞对对比评审点名的最易踩的坑）。失败回落显式
`RegisterSoftDelete(client)`。注册永不放进 `API()`/Mount：软删除语义不得
取决于建没建 HTTP 面。

### 4.4 行级授权（OwnedBy）：出界，只立边界

本版不设计。约束性边界六条（后三条来自 2026-08-08 竞对对比评审，与 entrest
的 ent-privacy 实践对齐——它与前三条方向相同，差别只在有可跑示例）：

1. 落点必须是 ent 扩展点（interceptor/privacy）;
2. "当前用户"契约走 runtime 的 stdlib context 取值函数（认证 middleware
   写入 context，在 Mount 外面）——**这一对函数本版即随 §2.1 落地**
   （`WithActor`/`ActorFrom`），本版框架自身不读它，读者是消费者的 privacy
   规则与定制点;
3. 永不生成进 wiring/handler;
4. **非请求路径的旁路契约必须显式规定**：cron/migration/测试持 client 无
   身份，privacy deny-by-default 会全线报错——旁路写法（ent 的
   `privacy.DecisionContext(ctx, privacy.Allow)` 或等价物）是契约的一部分;
5. 与 §4.3 软删除生成 init 的**共存**列入软删除 spike 验证场景;
6. privacy 过滤行之后 `ListPage` 的 `total` 是否仍正确——**待测试证据**，
   不推断（理论上 count 与 data 同源）。

---

## 5. 落地顺序、实现前必决、backlog 与显式 defer

**实现 arc 已开（2026-08-08）**：epic
[#68](https://github.com/githonllc/entapi/issues/68) 下八个垂直切片——
[#69](https://github.com/githonllc/entapi/issues/69) `Sensitive()` 收窄 ·
[#70](https://github.com/githonllc/entapi/issues/70) 软删除 spike ·
[#71](https://github.com/githonllc/entapi/issues/71) `entapi/api` 注解模型 ·
[#72](https://github.com/githonllc/entapi/issues/72) 查询面 ·
[#73](https://github.com/githonllc/entapi/issues/73) HTTP 打通 ·
[#74](https://github.com/githonllc/entapi/issues/74) 错误分类 ·
[#75](https://github.com/githonllc/entapi/issues/75) `With` 与 `Routes()` ·
[#76](https://github.com/githonllc/entapi/issues/76) OpenAPI。
下面的 backlog 八项**未建 issue**（有意为之：它们不阻塞第一片，现在建就是
永远排不上的 open issue），第一片落地后再开。

**第一片**：`Sensitive()` 从响应消失——最小可证伪推导切片，纯收窄（#69）。
**第二片**：软删除生成 init spike（#70；同时为 OwnedBy arc 前置验证，场景含
privacy 共存，§4.3）。它不碰注解模型，因此与主线**并行**，不是主线的下一环。

**实现前必决**（2026-08-08 竞对/service 接入评审暴露）：**已全部裁决**，
两项分别见 §2.2（`With` = Functional Options，ADR-0012）与 §2.6（事务边界）。

**backlog**（源自 entrest 对比评审的偷师清单，`docs/COMPARISON-entrest.md`；
按采纳态度排序，均为增量、不阻塞第一片）：

1. **ErrorHandler 观测钩子**（倾向采纳，形态已定稿）：挂第 3 步（写）不动
   第 2 步；收**已分类结果 + 原始 error**（分类留在消费者包，runtime 不认识
   ent）；**观测/替换两档**（观测档不可改响应；替换档自负 RFC 9457 合规）；
   默认写出器公开可回调；连带 `MaskErrors` 等价物（500 的 `detail` 防泄漏）；
   **全局一个 + op 参数**，不做逐操作（避免与定制点并行两套逐操作机制）;
2. **边端点** `GET /users/{id}/pets`（倾向采纳：不与"Summary 不带边"深度
   上限冲突）;
3. **spec 精修**：基础 spec 合并 + example/schema 覆写——比"全生成 vs 删
   marker 接管"柔和一档的中间态;
4. **生成测试包**（entrest `WithTesting`/`resttest` 同类）;
5. **内置 docs UI**（yaml 已 embed，边际成本低）;
6. **字段组 OR 过滤**（`WithFilterGroup` 同类，仍是生成期白名单）;
7. **HTTP 层生成开关**（原名"生成但不挂载"）——`Routes()` 清单已覆盖"生成但
   不挂载"那半边；真正的缺口是"整个不生成"，触发条件是出现只要 DTO+wiring
   的消费者（纯 gRPC/CLI），届时 §2.1 第 3 条的四条代价从理论变现实;
8. **count-only 列表**（`_size=0` 只回 `total`，源自第二轮评审 A3 残核 +
   arbitrate 裁决）——线值已预留（§3.2 显式 0 → 400），落地时定义语义。

显式 defer：**并发控制（ETag/`If-Match`/乐观锁）——本版无，丢失更新是已知
边界，明账在 §2.3**；**Summary 的字段收窄**（需要新注解，§1.2）；
`json:` 算子；软删除 HTTP 语义（restore/include_deleted）；
反向表第 5 行专用词；迁移说明落点（#23 收口时定）；跨边过滤与
OwnedBy（出界，§4.4）。

---

## 附录：样例——账号系统的 User

```go
type User struct{ ent.Schema }

func (User) Mixin() []ent.Mixin { return []ent.Mixin{entapi.SoftDeleteMixin{}} }

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New), // 不声明则 ent 默认 int，与下文 uuid.UUID 签名不符
		field.String("name").Annotations(api.Searchable(), api.Sortable()),
		field.String("email").Unique().Annotations(api.Filterable()),
		field.String("password_hash").Annotations(api.Hidden()),
		field.Enum("status").Values("active", "suspended").Default("active").
			Annotations(api.Filterable()),
		field.Time("created_at").Default(time.Now).Immutable().
			Annotations(api.ReadOnly(), api.Sortable()),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).
			Annotations(api.ReadOnly()),
	}
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource().Except(api.OpCreate)}
}
```

生成端点：`GET /users`、`GET/PATCH/DELETE /users/{id}`、`GET /openapi.yaml`。
`POST /users` 被 Except——注册不是 CRUD（请求体是 `{email,password}` + 哈希），
走自定义端点（external）；若漏写 Except，`Hidden` × 必填 × OpCreate 命中拒绝矩阵，
生成期失败并给出修复行。
此处 `Except(OpCreate)` 落在 §1.1 的**第二分支**：`password_hash` 是 ent 必填
（无 `Optional`/无 `Default`）又被 `Hidden` 挡在请求外，create 面不可用，
于是**整族不生成**——`Register` 因此直接用 ent 构建器的 `SetPasswordHash`，
而不是复用 `CreateUser`。若该实体没有这类被挡住的必填字段，`Except(OpCreate)`
就只关端点与 `CreateUserFn`，wiring 与请求照常保留。

**用 service 自定义实现（IoC = 闭包捕获，方法值就是最惯用的闭包）**：service 是
普通结构体，依赖随便注，框架对它的形状零规定；方法与定制点签名相同时，
方法值直接塞进 `With`——receiver 被方法值捕获，这就是全部的"注入"：

```go
type UserService struct{ mailer Mailer; audit AuditLog }

func (s *UserService) Patch(ctx context.Context, db *ent.Client,
	id uuid.UUID, v *ent.ValidUserPatchRequest) (*ent.UserResponse, error) {
	resp, err := ent.PatchUser(ctx, db, id, v)       // 生成的那一步照用
	if err == nil && v.HasStatus() {
		s.mailer.NotifyStatusChange(ctx, id)          // 你的业务副作用
	}
	return resp, err
}
```

**横切（AOP）走 `http.Handler` 洋葱，跑在三段式 handler之前**——身份认证、header
检查对生成的 handler 是全透明的，401 短路时绑参根本不发生；身份经 runtime
的 context 契约（§4.4 边界 2）向内传给定制点与 ent privacy：

```go
func withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := verifyToken(r.Header.Get("Authorization"))
		if err != nil {
			entapi.WriteProblem(w, http.StatusUnauthorized, "unauthorized", err)
			return                                    // 短路：不进绑参
		}
		next.ServeHTTP(w, r.WithContext(entapi.WithActor(r.Context(), u.ID)))
	})
}

func main() {
	svc := &UserService{mailer: mailer, audit: audit}
	api := ent.API(client).With(ent.PatchUserFn(svc.Patch))   // 自定义实现：方法值即闭包

	var h http.Handler = api                          // 洋葱：内层先写，外层先跑
	h = withHeaderCheck(h)
	h = withAuth(h)

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", h))  // 整树挂载（默认路径）
	mux.HandleFunc("POST /register", Register(client)) // external：生成的 SetPasswordHash/NewUserResponse/ErrorMap 照用
	mux.HandleFunc("POST /login", Login(client))
	http.ListenAndServe(":8080", mux)
}
```

**按路由选择性横切**（与整树挂载二选一）——`Routes()` 清单按元数据选切点：

```go
for _, rt := range api.Routes() {
	h := rt.Handler
	if rt.Method == "DELETE" { h = requireRole("admin", h) }  // 只有删除要求 admin
	mux.Handle(rt.Method+" "+rt.Path, h)
}
```

三种定制方式在此各就各位且互不越界：middleware 管**所有端点**的横切（认证/header/
审计），自定义实现管**单个操作**的业务替换，external 管**非 CRUD 动词**。框架不
提供注解式拦截器链——切面栈就是 main.go 里这几行显式包裹，一眼可读。
