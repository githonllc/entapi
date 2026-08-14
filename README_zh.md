# EntAPI

[![Go Reference](https://pkg.go.dev/badge/github.com/githonllc/entapi.svg)](https://pkg.go.dev/github.com/githonllc/entapi)
[![Go Report Card](https://goreportcard.com/badge/github.com/githonllc/entapi)](https://goreportcard.com/report/github.com/githonllc/entapi)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

一个 [Ent](https://entgo.io) 扩展。给实体标上 `api.Resource()`，它就生成请求类型、响应
类型、查询面、每个操作一个接线函数、标准库 HTTP 路由树，以及一份 OpenAPI 3.1 文档——
全部写进你自己的 `ent` 包，链接的运行时除标准库外不依赖任何东西。字段形态来自 Ent；
注解只表达偏离。

*[English](README.md)* · *[完整指南](docs/GUIDE_zh.md)*

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
`ArticleFilter` 的 `ParseArticleQuery`、多键排序白名单、带预加载计划的 `ArticleResponse`、
错误分类、五个三步 handler、`openapi.yaml`，以及 `ent.API(client)` 背后的端点 manifest。

> ### 状态：v0，从未发布过版本
>
> `git tag` 为空——这个仓库没有打过任何 tag，也没有对外承诺过任何 API。版本策略是 Go 自己
> 的 `v0.x` 约定：**自由破坏，不设弃用窗口**，删除的符号在本文末尾的[迁移注记](#迁移注记)里
> 点名，不留兼容别名。
>
> 代码本身是完整的：`docs/DESIGN-v2.md` 与 `docs/DESIGN-v3-final.md` 两份重设计都已全部
> 落地，剩下的偏离项列在指南的
> [与 DESIGN-v2 的偏离](docs/GUIDE_zh.md#与-design-v2-的偏离)与
> [与 DESIGN-v3 的偏离](docs/GUIDE_zh.md#与-design-v3-的偏离)。已知缺陷见
> [`docs/QUALITY-REVIEW.md`](docs/QUALITY-REVIEW.md)。
>
> 采用前请先读[陷阱](#陷阱)。其中有几条是静默的。

---

## 目录

- [快速开始](#快速开始)——仓库里两个可直接运行的服务
- [安装](#安装) · [三个 import 路径](#三个-import-路径)
- [注解模型](#注解模型)——Ent 事实加五个偏离词
- [生成了什么](#生成了什么) · [生成的 HTTP](#生成的-http)
- [查询面](#查询面) · [请求与响应](#请求与响应)
- [软删除](#软删除) · [生成会失败](#生成会失败而这正是设计)
- [陷阱](#陷阱) · [限制](#限制) · [迁移注记](#迁移注记)
- **[完整指南](docs/GUIDE_zh.md)**——下面每条规则的长版，带理由与源码指针

---

## 快速开始

仓库里有两个完整的服务。每个都是一份 ent schema、一次 `go generate`、一次 `go run`，
从 client 到端口之间没有任何手写代码：

```bash
cd examples/todo        # 单实体，五个端点
go generate ./...
go run .
```

```bash
cd examples/petstore    # 四个实体：边、软删除、一个被 Except 的操作
go generate ./...
go run .
```

两者都监听 `http://localhost:8080`，跑在内存 SQLite 上，并在启动时打印自己的端点
manifest——那份列表来自 `api.Endpoints()`，不是手写的表：

```
GET    /todos
POST   /todos
GET    /todos/{id}
PATCH  /todos/{id}
DELETE /todos/{id}
GET    /openapi.yaml
listening on http://localhost:8080
```

todo 服务的整个 API 面来自四个带注解的字段：

```go
field.String("title").Annotations(api.Searchable(), api.Filterable(), api.Sortable()),
field.Bool("done").Optional().Default(false).Annotations(api.Filterable()),
field.Int("priority").Optional().Default(0).Annotations(api.Filterable(), api.Sortable()),
field.Time("created_at").Immutable().Default(time.Now).Annotations(api.Sortable(), api.ReadOnly()),
```

[`examples/todo/README.md`](examples/todo/README.md) 用真实运行粘贴下来的 `curl` 记录逐个
走遍每个端点；[`examples/petstore/README.md`](examples/petstore/README.md) 对多实体场景做
同样的事，[`examples/petstore/ARCHITECTURE.md`](examples/petstore/ARCHITECTURE.md) 则标出
哪个生成文件回答哪个请求。

## 安装

```bash
go get github.com/githonllc/entapi
```

`go.mod` 声明 `go 1.23`。除 `golang.org/x` 外的直接依赖只有 `entgo.io/ent v0.14.4` 与
`github.com/google/uuid v1.3.0`。

在你的 `entc.go` 里装上扩展：

```go
ext := entapi.NewExtensionWithOptions()

if err := entc.Generate("./schema", &gen.Config{
    Target:  "../ent",
    Package: "your/module/ent",
}, entc.Extensions(ext)); err != nil {
    log.Fatal(err)
}
```

一共四个选项：`WithEntAPIPackage`（生成文件所 import 的 runtime 路径，默认值已经正确，
除非你 vendor 了一份）、`WithStrictQueryOperators()`（无法识别的操作符前缀变成校验失败），
以及 `WithOpenAPITitle` / `WithOpenAPIVersion`。

→ [接入](docs/GUIDE_zh.md#接入)

## 三个 import 路径

一个 module，三个包，按**代码何时运行**切分。根包与运行时包都叫 `entapi`；schema 包叫
`api`。

| Import | 谁 import 它 | 主要符号 |
|---|---|---|
| `github.com/githonllc/entapi` | 你的 `entc.go`；嵌入软删除的 schema | `Extension`、`SoftDeleteMixin` |
| `github.com/githonllc/entapi/api` | 你的 **schema** 文件 | `Resource`、`Hidden`、`ReadOnly`、`Searchable`、`Filterable`、`Sortable`、`Expand` |
| `github.com/githonllc/entapi/runtime` | **生成的代码**与你的 handler / service 代码 | `ListRequest`、`SortSpec`、`Page[R]`、`ListPage`、`GetOne`、`SaveOne`、`BindJSON`、`Status`、`WriteJSON`、`WriteProblem`、`FieldError`、`Endpoint`/`Op`、`WithActor`/`ActorFrom`、错误哨兵与 mapper、filter/指针/软删除辅助函数 |

这个切分是承重的。根包内嵌十个模板，并在**包初始化时**把它们全部从内嵌文件系统读出来，
连带拖进 `embed`、ent 的 codegen 包和 `golang.org/x/tools/imports`。`runtime/` 只 import
标准库，这就是它进得了你的生产二进制、而根包不必进的原因。

→ [三个 import 路径](docs/GUIDE_zh.md#三个-import-路径)

## 注解模型

`api.Resource()` 是唯一的实体开关。没有它就不生成 EntAPI 文件。
`api.Resource().Except(api.OpCreate, ...)` 去掉选定的公开操作面；请求 DTO 与接线函数仍留给
service 层使用，唯一例外是根本无法工作的 create 一族。

字段成员关系默认沉默，并由 Ent 推导：

| Ent/API 事实 | 生成效果 |
|---|---|
| `Optional`、`Default`、`Nillable` | create 的指针形态与必填性 |
| `Immutable` | 不出现在 PATCH |
| `Sensitive` | 不出现在响应与摘要，仍可写 |
| `api.Hidden()` | 不出现在 create、patch、响应与查询 |
| `api.ReadOnly()` | 不出现在 create 与 patch；响应保留 |
| `api.Searchable()` | 全文与子串查询维度 |
| `api.Filterable()` | 从 Ent 运算符推导的结构化谓词 |
| `api.Sortable()` | 进入排序白名单 |

边同样默认沉默，并且由它自己的注解选中——绝不从外键位置推断：

```go
edge.From("author", User.Type).Ref("posts").Unique().Field("author_id").
    Annotations(api.Expand().JSONKey("writer"))
```

`Expand()` 把 `Author *UserSummary` 放进 `PostResponse`，把 `WithAuthor()` 放进生成的预加载
计划；`JSONKey("writer")` 改写响应 key。展开只有一层深。

五个字段词共用一个可合并注解，所以分开书写是规范且安全的：
`Annotations(api.Searchable(), api.Sortable())` 会让两个词都穿过 Ent 的序列化 schema 加载器。

→ [注解模型](docs/GUIDE_zh.md#注解模型) ·
[边](docs/GUIDE_zh.md#边) ·
[从 scope 模型迁移](docs/GUIDE_zh.md#从-scope-模型迁移)

## 生成了什么

带 **`api.Resource()`** 的实体，每个生成四个文件。没有这个开关的实体被整体跳过。

| 文件 | 声明 |
|---|---|
| `{entity}_dto.go` | `{E}CreateRequest`、`{E}PatchRequest`，各自的 `Valid…` 对应物与 `Apply`；`{E}Response`、`{E}Summary` 与它们的构造函数；`{E}QueryWithResponseEdges`；`{E}ListResponse` |
| `{entity}_filter.go` | `{E}Filter`、`Parse{E}Query`、`Predicates()`、`{E}SortKeys`、`{E}Order` |
| `{entity}_wiring.go` | `Get{E}`、`List{Es}`、`Create{E}`、`Patch{E}`、`Delete{E}`、`DeleteBatch{Es}` |
| `{entity}_handler.go` | 可达操作的 `{Op}{E}Fn` 类型与 bind → call → write 三步 handler |

外加每个 schema 五个文件，各有自己的生成条件：

| 文件 | 何时生成 | 声明 |
|---|---|---|
| `entapi_errors.go` | 至少一个实体产生了 wiring | `ErrorMap` |
| `entapi_http.go` | 至少一个实体带 `api.Resource()` | `APIHandler`、`API(client)`、`With`、`Endpoints`、每个可达操作一个 `…Endpoint()` accessor、`ServeHTTP`、`Mount` |
| `openapi.yaml` | 至少一个实体产生了 wiring | 描述全部生成端点的 OpenAPI 3.1 文档 |
| `entapi_openapi.go` | 至少一个实体产生了 wiring | 该文档的 `//go:embed` 与提供它的 handler |
| `entapi_softdelete.go` | 至少一个实体嵌入 `SoftDeleteMixin` | 未导出的查询 traverser 与删除 hook |

产物落在**你的** `ent` 包（`gen.Config.Target`），所以读起来是 `ent.CreateArticle`、
`ent.ArticleFilter`、`ent.ErrorMap`。这也是实体名可能与它们撞车的原因。

→ [生成了什么](docs/GUIDE_zh.md#生成了什么)

## 生成的 HTTP

`ent.API(client)` 返回 `*ent.APIHandler`，它同时也是一个 `http.Handler`：

```go
api := ent.API(client)
api.Mount(mux)
mux.Handle("/v1/", http.StripPrefix("/v1", api))
```

每个未被 Except 的 Resource 恰好得到这些 Go 1.22 pattern：

| Pattern | 结果 |
|---|---|
| `GET /articles` | 裸的 `{"data","total","page","size"}` 分页，200 |
| `POST /articles` | 裸资源，201；无 `Location` 头 |
| `GET /articles/{id}` | 裸资源，200 |
| `PATCH /articles/{id}` | 裸资源，200 |
| `DELETE /articles/{id}` | 空 body，204 |
| `GET /openapi.yaml` | 生成的文档，200，`application/yaml` |

错误是 RFC 9457 的 `application/problem+json`。bind 失败为 400，生成 `Validate` 失败为
422，媒体类型不支持 415，body 过大 413，中间步骤的哨兵映射为 404/409，未分类的错误为
500。POST 与 PATCH 只接受 `application/json`，body 上限 **1 MiB，且没有配置旋钮**。

**组合是一架梯子**，你站在哪一级，取决于你为这个面命名到多细：

| 级 | 交给你 | 什么时候用 |
|---|---|---|
| `Get{E}`、`List{Es}`、… | 操作本身，不带任何 HTTP | handler 由你自己写 |
| `Get{E}Endpoint()`、`OpenAPIEndpoint()` | 按名字拿到单个 `entapi.Endpoint` | 你在自己的 router 上注册少数几个端点 |
| `Endpoints()` | 全部端点，按注册顺序 | 策略是按批的——「包住所有写操作」 |
| `Mount(mux)`、`ServeHTTP` | 整棵树 | 生成的路径就是你要的路径 |

上面三级 HTTP 是同一个面而不是三个：manifest 正是调用那些 accessor 构建出来的。
`entapi.ColonPath` 与 `Endpoint.Bind` 各用一行就能把一行端点接到 Gin、Echo、chi 或
fiber；`entapi.BindJSON`、`Status` 与 `WriteJSON` 让手写 handler 拿到生成 handler 用的
同样三步。

要在不动路由的前提下替换某个操作的行为，把生成的函数类型传给 `With`：

```go
api := ent.API(client).With(ent.PatchArticleFn(service.Patch))
```

`With` 只接受这些生成的类型，所以被 `Except` 移除的操作根本无法被命名——写错的替换是
编译错误。请在开始服务之前完成接线。

→ [自定义操作实现](docs/GUIDE_zh.md#用-with-提供自定义操作实现) ·
[生成的 OpenAPI 文档](docs/GUIDE_zh.md#生成的-openapi-文档) ·
[用 ogen 生成客户端](docs/GUIDE_zh.md#用-ogen-生成客户端) ·
[注册导出的端点](docs/GUIDE_zh.md#注册导出的端点) ·
[自己写 handler](docs/GUIDE_zh.md#自己写-handler)

## 查询面

```go
filter, req, err := ent.ParseArticleQuery(r.URL.Query())
```

线格式是 `field=op:value`，在第一个冒号处切分；裸值即等值。字段名一律使用 Ent 的
storage key。

```text
?title=ilike:go&score=gt:30&score=le:50&status=in:draft,published&_sort=created_at:desc&_page=2
```

| 写法 | 谓词 |
|---|---|
| 裸值、`eq:` | 等值 |
| `ne:` | 不等 |
| `gt:` `ge:` `lt:` `le:` | 比较 |
| `in:` `not_in:` | 逗号分隔的成员判定 |
| `like:` `ilike:` `prefix:` `suffix:` | 字符串匹配——额外要求 `api.Searchable()` |
| `is_null:` `not_null:` | 空值谓词 |
| `from:` `to:` `between:a,b` | 闭区间语法糖 |

每个字段只拿到 Ent 谓词与这份线格式词汇表的交集。同一字段的重复参数是多个以 `AND` 连接
的谓词。保留参数恰好四个——`_q`（在 searchable 字段上做 `OR`）、`_sort`、`_page`、
`_size`——别名与重复都会被拒绝。`{E}Order` 是排序的唯一白名单；主键天然 Filterable 与
Sortable，并作为最后的确定性 tiebreak 追加。每一次解析失败都是 `entapi.ErrValidation`，
并点出字段与值。

→ [查询面](docs/GUIDE_zh.md#查询面)

## 请求与响应

一个 PATCH body 必须区分普通 struct 无法区分的三件事：

| 载荷 | 含义 | `HasNickname()` | `Nickname` |
|---|---|---|---|
| `{}` | 别动它 | `false` | `nil` |
| `{"nickname": null}` | 清空它 | `true` | `nil` |
| `{"nickname": "sam"}` | 设置它 | `true` | `&"sam"` |

字段保持 `*T`；存在性住在旁边的 `present map[string]bool` 里，由生成的 `UnmarshalJSON`
从原始 key 集合填充。`Valid…PatchRequest` 上每个字段还有一个以字段自己的名字拼写的
comma-ok **值读取器**，因为只有存在性无法区分*携带了一个值*与*携带了显式 null*。
**create** 请求无法表达「清空」，所以那里的显式 `null` 记为不存在。

验证不是可选的——`Validate()` 返回**另一个类型**，而 `Apply` 只存在于那个类型上：

```go
valid, err := req.Validate()          // *ValidArticleCreateRequest
if err != nil { return err }          // 包装 entapi.ErrValidation
art, err := ent.CreateArticle(ctx, client, valid)
```

出站方向上，`New{E}Response` 返回 error，而 `New{E}Summary` 不会。差别在边：边的状态
通过 ent 的 `<Edge>OrErr()` 读取，绝不用 nil 判断，所以「已加载但不存在」是显式的
`null`，而「未加载」是一个点名该边的错误。**摘要不携带边**，这正是把展开限制在一层、
不需要运行时深度计数器、也不需要 visited 集合的原因。

接线函数是自由函数——没有接口，没有可嵌入的东西：

```go
func ListArticles(ctx context.Context, db *Client, f *ArticleFilter, r entapi.ListRequest) (*entapi.Page[ArticleResponse], error)
func PatchArticle(ctx context.Context, db *Client, id uuid.UUID, v *ValidArticlePatchRequest) (*ArticleResponse, error)
```

每一个都接收 `*Client`，这就是事务契约：**本包从不生成事务边界。** 要把生成的某一步拉进
你自己的事务，把 `tx.Client()` 递给它。每个导出的接线函数恰好经过 `ErrorMap.MapError`
一次，所以在你的 handler 边界上 `errors.Is(err, entapi.ErrNotFound)` 直接可用，不必解开
ent 的错误类型。

→ [三态存在性](docs/GUIDE_zh.md#请求三态存在性) ·
[响应、摘要与边](docs/GUIDE_zh.md#响应摘要与边) ·
[接线与错误映射](docs/GUIDE_zh.md#接线与错误映射) ·
[字段形态](docs/GUIDE_zh.md#字段形态)

## 软删除

基于注解，并且强制在 ent 那一层而不是生成的接线里：

```go
func (Doc) Mixin() []ent.Mixin { return []ent.Mixin{entapi.SoftDeleteMixin{}} }
```

生成的 `newConfig` 为每个可软删除实体装一个 interceptor 与一个 hook，所以没有注册调用，
也没有构造顺序依赖。被删除的行从**每一次**读取中消失——包括完全不经过本包生成物的
`client.Doc.Query()`，也包括预加载——而 `Delete` 变成对墓碑列的更新。两个互相独立的
context 开关可以按调用退出：

```go
entapi.WithSoftDeleted(ctx)   // 读取包含已删除行
entapi.WithHardDelete(ctx)    // 这次删除是真删除
```

软删除**不级联**。一个边指向已软删除目标的行仍保留外键、仍出现在每一次列表里，所以一条
声明为 `Required()` 且用 `api.Expand()` 展开的边，会在目标被软删除后返回 JSON `null`。

→ [软删除](docs/GUIDE_zh.md#软删除)

## 生成会失败，而这正是设计

检查跑在 `next.Generate(g)` **之前**，所以被拒绝的 schema 不会在磁盘上留下任何东西——
连 ent 自己的产物都没有。整张图会被一次性检查，所有问题一次性报出。

> 与 ent schema 相矛盾的注解会让生成失败，并同时报出双方事实与修法。任何能被正确生成的
> 东西都会被生成，而不是被拒绝。

拒绝矩阵是一张矛盾表，其中包括：`api.Hidden()` 与任何其他字段词同用；Ent `Sensitive()`
与查询词同用；required-no-default 字段被挡出 create；空的 PATCH 面；没有 `edge.Field(…)`
的 `Required()` 边；`OpList` 被 except 时仍带查询词；`api.Expand()` 指向非 resource；
实体名与本扩展生成的符号撞车；以及 patch 可见字段名与 patch DTO 在同一接收者上生成的方法
撞车。

一轮生成**整体是原子的**：第一阶段在内存里渲染并格式化所有文件，第二阶段才写盘，所以
模板 bug 会让上一轮的产物原封不动。之后清理会删掉首行带 `Code generated by entapi
extension`——或改名前的 `Code generated by entdomain extension`，这条识别是永久的而非
过渡窗口——且不是本轮写入的顶层文件。**那行 marker 就是你的逃生口**：删掉它，文件就是
你的。

→ [拒绝矩阵](docs/GUIDE_zh.md#生成会失败而这正是设计) ·
[生成器对你的目录做了什么](docs/GUIDE_zh.md#生成器对你的目录做了什么)

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
    个」的唯一途径。
10. **`Page.Size` 是钳制后的 size。** parser 拒绝零和负数 `_size`，接受大于 1000 的值，
    随后由 `ListRequest.Limit()` 钳制。
11. **`ErrorMap` 是普通包级变量，不带同步。** 请在构造 client 处赋值，不要在服务运行中改。
12. **一个你复制并修改过的生成文件仍带着 marker**——清理会删掉它。请剥掉首行。
13. **`ent/openapi.yaml` 是一个完全普通的文件名。** 升级后的第一次生成会静默覆盖已经放在
    那个路径上的手写文档。先把它挪走。

## 限制

- **只有 offset 分页**，且每页一次 `COUNT`。它是正确的（主键 tiebreak 保证全序），但深度
  仍是 O(n)，并且在*并发写入下*仍可能跳过或重复行。包内没有 keyset 替代方案，也没有游标
  类型。
- **摘要永远是一层深**，并且携带每个响应可见字段减去边。想收窄它需要一个新注解；schema
  里没有任何东西说明哪个字段是「简版的那个」。
- **没有乐观锁，丢失更新是已知边界。** 生成的 patch wiring 不带版本谓词，所以两个并发
  `PATCH` 打同一个字段时，后到者静默胜出。出路是定制点：用 `With(ent.PatchXFn(...))`
  整单元替换那一步，在里面写自己的 `Where(x.Version(v))`。
- **产物与 ent 的输出同处一个包。** 没有独立的 `dto` 子包，也没有换目录的配置项；所有权
  靠 marker 逐文件判定。
- **注解只控制 HTTP 层的生成。** 它们绝不限制你的 service 层能拿一个 ent 实体做什么。任何
  需要强制的东西，必须在构造查询的地方强制。
- **生成的 OpenAPI 文档在落盘前没有语法门禁**，因为标准库没有 YAML 解析器。模板 bug 会先
  落盘，之后才被 fixture 断言与 `internal/fixtures/httpdemo/e2e` 里的 3.1 校验器抓到。
- **过滤参数在文档里是单值的。** 重复同一个参数以 `AND` 谓词的做法服务端真的支持，但文档
  表达不了，所以生成的客户端要用原始 query 才能做到（#135）。
- **生成器包在包初始化时加载全部十个模板。** 这被限制在 `entc.go` 与 schema 文件里；
  `runtime/` 就是把它挡在你的二进制之外的那个东西。

→ [限制](docs/GUIDE_zh.md#限制)，每一条都带上它的理由

## 迁移注记

**破坏性变更（#118）：** `entapi.Route` 改名为 `entapi.Endpoint`，生成的
`APIHandler.Routes()` 改名为 `APIHandler.Endpoints()`。其余一切不变——字段
（`Method`、`Path`、`Handler`、`Entity`、`Op`）、注册顺序、每次调用返回新副本的保证，
以及 `Bind` 的行为都和以前一致，`Op`、它的常量与 `ColonPath` 也都保留原名。旧名字宣称
entapi 在做路由，它并没有：一个 `Endpoint` 是某个生成 handler 的契约记录——你把它组合进
你自己的 router 的数据——而 `ServeHTTP`/`Mount` 背后那个内部 `ServeMux`，只是用同一份
manifest 搭出来的可选便利层。重新生成后重命名调用点：`api.Routes()` → `api.Endpoints()`，
`[]entapi.Route` → `[]entapi.Endpoint`。

**新增，带一个升级隐患（#119）：** 生成的 `APIHandler` 现在为每个可达操作携带一个导出的
`…Endpoint()` 方法（wiring 函数名加 `Endpoint` 后缀：`GetArticle` → `GetArticleEndpoint`），
外加 `OpenAPIEndpoint()`。这些名字进入你 `ent` 包的方法集。如果你此前在 `*APIHandler` 上
手写过同名辅助方法——#119 之前"按身份取单个端点"的标准变通正是这样一个索引——重新生成后
它会变成方法重复定义的编译错误；删掉你那份，改调生成的 accessor。

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

旧的 scope 模型注解词汇（`DefaultField()`、`ScopeCreate`、`AsSearchable`、
`Edge().InResponse()`……）到当前词汇的对照表在
[从 scope 模型迁移](docs/GUIDE_zh.md#从-scope-模型迁移)。

## 还可以读什么

| | |
|---|---|
| [`docs/GUIDE_zh.md`](docs/GUIDE_zh.md) | **完整参考**——上面每条规则的长版，带理由与指向源码的指针 |
| [`examples/todo/`](examples/todo/) | 单实体、五个端点，附真实运行的 `curl` 记录 |
| [`examples/petstore/`](examples/petstore/) | 四个实体：边、软删除、一个被 Except 的操作，以及逐文件的架构地图 |
| [`docs/adr/`](docs/adr/) | 那些承重决定为什么是现在这样——严格 key 匹配、主键 tiebreak、整轮原子性、marker 所有权、操作符分类 |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | 模块地图，与指南用同一套源码锚点纪律 |
| [`docs/QUALITY-REVIEW.md`](docs/QUALITY-REVIEW.md) | 已知缺陷 |
| [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) | 如何构建、测试与新增 fixture |
| `internal/fixtures/` | 每一条规则的可编译证据：每个目录一份手写 schema 加一份提交进仓库的生成产物 |
| [`README.md`](README.md) | English documentation |

每一次 push 与 pull request 都会在 GitHub Actions 上跑 `make check`
（[`.github/workflows/check.yml`](.github/workflows/check.yml)）——格式化、`go vet`、本
module 的测试与五个嵌套 module——然后对它留下的工作树断言两件事：`gofmt -l .` 为空，
`git status --porcelain` 为空。第二条是生成产物的漂移门禁：干净检出加一次测试运行，必须
让提交进仓库的 fixture 逐字节不变。

## 许可证

MIT——见 [LICENSE](LICENSE)。
