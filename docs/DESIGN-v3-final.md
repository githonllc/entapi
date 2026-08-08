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

### 2.2 三步体与换脑

生成的 handler 永远三步：绑 → 调一个函数 → 写。**没有 override 点。**
第 2 步是换脑槽位，**类型与生成的 wiring 函数签名逐字相同**，默认值就是
wiring 函数本身：

```go
ent.API(client).With(ent.CreateArticleFn(myCreate))
// myCreate: func(ctx, *ent.Client, *ent.ValidArticleCreateRequest) (*ent.ArticleResponse, error)
```

**external 门零机制**：非 CRUD 端点由消费者直接注册在自己的 mux 上
（stdlib mux 最长匹配）。框架永不生成 Service 骨架。

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
`{Entity}Filter` 结构体，wiring/换脑/service 层 API 不变。

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

框架把注册代码生成进消费者 ent 包（生成代码可命名 `*Client` 与实体 hook 槽），
`init()` 自动挂接——消费者零仪式，对**所有**持 client 的进程生效（HTTP、
cron、测试一视同仁）。spike 验证挂接时机与多 client 场景；失败回落显式
`RegisterSoftDelete(client)`。注册永不放进 `API()`/Mount：软删除语义不得
取决于建没建 HTTP 面。

### 4.4 行级授权（OwnedBy）：出界，只立边界

本版不设计。三条约束性边界：落点必须是 ent 扩展点（interceptor/privacy）；
"当前用户"契约走 runtime 的 stdlib context 取值函数；永不生成进
wiring/handler。

---

## 5. 落地顺序与显式 defer

**第一片**：`Sensitive()` 从响应消失——最小可证伪推导切片，纯收窄。
**第二片**：软删除生成 init spike（同时为 OwnedBy arc 前置验证）。

显式 defer：`json:` 算子；软删除 HTTP 语义（restore/include_deleted）；
反向表第 5 行专用词；重复同名过滤参数语义（实现时定）；挂载前缀与 spec
`info` 元数据（实现时定）；迁移说明落点（#23 收口时定）。

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

```go
mux := http.NewServeMux()
ent.API(client).Mount(mux)
mux.HandleFunc("POST /register", Register(client))   // external：生成的 SetPasswordHash/NewUserResponse/ErrorMap 照用
mux.HandleFunc("POST /login", Login(client))
```

只改一步而不换请求形状时用换脑：

```go
ent.API(client).With(ent.PatchUserFn(func(ctx context.Context, db *ent.Client,
	id uuid.UUID, v *ent.ValidUserPatchRequest) (*ent.UserResponse, error) {
	resp, err := ent.PatchUser(ctx, db, id, v)
	if err == nil && v.HasStatus() { notify(ctx, id) }
	return resp, err
}))
```
