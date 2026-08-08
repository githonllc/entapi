# entapi v3 定稿：沉默即默认的注解模型 + 生成的 HTTP 层

> **状态：定稿（2026-08-08），实现未开始。**
>
> 本文只陈述最终设计。裁决过程、被推翻的判断、跨家族评审记录在
> `DESIGN-v3.md`（§6/§10）；术语表在根目录 `CONTEXT.md`；约束性决策的
> 单条记录在 `docs/adr/0006`–`0010`。冲突时以 ADR 为准。
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
| `ReadOnly` × ent 必填且无 Default | 拒绝（创建必然失败） |
| `Hidden` × ent 必填且无 Default，且 `OpCreate` 未 Except | 拒绝（同上；修复：`Except(OpCreate)` 或给 Default/Optional） |
| 全字段 Immutable 且未 `Except(OpPatch)` | 拒绝（空 PATCH 端点） |
| `Expand` → 非 Resource | 拒绝 |
| `_` 开头字段名进查询面 | 拒绝（保留命名空间） |
| `ReadOnly` × 查询三维度 | **允许** |

---

## 2. HTTP 层

### 2.1 拓扑

1. 生成的 handler、`API(client)`、路由注册落进消费者的 **`ent` 包**
   （与 DTO/wiring 同落点，marker/cleanup/保留名检查复用）;
2. 机械运行时助手（problem+json 写出、状态码映射、解析器共享部分）进
   **`entapi/runtime`**——`net/http` 是 stdlib，stdlib-only 不变量不破;
3. 入口：**`ent.API(client)` 返回 `http.Handler`**，
   `mux.Handle("/", ent.API(client))` 或便捷方法 `.Mount(mux)`。

### 2.2 三步体与替换实现

生成的 handler 永远三步：绑 → 调一个函数 → 写。**没有 override 点。**
第 2 步是替换点，**类型与生成的 wiring 函数签名逐字相同**，默认值就是
wiring 函数本身：

```go
ent.API(client).With(ent.CreateArticleFn(myCreate))
// myCreate: func(ctx, *ent.Client, *ent.ValidArticleCreateRequest) (*ent.ArticleResponse, error)
```

`With` 的组合语义 = Go **Functional Options**（owner 裁决，2026-08-08）：
变参且链式等价（`With(a,b).With(c)` ≡ `With(a,b,c)`）；重复设置同一替换点
**后写覆盖**（last-wins，支撑"先默认后条件覆写"的惯用组装）；唯一例外：
**nil 函数值构造期拒绝**——nil 替换函数不可能是有意的，静默落回默认是被
ADR-0001 判死的那类沉默。

**external 门零机制**：非 CRUD 端点由消费者直接注册在自己的 mux 上
（stdlib mux 最长匹配）。框架永不生成 Service 骨架。

**替换实现不是中间件，横切有自己的门。** 替换实现是逐操作的整单元替换；要对**所有**
端点统一加身份认证、header 检查、审计、租户注入这类横切关注，用 Go 的标准
AOP 形态——`http.Handler` 洋葱组合：整树包裹 `auth(ent.API(client))`，或经
`Routes()` 清单按 `Method`/`Path` 元数据**选择性逐路由包裹**。身份经
`context.Context` 向内传递（middleware 写入 → 替换点/wiring/ent privacy 读取），
context 就是这里的 IoC 容器。框架不提供注解式拦截器链——显式组合优于容器
魔法，且三步体内部没有注入点是规则不是缺口（第 3 步的 ErrorHandler 观测钩子
见 §5 backlog）。service 接入同理零机制：替换点签名里没有 receiver 位置，
依赖注入靠闭包捕获——先构造你的 service，再用匿名函数包成替换点类型。

### 2.3 错误与状态码

错误一律 **RFC 9457 problem+json**（带 `field` 扩展成员）；成功响应裸
DTO / `Page` 四字段（`{"data","total","page","size"}`），不包信封。

`201` Create · `200` Get/List/Patch · `204` Delete · `400` JSON/查询参数非法 ·
`404` not-found · `409` 唯一键冲突 · `422` 校验失败 · `500` 未分类
（含未装唯一键判定时的重复键——宁 500 不错答 409）。

