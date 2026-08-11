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
以及错误分类、五个三步 handler 和 `ent.API(client)` 背后的端点 manifest。

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
- [陷阱](#陷阱) · [限制](#限制) · [与 DESIGN-v2 的偏离](#与-design-v2-的偏离) · [与 DESIGN-v3 的偏离](#与-design-v3-的偏离) · [迁移注记](#迁移注记)

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
| `github.com/githonllc/entapi/runtime` | **生成的代码**与你的 handler / service 代码 | `ListRequest`、`SortSpec`、`Page[R]`、`ListPage`、`GetOne`、`SaveOne`、`BindJSON`、`Status`、`WriteJSON`、`WriteProblem`、`FieldError`、`Endpoint`/`Op`、`WithActor`/`ActorFrom`、错误 sentinel 与 mapper、filter/pointer/软删除 helper |

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

一共四个选项。`WithEntAPIPackage` 改写生成文件 import 的 runtime 路径，默认值就是
`github.com/githonllc/entapi/runtime`，所以只在你 vendor 了一份副本时才有意义。
`WithStrictQueryOperators()` 会把无法识别的操作符前缀变成校验错误；它默认关闭，让裸
RFC-3339 时间戳仍能作为整值等值字面量工作。`WithOpenAPITitle` 与
`WithOpenAPIVersion` 设置生成的 `openapi.yaml` 的 `info.title`
与 `info.version`；不设时分别默认为 ent 包名加 `" API"` 和 `0.0.0`。version **刻意不读
git tag**：生成不得依赖工作树状态，否则一次测试跑完，干净的 checkout 就不再干净。
`NewExtension(cfg)` 直接接受 `*ExtensionConfig` 且对 nil 安全。

扩展只挂一个 `gen.Hook`。`Templates()` 只返回软删除的 `config/init/fields/*` partial；
所有独立输出都由 hook 自己渲染并写盘。

> **实现：** `extension.go` — `Extension`、`ExtensionConfig`、`NewExtension`、
> `NewExtensionWithOptions`、`Option`、`WithEntAPIPackage`、`WithStrictQueryOperators`、
> `WithOpenAPITitle`、`WithOpenAPIVersion`、`defaultEntAPIPackage`、`Hooks`、`Templates`、
> `Annotations`、`Options`、`ConfigAnnotation`

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
边默认不出现在响应中；`api.Expand()` 把它纳入响应。对于自引用边对，两端的注解存在性必须
一致：纳入的一端使用 `api.Expand()`，排除的一端使用 `api.EdgeAnnotation{}`。这个零值注解
表示该端已经过明确考虑并被排除，从而无需引入第二个 edge 词也能表达单向自引用扩展。

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

外加每个 schema 五个文件，各自有独立的发射条件：

| 文件 | 生成条件 | 声明 |
|---|---|---|
| `entapi_errors.go` | 至少一个实体产出了接线 | `ErrorMap` |
| `entapi_http.go` | 至少一个实体带 `api.Resource()` | `APIOption`、`APIHandler`、`API(client)`、`With`、`Endpoints`、每个可达操作一个 `{Op}{E}Endpoint()` accessor、`OpenAPIEndpoint()`、`ServeHTTP`、`Mount` 与端点 manifest |
| `openapi.yaml` | 至少一个实体产出了接线 | 描述全部生成端点的 OpenAPI 3.1 文档 |
| `entapi_openapi.go` | 至少一个实体产出了接线 | 该文档的 `//go:embed` 与未导出的服务函数 |
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
> `renderHTTPFile`、`renderOpenAPIFile`、`renderOpenAPIEmbedFile`、
> `renderSoftDeleteFile`、`pendingFile`；`cleanup.go` —
> `errorMapFileName`、`httpFileName`、`openapiFileName`、`openapiEmbedFileName`、
> `softDeleteFileName`、`isCleanupCandidate`；`funcs_openapi.go` — 文档的 YAML 成形助手；
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

### 用 With 提供自定义操作实现

每个可达操作都会生成一个 `{Op}{Entity}Fn` 类型，其签名与对应 wiring 函数逐字相同。
`With` 只接受这些生成的函数类型：`APIOption` 的方法未导出；被 `Except` 删除的操作既不生成
Fn 类型，也没有第二条路径能指向那个定制点。

`With` 有三条固定规则：

- **可变参数等价于链式调用：** `With(a, b).With(c)` 等价于 `With(a, b, c)`。
- **后者胜出：** 两个 option 自定义同一操作时使用后一个。
- **nil 在构造时 panic：** nil `APIOption` 与 typed-nil Fn 都会在接线时被拒绝。

`With` 原地修改并返回同一个 `*APIHandler`。必须在开始服务前完成接线；请求已经开始后再调用
会造成 data race，行为未定义。method value 是一种很小的 service 注入：它以闭包形式保留 receiver：

```go
type ArticleService struct{ patches atomic.Int64 }

func (s *ArticleService) Patch(ctx context.Context, client *ent.Client, id uuid.UUID,
    request *ent.ValidArticlePatchRequest) (*ent.ArticleResponse, error) {
    s.patches.Add(1)
    return ent.PatchArticle(ctx, client, id, request)
}

service := new(ArticleService)
api := ent.API(client).With(ent.PatchArticleFn(service.Patch))
```

每个未 Except 的 Resource 恰好得到这些 Go 1.22 pattern：

| Pattern | 结果 |
|---|---|
| `GET /articles` | 裸 `{"data","total","page","size"}` page，200 |
| `POST /articles` | 裸 resource，201；没有 `Location` header |
| `GET /articles/{id}` | 裸 resource，200 |
| `PATCH /articles/{id}` | 裸 resource，200 |
| `DELETE /articles/{id}` | 空 body，204 |
| `GET /openapi.yaml` | 生成的文档，200，`application/yaml` |

错误统一是 RFC 9457 `application/problem+json`；`WriteProblem` 写出
`type: "about:blank"`、title、status、detail，并在 error chain 含 `*FieldError` 时加
`field`。bind 失败是 400，生成 `Validate` 失败是 422，不支持的 media type 是 415，超限
body 是 413；中间步骤的 sentinel 映射到 404/409，List validation 是 400，Create/Patch
validation 是 422。Get/Delete 没有 validation 分支。未分类错误是 500。Save-time Ent
`ValidationError` 映射为 422；Ent 给出字段名时，problem response 同时带 `field`。

POST 与 PATCH 只接受 `application/json`，允许 media-type 参数。body 在读取前被限制为
**1 MiB，且没有配置旋钮**。未知 key 会与生成的 create/patch tag 数据比较，因此 PATCH 中
的 Immutable key 会按名字被拒绝，不会静默丢弃。这三条规则都住在 `entapi.BindJSON` 里，
手写 handler 同样可以调用——见[自己写 handler](#自己写-handler)。

`WithActor` / `ActorFrom` 让认证主体穿过 middleware。

**它们走的是 request context，而生成的 handler 只看得到这一个容器。** handler 读的是
`r.Context()`，所以 actor 必须用 `r.WithContext(entapi.WithActor(...))` 写进去：

```go
next.ServeHTTP(w, r.WithContext(entapi.WithActor(r.Context(), user.ID)))
```

第三方 router 自带的 per-request 存储是**另一个**容器：Gin 的 `c.Set` 写进 `gin.Context`，
Echo 的 `c.Set` 写进 `echo.Context`。两者都到不了 `r.Context()`，于是生成的 handler 以及
它背后的定制点根本找不到 actor——又因为 `ActorFrom` 报告的是「不存在」而不是报错，这件事
的表现是 actor 莫名其妙是 nil，而不是一个错误。在这类框架的 middleware 里认证时，要把
request 本身替换掉：

```go
func withAuth(c *gin.Context) {
	user, err := verifyToken(c.GetHeader("Authorization"))
	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Request = c.Request.WithContext(entapi.WithActor(c.Request.Context(), user.ID))
	c.Next()
}
```

### 生成的 OpenAPI 文档

`ent/openapi.yaml` 与代码一起生成、一起提交，所以对外暴露的面出现在 PR diff 里，可以像代码
一样被评审。`entapi_openapi.go` 内嵌同一份字节并在 `GET /openapi.yaml` 上提供，磁盘上的与
线上服务的因此不可能漂开。

它是**推导物**，不是第二份描述：path 与 method 来自端点 manifest 用的同一个 `resourceOps`，
所以被 `Except` 掉的操作在两边同时消失；响应 schema 来自 DTO 用的同一个选择器，所以 Ent 的
`Sensitive()` 字段无法在其中一边复活；每个过滤参数的算子前缀来自该字段自己生成的允许集。

读它之前值得知道三件事：

- **没有 `servers`，path 不带前缀。** 挂载前缀是部署事实——`http.StripPrefix` 跑在你的
  `main` 里，远在生成之后，而同一个二进制可以同时挂在 `/api/v1` 和裸根上。3.1 的默认值是
  相对的 `/`，那是唯一不会说谎的取值。
- **过滤参数是 `type: string`**，带 `pattern` 与 description。这是算子进值的线格式的代价：
  `gt:5` 不是整数。description 承载 OpenAPI 表达不了的东西——该字段接受的算子前缀，以及
  「重复同一个参数会把谓词 AND 起来」。
- **`GET /openapi.yaml` 在 `Endpoints()` 里，但文档不描述它自己。** 在 manifest 里，是为了让
  你能用包裹 CRUD 端点的同一个循环包裹或丢弃它；不在文档里，是因为它不属于资源面。

首行是注释形式的归属 marker，cleanup 依据它删除陈旧文档，和其他生成文件一视同仁。删掉那一行，
文件就退出了 cleanup 的删除候选，因此在这份文档不再被生成之后仍会留存——但它**并不**阻止下一次
生成把它覆盖掉：写入方把新渲染的字节 rename 就位，全程不读磁盘上原有的内容。如果你需要
`servers`、需要前缀、或者需要任何本生成器拒绝猜测的东西，不要去改这份生成的文档：用你自己的
文件名另存一份，从 `Endpoints()` 出发自己注册路由、跳过或替换 `GET /openapi.yaml` 那一行，把你那
份挂在那里。（`Endpoints()` 返回的是副本，`ServeHTTP` 与 `Mount` 仍然提供生成的那一份；跳过必须
发生在你自己注册的 router 里。）

**升级风险：** `ent/openapi.yaml` 是一个再普通不过的文件名，所以升级前就在该路径手写了文档的
消费者，会在升级后的第一次生成时被静默覆盖——升级前请先把它挪走。

一条诚实的残余：与其他所有生成文件不同，这份文档落盘前没有语法门。标准库没有 YAML parser，
所以模板 bug 会先落盘、事后才被抓到——由 fixture 断言，以及
`internal/fixtures/httpdemo/e2e` 里的 OpenAPI 3.1 验证器。那是个嵌套模块，正是为了让验证器
依赖留在本模块之外。

### 注册导出的端点

组合方式是一架梯子，站哪一级取决于你要点名多大一片面：

| 级 | 拿到的东西 | 什么时候用 |
|---|---|---|
| `Get{E}`、`List{Es}`、`Create{E}`…… | 只有操作，不带 HTTP | handler 由你自己写 |
| `Get{E}Endpoint()`、`List{Es}Endpoint()`、`OpenAPIEndpoint()` | 按名字拿到的单个 `entapi.Endpoint` | 往自己的路由器上注册少数几个端点，路径也可以另选 |
| `Endpoints()` | 全部端点，按注册顺序 | 策略是成批的——「包住所有写操作」「按实体切分」 |
| `Mount(mux)`、`ServeHTTP` | 整棵树 | 生成的路径就是你要的路径 |

第一级不是梯子上的一级，而是梯子立在的那块地：wiring 函数就是操作本身，不带 HTTP，
也不涉及任何 `entapi.Endpoint`。`With` 换掉的是生成的 handler 调用的那个实现，不是你自己
代码调用的那个——直接调用 `ListArticles` 不受它影响。

它上面那三级是同一片面，不是三份：`Endpoints()` 就是 accessor 返回值组成的 slice，
`Mount` 走的也是这份 slice。混着用不可能给同一个端点得出两种描述。

#### 按名字取单个端点

每个可达操作都会生成一个 accessor，名字跟着它服务的 wiring 函数走——
`GetArticleEndpoint`、`ListArticlesEndpoint`、`CreateArticleEndpoint`、
`PatchArticleEndpoint`、`DeleteArticleEndpoint`——生成的文档则是 `OpenAPIEndpoint`：

```go
api := ent.API(client)
public := http.NewServeMux()

list := api.ListArticlesEndpoint()
public.Handle(list.Method+" /v1/articles", list.Handler)

// 换一条路径：Bind 喂给端点的是它自己的 placeholder 名，所以你注册的路径里一个都不必带。
featured := api.GetArticleEndpoint()
public.Handle(featured.Method+" /v1/featured", featured.Bind(func(string) string { return featuredID }))

public.Handle("GET /v1/openapi.yaml", api.OpenAPIEndpoint().Handler)
```

这一级的价值就在这个名字上。「端点存不存在」变成编译期事实：`Except(api.OpDelete)` 会连同
路由一起删掉 `DeleteAuditLogEndpoint`，于是点名它的注册语句直接编译不过，而不是启动时才对着
一个不存在的端点炸掉——而且从注册那一行可以直接跳到生成的 handler。这里刻意没有
`EndpointFor(entity, op)` 这类查表函数：查表会把上面两个退化原样留着。

这样取到的端点不是快照。它的 handler 在请求时透过 `*APIHandler` 读当前实现，所以取完之后
再调 `With` 依然生效。

#### 遍历全部端点

`Endpoints()` 按确定的注册顺序返回 `[]entapi.Endpoint{Method, Path, Handler, Entity, Op}`。每次
调用都返回一份新 slice，修改其中的行不会改变 `ServeHTTP` 或后续 `Mount` 的注册来源。但其中
的行同样不是快照——理由和单个 accessor 取到的端点一样：每一行带的 handler 都在请求时透过
`*APIHandler` 读当前实现，所以 `Endpoints()` 返回之后再调 `With` 依然生效。这是
数据导出，不是修改 API：用 `Except` 删除生成端点，用 `With` 提供自定义实现，额外端点直接
注册到消费者自己的路由器。

`Entity` 是 Ent 的类型名（`"Article"`），`Op` 是 `entapi.Op` —— `OpList`、`OpCreate`、
`OpGet`、`OpPatch`、`OpDelete`，不属于任何资源的端点则是 `OpNone`。它们带上了过去被路径
藏起来的身份，于是「按受众切分这棵树」变成一次编译器能检查的比较，而不是对路径文本做匹配
——后者拼错了就什么都选不中，且什么都不报：

```go
for _, endpoint := range api.Endpoints() {
    switch {
    case endpoint.Entity == "AuditLog":            // 整个实体划进内网面
        internal.Handle(endpoint.Method+" "+endpoint.Path, endpoint.Handler)
    case endpoint.Op == entapi.OpNone:             // 文档本身，不属于任何资源
        public.Handle(endpoint.Method+" "+endpoint.Path, endpoint.Handler)
    default:
        public.Handle(endpoint.Method+" "+endpoint.Path, requireScope(endpoint)(endpoint.Handler))
    }
}
```

`Op` 是独立类型而不是裸 string，正是为了这一点；它没有复用 `api.Op`，因为 runtime 不 import
任何 Ent 包，但两边的取值由一个 drift guard 钉在一起。

完整的 Gin adapter 写在消费者侧；框架不依赖 Gin：

```go
func mountGin(r *gin.Engine, api *ent.APIHandler) {
    for _, endpoint := range api.Endpoints() {
        r.Handle(endpoint.Method, entapi.ColonPath(endpoint.Path), func(c *gin.Context) {
            endpoint.Bind(c.Param).ServeHTTP(c.Writer, c.Request)
        })
    }
}
```

`ColonPath` 只把完整的 `{name}` segment 改写成 `:name`，其他 segment 原样保留。
`Endpoint.Bind` 接受一个 `func(string) string`，与 `gin.Context.Param`、
`echo.Context.Param` 的签名完全一致。chi 与 fiber 各需一行 closure：`chi.URLParam`
还要接收 request，而 `fiber.Ctx.Params` 带有 `defaultValue ...string` 变参，签名实为
`func(string, ...string) string`，无法直接赋值。因此 Echo 使用 `entapi.ColonPath(endpoint.Path)` 与
`endpoint.Bind(c.Param)` 这两个调用，再传入它的 response writer 与 request 即可。

placeholder 名称取自 `Endpoint.Path`，因此没有任何地方硬编码 `"id"`：如果生成器以后发射
第二个 placeholder，这样写的 adapter 无需修改就会自动接上。端点没有 placeholder 时，
`Bind` 返回 `e.Handler` 本身，所以每个端点都调用它不会给不需要绑定的端点增加成本。

挂载时传入常量 closure 可以把端点钉到固定 id：
`endpoint.Bind(func(string) string { return actorID })` 能用同一条生成的
`GET /users/{id}` 端点提供 `/v1/me`，无需再手写第二层 wrapper。

有一个路由差异不会被这层 adapter 掩盖。Go 1.22 `ServeMux` 把 `%2F` 视为同一个编码 segment
的一部分，并在 `PathValue` 中给 handler 解码后的 `/`；Gin 默认按已经解码的 `URL.Path` 匹配，
所以 `/articles/a%2Fb` 不匹配 `/articles/:id`。若标识符需要编码斜杠，应显式选择并测试消费者
路由器的策略。

这份元数据也能选择性包裹外层 middleware，不必在生成 handler 内增加 hook。`Op` 说明这个端点
在做什么，于是「所有写操作」不必再拼成一串 method：

```go
for _, endpoint := range api.Endpoints() {
    handler := endpoint.Handler
    switch endpoint.Op {
    case entapi.OpCreate, entapi.OpPatch, entapi.OpDelete:
        handler = requireAuth(handler)
    }
    mux.Handle(endpoint.Method+" "+endpoint.Path, handler)
}
```

router 层未匹配的 path/method 仍保留 stdlib mux 的纯文本 404/405（405 含 `Allow`），而非
problem+json。这个 residue 是有意的：catch-all 会让挂进消费者 mux 与直接服务整棵生成树的
行为不同。

### 自己写 handler

每个生成的 handler 都是 bind → call → write，而这三步都是导出的 runtime 函数。手写的
endpoint——不论跑在 `net/http`、第三方 router，还是根本不属于本生成器的实体——只要调用它们，
就能得到同样的行为，不必抄一份生成的函数体：

```go
func BindJSON(w http.ResponseWriter, r *http.Request, tags []string, dst any) error
func Status(err error, onValidation int) int
func WriteJSON(w http.ResponseWriter, status int, v any) error
```

`BindJSON` 施加的是同样三条 bind 规则：1 MiB 的 `MaxBytesReader` 上限、
`application/json` media-type 检查，以及拒绝任何不在 `tags` 里的 body key。它的错误是
**完备的**：它返回的每一个错误都恰好包装
`entapi.ErrUnsupportedMediaType`、`entapi.ErrRequestTooLarge`、`entapi.ErrValidation`
三者之一，所以 `Status` 能把它们全部分类，不存在需要你另写分支的第四种情况。它接收 `w` 只是
因为 `http.MaxBytesReader` 需要，**它自己什么都不写**——response 完全归调用者所有，包括它长什么样。

`Status` 对两个 bind sentinel 返回 415 与 413，对 `entapi.ErrNotFound` 返回 404，对
`entapi.ErrAlreadyExists` 返回 409，对 `entapi.ErrValidation` 返回 `onValidation`，其余
一律 500，nil 错误返回 0。`onValidation` 这个参数就是 400-vs-422 约定的全部：生成的 handler
对 **bind 失败传 400**（请求本身不成形），对**中间步骤失败传 422**（请求解析成功，被领域逻辑
拒绝）。照传这两个值，自定义 endpoint 的回答就与生成的一致；传别的，它就按你选的回答。

`WriteJSON` 先 marshal 再碰 response，所以 marshal 失败会变成一个干净的 500 problem
response，而不是一个写了一半的 200。

这里没有任何东西依赖 Ent——`entapi/runtime` 只 import 标准库——所以你可以配自己的请求类型、
自己的 tag 列表和自己的 response 信封。下面这个 Gin endpoint 用调用者自己的信封作答，而非
problem+json：

```go
type createTicketRequest struct {
    Subject string `json:"subject"`
}

var createTicketTags = []string{"subject"}

func (s *server) createTicket(c *gin.Context) {
    var req createTicketRequest
    if err := entapi.BindJSON(c.Writer, c.Request, createTicketTags, &req); err != nil {
        c.JSON(entapi.Status(err, http.StatusBadRequest), envelope{Error: err.Error()})
        return
    }

    ticket, err := s.tickets.Create(c.Request.Context(), req.Subject)
    if err != nil {
        c.JSON(entapi.Status(err, http.StatusUnprocessableEntity), envelope{Error: err.Error()})
        return
    }

    c.JSON(http.StatusCreated, envelope{Data: ticket})
}
```

生成的 tag 切片（`articleCreateRequestTags` 等）是你 `ent` 包里的未导出成员，所以包外的
handler 自己声明一份——这正是本意：`tags` 是调用者的白名单，不是生成的那一份。

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

`Has<Field>()` 在请求类型和它的 `Valid…` 包装器上**都会**生成。定制点的签名只交给它
验证过的类型——原始请求是包装器里的未导出字段，而 body 已被 handler 读完——所以存在性
若止步于 `Validate`，真正要用它的那段代码就永远读不到：

```go
func (s *UserService) Patch(ctx context.Context, db *ent.Client,
	id uuid.UUID, v *ent.ValidUserPatchRequest) (*ent.UserResponse, error) {
	resp, err := ent.PatchUser(ctx, db, id, v)
	if err == nil && v.HasStatus() {          // 包装器自己就能回答，不必回到原始请求
		s.mailer.NotifyStatusChange(ctx, id)
	}
	return resp, err
}
```

### 读出值，而不只是存在性

存在性只是三态里的两态。它区分**缺席**与**带来了**，但不区分*带来了一个值*与*带来了一个
显式 null*——而这两者是相反的请求。所以 `Valid…PatchRequest` 上每个字段还有一个 comma-ok
的**值读取器**，用字段自己的名字命名：

```go
func (v *ValidUserPatchRequest) SuspendedUntil() (time.Time, bool)
```

| 读取器 | `Has<Field>()` | payload 带来的 | `Apply` 将会 |
|---|---|---|---|
| `ok == true` | `true` | 一个值 | `Set` 它 |
| `ok == false` | `true` | 一个显式 `null` | `Clear` 它 |
| `ok == false` | `false` | 什么都没有 | 什么都不写 |

中间那一行只对可清空字段可达——`Validate` 会拒绝任何 schema 未声明 `Optional()` 的字段上的
显式 null，所以在其余字段上 `ok` 恰好等于 `Has<Field>()`。用 Go 手工构造、没有经过解码的
请求一律读作缺席，这与 `Apply` 的行为是同一个答案。

这正是让跨字段规则只靠包装器就能写出来的东西：

```go
if _, ok := v.SuspendedUntil(); ok && status != user.StatusSuspended {
	return nil, fieldError("suspended_until", "only settable while suspended")
}
```

在读取器出现之前，出包装器的唯一出口是 `Apply`，所以这条规则必须分配一个永不执行的 update
builder，把请求 apply 上去，再从 `Mutation()` 读回来——为回答一个关于请求的问题，把业务逻辑
耦合到了 Ent 的 mutation 词汇上（#113）。

只有包装器有读取器。**原始**请求本就把它的 `*T` 字段导出为结构体字段，值在那里已经可达；
包装器是唯一藏起它的东西，也是定制点唯一拿得到的东西。

因此有两个字段名会被拒绝：Go 名为 `Apply` 的 patch 可见字段，以及 `x` / `has_x` 这样的
patch 可见字段对——见[生成会失败，而这正是设计](#生成会失败而这正是设计)。

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
> `WidgetPatchRequestTags()`、
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

每个字段只得到 Ent 谓词与上述 wire 词汇的交集。`like:`、`ilike:`、`prefix:` 与
`suffix:` 还要求 `api.Searchable()`。`_ieq` 没有 wire 写法。无前缀的 `*` 与 `?`
仍是普通等值字面量，不会隐式变成 `LIKE`。

解析严格按六条规则执行：空的无前缀值忽略，但空 `eq:` 有效；无冒号就是等值；字段允许的
前缀应用该操作符；全局已知但字段不允许的前缀报校验错误；未知前缀把整个值回退为等值
（因此裸 RFC-3339 时间戳可用），但启用 `WithStrictQueryOperators()` 后改为校验错误，
此时时间戳必须写成 `eq:`；显式 `eq:` 用来转义看似操作符的值。转换错误会点名字段和值并
包裹 `entapi.ErrValidation`。

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

**它们每一个都收 `*Client`，这就是事务契约：本包永不生成事务边界。** 没有 `*Tx` 变体，
也没有 tx-from-context 查找。要把生成的某一步纳入你自己的事务，把 ent 已经绑定到该事务的
那个 client 递给它：

```go
tx, err := db.Tx(ctx)
// ...
resp, err := ent.PatchUser(ctx, tx.Client(), id, v)   // 生成的那一步，在你的事务里
_, err = tx.Client().AuditLog.Create(). /* ... */ .Save(ctx)
err = tx.Commit()
```

两条后果值得直说。`*Tx` 变体会让每个定制点长出一个孪生签名，而「定制点签名与 wiring 函数
逐字相同」正是让一次写错的替换变成**编译错误**而不是运行期意外的那个东西。另外
`ent.API(client)` 持有的是根 client，所以自定义实现在 HTTP 路径上拿到的 `db` **不是**
tx-bound 的——那里若要事务，自己开，再经 `tx.Client()` 调生成的 wiring。

任何地方都没有硬编码的标识符类型——id 来自你的 schema 的 `$.ID.Type`，并作为类型参数抵达
运行时，所以一个 `int` 主键连 import 都不需要。

每个导出的接线函数都**恰好一次**地经由 `ErrorMap.MapError` 返回。文件里还有一组未导出的
辅助函数（`{entity}Get`、`{entity}ByID`、`{entity}Reloaded`），它们存在正是为了让一次
create 或 update 在重新读取时不会映射两次。于是
`errors.Is(err, entapi.ErrNotFound)` 在你的 handler 边界上直接可用，无需拆开 ent 的
错误类型。

`ErrorMap` 由模板连同 Ent 的三个生成谓词和字段名提取器一起发射：

```go
var ErrorMap = entapi.NewErrorMapper(IsNotFound, IsConstraintError).
    WithValidation(IsValidationError, func(err error) (string, bool) {
        var ve *ValidationError
        if errors.As(err, &ve) { return ve.Name, true }
        return "", false
    })
```

三个谓词与 `ValidationError` 都**不带限定符**，所以它们绑定到 Ent 生成到**同一个包**里的符号。
这些类型与谓词属于每个消费者项目，stdlib-only runtime 因而只接收函数，永远不命名 Ent 类型。

`MapError` 的固定顺序是：nil → not-found → validation（提取成功时再带 `FieldError`）→
constraint **且** unique → 原样返回。unique 判定仍受 Ent 的 `IsConstraintError` 门控；纯文本
永远不能把任意错误直接分类。

`API(client)` 会按 dialect 自动安装 unique 判定，除非 `HasUniqueViolation` 发现消费者已经安装：

| Dialect | 判为 unique | 闭合失败并落到 500 |
|---|---|---|
| `postgres` | 实现 `SQLState() string` 且值为 `23505`；没有该方法时，文本含 `violates unique constraint` | 一旦 SQLSTATE 存在，非 `23505` 就是权威结果，绝不继续匹配文本。旧 lib/pq 没有 `SQLState()` 时，非英文 `lc_messages` 会漏判 |
| `mysql` | 文本含 `Error 1062` | 其他文本。该 marker 由 go-sql-driver/mysql 从 `MySQLError.Number` 格式化，免疫 locale |
| `sqlite3` | 文本含 `UNIQUE constraint failed` | 其他文本 |
| 其他 | 什么都不装 | 所有 duplicate 都保持未分类 |

三个文本 marker 逐字钉在 Ent v0.14.4 的 `sqlgraph.IsUniqueConstraintError` 上，因此文本契约
与上游共享漂移，而不是另造一份。所有漏判都闭合为 500。`API()` 前安装的自定义判定会被保留；
在 `API()` 后安装则覆盖自动判定。`ErrorMap` 是普通包级变量，不带同步，请在开始服务请求前配置。

一个命名 residue 仍故意不分类：Ent 把清除 required unique edge `Article.author` 报成裸错误
`wiringent: clearing a required unique edge "Article.author"`。它不是 `ValidationError`，不携带任何
sentinel，HTTP 结果是 500。生成的 PATCH 表面无法触发它；只有直接调用 builder 才能触发。

> **实现：** `templates/wiring.tmpl`、`templates/errors.tmpl`；
> `runtime/errors.go` — `ErrNotFound`、`ErrAlreadyExists`、`ErrValidation`、
> `ErrUnsupportedMediaType`、`ErrRequestTooLarge`、`IsNotFound`、
> `IsAlreadyExists`、`IsValidation`；
> `runtime/bind.go` — `BindJSON`、`Status`、`WriteJSON`；
> `runtime/errors_map.go` — `ErrorMapper`、`NewErrorMapper`、`WithUniqueViolation`、
> `WithValidation`、`HasUniqueViolation`、`MapError`；`runtime/errors_dialect.go` —
> `UniqueViolation`；`runtime/query.go` — `ListPage`、`GetOne`、`SaveOne`、`Saver[E]`；
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

过滤同样覆盖**预加载**，不只是顶层查询：`With<Edge>()` 发出的子查询就是目标类型 builder 上
的一次普通查询，走的是同一个 interceptor，所以被删除的行也不会绕过父实体回来。

软删除**不级联**。一行如果它的边指向一个被软删除的目标，它的外键原封不动，并且仍然出现在
每一份列表里——没有任何东西给它打墓碑，也没有任何东西发出警告。把这两条放在一起读，就得到
一个值得在撞上之前先知道的形状：一条声明为 `Required()` 且带 `api.Expand()` 的边，在目标被
软删除之后会以 JSON `null` 的形式返回。这不是违反契约——`openapi.yaml` 为每条展开的边写的
就是 `oneOf [<Target>Summary, null]`——但 schema 说过这条边是必填的，所以它读起来像违反。

**手写代码必须通过 `<Edge>OrErr()` 读取边的状态。** 一次普通预加载之后，被软删除的目标留下
的是 `Edges.X == nil` 且**没有 error**，这跟「根本没人加载这条边」看到的东西一模一样。而 nil
判断正是消费者默认会写的那种代码，它会悄无声息地丢掉这个区分：

```go
d, err := client.Draft.Query().Where(draft.ID(id)).WithDoc().Only(ctx)
// ...
target, err := d.Edges.DocOrErr()
switch {
case err == nil:
	// 目标是活的
case ent.IsNotFound(err):
	// 已加载，但没有对应的行：目标被软删除，或者外键悬空
	target = nil
default:
	// 从未加载——这是查询写错了，不是一种数据状态
	return err
}
```

生成的代码本来就是这么做的；`{entity}_dto.go` 里的 `New<Entity>Response` 就是范例。

要把这些行排除掉，就在普通 ent 里按边过滤。边谓词是一条普通的 SQL 子查询，自己不带
traverser，所以墓碑条件必须显式写出来：

```go
client.Draft.Query().Where(draft.HasDocWith(doc.DeletedAtIsNil())).All(ctx)
```

> **证明：** `internal/softdeleteproof/softdelete_test.go` —
> `TestRequiredExpandedEdgeToSoftDeletedTarget` 对着真实 SQLite 断言了全部四条：预加载到的边
> 是「已加载但不存在」而不是「未加载」、`NewDraftResponse` 返回 `"doc": null` 且无 error、
> 拥有这条边的 `Draft` 仍然在列表里、以及 `HasDocWith(DeletedAtIsNil())` 能把它排除掉。

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
| 边被 Ent 标为 `Required()` 却没有声明 `edge.Field(…)`，且未 `Except(OpCreate)` | Ent 要求每次 create 都设置这条边，但没有任何 setter 能进入 create 请求，于是每次 create 都会栽在 `missing required edge` 上。修复建议只对**持有外键**的那一端提供 `edge.Field(…)`——那是 Ent 唯一接受它的一端 |
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
| **patch 可见字段名与 patch DTO 生成的方法相撞** | 见下 |

图级的 `API`、`APIHandler`、`ErrorMap` 都是保留名。一个名为 `ErrorMap` 的实体会让 ent 发射 `type ErrorMap`，而 `entapi_errors.go` 发射
`var ErrorMap`——Go 每个包只有一个标识符命名空间，于是 `redeclared in this block`，发生在
两个你从没写过的文件里，且没有任何东西指出原因。跨实体相撞同理：一个字面叫
`ArticleResponse` 或 `DeleteArticleFn` 的实体会撞上实体 `Article` 生成的类型。五个 Fn 名称
按最大宽度保留，即使对应操作已 Except 也不收窄。

保留名检查在图这一层跑，而不是在节点循环里：**相撞的实体不需要带任何注解**——ent 为每个
实体都生成类型，一个光秃秃的 `type ErrorMap struct{ ent.Schema }` 撞得一样狠，而节点循环
恰好会跳过它。派生名单取 resource 可能产出的**最大集合**，宁可拒绝一个理论上今天不会相撞
的名字，也不因日后加注解而突然报错。

字段名有它自己的、**方法层面**的同类问题，而两个检查谁也不覆盖谁：方法名活在接收者的命名
空间里，而不是包的命名空间里。[值读取器](#读出值而不只是存在性)用字段自己的名字命名，于是
有两种 patch 可见的字段名会把构建弄坏：

| 被拒绝的 | 撞上了 |
|---|---|
| Go 名为 `Apply` 的字段 | 同一个包装器上的 `Apply(b *<Entity>UpdateOne)`——`method Apply already declared` |
| `x` 与 `has_x` 这一对 | 为 `x` 生成的存在性方法 `HasX()` 撞上结构体字段 `HasX`——`field and method with the same name HasX` |

第二种**比读取器更老**：`Has<Field>()` 自 #98 起就在原始请求上，而 `has_x` 在那里正是一个
同名的结构体字段，所以这一对早就编译不过。两条消息都指向 `.StorageKey(…)`，因为 JSON tag
是从它拼出来的——Go 名要改，wire key 不必跟着改。

`Except(api.OpPatch)` 不能豁免其中任何一条。它移除的是端点与 wiring，从不移除请求 DTO，
所以 patch 请求、它的包装器、`Apply` 和读取器照样会生成。

HTTP 检查全部跳过没有 `api.Resource()` 的实体——与生成循环条件一致；软删除与保留名仍是
全图检查。

反过来，`Optional().Nillable()` 以及切片/映射上的具名类型*是*会被生成的，因为对它们存在
正确的产物。见[字段形态](#字段形态)。

> **实现：** `schema_conflicts.go` — `checkGraphConflicts`、`nodeConflicts`、
> `queryConflicts`、`immutableUpdateConflict`、`unusableSoftDeleteField`、
> `asymmetricSelfEdgeConflicts`、`asymmetricSelfEdgeConflict`、`reservedNameConflicts`、
> `graphSymbolConflicts`、`derivedName`、`derivedEntityDecls`、`derivedEntityNames`、
> `derivedNameConflict`、`patchMethodCollisions`、`patchApplyCollision`、
> `patchPresenceCollision`、`fieldHasOp`、`markerList`、`errorMapSymbol`、
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
（`<entity>/`、`predicate/`、`migrate/` …）位于目标目录之下，永远不是候选。候选是目标目录的
`.go` 文件，外加恰好一个名字 `openapi.yaml`：那是本扩展唯一不是 Go 源码的产物，而把后缀放宽到
全部 YAML 只会白白把消费者自己的文档拉进删除面。不带 marker 的
文件会被留下并**打一条日志**说明为什么；ent 自己的 `Code generated by ent, DO NOT EDIT.`
刻意不匹配。

> **你的逃生舱就是那行 marker——但它只挡清理。** 删掉一个生成文件的首行，cleanup 就不再把它
> 当作删除候选，于是在该文件不再被生成之后，你这份仍会留存。它**不**保护仍在被生成的文件：
> 每一轮生成都会把新字节 rename 覆盖到该路径上，全程不读原有内容。反过来，一个你从生成产物
> 复制来、却忘了剥掉文件头的文件**会被删除**。
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

1. **未知 dialect 不会得到宽松的 unique 猜测。** 重复键保持未分类并表现为 500。自定义
   dialect 请在 `API()` 前调用 `WithUniqueViolation`。
2. **`New{E}Response(nil)` 返回 `(nil, nil)`。** 不是错误。如果你把一次未命中的查询直接喂
   进它，拿到的是一对 nil，而不是 not-found。
3. **`like:`、`ilike:`、`prefix:` 与 `suffix:` 需要 `api.Searchable()`。** 用在只
   Filterable 的字符串字段时，它们是已知但不允许的操作符，生成 parser 返回
   `ErrValidation`。
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
- **摘要携带哪些标量字段无法从 schema 判定**，所以摘要携带每个响应可见字段减去边。想收窄
  它需要一个新注解。
- **没有乐观锁，丢失更新是已知边界。** 生成的 patch wiring 是
  `v.Apply(db.X.UpdateOneID(id))`，不带版本谓词（`templates/wiring.tmpl` — `Patch{Entity}`），
  所以两个并发 `PATCH` 打同一个字段时，后到者静默胜出。受害面比 PUT 语义窄——本包只有部分
  更新，改不同字段的两个 patch 互不影响。不加版本词，是因为框架没有合法途径知道哪一列是
  版本列，而按名猜 `version`/`updated_at` 正是 #18 退役掉的那类约定推导。出路是定制点：
  用 `With(ent.PatchXFn(...))` 整单元替换那一步，在里面写自己的 `Where(x.Version(v))`，
  并对零行受影响返 409。
- **产物与 ent 的输出同处一个包。** 生成器不建独立的 `dto` 子包，也没有配置项可以换目录；
  它对目标目录的所有权靠 marker 逐文件判定，而不是靠独占一个目录。
- **注解只控制 HTTP 层的生成。** 它们绝不限制你的 service 层能拿一个 ent 实体做什么——
  `Except` 关掉的是 handler、它那一行端点与 `{Op}{Entity}Fn` 类型，wiring 函数与请求 DTO 留在原地。
  唯一例外是可证明无法工作的 create 一族（见「注解模型」）。任何需要强制的东西，必须在
  构造查询的地方强制。
- **生成器包在包初始化时加载全部十个模板。** 这被限制在 `entc.go` 与 schema 文件里；
  `runtime/` 就是把它挡在你的二进制之外的那个东西。

## 与 DESIGN-v2 的偏离

[`docs/DESIGN-v2.md`](docs/DESIGN-v2.md) 的抬头写着「实现未开始」，那是**陈旧的**：它提出的
T3 已经全部落地。三处偏离，都是有意的：

| 设计条目 | 现状 |
|---|---|
| §1.6 产物移到 `ent/dto` 子包 | **未做，且已被取代**。产物落在消费者的 `ent` 包里，handler 解耦由生成的自由函数达成，而不是由包放置达成 |
| §8.1 目录里存在「不是我的」文件 → 拒绝生成 | **未做**。清理把这类文件**留下并记日志**。它原本依赖 §1.6 的独占目录 |
| §8.4 `OutputPackage` 配置项 | **未做**，因 §1.6 未做而无意义。现有选项只配置 runtime import 路径、严格 query 解析或 OpenAPI info；没有一个会移动输出目录 |

设计文档里明确「延后」的 T2（受众维度）同样没有实现，这与设计一致。

## 与 DESIGN-v3 的偏离

[`docs/DESIGN-v3-final.md`](docs/DESIGN-v3-final.md) 的抬头同样写着「实现未开始」，
那也是**陈旧的**：它列的八个切片（#69–#76）已全部落地，分别由 #77、#78、#81、#82、#84、
#85、#86、#87 关闭。读它是为了拿决策与理由，**不要拿它当现行 API** ——其中三处在实现期被
取代了，以代码为准：

| 设计条目 | 现状 |
|---|---|
| §2.1 / §2.5 `ent.API(client)` 返回 `*API`；`func (a *API) Routes()` | 类型是 **`*APIHandler`**（`templates/http.tmpl` — `API`）。`API` 是构造函数的名字，同一个包里 handler 不可能也叫这个。方法自 #118 起叫 **`Endpoints()`** —— 这份 manifest 记的是 handler 契约，不是路由 |
| §4.3 软删除由生成的 `init()` 注册，失败回落显式 `RegisterSoftDelete(client)` | 两者都不存在。#78 改用 Ent 在 `newConfig` 内部执行的 `config/init/fields/*` **partial**（`templates/softdelete_config_init.tmpl`）直接填 hook 与 interceptor，于是 `NewClient`、`Open`、`enttest.Open` 以及之后每一份 config 拷贝都自带它们，既无注册调用也无初始化顺序依赖。`RegisterSoftDelete` 是被**删除**，不是留作回落 |
| §2.3 生成的 handler 开 `DisallowUnknownFields`，被拒字段名从 `encoding/json` 的错误文本里抠 | handler 先把 body 解进 `map[string]json.RawMessage`，再拿 key 与生成的 `{entity}{Op}RequestTags` 数据比对（`templates/handler.tmpl`），经 `entapi.FieldError` 报出那个 key。自行编写 bind 步骤的消费者可从导出的 `{Entity}{Op}RequestTags()` 访问器取得同一份数据，该访问器返回副本。设计文档把「抠错误文本」列为已知残余，这个实现把它消掉了——字段名现在是生成的数据，不是解析出来的字符串。`DisallowUnknownFields` 仍然是消费者自己单独解 DTO 时的决定 |

还有第四条，它既不是被取代、也不是本来就为真：service 示例在验证过的 patch 请求上调
`v.HasStatus()`，而 `Valid…` 包装器根本没有这个方法。这个缺口是**靠生成转发方法**补上的，
不是记成一条偏离——示例对定制点的需要判断得没错，因为定制点收到的只有验证过的类型。
那份文档里的这一行现在能编译了。

那份文档里的其余内容——五个偏离词、`Except` 的三层语义与 create 一族例外、op-in-value 线
格式、`_` 命名空间、413/415 请求硬化、RFC 9457 错误、导出的 manifest，以及 OpenAPI 那几条
裁决——描述的就是已经发布的东西。

## 迁移注记

**破坏性变更（#118）：** `entapi.Route` 改名为 `entapi.Endpoint`，生成的
`APIHandler.Routes()` 改名为 `APIHandler.Endpoints()`。其余一切不变——字段
（`Method`、`Path`、`Handler`、`Entity`、`Op`）、注册顺序、每次调用返回新副本的保证，
以及 `Bind` 的行为都和以前一致，`Op`、它的常量与 `ColonPath` 也都保留原名。旧名字宣称
entapi 在做路由，它并没有：一个 `Endpoint` 是某个生成 handler 的契约记录——你把它组合进
你自己的 router 的数据——而 `ServeHTTP`/`Mount` 背后那个内部 `ServeMux`，只是用同一份
manifest 搭出来的可选便利层。重新生成后重命名调用点：`api.Routes()` → `api.Endpoints()`，
`[]entapi.Route` → `[]entapi.Endpoint`。

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
| `entapi.Route`、`Route.Bind`、生成的 `Routes()` | `entapi.Endpoint`、`Endpoint.Bind`、生成的 `Endpoints()`——字段、顺序、行为完全一致（#118） |
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
