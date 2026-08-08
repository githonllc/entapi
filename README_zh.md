# EntAPI

[![Go Reference](https://pkg.go.dev/badge/github.com/githonllc/entapi.svg)](https://pkg.go.dev/github.com/githonllc/entapi)
[![Go Report Card](https://goreportcard.com/badge/github.com/githonllc/entapi)](https://goreportcard.com/report/github.com/githonllc/entapi)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

一个 [Ent](https://entgo.io) 扩展。给实体标上 `api.Resource()`，它就生成请求类型、响应
类型、查询面、每个操作一个接线函数，以及标准库 HTTP 路由树——全部写进你自己的 `ent`
包，链接的运行时除标准库外不依赖任何东西。字段形态来自 Ent；注解只表达偏离。

*[English](README.md)*

```go
// schema/article.go —— 你写这个
field.String("title").
    Annotations(api.Searchable(), api.Filterable(), api.Sortable())

func (Article) Annotations() []schema.Annotation {
    return []schema.Annotation{api.Resource()}
}
```

```go
// main.go —— 你得到这个入口
http.ListenAndServe(":8080", ent.API(client))
```

这两个声明之间，生成器写出了带三态存在性的 `ArticleCreateRequest`、把 query 解析为强类型
`ArticleFilter` 的 `ParseArticleQuery`、多键排序白名单、带预加载计划的 `ArticleResponse`，
以及错误分类、五个三步 handler 和 `ent.API(client)` 背后的路由 manifest。

> ### 状态：v0，从未发布过版本
>
> `git tag` 为空——这个仓库没有打过任何 tag，也没有对外承诺过任何 API。版本策略是 Go 自己
> 的 `v0.x` 约定：**自由破坏，不设弃用窗口**，删除的符号在本文末尾的迁移注记里点名，不留
> 兼容别名。
>
> 代码本身是完整的：`docs/DESIGN-v2.md` 提出的重设计（删除基类、parse-don't-validate、
> 边走 `OrErr()`、标记扫描清理、错误映射手写在 runtime、只发 offset 分页）**已经全部落地**，
> 唯三的偏离项列在[与 DESIGN-v2 的偏离](#与-design-v2-的偏离)。已知缺陷见
> [`docs/QUALITY-REVIEW.md`](docs/QUALITY-REVIEW.md)。
>
> 采用前请先读[陷阱](#陷阱)。其中有几条是静默的。

---

## 目录

- [安装](#安装) · [三个 import 路径](#三个-import-路径) · [接入](#接入)
- [注解模型](#注解模型)——Ent 事实加五个偏离词
- [生成了什么](#生成了什么)
- [生成的 HTTP](#生成的-http)
- [请求：三态存在性](#请求三态存在性)
- [响应、摘要与边](#响应摘要与边)
- [查询面](#查询面)——过滤、全文、排序、分页
- [接线与错误映射](#接线与错误映射)
- [软删除](#软删除)
- [生成会失败，而这正是设计](#生成会失败而这正是设计)
- [生成器对你的目录做了什么](#生成器对你的目录做了什么)
- [字段形态](#字段形态) · [被接受但不被消费的](#被接受但不被消费的)
- [陷阱](#陷阱) · [限制](#限制) · [与 DESIGN-v2 的偏离](#与-design-v2-的偏离) · [迁移注记](#迁移注记)

---

## 安装

```bash
go get github.com/githonllc/entapi
```

`go.mod` 声明 `go 1.23`，唯一的非 `golang.org/x` 直接依赖是 `entgo.io/ent v0.14.4` 与
`github.com/google/uuid v1.3.0`。

> **实现：** `go.mod`

## 三个 import 路径

一个 module，三个包，按**代码何时运行**切分。

| Import | 被谁 import | 主要符号 |
|---|---|---|
| `github.com/githonllc/entapi` | 你的 `entc.go`；嵌软删除的 schema | `Extension`、`SoftDeleteMixin` |
| `github.com/githonllc/entapi/api` | 你的 **schema** 文件 | `Resource`、`Hidden`、`ReadOnly`、`Searchable`、`Filterable`、`Sortable`、`Expand` |
| `github.com/githonllc/entapi/runtime` | **生成的代码**与你的 handler / service 代码 | `ListRequest`、`SortSpec`、`Page[R]`、`ListPage`、`GetOne`、`SaveOne`、`WriteProblem`、`FieldError`、`Route`、`WithActor`/`ActorFrom`、错误 sentinel 与 mapper、filter/pointer/软删除 helper |

这个切分是承重的，不是整洁癖：根包用 `//go:embed` 内嵌八个模板，并在**包初始化时**把八份
全部从内嵌文件系统里读出来，读不到就 panic。只要 import 根包，这件事就会发生，无论你是否
真的生成任何东西，而且它连带拖进 `embed`、ent 的 codegen 包和
`golang.org/x/tools/imports`。（解析发生在之后，每次渲染时——加载器返回的是模板源码
`string`。）`runtime/` 只 import 标准库，所以它进得了你的生产二进制而根包不必进。

> **实现：** `template_loader.go` — `//go:embed templates/*.tmpl`、`templateFS`、
> `loadTemplate`、`mustLoadTemplate`（返回 `string`）；
> `template_index.go` — `dtoTemplate`、`filterTemplate`、`wiringTemplate`、
> `handlerTemplate`、`errorMapTemplate`、`httpTemplate`、`softDeleteTemplate`、
> `softDeleteConfigInitTemplate`（八个包级 `var`，全部在 init 时求值）；
> `extension.go` — `renderDTOFile` 及其同类，`template.New(…).Funcs(…).Parse(…)` 真正
> 发生的地方；`runtime/types.go`、`runtime/query.go`、`runtime/errors.go`、
> `runtime/errors_map.go`、`runtime/filter.go`、`runtime/softdelete_context.go`

## 接入

```go
//go:build ignore

package main

import (
    "log"

    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
    "github.com/githonllc/entapi"
)

func main() {
    ext := entapi.NewExtensionWithOptions()

    if err := entc.Generate("./schema", &gen.Config{
        Target:  "../ent",
        Package: "your/module/ent",
    }, entc.Extensions(ext)); err != nil {
        log.Fatal(err)
    }
}
```

`WithEntAPIPackage` 是**仅有的一个**选项，它改写生成文件 import 的 runtime 路径，默认值
就是 `github.com/githonllc/entapi/runtime`，所以只在你 vendor 了一份副本时才有意义。
`NewExtension(cfg)` 直接接受 `*ExtensionConfig` 且对 nil 安全。

扩展只挂一个 `gen.Hook`。`Templates()` 只返回软删除的 `config/init/fields/*` partial；
所有独立输出都由 hook 自己渲染并写盘。

> **实现：** `extension.go` — `Extension`、`ExtensionConfig`、`NewExtension`、
> `NewExtensionWithOptions`、`Option`、`WithEntAPIPackage`、`defaultEntAPIPackage`、
> `Hooks`、`Templates`、`Annotations`、`Options`、`ConfigAnnotation`

## 注解模型

`api.Resource()` 是唯一的实体开关。没有它就不生成 EntAPI 文件。
`api.Resource().Except(api.OpCreate, ...)` 移除选中的公开操作面；请求 DTO 和接线函数仍留给
service 层，唯一例外是根本无法工作的 create family。

字段归属默认静默，从 Ent 推导：

| Ent/API 事实 | 生成效果 |
|---|---|
| `Optional`、`Default`、`Nillable` | create 指针与必填性 |
| `Immutable` | 不出现在 PATCH |
| `Sensitive` | 不出现在 response/summary，但仍可写 |
| `api.Hidden()` | 不出现在 create、patch、response、query |
| `api.ReadOnly()` | 不出现在 create、patch；保留 response |
| `api.Searchable()` | 全文与子串查询维度 |
| `api.Filterable()` | 从 Ent 运算符派生结构化谓词 |
| `api.Sortable()` | 进入排序白名单 |

五个字段词共享一个可合并注解；`Annotations(api.Searchable(), api.Sortable())` 经过 Ent 的
序列化 schema loader 后仍保留两个词。所有构造器都是值接收者并返回副本。

### 从 scope 模型迁移

没有兼容别名，按效果迁移旧词汇：

| 旧写法 | 新写法 |
|---|---|
| `DefaultField()` | 无字段注解 |
| `InputOnlyField()` | Ent `Sensitive()` |
| `OutputOnlyField()` | `api.ReadOnly()` |
| `CreateOnlyField()` | Ent `Immutable()` |
| `IdField()` | 无注解；Ent 自动识别 ID |
| `AuditLogField()` | `api.ReadOnly()` |
| `NewDomainField()` | `api.Hidden()` |
| `DomainFieldWithScopes(...)` | 用 Ent 事实和五个词直接写意图 |
| `ScopeCreate` / `ScopeUpdate` | 从 `Optional`、`Default`、`Nillable`、`Immutable` 推导 |
| `ScopeResponse` | 默认推导；用 `Hidden` 或 Ent `Sensitive` 移除 |
| `ScopeQuery` | `Searchable`、`Filterable`、`Sortable` 中的一个或多个 |
| `WithRequired(ScopeCreate)` | 无后继；必填等于 `!Optional && !Default` |
| `AsSearchable` / `AsFilterable` / `AsSortable` | `api.Searchable()` / `api.Filterable()` / `api.Sortable()` |
| `AsReadOnly` | `api.ReadOnly()` |
| `AsWriteOnly` | Ent `Sensitive()` |
| metadata 构造器 | 无后继 |
| `Edge().InResponse().As("key")` | `api.Expand().JSONKey("key")` |

`InputOnlyField()` 过去只是 HTTP 层承诺；Ent `Sensitive()` 还约束 service 层与日志。这种更宽
的语义是有意的：保密事实应声明在拥有它的层。

### 边

边由它自己的注解选中，绝不从外键位置推断：

```go
func (Post) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("author", User.Type).Ref("posts").Unique().Field("author_id").
            Annotations(api.Expand().JSONKey("writer")),
    }
}
```

`Expand()` 把 `Author *UserSummary` 放进 `PostResponse`，把 `WithAuthor()` 放进生成的
预加载计划；`JSONKey("writer")` 覆盖响应 key。扩展只深入一层，绝不从外键位置推断。

注解到达 codegen 时可能是 Go 类型，也可能是序列化 schema loader 产生的
`map[string]interface{}`；读取一律经过一次 JSON 归一化。

> **实现：** `api/annotations.go`；`funcs_scope.go` — `getResourceAnnotation`、
> `getFieldAnnotation`、`getEdgeAnnotation`；`annotations_edge.go` — `responseEdgeSet`、`edgeJSONKey`

## 生成了什么

每个带 **`api.Resource()`** 的实体产出四个文件；没有这个开关的实体不产生 EntAPI 文件。

| 文件 | 声明 |
|---|---|
| `{entity}_dto.go` | `{E}CreateRequest`、`{E}PatchRequest` 及各自的 `Valid…` 类型与 `Apply`；`{E}Response`、`{E}Summary` 及构造器；`{E}QueryWithResponseEdges`；`{E}ListResponse` 与 `New{E}ListResponse` |
| `{entity}_filter.go` | `{E}Filter`、`Parse{E}Query`、`Predicates()`、`{E}SortKeys`、`{E}Order` |
| `{entity}_wiring.go` | `Get{E}`、`List{Es}`、`Create{E}`、`Patch{E}`、`Delete{E}`、`DeleteBatch{Es}` |
| `{entity}_handler.go` | 可达的 `{Op}{E}Fn` 类型，以及 bind → call → write 三步 handler |

外加每个 schema 三个文件，各自有独立的发射条件：

| 文件 | 生成条件 | 声明 |
|---|---|---|
| `entapi_errors.go` | 至少一个实体产出了接线 | `ErrorMap` |
| `entapi_http.go` | 至少一个实体带 `api.Resource()` | `APIHandler`、`API(client)`、`ServeHTTP`、`Mount` 与未导出的路由 manifest |
| `entapi_softdelete.go` | 至少一个实体嵌入 `SoftDeleteMixin` | 未导出的查询 traverser 与删除 hook |

软删除文件的条件独立于 `api.Resource()`：一个不是 HTTP resource 的实体只要嵌了 mixin，仍会被写进
traverser 的类型开关。扩展还提供一个 `config/init/fields/*` partial，扩展 Ent 自己的
`client.go`：`newConfig` 会为每个这类实体初始化 hook 与 interceptor slice。这个 partial
不产生独立文件；图里没有 mixin 时，它一个字节也不渲染。

产物落在**你的** `ent` 包里（`gen.Config.Target`），所以读起来是 `ent.CreateArticle`、
`ent.ArticleFilter`、`ent.ErrorMap`。这也正是实体名会与它们相撞的原因——见
[保留名](#生成会失败而这正是设计)。

> **实现：** `extension.go` — `generatePerTypeFiles`、`perTypeFileName`、`renderDTOFile`、
> `renderFilterFile`、`renderWiringFile`、`renderHandlerFile`、`renderErrorMapFile`、
> `renderHTTPFile`、`renderSoftDeleteFile`、`pendingFile`；`cleanup.go` —
> `errorMapFileName`、`httpFileName`、`softDeleteFileName`；
> `funcs_scope.go` — `isResource`；`funcs_softdelete.go` — `softDeleteTypes`；
> 权威符号清单：`schema_conflicts.go` — `derivedEntityDecls`

## 生成的 HTTP

`ent.API(client)` 返回 `*ent.APIHandler`，它同时实现 `http.Handler`。可以直接服务、挂进
消费者自己的 mux，或用标准库 middleware 组合：

```go
api := ent.API(client)
api.Mount(mux)
mux.Handle("/v1/", http.StripPrefix("/v1", api))
```

每个未 Except 的 Resource 恰好得到这些 Go 1.22 pattern：

| Pattern | 结果 |
|---|---|
| `GET /articles` | 裸 `{"data","total","page","size"}` page，200 |
| `POST /articles` | 裸 resource，201；没有 `Location` header |
| `GET /articles/{id}` | 裸 resource，200 |
| `PATCH /articles/{id}` | 裸 resource，200 |
| `DELETE /articles/{id}` | 空 body，204 |

错误统一是 RFC 9457 `application/problem+json`；`WriteProblem` 写出
`type: "about:blank"`、title、status、detail，并在 error chain 含 `*FieldError` 时加
`field`。bind 失败是 400，生成 `Validate` 失败是 422，不支持的 media type 是 415，超限
body 是 413；中间步骤的 sentinel 映射到 404/409/400，未分类错误是 500。Save-time Ent
`ValidationError` 的分类留给 #74，本 slice 仍落到 500。

POST 与 PATCH 只接受 `application/json`，允许 media-type 参数。body 在读取前被限制为
**1 MiB，且没有配置旋钮**。未知 key 会与生成的 create/patch tag 数据比较，因此 PATCH 中
的 Immutable key 会按名字被拒绝，不会静默丢弃。

`WithActor` / `ActorFrom` 让认证主体穿过 middleware。`Route` 是 `Mount` 内部使用的
stdlib-only manifest 行；导出 route accessor 和 `With(...)` 函数替换属于 #75。

router 层未匹配的 path/method 仍保留 stdlib mux 的纯文本 404/405（405 含 `Allow`），而非
problem+json。这个 residue 是有意的：catch-all 会让挂进消费者 mux 与直接服务整棵生成树的
行为不同。

## 请求：三态存在性

一个 PATCH body 必须区分三件事，而普通结构体做不到：

| Payload | 含义 | `HasNickname()` | `Nickname` |
|---|---|---|---|
| `{}` | 别动它 | `false` | `nil` |
| `{"nickname": null}` | 清空它 | `true` | `nil` |
| `{"nickname": "sam"}` | 设置它 | `true` | `&"sam"` |

字段保持 `*T`，存在性记在一张与结构体并列的 `present map[string]bool` 里，由生成的
`UnmarshalJSON` 从原始 payload 的 key 集合填充。`Apply` 读的正是这个：

```go
if r.HasNickname() {
    if r.Nickname == nil { b.ClearNickname() } else { b.SetNickname(*r.Nickname) }
}
```

**创建**请求无法表达「清空」，所以那里的显式 `null` 被记为*缺席*——字段不会被写入，你
schema 的 `Default()` 因而生效。这也正是「必填创建字段上的显式 null」成为一个「必填」错误
而不是空指针解引用的原因。

必填性**按存在性检查，而不是按零值**（字符串是例外：`== ""` 说的是同一件事）。

### key 是严格匹配的

`encoding/json` 在精确匹配**或**大小写不敏感匹配时都会填充结构体字段，而存在性是按原始
payload key 记录的。这两者对每一个大小写变体都不一致，且失败是静默的：`PATCH
{"Nickname":"sam"}` 填进了字段，`HasNickname()` 仍为 `false`，`Apply` 什么也没写，更新报告
成功却没有改动任何一行。

所以生成的 `UnmarshalJSON` 会在**原始解码之后、结构体解码之前**拒绝任何折叠后等于某个已知
tag、但不精确相等的 key：

```
unknown key "Nickname" (did you mean "nickname"?)   // 包装 entapi.ErrValidation
```

被拒绝的请求不会触碰你的接收者。单独使用 DTO 时，折叠后**不匹配任何** tag 的 key 仍被
忽略；生成的 HTTP handler 更严格：调用这个自定义 unmarshaller 前，它会把 raw key 与生成的
tag slice 比较，并以 400 返回未知或 Immutable 字段的名字。DTO 大小写规则的理由见
[ADR-0001](docs/adr/0001-presence-follows-encoding-json-key-matching.md)。

### 验证不是可选的

`Validate()` 返回一个*不同的类型*，而 `Apply` 只存在于那个类型上：

```go
valid, err := req.Validate()          // *ValidArticleCreateRequest
if err != nil { return err }          // 包装 entapi.ErrValidation
art, err := ent.CreateArticle(ctx, client, valid)
```

`Valid{E}CreateRequest` 的唯一字段是未导出的 `r *{E}CreateRequest`，所以除了 `Validate()`
之外没有任何方式在包外构造它。不存在可以应用未验证请求的导出函数——跳过验证是编译错误。

> **实现：** `funcs_presence.go` — `isCreatePointer`、`isCreateRequired`、`isPatchClearable`；
> `funcs_fields.go` — `createFields`、`patchFields`（与 `node.MutableFields()` 求交）；
> `templates/dto.tmpl`；生成物样例：`internal/fixtures/basic/basicent/widget_dto.go` —
> `WidgetPatchRequest.present`、`UnmarshalJSON`、`widgetPatchRequestTags`、
> `ValidWidgetCreateRequest`、`Validate`、`Apply`

## 响应、摘要与边

`New{E}Response` 返回 error，`New{E}Summary` 不会。差别在边：

- 边的状态通过 ent 的 `<Edge>OrErr()` 读取，绝不用 nil 判断——`loadedTypes` 是未导出的，
  所以一个 nil 指针分辨不出*确实没有*和*没有加载*。
- **一对一边**：`err == nil` → 填充 summary；`IsNotFound(err)` → 字段置 `nil`（**已加载但
  不存在是显式的 `null`**，边字段不带 `omitempty`）；其他 error → 整个函数返回 error。
- **一对多边**：没有 not-found 态，加载了但为空就是空切片，所以任何 error 都意味着没加载，
  直接返回。
- **摘要不携带边。** 这就是扩展有界的原因：不存在第二层让环闭合，因此不需要运行时深度
  计数器，也不需要 visited 集合。一棵三层的树回来时只有一层深。

`{E}QueryWithResponseEdges(q)` 施加的正是 `New{E}Response` 需要的那份预加载计划。要么用
它，要么处理那个错误。

摘要携带**每个响应作用域的标量字段**，减去边——`{E}Summary` 与 `{E}Response` 的标量部分
一模一样。收窄它需要一个新注解，schema 里没有任何东西说明哪个字段是「简要」的那个。

一条 `api.Expand()` 边若指向**没有 `api.Resource()`** 的实体，就是生成错误：那个实体被
跳过，没有 `<Target>Summary` 可以引用。

> **实现：** `funcs_fields.go` — `responseFields`、`responseEdges`（返回 error）；
> `annotations_edge.go` — `responseEdgeSet`、`edgeJSONKey`；
> `funcs_codegen.go` — `fieldValueExpr`；`funcs_typechecks.go` — `isComplexFieldType`；
> `funcs_imports.go` — `dtoImports`；
> 生成物样例：`internal/fixtures/edges/edgesent/post_dto.go` — `NewPostResponse`（一对一
> 三分支）、`internal/fixtures/edges/edgesent/user_dto.go` — `NewUserResponse`（一对多）、
> `UserSummary`、`UserQueryWithResponseEdges`

### 具名列表类型

`List{Es}` 返回 `*entapi.Page[{E}Response]`。`{E}ListResponse` 是同一形状的非泛型具名
版本，因为 OpenAPI / swaggo 一类的工具无法表达泛型实例化：

```go
page, err := ent.ListArticles(ctx, client, filter, req)
if err != nil { return err }

// @Success 200 {object} ent.ArticleListResponse
return c.JSON(200, ent.NewArticleListResponse(page))
```

转换器的函数体是一次 Go 类型转换（`r := WidgetListResponse(*p)`），这正是要点：一旦
`{E}ListResponse` 与 `entapi.Page` 在字段集、类型或顺序上出现分歧，那一行会让**每一个**
生成包编译失败。类型转换忽略 struct tag，所以 tag 那一半由一个 golden JSON 测试单独守住。

> **实现：** `runtime/query.go` — `Page[R]`；生成物样例：
> `internal/fixtures/basic/basicent/widget_dto.go` — `WidgetListResponse`、
> `NewWidgetListResponse`；`internal/fixtures/basic/basicent/listresponse_shape_test.go`

## 查询面

生成的入口是：

```go
filter, req, err := ent.ParseArticleQuery(r.URL.Query())
```

它把 wire 解析成现有 `ListArticles` 签名使用的强类型 `ArticleFilter` 与
`entapi.ListRequest`。字段参数名始终取 Ent 的 storage key，而不是 Go 字段名。

### 结构化过滤——`api.Filterable()`

wire 采用 `field=op:value`，只在第一个冒号处分割；无前缀值就是等值：

```text
?title=ilike:go&score=gt:30&score=le:50&status=in:draft,published
```

| 写法 | 谓词 |
|---|---|
| 无前缀、`eq:` | 等值 |
| `ne:` | 不等 |
| `gt:` `ge:` `lt:` `le:` | 比较 |
| `in:` `not_in:` | 逗号分隔的集合 |
| `like:` `ilike:` `prefix:` `suffix:` | 字符串匹配 |
| `is_null:` `not_null:` | 空值谓词 |
| `from:` `to:` `between:a,b` | 闭区间语法糖 |

每个字段只得到 Ent 谓词与上述 wire 词汇的交集。`like:`、`ilike:`、`suffix:` 还要求
`api.Searchable()`，`prefix:` 不要求。`_ieq` 没有 wire 写法。无前缀的 `*` 与 `?`
仍是普通等值字面量，不会隐式变成 `LIKE`。

解析严格按六条规则执行：空的无前缀值忽略，但空 `eq:` 有效；无冒号就是等值；字段允许的
前缀应用该操作符；全局已知但字段不允许的前缀报校验错误；未知前缀把整个值回退为等值；
显式 `eq:` 用来转义看似操作符的值。转换错误会点名字段和值并包裹
`entapi.ErrValidation`。

同一字段的重复参数形成多个独立且以 `AND` 连接的谓词，重复等值也一样。因此标量 filter
槽位是 slice，`in:`/`not_in:` 槽位是 slice-of-slices。主键天然 Filterable，遵循相同规则，
且永远没有仅 Searchable 才允许的操作符。

### 全文——`api.Searchable()`

`_q=value` 在所有 Searchable 字段之间做 `OR`，再与结构化过滤做 `AND`。资源没有任何
Searchable 字段时传 `_q` 会报校验错误。只 Searchable、不 Filterable 的字段参与 `_q`，
但作为裸字段参数会被拒绝。

### 排序——`api.Sortable()`

`_sort=created_at:desc,title,id` 生成有序的 `[]entapi.SortSpec`。`{E}Order` 是唯一的白名单
校验点：非法 key 返回 `entapi.ErrValidation`，并列出全部合法 storage key。主键天然
Sortable。除非主键已出现在列表任意位置，否则它会作为最后的确定性 tiebreak 追加；空排序
等价于主键升序。

### 分页与保留参数

恰好存在四个下划线保留参数：`_sort`、`_page`、`_size`、`_q`；别名与重复保留参数都拒绝。
`_page`、`_size` 必须是正十进制整数。`_size=0` 保留给未来的 count-only 模式，目前报错；
大于 1000 的值允许进入，随后由 `Limit()` 钳制。Go 层的零值修复语义不变：`Limit()` 永不
返回 0，`Offset()` 把小于 1 的页码钳到 0。未知下划线参数、未知裸字段名、非 Filterable
字段的裸参数都报校验错误。URL key 按排序后的顺序处理，保证第一个错误稳定。

分页只有 offset 一种，`Page` 携带 `Data`、`Total`、`Page`、`Size`，没有别的。

> **实现：** `funcs_filter.go` — `queryFields`、`parseFields`、`searchFields`、
> 每字段操作符集合与转换表达式；`runtime/types.go` — `ListRequest`、`SortSpec`、
> `DefaultPageSize`、`MaxPageSize`；`runtime/urlquery.go` — query 词法解析；
> `runtime/query.go` — `Limit`、`Offset`、`Page[R]`、`Query[Q,P,O,E]`、`ListPage`；
> `runtime/filter.go` — `AppendEach`、`AppendEachSlice`；`templates/filter.tmpl`；
> 生成物样例：`internal/fixtures/query/queryent/record_filter.go` — `RecordFilter`、
> `ParseRecordQuery`、`Predicates`、`recordSortOptions`、`RecordSortKeys`、`RecordOrder`

## 接线与错误映射

自由函数，没有接口，没有需要嵌入的东西。若你需要不同的行为，写你自己的函数并停止调用
生成的那个。

```go
func GetArticle(ctx context.Context, db *Client, id uuid.UUID) (*ArticleResponse, error)
func ListArticles(ctx context.Context, db *Client, f *ArticleFilter, r entapi.ListRequest) (*entapi.Page[ArticleResponse], error)
func CreateArticle(ctx context.Context, db *Client, v *ValidArticleCreateRequest) (*ArticleResponse, error)
func PatchArticle(ctx context.Context, db *Client, id uuid.UUID, v *ValidArticlePatchRequest) (*ArticleResponse, error)
func DeleteArticle(ctx context.Context, db *Client, id uuid.UUID) error
func DeleteBatchArticles(ctx context.Context, db *Client, ids []uuid.UUID) (int, error)
```

任何地方都没有硬编码的标识符类型——id 来自你的 schema 的 `$.ID.Type`，并作为类型参数抵达
运行时，所以一个 `int` 主键连 import 都不需要。

每个导出的接线函数都**恰好一次**地经由 `ErrorMap.MapError` 返回。文件里还有一组未导出的
辅助函数（`{entity}Get`、`{entity}ByID`、`{entity}Reloaded`），它们存在正是为了让一次
create 或 update 在重新读取时不会映射两次。于是
`errors.Is(err, entapi.ErrNotFound)` 在你的 handler 边界上直接可用，无需拆开 ent 的
错误类型。

`ErrorMap` 由模板发射**一行**：

```go
var ErrorMap = entapi.NewErrorMapper(IsNotFound, IsConstraintError)
```

两个谓词都**不带限定符**，所以它们绑定到 ent 生成到**同一个包**里的那两个函数。这是必须的：
`ent.NotFoundError` 和 `ent.ConstraintError` 是 ent 为每个消费者项目单独生成的类型，框架里
并不存在它们，所以 runtime 只能收 `func(error) bool` 而永远不认识任何 ent 类型。

**`ErrorMap` 开箱状态下不会报告唯一性冲突。** `MapError` 的分支是：not-found → 包装
`ErrNotFound`；constraint **且** 你装了唯一性判定 → 包装 `ErrAlreadyExists`；其余**原样
返回**。ent 的 `IsConstraintError` 分辨不出 `UNIQUE` 与 `FOREIGN KEY`，所以这是按驱动
opt-in 的：

```go
func init() {
    ent.ErrorMap = ent.ErrorMap.WithUniqueViolation(func(err error) bool {
        var pgErr *pgconn.PgError
        return errors.As(err, &pgErr) && pgErr.Code == "23505"
    })
}
```

跳过它的代价是本该 409 的地方给你 500；它绝不会产生一个错误的 409。`ErrorMap` 是一个普通的
包级变量，自身不带任何同步——请在构造 client 处、第一个请求之前赋值。

> **实现：** `templates/wiring.tmpl`、`templates/errors.tmpl`；
> `runtime/errors.go` — `ErrNotFound`、`ErrAlreadyExists`、`ErrValidation`、`IsNotFound`、
> `IsAlreadyExists`、`IsValidation`；
> `runtime/errors_map.go` — `ErrorMapper`、`NewErrorMapper`、`WithUniqueViolation`、
> `MapError`；`runtime/query.go` — `ListPage`、`GetOne`、`SaveOne`、`Saver[E]`；
> `funcs_imports.go` — `wiringImports`；生成物样例：
> `internal/fixtures/wiring/wiringent/article_wiring.go`

## 软删除

基于注解，且在 ent 的层面强制执行，而不是在生成的接线里：

```go
func (Doc) Mixin() []ent.Mixin { return []ent.Mixin{entapi.SoftDeleteMixin{}} }
```

```go
client := ent.NewClient(ent.Driver(drv))
```

mixin 声明一个 `Optional().Nillable()` 的 `field.Time("deleted_at")`，并挂上
`DomainSoftDelete` 标记；ent 会把 mixin 的注解合并到实体上，所以标记就是「这个实体选择了
软删除」的判据——不是列名约定。

生成的 `newConfig` 为每个可软删除实体安装一个 interceptor 和一个 hook。没有注册调用，也
没有构造顺序依赖：`NewClient`、`Open` 与 `enttest.Open` 全都使用这份 config。此后被删除的
行会从**每一次**读取中消失——包括那些完全不碰本包生成物的 `client.Doc.Query()` 调用——而
`Delete` 变成对墓碑列的一次更新。不存在第二次写入，生成的接线里也没有任何东西知道软删除
的存在：`DeleteArticle` 发的是 `DeleteOneID(...).Exec`，`DeleteBatchArticles` 发的是
`Delete().Where(IDIn(...)).Exec`，两者都由 hook 改写。

两个互相独立的上下文开关让你按调用退出：

```go
entapi.WithSoftDeleted(ctx)   // 读取包含已删除的行
entapi.WithHardDelete(ctx)    // 这次删除是真删
```

两者互不蕴含——它们用的是两个不同的未导出 context key 类型。

注入的 hook 位于索引 0，而 Ent 把索引 0 应用在最外层。因此以后通过 `client.Use` 添加的
hook 都在软删除 hook 内层运行。

> **实现：** `softdelete.go` — `SoftDeleteMixin`、`SoftDeleteField`（`"deleted_at"`）、
> `DomainSoftDelete`、`SoftDeleteAnnotationName`；
> `funcs_softdelete.go` — `isSoftDeletable`、`softDeleteTypes`、`softDeleteField`、
> `softDeleteImports`；`runtime/softdelete_context.go` — `softDeletedKey`、`hardDeleteKey`、
> `WithSoftDeleted`、`SoftDeletedIncluded`、`WithHardDelete`、`HardDeleteRequested`；
> `templates/softdelete.tmpl`、`templates/softdelete_config_init.tmpl`；生成物样例：
> `internal/fixtures/softdelete/softdeleteent/entapi_softdelete.go` —
> `softDeleteTraverser`、`softDeleteHook`；以及其 `client.go` 中的 `newConfig`

## 生成会失败，而这正是设计

检查在 `next.Generate(g)` **之前**运行，所以一个被拒绝的 schema 不会在磁盘上留下任何东西
——连 ent 自己的产物都不会。整张图会被检查，所有问题**一次性全部报告**（错误形如
`entapi: N schema problem(s) prevent generation:` 后跟一条一行的清单）。政策是：

> 与 ent schema 相矛盾的注解会让生成失败，并同时报告两个事实和修法。凡是能被正确生成的，
> 就生成，不拒绝。

拒绝矩阵覆盖这些矛盾：

| 被拒绝的 | 原因 |
|---|---|
| `api.Hidden()` 与任何其他字段词并用 | hidden 没有可供其他偏离生效的表面 |
| Ent `Sensitive()` 与查询词或 `api.ReadOnly()` 并用 | secret 不能成为查询 oracle；完全不可访问的数据用 `Hidden` |
| required-no-default 字段被 `Hidden`/`ReadOnly` 挡出 create，且未 `Except(OpCreate)` | Ent 无法从该请求插入行 |
| PATCH 字段集为空且未 `Except(OpPatch)` | 公开 PATCH 面没有用途 |
| 字段词挂在 edge，或 `Expand` 挂在 field | 词挂错了 schema 元素 |
| 主键上出现任何 EntAPI 词 | ID 已天然 Filterable 与 Sortable；其固定查询面不使用注解 |
| `OpList` 被 except，同时字段带查询词 | 查询面已关闭 |
| 在没有 `Contains` 的类型上标 `api.Searchable()` | 没有子串谓词可发射 |
| 在没有操作符的类型上标 `api.Filterable()` | 过滤器组会静默无效 |
| Filterable 字段或主键的类型无法解析 wire 文本 | 改用基础标量、enum、`time.Time` 或实现 `encoding.TextUnmarshaler` 的类型 |
| 在不可比较类型上标 `api.Sortable()` | Ent 不生成 `ByX` |
| 查询 storage key 以 `_` 开头 | 与保留查询控制项相撞 |
| `api.Expand()` 指向非 resource | 目标 Summary 不存在 |
| `DomainSoftDelete` 指名了实体没有的字段 | 手工挂这个标记不受支持，正确方式是嵌 `SoftDeleteMixin` |
| 墓碑字段不是 `Optional` | ent 不生成 `DeletedAtIsNil` 谓词，traverser 编译不过 |
| 自引用边只在一端带注解 | ent 把链式的 `edge.To(…).From(…).Annotations(…)` 交给了*反向* builder，于是关联端静默丢失了它的注解 |
| **实体名与本扩展生成的符号相撞** | 见下 |

图级的 `API`、`APIHandler`、`ErrorMap` 都是保留名。一个名为 `ErrorMap` 的实体会让 ent 发射 `type ErrorMap`，而 `entapi_errors.go` 发射
`var ErrorMap`——Go 每个包只有一个标识符命名空间，于是 `redeclared in this block`，发生在
两个你从没写过的文件里，且没有任何东西指出原因。跨实体相撞同理：一个字面叫
`ArticleResponse` 或 `DeleteArticleFn` 的实体会撞上实体 `Article` 生成的类型。五个 Fn 名称
按最大宽度保留，即使对应操作已 Except 也不收窄。

保留名检查在图这一层跑，而不是在节点循环里：**相撞的实体不需要带任何注解**——ent 为每个
实体都生成类型，一个光秃秃的 `type ErrorMap struct{ ent.Schema }` 撞得一样狠，而节点循环
恰好会跳过它。派生名单取 resource 可能产出的**最大集合**，宁可拒绝一个理论上今天不会相撞
的名字，也不因日后加注解而突然报错。

HTTP 检查全部跳过没有 `api.Resource()` 的实体——与生成循环条件一致；软删除与保留名仍是
全图检查。

反过来，`Optional().Nillable()` 以及切片/映射上的具名类型*是*会被生成的，因为对它们存在
正确的产物。见[字段形态](#字段形态)。

> **实现：** `schema_conflicts.go` — `checkGraphConflicts`、`nodeConflicts`、
> `queryConflicts`、`immutableUpdateConflict`、`unusableSoftDeleteField`、
> `asymmetricSelfEdgeConflicts`、`asymmetricSelfEdgeConflict`、`reservedNameConflicts`、
> `graphSymbolConflicts`、`derivedName`、`derivedEntityDecls`、`derivedEntityNames`、
> `derivedNameConflict`、`fieldHasOp`、`markerList`、`errorMapSymbol`、
> `registerSoftDeleteSymbol`、`entPlural`

## 生成器对你的目录做了什么

**生成是整轮原子的。** 阶段一把每个文件渲染并用 `golang.org/x/tools/imports` 格式化进
**内存**；阶段二才逐个写盘。任何确定性失败——模板 bug、被拒绝的 schema、格式化不了的
源码——都落在阶段一，让上一轮的产物完整无损。格式化失败会**中止整轮生成并返回 error**：
`imports.Process` 只会在它解析不了源码时失败，那是模板 bug，不是可以容忍的格式瑕疵。

每个文件的写入本身也是原子的：在目标目录里建临时文件、写、`chmod 0644`、`rename` 就位。
诚实的残余：阶段二里两次 rename *之间*被硬杀，是一个仍然存在的毫秒级窗口。关掉它需要目录
交换，而那在跨平台上并不原子。（[ADR-0003](docs/adr/0003-per-run-atomic-generation.md)）

**清理会删除陈旧文件，并以一个 marker 判定所有权。** 一次成功的生成之后（**只在成功之后**
——失败的一轮什么都不删），生成器扫描目标目录，删除任何满足以下**两条**的 `.go` 文件：

1. **首行**（读文件头 4096 字节后按第一个换行截断）包含
   `Code generated by entapi extension`，且
2. 不是本轮写入的。

这就是为什么一次 schema 编辑不再弄红你的构建：删掉一个实体，它的
`_dto.go`/`_filter.go`/`_wiring.go`/`_handler.go` 会随之而去，而不是作为「引用 ent 已不再生成的 builder」
的残骸留下来。对于跨过基类删除升级的人，它也会移除 `_base_service.go` / `_base_handler.go`
——这些名字不在任何清单里，它们只是同样带着 marker。

扫描**仅限顶层**（`os.ReadDir`，不是 `filepath.Walk`），也**跳过目录项**——ent 生成的子包
（`<entity>/`、`predicate/`、`migrate/` …）位于目标目录之下，永远不是候选。不带 marker 的
文件会被留下并**打一条日志**说明为什么；ent 自己的 `Code generated by ent, DO NOT EDIT.`
刻意不匹配。

> **你的逃生舱就是那行 marker。** 想把一个生成的文件据为己有，删掉它的首行。反过来，
> 一个你从生成产物复制来、却忘了剥掉文件头的文件**会被删除**。
> （[ADR-0004](docs/adr/0004-cleanup-ownership-by-marker.md)）

> **实现：** `extension.go` — `generatePerTypeFiles`（两阶段循环）、`formatFile`、
> `writeFormatted`、`pendingFile`；`cleanup.go` — `generatedMarker`、`markerScanBytes`、
> `removeStaleArtifacts`、`removeIfStale`、`hasGeneratedMarker`

## 字段形态

ent 的修饰符如何决定生成的请求形态。这是从 ent 推导的，绝不来自第二意见——ent 决定哪些
setter 存在，所以任何独立推导出的形态都会表现为「调用一个从未被生成的方法」。

| ent schema | 创建字段 | 必填？ | patch 时 `null` 可清空？ |
|---|---|---|---|
| `field.String("a")` | `string` | 是 | 否 |
| `field.String("a").Default("x")` | `*string` | 否 | 否 |
| `field.String("a").Optional()` | `*string` | 否 | **是** |
| `field.String("a").Optional().Nillable()` | `*string` | 否 | 是 |
| `field.String("a").Immutable()` | `string` | 是 | *不出现在 PATCH 中* |
| `field.JSON("tags", []string{}).Optional()` | `*[]string` | 否 | 是 |

三条规则，各一行：

- 创建字段是**指针**，恰好当 `Optional || Default || Nillable`——即 ent 能在调用方不给的
  情况下自行填充它时。
- 创建字段**必填**，恰好当 `!Optional && !Default`。
- patch 字段**可清空**，恰好当 `Optional`。

`patchFields` 遍历 `node.MutableFields()`，所以活下来的字段必有 `Set<Field>`；随后再去掉
`Hidden` 与 `ReadOnly`。`createFields` 对全部 Ent 字段应用同样两项偏离。`Sensitive` 在两个
请求里仍可写，只从响应中移除。

在响应一侧，`Optional` 的可比较字段走 `entapi.PtrOrNil`，`Optional` 的切片与映射
——**包括**它们之上的具名类型——走 `entapi.PtrNilSafe`，靠检查 `field.Type.RType.Kind`
而非渲染出来的类型名来选择。

`Apply` 一律发射 `if r.X != nil { b.SetX(*r.X) }`，绝不用 `SetNillableX`：对一个本身就可为
nil 的类型，ent 不生成 nillable setter，所以 `SetNillableTags` 对一个 optional 的
`field.JSON` 并不存在。一个统一的分支对每种形态都正确。

> **实现：** `funcs_presence.go` — `isCreatePointer`、`isCreateRequired`、`isPatchClearable`；
> `funcs_fields.go` — `patchFields`；`funcs_codegen.go` — `fieldValueExpr`；
> `funcs_typechecks.go` — `isComplexFieldType`；
> `runtime/types.go` — `Ptr`、`PtrOrNil`、`PtrNilSafe`；
> fixture：`internal/fixtures/fieldshapes/`

## 注解表面

公开 schema API 只有三个可合并注解类型，没有 pending knob。反射测试逐个切换导出字段和
构造器，并检查它们是否抵达已注册模板函数；新增不可达 knob 会直接弄红 CI。

> **实现：** `api/annotations.go`；`annotation_surface_test.go` — `pendingKnobs`；
> `funcs.go` — `templateFuncs`

## 陷阱

按「有多安静地伤害你」排序。

1. **在你调用 `WithUniqueViolation` 之前，`ErrorMap` 永远不会返回 `ErrAlreadyExists`。**
   一个重复键会原样穿过 `MapError`，表现为 500。
2. **`New{E}Response(nil)` 返回 `(nil, nil)`。** 不是错误。如果你把一次未命中的查询直接喂
   进它，拿到的是一对 nil，而不是 not-found。
3. **`like:`、`ilike:` 与 `suffix:` 需要 `api.Searchable()`。** 用在只 Filterable 的
   字符串字段时，它们是已知但不允许的操作符，生成 parser 返回 `ErrValidation`；`prefix:`
   仍然可用。
4. **只有主键的查询维度会推导。** 它天然 Filterable 与 Sortable；所有非 ID 字段仍需自己的
   查询词。
5. **全 Immutable 的 PATCH 会被拒绝**，除非 resource 写 `Except(api.OpPatch)`；请求类型与
   接线函数仍保留。
6. **PATCH body 里的 `Immutable()` 字段不在 DTO 中。** 生成的 HTTP handler 会把 raw key
   与生成的 patch tag 数据比较并拒绝它；单独解码 DTO 仍会忽略无关 key。
7. **`entapi.IsNotFound` 不是 ent 的 `IsNotFound`。** 生成模板以*不限定*的形式调用后者，
   使其绑定到你包内 ent 生成的谓词。加上限定符照样编译，然后静默地什么都匹配不上。
8. **required 字段被挡出 create 会阻止生成**，除非 except create、改 optional 或给 default。
9. **`DeleteBatch` 对匹配不到的 id 返回计数而非错误。** 那个 `int` 是你了解「实际存在多少
    个」的唯一途径；空列表删除零行，这是 ent 对无参 `IDIn` 的读法，不是这里写的守卫。
10. **`Page.Size` 是钳制后的 size。** parser 拒绝零和负数 `_size`，接受大于 1000 的值，
    随后由 `ListRequest.Limit()` 钳制。
11. **`ErrorMap` 是普通包级变量，不带同步。** 请在构造 client 处赋值，不要在服务运行中改。
12. **一个你复制并修改过的生成文件仍带着 marker**——清理会删掉它。请剥掉首行。

## 限制

- **只有 offset 分页**，且每页一次 `COUNT`。它现在是正确的（主键 tiebreak 保证全序），但
  深度仍是 O(n)，并且在*并发写入下*仍可能跳过或重复行。包内没有 keyset 替代方案，也没有
  游标类型。
- **摘要永远是一层深。** 没有深度选项。
- **摘要携带哪些标量字段无法从 schema 判定**，所以摘要携带每个响应作用域字段减去边。想收窄
  它需要一个新注解。
- **产物与 ent 的输出同处一个包。** 生成器不建独立的 `dto` 子包，也没有配置项可以换目录；
  它对目标目录的所有权靠 marker 逐文件判定，而不是靠独占一个目录。
- **作用域只控制 HTTP 结构体的生成。** 它们绝不限制你的 service 层能拿一个 ent 实体做什么。
  任何需要强制的东西，必须在构造查询的地方强制。
- **生成器包在包初始化时加载全部五个模板。** 这被限制在 `entc.go` 与 schema 文件里；
  `runtime/` 就是把它挡在你的二进制之外的那个东西。

## 与 DESIGN-v2 的偏离

[`docs/DESIGN-v2.md`](docs/DESIGN-v2.md) 的抬头写着「实现未开始」，那是**陈旧的**：它提出的
T3 已经全部落地。三处偏离，都是有意的：

| 设计条目 | 现状 |
|---|---|
| §1.6 产物移到 `ent/dto` 子包 | **未做，且已被取代**。产物落在消费者的 `ent` 包里，handler 解耦由生成的自由函数达成，而不是由包放置达成 |
| §8.1 目录里存在「不是我的」文件 → 拒绝生成 | **未做**。清理把这类文件**留下并记日志**。它原本依赖 §1.6 的独占目录 |
| §8.4 `OutputPackage` 配置项 | **未做**，因 §1.6 未做而无意义。唯一的选项是 `WithEntAPIPackage` |

设计文档里明确「延后」的 T2（受众维度）同样没有实现，这与设计一致。

## 迁移注记

**破坏性与行为变更（#70）：** 生成的 `RegisterSoftDelete` 已删除。重新生成后，删掉所有
`ent.RegisterSoftDelete(client)` 调用；在 schema 中嵌入 `SoftDeleteMixin` 现在会自动配置
`NewClient`、`Open` 与 `enttest.Open`。hook 也从过去的注册顺序位置移到了 `hooks[0]`；Ent
把它应用在最外层，先于以后通过 `client.Use` 添加的 hook。

**行为变更：** Ent 标记为 `Sensitive()` 的字段不再出现在 `{Entity}Response` 或
`{Entity}Summary` 中。这关闭了带响应 scope 的敏感字段仍被生成并序列化的泄漏；create
与 patch 请求保持不变。

**破坏性与行为变更（#72）：** query 解析从外部 form binding 迁移到生成的
`Parse{Entity}Query`。两个 entigo 差异必须一起迁移：同一字段的重复参数现在生成多个以
`AND` 连接的谓词，而非 `OR`/`IN`；无法转换的值现在返回 `ErrValidation`，而非静默跳过。
此外 `_ieq` 在 v2 wire 中没有写法，已从查询面移除。

| 旧写法 | Query wire v2 |
|---|---|
| `q=words` | `_q=words` |
| `sort_by=created_at&order=desc` | `_sort=created_at:desc` |
| `page=2&size=50` | `_page=2&_size=50` |
| `score_gt=30` | `score=gt:30` |

所有 v2 wire 名都使用字段的 storage key。先重新生成，再迁移调用方；新旧写法不会同时接受。

以下符号曾经存在于本 module，现已删除，且**没有兼容别名**——一个保留耦合的别名，比破坏
更糟。

| 已删除 | 改用 |
|---|---|
| 生成的 `Update{Entity}` | 生成的 `Patch{Entity}`；重新生成并重命名调用点 |
| 生成的 `RegisterSoftDelete` | client 构造处无需任何调用——在 schema 中嵌入 `SoftDeleteMixin` 并重新生成 |
| `Base{Entity}Service`、`Base{Entity}Handler`、`SetSelf`、生成的 hook | 生成的自由函数（`Get{E}`、`List{Es}`、…）；需要不同行为就写你自己的函数 |
| `ExtensionConfig.GenerateBaseService`、`.GenerateBaseHandler`、`WithBaseService`、`WithBaseHandler` | 无对应物——基类不再存在 |
| `{Entity}EntToResponse` | `New{Entity}Response`，它返回 error 而不是在错误时返回 nil |
| `Apply{Entity}CreateRequest`、`Apply{Entity}UpdateRequest`（自由函数） | `Valid{Entity}…Request.Apply` |
| `Cursor`、`PageInfo`、`EncodeCursor`、`DecodeCursor`、`ListRequest.Cursor` | 无——分页只有 offset |
| `DomainField.Sensitive`、`AsSensitive` | 在 Ent schema 给字段标 `Sensitive()`；它仍可写，但从两层响应中移除 |
| `DomainField.UniqueLookup`、`.RangeLookup`、`.Validation` | `api.Filterable()`（运算符从 Ent 的 `$field.Ops` 导出）；生成请求的 `Validate()` |
| `DomainConfig.EntityName` | 无——没有读者 |
| 运行时符号住在根包 | 全部搬到 `github.com/githonllc/entapi/runtime` |
| `ListRequest.SortBy`、`ListRequest.Order`、`ListRequest.Validate`、`ListRequest.SortKey` | `ListRequest.Sort []SortSpec`；用生成的 `Parse{Entity}Query` 解析，在 `{Entity}Order` 校验 key |
| `AppendIf`、`AppendIfSlice` | `AppendEach`、`AppendEachSlice`；filter 槽位为 slice，重复值以 `AND` 连接 |
| query `form` tag | 生成的 `Parse{Entity}Query`；`ListRequest` v2 不带 form tag |

## 还可以读什么

| | |
|---|---|
| [`docs/adr/`](docs/adr/) | 那些承重决定为什么是现在这样——严格 key 匹配、主键 tiebreak、整轮原子性、marker 所有权、操作符分类 |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | 模块地图，与本文用同一套源码锚点纪律 |
| [`docs/DESIGN-v2.md`](docs/DESIGN-v2.md) | 这次重设计的论证，以及它自己第一稿里哪些论断是错的（抬头的状态行已过时，见上） |
| [`docs/QUALITY-REVIEW.md`](docs/QUALITY-REVIEW.md) | 已知缺陷 |
| [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) | 如何构建、测试与新增 fixture |
| `internal/fixtures/` | 每一条规则的可编译证据：每个目录一份手写 schema 加一份提交进仓库的生成产物 |
| [`README.md`](README.md) | English documentation |

## 许可证

MIT——见 [LICENSE](LICENSE)。