生成的 handler 开 **`DisallowUnknownFields`**：未知/Immutable 字段出现在
body → 400 点名字段，绝不静默丢弃（ADR-0001 精神）。

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
	Handler http.Handler  // 三步体，路径参数经 r.PathValue 读取
}
// 生成：
func (a *API) Routes() []entapi.Route
```

`Mount`/`http.Handler` 整树挂载仍是默认路径（内部就是遍历这份清单注册进
自己的 ServeMux）。清单的存在解决第三方路由器的逐路由原生集成：gin/echo
适配器把自家路径参数注入回 `r.SetPathValue`（Go 1.22+ 标准方法）再调
`Handler`——每框架 ~10 行、消费者侧手写，**框架自身零路由器依赖**。
清单是纯数据导出，不是行为扩展点：替换实现仍走 `With(...)`，端点增删仍走
`Except`/external，清单只回答"有哪些路由"。

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
4. 前缀是全局已知算子但该字段不允许 → **400**（门控大声，ADR-0005）;
5. 前缀不是任何已知算子 → 整值回落 `eq` 字面值（`12:30`、URL 直接可用）;
6. `eq:` 显式前缀 = 字面值转义舱。

每字段的允许算子集在 **codegen 期**从 `$field.Ops` ∩ 三维度标记算出，生成为
parse switch——没有运行期算子表。解析结果写入保留的类型化
`{Entity}Filter` 结构体，wiring/替换实现/service 层 API 不变。

已知限制：`in:`/`between:` 值内逗号不可表达（百分号编码在 `r.URL.Query()`
处已被解开，救不了）；OpenAPI 过滤参数为 `type: string` + `pattern` +
description 枚举前缀集。

### 3.2 保留参数：`_` 前缀命名空间

**`_` 开头 = 框架的，裸名 = schema 的。** 恰四个保留参数，唯一写法、无别名：

- **`_sort=field:desc,field2`**——逗号多字段，裸字段名即 asc；白名单只来自
  `Sortable`；非法字段 **400**（不静默跳）；PK tiebreak 永远追加在末尾
  （ADR-0002）;
- **`_page` / `_size`**——1 起数，默认 20，上限 1000，钳制在唯一入口;
- **`_q`**——对全部 Searchable 字段 Contains 后 OR 的自由文本析取。

叫 `size`/`sort`/`q` 的字段照常过滤。`ListRequest` v2：排序项列表化，
`sort_by`/`order` 字段退役。

---

## 4. 周边接管

### 4.1 OpenAPI

- 生成到消费者 **`ent/openapi.yaml`**，随生成物 commit（PR diff 即暴露面
  审查）；首行自带 `# Code generated by entapi extension` 注释 marker，
  cleanup 按该文件自己的 marker 决定可删性（ADR-0004 一致，删 marker 即接管）;
- 生成 `entapi_openapi.go`：`go:embed` 该 yaml 并注册 `GET /openapi.yaml`
  ——磁盘与服务内容同源;
- 版本 **OpenAPI 3.1**。

### 4.2 唯一键分类：接口探测 + 文本兜底

`runtime` 判定函数（stdlib-only）：**主通道**沿 `errors.As`/`Unwrap` 链探测
`interface{ SQLState() string }`（pgx/lib/pq 均实现）比对 `23505`；MySQL 探测
错误号能力；探测不到回落文本匹配（`duplicate key value`、`Error 1062`/
`Duplicate entry`、`UNIQUE constraint failed`）。生成的 `API(client)` 构造时
按 `client.driver.Dialect()` 装配；未知方言不装（重复键回 500）；
`WithUniqueViolation` 逃生舱保留。

### 4.3 软删除：生成 init 注册（spike 门控）

框架把注册代码生成进消费者 ent 包（生成代码可命名 `*Client` 与实体 hook 列表），
`init()` 自动挂接——消费者零仪式，对**所有**持 client 的进程生效（HTTP、
cron、测试一视同仁）。spike 验证三个场景：挂接时机、多 client、
**与 ent privacy 共存**（deny-by-default 的 privacy 规则不得拒掉软删除 hook
自己的读写——竞对对比评审点名的最易踩的坑）。失败回落显式
`RegisterSoftDelete(client)`。注册永不放进 `API()`/Mount：软删除语义不得
取决于建没建 HTTP 面。

### 4.4 行级授权（OwnedBy）：出界，只立边界

本版不设计。约束性边界六条（后三条来自 2026-08-08 竞对对比评审，与 entrest
的 ent-privacy 实践对齐——它与前三条方向相同，差别只在有可跑示例）：

1. 落点必须是 ent 扩展点（interceptor/privacy）;
2. "当前用户"契约走 runtime 的 stdlib context 取值函数（认证 middleware
   写入 context，在 Mount 外面）;
