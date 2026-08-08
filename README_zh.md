# EntAPI

[![Go Reference](https://pkg.go.dev/badge/github.com/githonllc/entapi.svg)](https://pkg.go.dev/github.com/githonllc/entapi)
[![Go Report Card](https://goreportcard.com/badge/github.com/githonllc/entapi)](https://goreportcard.com/report/github.com/githonllc/entapi)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

一个 [Ent](https://entgo.io) 扩展。你在 ent schema 的字段上标注「HTTP 层可以拿它做什么」，
它就为你生成请求类型、响应类型、查询面，以及每个操作一个接线函数——全部写进你自己的
`ent` 包，链接的运行时除标准库外不依赖任何东西。

*[English](README.md)*

```go
// schema/article.go —— 你写这个
field.String("title").
    Annotations(entapi.DefaultField().AsSearchable().AsFilterable().AsSortable()),
```

```go
// handler.go —— 你得到这个
page, err := ent.ListArticles(ctx, client, filter, req)   // GET /articles?title_contains=go&sort_by=title
art,  err := ent.CreateArticle(ctx, client, validReq)     // POST /articles
```

这两行之间，生成器写出了带三态存在性的 `ArticleCreateRequest`、每个 ent 派生操作符一个
参数的 `ArticleFilter`、排序白名单、带预加载计划的 `ArticleResponse`，以及错误分类。

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

- [安装](#安装) · [两个 import 路径](#两个-import-路径) · [接入](#接入)
- [注解模型](#注解模型)——作用域与标记是两个不同的轴
- [生成了什么](#生成了什么)
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

## 两个 import 路径

一个 module，两个包，按**代码何时运行**切分。两者的包名都是 `entapi`，所以无论从哪条
路径来，调用点都写作 `entapi.X`；同时需要两者的文件就都 import，并给其中一个起别名。

| Import | 被谁 import | 主要符号 |
|---|---|---|
| `github.com/githonllc/entapi` | 你的 `entc.go` 和你的 **schema** 文件 | `Extension`、`DomainField` 及其构造器、`Edge()`、`SoftDeleteMixin` |
| `github.com/githonllc/entapi/runtime` | **生成的代码**与你的 handler / service 代码 | `ListRequest`、`Page[R]`、`ListPage`、`GetOne`、`SaveOne`、`ErrNotFound`/`ErrAlreadyExists`/`ErrValidation`、`ErrorMapper`、`AppendIf`、`Ptr`/`PtrOrNil`/`PtrNilSafe`、`WithSoftDeleted`/`WithHardDelete` |

这个切分是承重的，不是整洁癖：根包用 `//go:embed` 内嵌五个模板，并在**包初始化时**把五份
全部从内嵌文件系统里读出来，读不到就 panic。只要 import 根包，这件事就会发生，无论你是否
真的生成任何东西，而且它连带拖进 `embed`、ent 的 codegen 包和
`golang.org/x/tools/imports`。（解析发生在之后，每次渲染时——加载器返回的是模板源码
`string`。）`runtime/` 只 import 标准库，所以它进得了你的生产二进制而根包不必进。

> **实现：** `template_loader.go` — `//go:embed templates/*.tmpl`、`templateFS`、
> `loadTemplate`、`mustLoadTemplate`（返回 `string`）；
> `template_index.go` — `dtoTemplate`、`filterTemplate`、`wiringTemplate`、
> `errorMapTemplate`、`softDeleteTemplate`（五个包级 `var`，全部在 init 时求值）；
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

扩展只挂一个 `gen.Hook`。`Templates()` 返回**空切片**——本扩展不走 ent 的 `GraphTemplate`
机制，它自己渲染并写盘。

> **实现：** `extension.go` — `Extension`、`ExtensionConfig`、`NewExtension`、
> `NewExtensionWithOptions`、`Option`、`WithEntAPIPackage`、`defaultEntAPIPackage`、
> `Hooks`、`Templates`、`Annotations`、`Options`、`ConfigAnnotation`

## 注解模型

两个轴，把它们搞混是最常见的错误。

**作用域（scope）** 回答*哪些 HTTP 结构体可以携带这个字段*。共四个：

| 作用域 | 出现在 |
|---|---|
| `ScopeCreate` | `{E}CreateRequest` |
| `ScopeUpdate` | `{E}PatchRequest`（且必须同时在 ent 的 `MutableFields` 里） |
| `ScopeQuery` | `{E}Filter` / `{E}SortKeys` |
| `ScopeResponse` | `{E}Response` / `{E}Summary` |

**标记（marker）** 回答*对一个已经有 `ScopeQuery` 的字段，查询 API 可以做什么*。共三个：
`AsFilterable()`、`AsSearchable()`、`AsSortable()`。

```go
entapi.DefaultField()                    // create + update + query + response
entapi.InputOnlyField()                  // create + update           （密码）
entapi.OutputOnlyField()                 // query + response          （时间戳、计算态）
entapi.CreateOnlyField()                 // create + query + response （创建后不可变）
entapi.IdField()                         // OutputOnly + 预置描述 + ReadOnly 元数据
entapi.AuditLogField()                   // OutputOnly + ReadOnly 元数据
entapi.NewDomainField()                  // 零作用域——ent 追踪它，但它不出现在任何 HTTP 结构体里
entapi.DomainFieldWithScopes(scopes...)  // 其他任意组合
```

**没有任何预设会授予标记。** 六个预设的函数体只写 `Scopes` 字段，`Searchable` /
`Sortable` / `Filterable` 三个布尔一律保持零值。在你链上一个之前，`DefaultField()` 给你的
是一个**空的** `{E}Filter` 结构体和一个**空的**排序白名单：

```go
field.String("title").
    Annotations(entapi.DefaultField().
        AsFilterable().     // 结构化 URL 参数：title、title_neq、title_in、title_prefix……
        AsSearchable().     // 加入全文 q 析取，并解锁子串类操作符
        AsSortable()),      // 进入 {E}SortKeys
```

一个**没有** `ScopeQuery` 的标记是生成错误，不是警告——见[生成会失败](#生成会失败而这正是设计)。

每个构造器都是**值接收者且返回副本**：链式调用有效，原地修改无效。切片和 map 字段在拷贝时
都会重新分配，所以从同一个基注解分叉出的两条链互不影响。

> **实现：** `annotations.go` — `FieldScope`、`ScopeCreate`、`ScopeUpdate`、`ScopeQuery`、
> `ScopeResponse`、`AllFieldScopes`、`DomainField`、`NewDomainField`、
> `DomainFieldWithScopes`、`DefaultField`、`InputOnlyField`、`OutputOnlyField`、
> `CreateOnlyField`、`IdField`、`AuditLogField`、`WithRequired`、`AsSearchable`、
> `AsSortable`、`AsFilterable`、`copyScopes`、`copyEnum`、`copyTags`

### 边

边由它自己的注解选中，绝不从外键位置推断：

```go
func (Post) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("author", User.Type).Ref("posts").Unique().Field("author_id").
            Annotations(entapi.Edge().InResponse().As("writer")),
    }
}
```

`InResponse()` 把 `Author *UserSummary` 放进 `PostResponse`，把 `WithAuthor()` 放进生成的
预加载计划；`As("writer")` 覆盖 JSON key。`DomainEdge` 只有 `Scopes` 和 `JSONKey` 两个
字段，且今天只有 `ScopeResponse` 被读取。

注解到达 codegen 时可能是 `*DomainEdge`，也可能是 `map[string]interface{}`（从序列化 schema
加载时），读取一律经过一次 JSON 归一化。字段注解同理。

> **实现：** `annotations_edge.go` — `DomainEdge`、`Edge`、`InResponse`、`As`、`hasScope`、
> `getDomainEdgeAnnotation`、`hasEdgeScope`、`responseEdgeSet`、`edgeJSONKey`；
> `funcs_scope.go` — `getDomainFieldAnnotation`、`hasDomainScope`、`isDomainRequired`

## 生成了什么

**至少携带一个带注解字段**的实体，每个产出三个文件。一个都没有的实体被整体跳过，不产生
任何文件——生成循环的第一行就是 `if len(domainFields(node)) == 0 { continue }`。

| 文件 | 声明 |
|---|---|
| `{entity}_dto.go` | `{E}CreateRequest`、`{E}PatchRequest` 及各自的 `Valid…` 类型与 `Apply`；`{E}Response`、`{E}Summary` 及构造器；`{E}QueryWithResponseEdges`；`{E}ListResponse` 与 `New{E}ListResponse` |
| `{entity}_filter.go` | `{E}Filter` 及其 `Predicates()`、`{E}SortKeys`、`{E}Order` |
| `{entity}_wiring.go` | `Get{E}`、`List{Es}`、`Create{E}`、`Update{E}`、`Delete{E}`、`DeleteBatch{Es}` |

外加每个 schema 两个文件，各自有独立的发射条件：

| 文件 | 生成条件 | 声明 |
|---|---|---|
| `entapi_errors.go` | 至少一个实体产出了接线 | `ErrorMap` |
| `entapi_softdelete.go` | 至少一个实体嵌入 `SoftDeleteMixin` | `RegisterSoftDelete`、查询 traverser、删除 hook |

软删除文件的条件独立于注解：一个**没有任何 domain 字段**的实体只要嵌了 mixin，仍然会被写进
traverser 的类型开关。

产物落在**你的** `ent` 包里（`gen.Config.Target`），所以读起来是 `ent.CreateArticle`、
`ent.ArticleFilter`、`ent.ErrorMap`。这也正是实体名会与它们相撞的原因——见
[保留名](#生成会失败而这正是设计)。

> **实现：** `extension.go` — `generatePerTypeFiles`、`perTypeFileName`、`renderDTOFile`、
> `renderFilterFile`、`renderWiringFile`、`renderErrorMapFile`、`renderSoftDeleteFile`、
> `pendingFile`；`cleanup.go` — `errorMapFileName`、`softDeleteFileName`；
> `funcs_fields.go` — `domainFields`；`funcs_softdelete.go` — `softDeleteTypes`；
> 权威符号清单：`schema_conflicts.go` — `derivedEntityDecls`

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

被拒绝的请求不会触碰你的接收者。折叠后**不匹配任何** tag 的 key 仍被忽略——拒绝那些是
`DisallowUnknownFields`，仍归你的 handler 决定。理由见
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

一条被响应作用域选中、却指向**没有任何 domain 字段**的实体的边，是生成错误：那个实体被
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

三个互相独立的维度，每个都按字段 opt-in，外层的门是 `ScopeQuery`。

### 结构化过滤——`AsFilterable()`

**ent** 为该类型派生的每个操作符对应一个参数；本包从不维护自己的操作符表，只维护一张
**命名**表（哪个操作符叫什么后缀）。线上名字是字段的**存储 key** 加后缀，`form:` 与
`json:` 共用：

| 后缀 | |
|---|---|
| *（无）* | `EQ`——等值参数就是字段本身的名字 |
| `_neq` `_in` `_not_in` `_gt` `_gte` `_lt` `_lte` | 比较类，来自 ent |
| `_prefix` | 左锚定 `LIKE`——走索引 |
| `_contains` `_icontains` `_suffix` `_ieq` | **子串类，见下** |
| `_is_null` | 一个 `*bool`，把 `IsNil`/`NotNil` 合并 |

一个只标了 `AsFilterable()` 的 optional `string` 字段得到十个参数：

```
ref  ref_neq  ref_in  ref_not_in  ref_gt  ref_gte  ref_lt  ref_lte  ref_prefix  ref_is_null
```

`IsNil` 与 `NotNil` 合并成一个 `*bool`，因为可空性是**一个**问题；拆成两个参数会允许一个
自相矛盾的请求，而「is null AND is not null」没有诚实的答案。

ent 认识而本包没有命名的操作符会被跳过，不会以错误的名字发射——今天不存在这样的操作符。

### 子串类还需要 `AsSearchable()`

`_contains`、`_icontains`、`_suffix`、`_ieq` 正是那些让 B-tree 索引失效的 `LIKE '%x%'`
形态——与全文搜索那道门要挡住的成本画像完全相同。它们只在字段**同时**携带 `AsSearchable()`
时才发射。

`_ieq` 在*语义*上是精确匹配，但因其*成本*被划入昂贵类：没有函数索引时
`LOWER(x) = LOWER(?)` 会全扫，和子串匹配一模一样。理由见
[ADR-0005](docs/adr/0005-contains-operators-gated-by-searchable.md)。

### 全文——`AsSearchable()`

仅当至少有一个字段可搜索时才发射：单个 `q` 参数，作为一个跨全部可搜索字段的 `OR` 析取施加，
并与其他一切 `AND`。为 nil **或空串**时跳过。一个标了 `AsSearchable()` 但没标
`AsFilterable()` 的字段只贡献给 `q`，不会得到属于自己的结构化参数。

一个什么都没标的实体得到 `type PlainFilter struct{}` 和 `var PlainSortKeys = []string{}`——
空的，但存在，因为接线签名需要它们。

### 排序——`AsSortable()`

`{E}SortKeys` 是白名单，`{E}Order` 是把请求翻译成 ent order option 的函数。白名单之外的
`sort_by` 是 `entapi.ErrValidation`，绝不静默回退。通过校验的 key 随后被**丢弃**——进入
查询的是 ent 为那一列生成的 order builder，从一张 `map[string]func(...) OrderOption` 里按
已验证的 key 查出来。没有任何调用方字符串会被拼进 SQL。

**没有默认排序列**——你的 schema 里没有任何东西说明哪一列是天然的那一列，所以生成器不猜。

确定性是另一个问题，而它确实有一个由 schema 给出的答案。基于 offset 的分页在非全序上按构造
就是错的：在**零并发写入**的情况下，行也可能在第 1 页和第 2 页之间重复或消失。所以每个
生成的排序都以主键收尾：

```go
// 请求了排序：tiebreak 跟随请求的方向
[]OrderOption{by(dir), ByID(dir)}       // 当请求的 key 就是主键时跳过追加
// 什么都没请求：确定，且不声称自己是「默认排序」
[]OrderOption{ByID(sql.OrderAsc())}
```

理由见 [ADR-0002](docs/adr/0002-deterministic-pagination-pk-tiebreak.md)。

### 分页

`entapi.ListRequest{Size, Page, SortBy, Order}`——零值可直接用，四个字段都带 `form:` 与
`json:` tag。

- `Limit()`：`Size <= 0` → 20（`DefaultPageSize`）；`Size > 1000` → 1000（`MaxPageSize`）；
  否则原样。**钳制，绝不拒绝。**
- `Offset()`：`Page <= 1` → 0；否则 `(Page-1) * Limit()`，并在乘法溢出时**饱和到
  `math.MaxInt`** 而不是回绕成负数。
- `Validate()` 对 `Size` 和 `Page` **一个字都不说**——它只检查 `Order`。如果你想让超界的
  size 变成 4xx，自己和 `entapi.MaxPageSize` 比较。
- `Page.Size` 报告的是**实际用掉的** size，所以钳制是可见的。

分页只有 offset 一种，`Page` 携带 `Data`、`Total`、`Page`、`Size`，没有别的。

> **实现：** `funcs_filter.go` — `queryFields`、`isFilterable`、`isSearchable`、
> `isSortable`、`searchFields`、`filterParam`、`filterParams`、`opTagSuffix`、
> `substringOps`、`nullTagSuffix`、`filterImports`；
> `runtime/types.go` — `ListRequest`、`Validate`、`DefaultPageSize`、`MaxPageSize`；
> `runtime/query.go` — `Limit`、`Offset`、`SortKey`、`Page[R]`、`Query[Q,P,O,E]`、
> `ListPage`；`runtime/filter.go` — `AppendIf`、`AppendIfSlice`；
> `templates/filter.tmpl`；生成物样例：
> `internal/fixtures/query/queryent/record_filter.go` — `RecordFilter`、`Predicates`、
> `recordSortOptions`、`RecordSortKeys`、`RecordOrder`

## 接线与错误映射

自由函数，没有接口，没有需要嵌入的东西。若你需要不同的行为，写你自己的函数并停止调用
生成的那个。

```go
func GetArticle(ctx context.Context, db *Client, id uuid.UUID) (*ArticleResponse, error)
func ListArticles(ctx context.Context, db *Client, f *ArticleFilter, r entapi.ListRequest) (*entapi.Page[ArticleResponse], error)
func CreateArticle(ctx context.Context, db *Client, v *ValidArticleCreateRequest) (*ArticleResponse, error)
func UpdateArticle(ctx context.Context, db *Client, id uuid.UUID, v *ValidArticlePatchRequest) (*ArticleResponse, error)
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
ent.RegisterSoftDelete(client)          // 恰好一次，在构造时
```

mixin 声明一个 `Optional().Nillable()` 的 `field.Time("deleted_at")`，并挂上
`DomainSoftDelete` 标记；ent 会把 mixin 的注解合并到实体上，所以标记就是「这个实体选择了
软删除」的判据——不是列名约定。

`RegisterSoftDelete` 安装一个 interceptor 和一个 hook。此后被删除的行会从**每一次**读取中
消失——包括那些完全不碰本包生成物的 `client.Doc.Query()` 调用——而 `Delete` 变成对墓碑列的
一次更新。不存在第二次写入，生成的接线里也没有任何东西知道软删除的存在：
`DeleteArticle` 发的是 `DeleteOneID(...).Exec`，`DeleteBatchArticles` 发的是
`Delete().Where(IDIn(...)).Exec`，两者都由 hook 改写。

两个互相独立的上下文开关让你按调用退出：

```go
entapi.WithSoftDeleted(ctx)   // 读取包含已删除的行
entapi.WithHardDelete(ctx)    // 这次删除是真删
```

两者互不蕴含——它们用的是两个不同的未导出 context key 类型。

**一个没有调用 `RegisterSoftDelete` 就构造出来的 client 不过滤任何东西，并且是硬删除
——包括在你的测试里。**

> **实现：** `softdelete.go` — `SoftDeleteMixin`、`SoftDeleteField`（`"deleted_at"`）、
> `DomainSoftDelete`、`SoftDeleteAnnotationName`；
> `funcs_softdelete.go` — `isSoftDeletable`、`softDeleteTypes`、`softDeleteField`、
> `softDeleteImports`；`runtime/softdelete_context.go` — `softDeletedKey`、`hardDeleteKey`、
> `WithSoftDeleted`、`SoftDeletedIncluded`、`WithHardDelete`、`HardDeleteRequested`；
> `templates/softdelete.tmpl`；生成物样例：
> `internal/fixtures/softdelete/softdeleteent/entapi_softdelete.go` —
> `RegisterSoftDelete`、`softDeleteTraverser`、`softDeleteHook`

## 生成会失败，而这正是设计

检查在 `next.Generate(g)` **之前**运行，所以一个被拒绝的 schema 不会在磁盘上留下任何东西
——连 ent 自己的产物都不会。整张图会被检查，所有问题**一次性全部报告**（错误形如
`entapi: N schema problem(s) prevent generation:` 后跟一条一行的清单）。政策是：

> 与 ent schema 相矛盾的注解会让生成失败，并同时报告两个事实和修法。凡是能被正确生成的，
> 就生成，不拒绝。

今天检测九种情形：

| 被拒绝的 | 原因 |
|---|---|
| 带 `ScopeUpdate` 的 `Immutable()` 字段——而 `DefaultField()` 恰好授予它 | ent 的 update builder 遍历 `MutableFields`，因此 `SetX` 不存在，没有任何模板能发射出编译得过的调用 |
| 没有 `ScopeQuery` 的标记 | 该字段被标记为可过滤/可搜索/可排序，却从查询 API 无从抵达，不会产生任何查询产物 |
| 在没有 `Contains` 的类型上标 `AsSearchable()` | 没有子串谓词可放进全文析取 |
| 在没有任何操作符的类型上标 `AsFilterable()` | 过滤器组会是空的，参数会静默地什么都不做 |
| 在不可比较的类型上标 `AsSortable()` | ent 的排序 builder 会跳过它，因而没有 `ByX` 可放进白名单 |
| `DomainSoftDelete` 指名了实体没有的字段 | 手工挂这个标记不受支持，正确方式是嵌 `SoftDeleteMixin` |
| 墓碑字段不是 `Optional` | ent 不生成 `DeletedAtIsNil` 谓词，traverser 编译不过 |
| 自引用边只在一端带注解 | ent 把链式的 `edge.To(…).From(…).Annotations(…)` 交给了*反向* builder，于是关联端静默丢失了它的注解 |
| **实体名与本扩展生成的符号相撞** | 见下 |

一个名为 `ErrorMap` 的实体会让 ent 发射 `type ErrorMap`，而 `entapi_errors.go` 发射
`var ErrorMap`——Go 每个包只有一个标识符命名空间，于是 `redeclared in this block`，发生在
两个你从没写过的文件里，且没有任何东西指出原因。`RegisterSoftDelete` 同理，跨实体相撞同理：
一个字面叫 `ArticleResponse` 的实体会撞上实体 `Article` 生成的响应类型。

保留名检查在图这一层跑，而不是在节点循环里：**相撞的实体不需要带任何注解**——ent 为每个
实体都生成类型，一个光秃秃的 `type ErrorMap struct{ ent.Schema }` 撞得一样狠，而节点循环
恰好会跳过它。派生名单取**最大集合**（一个实体今天没有 create 作用域字段，明天加一个就会
发射 `<Name>CreateRequest`），宁可拒绝一个理论上今天不会相撞的名字，也不因日后加注解而
突然报错。

被拒绝的检查中，除软删除和保留名两类外，其余全部**跳过没有任何 domain 字段的实体**——与
生成循环用的是同一个条件。

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
`_dto.go`/`_filter.go`/`_wiring.go` 会随之而去，而不是作为「引用 ent 已不再生成的 builder」
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
- 创建字段**必填**，当注解显式要求，或 `!Optional && !Default`。`WithRequired(ScopeCreate)`
  只能*增加*严格性，绝不能减少。
- patch 字段**可清空**，当 `Optional && !` 注解要求 update 必填。

`patchFields` 遍历的是 `node.MutableFields()` 而不是 `node.Fields`，所以活下来的字段**必然**
有一个 `Set<Field>`。（`Immutable` + `ScopeUpdate` 会先被生成期检查拒掉，所以这个交集今天
一个字段都不丢——拒绝是作者看到的东西，过滤是让产物保持正确的东西。）

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

## 被接受但不被消费的

**十五个 metadata knob 被存储下来，且不抵达任何模板**：`DomainField.Metadata` 本身，以及
`FieldMetadata` 的十四个字段。它们由十三个构造器写入：

`WithMetadata` · `WithTitle` · `WithDescription` · `WithExample` · `WithFormat` ·
`WithPattern` · `WithRange` · `WithLength` · `WithEnum` · `AsReadOnly` · `AsWriteOnly` ·
`AsDeprecated` · `WithTags`

它们是为 OpenAPI / Swagger spec 生成保留的，是刻意保留而非疏漏——每一个的 godoc 首行都言明
此事。一个测试用反射列出全部 knob，再逐个开关它们、渲染模板，判定谁真的可达；不可达且不在
待接线台账里的 knob 会弄红 CI，**已经可达却还留在台账里的也会**。台账是一份带期限的声明，
不是豁免。

**今天被消费的**：`DomainField.Scopes`、`.Required`、`.Searchable`、`.Sortable`、
`.Filterable`，以及 `DomainEdge.Scopes`、`.JSONKey`。

一条相关的、更高一层的契约：本仓库把死代码当作**测试失败**。一个没人调用的模板函数、一个
没人加载的模板、一个既未被消费也未声明待接线的 knob，都会弄红 CI。

> **实现：** `annotations.go` — `FieldMetadata`、`DomainField.Metadata`、`ensureMetadata` 及
> 十三个构造器；`annotation_surface_test.go` — `pendingKnobs`；
> `funcs.go` — `templateFuncs`（注册表本身：一个 helper 只有出现在这里才能被模板调用）

## 陷阱

按「有多安静地伤害你」排序。

1. **没有调用 `ent.RegisterSoftDelete(client)` 的 client 不过滤任何东西，并且硬删除。**
   包括在测试里。
2. **在你调用 `WithUniqueViolation` 之前，`ErrorMap` 永远不会返回 `ErrAlreadyExists`。**
   一个重复键会原样穿过 `MapError`，表现为 500。
3. **`New{E}Response(nil)` 返回 `(nil, nil)`。** 不是错误。如果你把一次未命中的查询直接喂
   进它，拿到的是一对 nil，而不是 not-found。
4. **`_contains` 需要 `AsSearchable()`。** 一个只标了 filterable 的字符串字段的四个子串参数
   不会发射；form 与 JSON 绑定会丢弃未知 key 而不报错，所以 `?name_contains=x` 变成一个
   *未过滤*的查询，而不是一个 400。
5. **没有任何预设授予查询标记。** 只写 `DefaultField()` 得到的是一个空过滤器结构体和一个空
   排序白名单。
6. **在 `Immutable()` 字段上用 `DefaultField()` 一定会让生成失败**——它授予了 `ScopeUpdate`。
   改用 `CreateOnlyField()` 或 `OutputOnlyField()`。
7. **PATCH body 里出现的 `Immutable()` 字段会被 `encoding/json` 在任何验证器运行之前丢弃。**
   拒绝它需要你的 handler 用 `DisallowUnknownFields`；生成器看不见它。（合法 key 的大小写
   *变体*会被拒绝——真正未知的 key 不会。）
8. **`entapi.IsNotFound` 不是 ent 的 `IsNotFound`。** 生成模板以*不限定*的形式调用后者，
   使其绑定到你包内 ent 生成的谓词。加上限定符照样编译，然后静默地什么都匹配不上。
9. **每个 metadata 构造器都是 no-op。** `WithFormat("email")` 不验证任何东西。
10. **`DeleteBatch` 对匹配不到的 id 返回计数而非错误。** 那个 `int` 是你了解「实际存在多少
    个」的唯一途径；空列表删除零行，这是 ent 对无参 `IDIn` 的读法，不是这里写的守卫。
11. **`Page.Size` 是钳制后的 size**，而一个超界的请求永远不是错误——`ListRequest.Validate()`
    对 `Size` 和 `Page` 只字不提。
12. **`ErrorMap` 是普通包级变量，不带同步。** 请在构造 client 处赋值，不要在服务运行中改。
13. **一个你复制并修改过的生成文件仍带着 marker**——清理会删掉它。请剥掉首行。

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

以下符号曾经存在于本 module，现已删除，且**没有兼容别名**——一个保留耦合的别名，比破坏
更糟。

| 已删除 | 改用 |
|---|---|
| `Base{Entity}Service`、`Base{Entity}Handler`、`SetSelf`、生成的 hook | 生成的自由函数（`Get{E}`、`List{Es}`、…）；需要不同行为就写你自己的函数 |
| `ExtensionConfig.GenerateBaseService`、`.GenerateBaseHandler`、`WithBaseService`、`WithBaseHandler` | 无对应物——基类不再存在 |
| `{Entity}EntToResponse` | `New{Entity}Response`，它返回 error 而不是在错误时返回 nil |
| `Apply{Entity}CreateRequest`、`Apply{Entity}UpdateRequest`（自由函数） | `Valid{Entity}…Request.Apply` |
| `Cursor`、`PageInfo`、`EncodeCursor`、`DecodeCursor`、`ListRequest.Cursor` | 无——分页只有 offset |
| `DomainField.Sensitive`、`AsSensitive` | 不给字段 `ScopeResponse` |
| `DomainField.UniqueLookup`、`.RangeLookup`、`.Validation` | `AsFilterable()`（运算符从 ent 的 `$field.Ops` 导出）；`Validate()` |
| `DomainConfig.EntityName` | 无——没有读者 |
| 运行时符号住在根包 | 全部搬到 `github.com/githonllc/entapi/runtime` |

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