3. 永不生成进 wiring/handler;
4. **非请求路径的旁路契约必须显式规定**：cron/migration/测试持 client 无
   身份，privacy deny-by-default 会全线报错——旁路写法（ent 的
   `privacy.DecisionContext(ctx, privacy.Allow)` 或等价物）是契约的一部分;
5. 与 §4.3 软删除生成 init 的**共存**列入软删除 spike 验证场景;
6. privacy 过滤行之后 `ListPage` 的 `total` 是否仍正确——**待测试证据**，
   不推断（理论上 count 与 data 同源）。

---

## 5. 落地顺序、实现前必决、backlog 与显式 defer

**第一片**：`Sensitive()` 从响应消失——最小可证伪推导切片，纯收窄。
**第二片**：软删除生成 init spike（同时为 OwnedBy arc 前置验证，场景含
privacy 共存，§4.3）。

**实现前必决**（2026-08-08 竞对/service 接入评审暴露，未决会冻进契约）：

- **事务边界**：替换点与 wiring 收 `*Client` 不收 `*Tx`——跨实体事务性逻辑
  无法把生成的那一步纳入自己的事务。需定：提供 `*Tx` 变体、或统一走 ent
  的 tx-from-context 惯例、或如实入档"生成面不参与外部事务"。
  （原并列的第二项"`With` 组合语义"已裁决为 Functional Options，见 §2.2。）

**backlog**（源自 entrest 对比评审的偷师清单，`docs/COMPARISON-entrest.md`；
按采纳态度排序，均为增量、不阻塞第一片）：

1. **ErrorHandler 观测钩子**（倾向采纳，形态已定稿）：挂第 3 步（写）不动
   第 2 步；收**已分类结果 + 原始 error**（分类留在消费者包，runtime 不认识
   ent）；**观测/替换两档**（观测档不可改响应；替换档自负 RFC 9457 合规）；
   默认写出器公开可回调；连带 `MaskErrors` 等价物（500 的 `detail` 防泄漏）；
   **全局一个 + op 参数**，不做逐操作（避免与替换点并行两套逐操作机制）;
2. **边端点** `GET /users/{id}/pets`（倾向采纳：不与"Summary 不带边"深度
   上限冲突）;
3. **spec 精修**：基础 spec 合并 + example/schema 覆写——比"全生成 vs 删
   marker 接管"柔和一档的中间态;
4. **生成测试包**（entrest `WithTesting`/`resttest` 同类）;
5. **内置 docs UI**（yaml 已 embed，边际成本低）;
6. **字段组 OR 过滤**（`WithFilterGroup` 同类，仍是生成期白名单）;
7. **"生成但不挂载"第三档逃生舱**（`Routes()` 清单已覆盖大半需求，优先级最低）。

显式 defer：`json:` 算子；软删除 HTTP 语义（restore/include_deleted）；
反向表第 5 行专用词；重复同名过滤参数语义（实现时定）；挂载前缀与 spec
`info` 元数据（实现时定）；迁移说明落点（#23 收口时定）；跨边过滤与
OwnedBy（出界，§4.4）。

---

## 附录：样例——账号系统的 User

```go
type User struct{ ent.Schema }

func (User) Mixin() []ent.Mixin { return []ent.Mixin{entapi.SoftDeleteMixin{}} }

func (User) Fields() []ent.Field {
	return []ent.Field{
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
走 external 门；若漏写 Except，`Hidden` × 必填 × OpCreate 命中拒绝矩阵，
生成期失败并给出修复行。

**用 service 替换实现（IoC = 闭包捕获，方法值就是最惯用的闭包）**：service 是
普通结构体，依赖随便注，框架对它的形状零规定；方法与替换点签名相同时，
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

**横切（AOP）走 `http.Handler` 洋葱，跑在三步体之前**——身份认证、header
检查对生成的 handler 是全透明的，401 短路时绑参根本不发生；身份经 runtime
的 context 契约（§4.4 边界 2）向内传给替换点与 ent privacy：

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
	api := ent.API(client).With(ent.PatchUserFn(svc.Patch))   // 替换实现：方法值即闭包

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

三扇门在此各就各位且互不越界：middleware 管**所有端点**的横切（认证/header/
审计），替换实现管**单个操作**的业务替换，external 管**非 CRUD 动词**。框架不
提供注解式拦截器链——切面栈就是 main.go 里这几行显式包裹，一眼可读。
