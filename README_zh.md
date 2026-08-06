# EntDomain

[![Go Reference](https://pkg.go.dev/badge/github.com/githonllc/entdomain.svg)](https://pkg.go.dev/github.com/githonllc/entdomain)
[![Go Report Card](https://goreportcard.com/badge/github.com/githonllc/entdomain)](https://goreportcard.com/report/github.com/githonllc/entdomain)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

一个 [Ent](https://entgo.io) 扩展。你在 ent schema 的字段上标注「HTTP 层可以拿它做什么」，
它就为你生成请求类型、响应类型、查询面，以及每个操作一个接线函数——全部写进你自己的
`ent/` 包，链接的运行时除标准库外不依赖任何东西。

*[English](README.md)*

```go
// schema/article.go —— 你写这个
field.String("title").
    Annotations(entdomain.DefaultField().AsSearchable().AsFilterable().AsSortable()),
```

```go
// handler.go —— 你得到这个
page, err := ent.ListArticles(ctx, client, filter, req)   // GET /articles?title_contains=go&sort_by=title
art,  err := ent.CreateArticle(ctx, client, validReq)     // POST /articles
```

这两行之间，生成器写出了带三态存在性的 `ArticleCreateRequest`、每个 ent 派生操作符
一个参数的 `ArticleFilter`、排序白名单、带预加载计划的 `ArticleResponse`，以及错误
分类——每个实体约 700 行，否则你得手写，并且每次改 schema 都得重写。

> ### 状态：原型，正在重新设计
>
> 它能工作，测试套件是绿的，但 API 的形态正在被重新考虑。方向记录在
> [`DESIGN-v2.md`](DESIGN-v2.md)——**其中没有任何一部分已实现**；下文描述的全部是今天
> 的现状。已知缺陷见 [`QUALITY-REVIEW.md`](QUALITY-REVIEW.md)。
>
> 采用前请先读[陷阱](#陷阱)。其中有几条是静默的。

---

## 目录

- [安装](#安装) · [两个 import 路径](#两个-import-路径) · [接入](#接入)
- [注解模型](#注解模型)——作用域与标记是两个不同的轴
- [生成了什么](#生成了什么)
- [请求：三态存在性](#请求三态存在性)
- [响应、摘要与边](#响应摘要与边)
- [查询面](#查询面)——过滤、全文、排序
- [接线与错误映射](#接线与错误映射)
- [软删除](#软删除)
- [生成会失败，而这正是设计](#生成会失败而这正是设计)
- [生成器对你的目录做了什么](#生成器对你的目录做了什么)
- [字段形态](#字段形态) · [被接受但不被消费的](#被接受但不被消费的)
- [陷阱](#陷阱) · [限制](#限制) · [还可以读什么](#还可以读什么)

---

## 安装

```bash
go get github.com/githonllc/entdomain
```

需要 Go 1.23+ 与 ent v0.14+。

## 两个 import 路径

一个 module，两个包，按**代码何时运行**切分：

| Import | 被谁 import | 拉进什么 |
|---|---|---|
| `github.com/githonllc/entdomain` | 你的 `entc.go` 和你的 **schema** 文件——注解构造器、`Edge()`、`SoftDeleteMixin`、扩展本体 | ent 的 codegen 包、源码格式化器、五个内嵌模板 |
| `github.com/githonllc/entdomain/runtime` | **生成的代码**与你的 service / handler 代码——`ListPage`、`GetOne`、`SaveOne`、`ListRequest`、错误哨兵、`ErrorMapper`、软删除上下文开关 | 标准库，仅此而已 |

两者的包名都是 `entdomain`，所以无论从哪条路径来，调用点都写作 `entdomain.X`；同时需要
两者的文件就都 import，并给其中一个起别名。

这个切分是承重的，不是整洁癖。模板加载发生在包初始化时——只要 import 根包，加载器就会
运行，无论你是否真的生成任何东西。把它挡在你的生产二进制之外，就是 `runtime/` 存在的
全部理由；有一个测试（`TestRuntimePackageIsGeneratorFree`）会在任何 ent 形态、`embed`
形态或格式化器形态的东西渗进去时弄红构建。如果你新增的代码会被生成产物在运行时调用，
它属于 `runtime/`，且只能 import 标准库。

## 接入

```go
//go:build ignore

package main

import (
    "log"

    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
    "github.com/githonllc/entdomain"
)

func main() {
    ext := entdomain.NewExtensionWithOptions(
        // RUNTIME 路径——这是生成文件真正 import 的东西。
        // 它同时也是默认值，所以这一行只在你 vendor 了一份副本时才有意义。
        entdomain.WithEntDomainPackage("github.com/githonllc/entdomain/runtime"),
    )

    if err := entc.Generate("./schema", &gen.Config{
        Target:  "../ent",
        Package: "your/module/ent",
    }, entc.Extensions(ext)); err != nil {
        log.Fatal(err)
    }
}
```

`WithEntDomainPackage` 是仅有的一个选项。`NewExtension(cfg)` 直接接受
`*ExtensionConfig`，且对 nil 安全。然后 `go generate ./...`。

## 注解模型

两个轴，把它们搞混是最常见的错误：

**作用域（scope）** 回答*哪些 HTTP 结构体可以携带这个字段*。共四个：`ScopeCreate`、
`ScopeUpdate`、`ScopeQuery`、`ScopeResponse`。

**标记（marker）** 回答*对一个已经有 `ScopeQuery` 的字段，查询 API 可以做什么*。共三个：
`AsFilterable()`、`AsSearchable()`、`AsSortable()`。

```go
entdomain.DefaultField()                    // create + update + query + response
entdomain.InputOnlyField()                  // create + update           （密码）
entdomain.OutputOnlyField()                 // query + response          （时间戳、计算态）
entdomain.CreateOnlyField()                 // create + query + response （创建后不可变）
entdomain.IdField()                         // OutputOnly，预置描述
entdomain.AuditLogField()                   // OutputOnly，只读
entdomain.NewDomainField()                  // 无作用域——ent 追踪它，但它不出现在任何 HTTP 结构体里
entdomain.DomainFieldWithScopes(scopes...)  // 其他任意组合
```

**没有任何预设会授予标记。** 在你链上一个之前，`DefaultField()` 给你的是一个空的
`{Entity}Filter` 和一个空的排序白名单：

```go
field.String("title").
    Annotations(entdomain.DefaultField().
        AsFilterable().     // 结构化 URL 参数：title、title_neq、title_in、title_prefix……
        AsSearchable().     // 加入全文 q 析取，并解锁子串类操作符
        AsSortable()),      // 进入 {Entity}SortKeys
```

这是刻意的（#27）。这些标记生成的是真实的查询参数和真实的 `ORDER BY` 白名单；一个宽松的
默认值会让几乎每个响应可见的列都能在你从未为此建过索引的表上被排序和子串搜索。一个
**没有** `ScopeQuery` 的标记是生成错误，不是警告——见[生成会失败](#生成会失败而这正是设计)。

每个构造器都是**值接收者且返回副本**。链式调用有效；原地修改无效。

### 边

边由它自己的注解选中，绝不从外键位置推断：

```go
func (Post) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("author", User.Type).Ref("posts").Unique().Field("author_id").
            Annotations(entdomain.Edge().InResponse().As("writer")),
    }
}
```

`InResponse()` 把 `Author *UserSummary` 放进 `PostResponse`，把 `WithAuthor()` 放进生成的
预加载计划；`As("writer")` 覆盖 JSON key。从外键位置推断的做法试过并被否决：它让一对多的边
永久不可达（当列在对方实体上时 `edge.Field()` 为 nil），而且把「暴露 `author_id`」与
「暴露嵌套的 `author`」这两个不同的决定焊死在一起。

## 生成了什么

对每个至少携带一个带注解字段的实体——一个都没有的实体会被整体跳过，不产生任何文件：

| 文件 | 声明 |
|---|---|
| `{entity}_dto.go` | `{E}CreateRequest`、`{E}PatchRequest` 及其 `Valid…` 对应类型与 `Apply`；`{E}Response`、`{E}Summary` 及其构造器；`{E}QueryWithResponseEdges`；`{E}ListResponse` 与 `New{E}ListResponse` |
| `{entity}_filter.go` | `{E}Filter` 及其 `Predicates()`、`{E}SortKeys`、`{E}Order` |
| `{entity}_wiring.go` | `Get{E}`、`List{Es}`、`Create{E}`、`Update{E}`、`Delete{E}`、`DeleteBatch{Es}` |

外加每个 schema 两个文件，仅在确有内容可写时生成：

| 文件 | 生成条件 | 声明 |
|---|---|---|
| `entdomain_errors.go` | 存在任一带注解实体 | `ErrorMap`——每个生成操作返回时都经过的分类器 |
| `entdomain_softdelete.go` | 存在任一实体嵌入 `SoftDeleteMixin` | `RegisterSoftDelete`，以及查询 traverser 与删除 hook |

它们落在**你的** `ent/` 包里，所以读起来是 `ent.CreateArticle`、`ent.ArticleFilter`、
`ent.ErrorMap`。这也正是实体名会与它们相撞的原因——见[保留名](#生成会失败而这正是设计)。

## 请求：三态存在性

一个 PATCH body 必须区分三件事，而普通结构体做不到：

| Payload | 含义 | `HasNickname()` | `Nickname` |
|---|---|---|---|
| `{}` | 别动它 | `false` | `nil` |
| `{"nickname": null}` | 清空它 | `true` | `nil` |
| `{"nickname": "sam"}` | 设置它 | `true` | `&"sam"` |

`Apply` 读的正是这个：

```go
if r.HasNickname() {
    if r.Nickname == nil { b.ClearNickname() } else { b.SetNickname(*r.Nickname) }
}
```

**创建**请求无法表达「清空」，所以那里的显式 `null` 被记为*缺席*——字段不会被写入，你
schema 的 `Default()` 因而生效。这也正是「必填创建字段上的显式 null」成为一个「必填」
错误而不是空指针解引用的原因。

必填性**按存在性检查，而不是按零值**——`0` 和 `false` 是值。（字符串是例外：`== ""`
说的是同一件事。）

### key 是严格匹配的

`encoding/json` 在精确匹配**或**大小写不敏感匹配时都会填充结构体字段，而存在性是按原始
payload key 记录的。这两者对每一个大小写变体都不一致，且失败是静默的：`PATCH
{"Nickname":"sam"}` 填进了字段，`HasNickname()` 仍为 `false`，`Apply` 什么也没写，更新
报告成功却没有改动任何一行。

所以 `UnmarshalJSON` 现在会**拒绝**任何折叠后等于某个已知 tag、但不精确相等的 key：

```
unknown key "Nickname" (did you mean "nickname"?)   // 包装 entdomain.ErrValidation
```

该检查在原始解码之后、结构体解码之前运行，所以被拒绝的请求不会触碰你的接收者。折叠后
**不匹配任何** tag 的 key 仍被忽略——拒绝那些是 `DisallowUnknownFields`，仍归你的 handler
决定。理由见 [ADR-0001](docs/adr/0001-presence-follows-encoding-json-key-matching.md)。

### 验证不是可选的

`Validate()` 返回一个*不同的类型*，而 `Apply` 只存在于那个类型上：

```go
valid, err := req.Validate()          // *ValidArticleCreateRequest
if err != nil { return err }          // 包装 entdomain.ErrValidation
art, err := ent.CreateArticle(ctx, client, valid)
```

不存在任何可以应用未验证请求的导出函数。跳过验证是编译错误，由构造保证。

## 响应、摘要与边

`New{E}Response` 返回 error，`New{E}Summary` 不会。差别在边：

- 边的状态通过 ent 的 `<Edge>OrErr()` 读取，绝不用 nil 判断——`loadedTypes` 是未导出的，
  所以一个 nil 指针分辨不出*确实没有*和*没有加载*。
- **已加载但不存在是显式的 `null`**（没有任何边字段带 `omitempty`）。**未加载是一个错误**，
  并指名是哪条边——因为把「我忘了预加载」静默地输出成 `null`，是一个会一路抵达你的 API
  消费者的 bug。
- **摘要不携带边。** 这就是扩展有界的原因：不存在第二层让环闭合，因此不需要运行时深度
  计数器，也不需要 visited 集合。一棵三层的树回来时只有一层深。

`{E}QueryWithResponseEdges(q)` 施加的正是 `New{E}Response` 需要的那份预加载计划。要么用
它，要么处理那个错误。

### 具名列表类型

`List{Es}` 返回 `*entdomain.Page[{E}Response]`。`{E}ListResponse` 是同一形状的非泛型
具名版本，因为 OpenAPI / swaggo 一类的工具无法表达泛型实例化：

```go
page, err := ent.ListArticles(ctx, client, filter, req)
if err != nil { return err }

// @Success 200 {object} ent.ArticleListResponse
return c.JSON(200, ent.NewArticleListResponse(page))
```

转换器的函数体是一次 Go 类型转换，这正是要点：一旦 `{E}ListResponse` 与
`entdomain.Page` 在字段集、类型或顺序上出现分歧，那一行会让**每一个**生成包编译失败。
（类型转换忽略 struct tag，所以 tag 那一半由一个 golden JSON 测试单独守住。）

## 查询面

三个互相独立的维度，每个都按字段 opt-in。

### 结构化过滤——`AsFilterable()`

**ent** 为该类型派生的每个操作符对应一个参数；本包从不维护自己的操作符表。线上名字是
存储 key 加后缀，`form:` 与 `json:` 共用：

| | |
|---|---|
| `_neq` `_in` `_not_in` `_gt` `_gte` `_lt` `_lte` | 比较类，来自 ent |
| `_prefix` | 左锚定 `LIKE`——走索引 |
| `_contains` `_icontains` `_suffix` `_ieq` | **子串类，见下** |
| `_is_null` | 一个 `*bool`，把 `IsNil`/`NotNil` 合并 |

一个只标了 `AsFilterable()` 的 `string` 字段得到十个参数：

```
ref  ref_neq  ref_in  ref_not_in  ref_gt  ref_gte  ref_lt  ref_lte  ref_prefix  ref_is_null
```

### 子串类还需要 `AsSearchable()`

`_contains`、`_icontains`、`_suffix`、`_ieq` 正是那些让 B-tree 索引失效的 `LIKE '%x%'`
形态——与排序、搜索两道门要挡住的成本画像完全相同。它们只在字段**同时**携带
`AsSearchable()` 时才发射。

`_ieq` 在*语义*上是精确匹配，但因其*成本*被划入昂贵类：没有函数索引时
`LOWER(x) = LOWER(?)` 会全扫，和子串匹配一模一样。理由见
[ADR-0005](docs/adr/0005-contains-operators-gated-by-searchable.md)。

> **升级注意：** 一个此前只标了 `AsFilterable()` 的 `string` 字段会静默失去它的四个子串
> 参数。form 与 JSON 绑定会丢弃未知 key 而不报错，所以一个原本工作的
> `?name_contains=x` 会变成一个*未过滤*的查询，而不是一个 400。补上
> `AsSearchable()` 可恢复它们——同时该字段也会进入全文 `q` 析取，这是被接受的耦合代价。

### 全文——`AsSearchable()`

仅当至少有一个字段可搜索时才发射：单个 `q` 参数，作为一个跨全部可搜索字段的 `OR` 析取
施加，并与其他一切 `AND`。为 nil **或空串**时跳过。一个标了 `AsSearchable()` 但没标
`AsFilterable()` 的字段只贡献给 `q`，不会得到属于自己的结构化参数。

### 排序——`AsSortable()`

`{E}SortKeys` 是白名单。白名单之外的 `sort_by` 是 `entdomain.ErrValidation`，绝不静默
回退。**没有默认排序列**——你的 schema 里没有任何东西说明哪一列是天然的那一列，所以生成器
不猜。

确定性是另一个问题，而它确实有一个由 schema 给出的答案。基于 offset 的分页在非全序上
按构造就是错的：在**零并发写入**的情况下，行也可能在第 1 页和第 2 页之间重复或消失。
所以每个生成的排序都以主键收尾：

```go
// 请求了排序：tiebreak 跟随请求的方向
[]OrderOption{by(dir), ByID(dir)}       // 当请求的 key 就是主键时跳过追加
// 什么都没请求：确定，且不声称自己是「默认排序」
[]OrderOption{ByID(sql.OrderAsc())}
```

理由见 [ADR-0002](docs/adr/0002-deterministic-pagination-pk-tiebreak.md)。

### 分页

`entdomain.ListRequest{Size, Page, SortBy, Order}`——零值可直接用。`Limit()` 钳制到
`[1, MaxPageSize]`（1000），默认 20；`Offset()` 会饱和而不是溢出。超界的 size 是
**钳制，绝不拒绝**：`Validate()` 对 `Size` 和 `Page` 一个字都不说。如果你想要 4xx，
自己和 `entdomain.MaxPageSize` 比较。

分页只有 offset 一种。游标编解码器已被移除（#6）；`Page` 携带 `Data`、`Total`、`Page`、
`Size`，没有别的。

## 接线与错误映射

自由函数，没有接口，没有需要嵌入的东西。若你需要不同的行为，写你自己的函数并停止调用
生成的那个。

```go
func GetArticle(ctx context.Context, db *Client, id uuid.UUID) (*ArticleResponse, error)
func ListArticles(ctx context.Context, db *Client, f *ArticleFilter, r entdomain.ListRequest) (*entdomain.Page[ArticleResponse], error)
func CreateArticle(ctx context.Context, db *Client, v *ValidArticleCreateRequest) (*ArticleResponse, error)
func UpdateArticle(ctx context.Context, db *Client, id uuid.UUID, v *ValidArticlePatchRequest) (*ArticleResponse, error)
func DeleteArticle(ctx context.Context, db *Client, id uuid.UUID) error
func DeleteBatchArticles(ctx context.Context, db *Client, ids []uuid.UUID) (int, error)
```

任何地方都没有硬编码的标识符类型——id 来自你的 schema，并作为类型参数抵达运行时，所以
一个 `int` 主键根本不需要任何 import。

每个导出的接线函数都**恰好一次**地经由 `ErrorMap.MapError` 返回，因此
`errors.Is(err, entdomain.ErrNotFound)` 在你的 handler 边界上直接可用，无需拆开 ent 的
错误类型。

**`ErrorMap` 开箱状态下不会报告唯一性冲突。** ent 的 `IsConstraintError` 分辨不出
`UNIQUE` 与 `FOREIGN KEY`，所以这是按驱动 opt-in 的：

```go
func init() {
    ent.ErrorMap = ent.ErrorMap.WithUniqueViolation(func(err error) bool {
        var pgErr *pgconn.PgError
        return errors.As(err, &pgErr) && pgErr.Code == "23505"
    })
}
```

跳过它的代价是本该 409 的地方给你 500；它绝不会产生一个错误的 409。

## 软删除

基于注解，且在 ent 的层面强制执行，而不是在生成的接线里：

```go
func (Doc) Mixin() []ent.Mixin { return []ent.Mixin{entdomain.SoftDeleteMixin{}} }
```

```go
client := ent.NewClient(ent.Driver(drv))
ent.RegisterSoftDelete(client)          // 恰好一次，在构造时
```

这次注册安装了一个 interceptor 和一个 hook。此后被删除的行会从**每一次**读取中消失——
包括那些完全不碰本包生成物的 `client.Doc.Query()` 调用——而 `Delete` 变成对墓碑列的一次
更新。不存在第二次写入，生成的接线里也没有任何东西知道软删除的存在。

两个互相独立的上下文开关让你按调用退出：

```go
entdomain.WithSoftDeleted(ctx)   // 读取包含已删除的行
entdomain.WithHardDelete(ctx)    // 这次删除是真删
```

两者互不蕴含。

**一个没有调用 `RegisterSoftDelete` 就构造出来的 client 不过滤任何东西，并且是硬删除
——包括在你的测试里。**

## 生成会失败，而这正是设计

检查在 ent 写入任何东西**之前**运行，所以一个被拒绝的 schema 不会在磁盘上留下任何东西
——连 ent 自己的产物都不会。整张图会被检查，所有问题一次性全部报告。政策是：

> 与 ent schema 相矛盾的注解会让生成失败，并同时报告两个事实和修法。凡是能被正确生成的，
> 就生成，不拒绝。

| 被拒绝的 | 原因 |
|---|---|
| 带 `ScopeUpdate` 的 `Immutable()` 字段——而 `DefaultField()` 恰好授予它 | ent 的 update builder 遍历 `MutableFields`，因此 `SetX` 不存在，没有任何模板能发射出编译得过的调用。静默丢弃该字段的方案被否决了：它会让字段从你的 PATCH API 里消失，而 `encoding/json` 和 `Validate()` 都观察不到 |
| 没有 `ScopeQuery` 的标记 | 该字段会被标记为可过滤，却从查询 API 无从抵达 |
| 在没有 `Contains` 的类型上标 `AsSearchable()` | 没有子串谓词可发射 |
| 在没有任何操作符的类型上标 `AsFilterable()` | 过滤器组会是空的 |
| 在不可比较的类型上标 `AsSortable()` | ent 的排序 builder 会跳过它，因而没有 `ByX` 可放进白名单 |
| `DomainSoftDelete` 指名了实体没有的字段，或墓碑字段不是 `Optional` | ent 不会生成 `DeletedAtIsNil` 谓词，traverser 编译不过 |
| 自引用边只在一端带注解 | ent 把链式的 `edge.To(…).From(…).Annotations(…)` 交给了*反向* builder，于是关联端静默丢失了它的注解 |
| **实体名与本扩展生成的符号相撞** | 见下 |

一个名为 `ErrorMap` 的实体会让 ent 发射 `type ErrorMap`，而 `entdomain_errors.go` 发射
`var ErrorMap`——`redeclared in this block`，发生在两个你从没写过的文件里，且没有任何
东西指出原因。`RegisterSoftDelete` 同理，跨实体相撞同理：一个字面叫 `ArticleResponse`
的实体会撞上实体 `Article` 生成的响应类型。现在这些都会被拒绝，消息会指名两个实体、
相撞的符号、它所在的文件，以及修法。

保留名列表刻意取**最大集合**——条件发射不用于收窄它。宁可拒绝一个理论上今天不会相撞的
名字，也不因模板日后扩展而漏检，这是被接受的代价。

反过来，`Optional().Nillable()` 以及切片/映射上的具名类型*是*会被生成的，因为对它们
存在正确的产物。见[字段形态](#字段形态)。

## 生成器对你的目录做了什么

**生成是整轮原子的。** 阶段一把每个文件渲染并格式化进内存；阶段二才写盘。任何确定性
失败——模板 bug、被拒绝的 schema、格式化不了的 import——都落在阶段一，让上一轮的产物
完整无损。此前，实体 B 处的失败可能留下已经被替换掉的实体 A 的文件，给你一棵混合了两代
产物的树，而 ent 自己的输出看上去一切正常。

诚实的残余：阶段二里两次 rename *之间*被硬杀，是一个仍然存在的毫秒级窗口。关掉它需要
目录交换，而那在跨平台上并不原子。（[ADR-0003](docs/adr/0003-per-run-atomic-generation.md)）

**清理会删除陈旧文件，并以一个 marker 判定所有权。** 一次成功的生成之后，生成器扫描
目标目录，删除任何满足以下两条的 `.go` 文件：

1. **首行**携带 `Code generated by entdomain extension`，且
2. 不是本轮写入的。

这就是为什么一次 schema 编辑不再弄红你的构建：删掉一个实体，它的
`_dto.go`/`_filter.go`/`_wiring.go` 会随之而去，而不是作为「引用 ent 已不再生成的
builder」的残骸留下来。对于跨过 #29 升级的人，它也会移除
`_base_service.go` / `_base_handler.go`。

扫描**仅限顶层**——ent 生成的子包位于目标目录之下，永远不是候选。不带 marker 的文件会被
留下并记录日志；ent 自己的 `Code generated by ent, DO NOT EDIT.` 刻意不匹配。

> **你的逃生舱就是那行 marker。** 想把一个生成的文件据为己有，删掉它的首行。反过来，
> 一个你从生成产物复制来、却忘了剥掉文件头的文件**会被删除**。
> （[ADR-0004](docs/adr/0004-cleanup-ownership-by-marker.md)）

## 字段形态

ent 的修饰符如何决定生成的请求形态。这是从 ent 推导的，绝不来自第二意见——ent 决定
哪些 setter 存在，所以任何独立推导出的形态都会表现为「调用一个从未被生成的方法」。

| ent schema | 创建字段 | 必填？ | patch 时 `null` 可清空？ |
|---|---|---|---|
| `field.String("a")` | `string` | 是 | 否 |
| `field.String("a").Default("x")` | `*string` | 否 | 否 |
| `field.String("a").Optional()` | `*string` | 否 | **是** |
| `field.String("a").Optional().Nillable()` | `*string` | 否 | 是 |
| `field.String("a").Immutable()` | `string` | 是 | *不出现在 PATCH 中* |
| `field.JSON("tags", []string{}).Optional()` | `*[]string` | 否 | 是 |

创建字段是指针，恰好当 ent 能在调用方不给的情况下自行填充它时
（`Optional || Default || Nillable`）。`WithRequired(ScopeCreate)` 只能*增加*严格性，
绝不能减少。

在响应一侧，`Optional` 的可比较字段走 `entdomain.PtrOrNil`，`Optional` 的切片与映射
——**包括**它们之上的具名类型——走 `entdomain.PtrNilSafe`，靠检查 reflect kind 而非渲染
出来的类型名来选择。

`Apply` 一律发射 `if r.X != nil { b.SetX(*r.X) }`，绝不用 `SetNillableX`：对一个本身就
可为 nil 的类型，ent 不生成 nillable setter，所以 `SetNillableTags` 对一个 optional 的
`field.JSON` 并不存在。一个统一的分支对每种形态都正确。

## 被接受但不被消费的

十五个 metadata knob 被存储下来，且不抵达任何模板。它们是为 OpenAPI spec 生成保留的
（#17），是刻意保留而非疏漏：

`WithTitle` · `WithDescription` · `WithExample` · `WithFormat` · `WithPattern` ·
`WithRange` · `WithLength` · `WithEnum` · `AsReadOnly` · `AsWriteOnly` ·
`AsDeprecated` · `WithTags` · `WithMetadata`

它们每一个的 godoc 首行都言明此事，并有一个测试双向强制「免责声明」与「待接线台账」
保持同步——把某个 knob 真正接线，就必须在同一个 commit 里删掉它的声明。

**今天被消费的：** 四个作用域、`Required`、三个查询标记，以及边注解的 `Scopes` 与
`JSONKey`。其余全部只是存储。

一条相关的、更高一层的契约：本仓库把死代码当作**测试失败**。一个没人调用的模板函数、
一个没人加载的模板、一个既未被消费也未声明待接线的 knob，都会弄红 CI。

## 陷阱

按「有多安静地伤害你」排序。

1. **没有调用 `ent.RegisterSoftDelete(client)` 的 client 不过滤任何东西，并且硬删除。**
   包括在测试里。
2. **在你调用 `WithUniqueViolation` 之前，`ErrorMap` 永远不会返回
   `ErrAlreadyExists`。** 一个重复键会表现为 500。
3. **`_contains` 现在需要 `AsSearchable()`。** 升级时，一个只标了 filterable 的字符串
   字段的子串参数会消失，查询会静默返回*未过滤*的结果而不是报错。
4. **没有任何预设授予查询标记。** 只写 `DefaultField()` 得到的是一个空过滤器结构体和一个
   空排序白名单。
5. **在 `Immutable()` 字段上用 `DefaultField()` 一定会让生成失败**——它授予了
   `ScopeUpdate`。改用 `CreateOnlyField()` 或 `OutputOnlyField()`。
6. **PATCH body 里出现的 `Immutable()` 字段会被 `encoding/json` 在任何验证器运行之前
   丢弃。** 拒绝它需要你的 handler 用 `DisallowUnknownFields`；生成器看不见它。
   （合法 key 的大小写*变体*会被拒绝——真正未知的 key 不会。）
7. **`entdomain.IsNotFound` 不是 ent 的 `IsNotFound`。** 生成模板以*不限定*的形式调用后者，
   使其绑定到你包内 ent 生成的谓词。加上限定符照样编译，然后静默地什么都匹配不上。
8. **每个 metadata 构造器都是 no-op。** `WithFormat("email")` 不验证任何东西。
9. **链式的自引用边会丢失关联端的注解**——ent 把它交给了反向 builder。请把两端分开声明。
10. **`DeleteBatch` 对匹配不到的 id 返回计数而非错误。** 那个 `int` 是你了解「实际存在
    多少个」的唯一途径。
11. **`Page.Size` 是钳制后的 size**，而一个超界的请求永远不是错误。
12. **一个你复制并修改过的生成文件仍带着 marker**——清理会删掉它。请剥掉首行。

## 限制

- **只有 offset 分页**，且每页一次 `COUNT`。它现在是正确的（全序有保证），但深度仍是
  O(n)，并且在*并发写入下*仍可能跳过或重复行。包内没有 keyset 替代方案。
- **摘要永远是一层深。** 没有深度选项。
- **摘要携带哪些标量字段无法从 schema 判定**，所以摘要携带每个响应作用域字段减去边。
  想收窄它需要一个新注解。
- **作用域只控制 HTTP 结构体的生成。** 它们绝不限制你的 service 层能拿一个 ent 实体做
  什么。任何需要强制的东西，必须在构造查询的地方强制。
- **生成器包在包初始化时加载全部五个模板。** 这被限制在 `entc.go` 与 schema 文件里；
  `runtime/` 就是把它挡在你的二进制之外的那个东西。

## 还可以读什么

| | |
|---|---|
| [`docs/adr/`](docs/adr/) | 那些承重决定为什么是现在这样——严格 key 匹配、主键 tiebreak、整轮原子性、marker 所有权、操作符分类 |
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | 模块地图与图 |
| [`DESIGN-v2.md`](DESIGN-v2.md) | 这个项目要去哪里，以及它自己第一稿里哪些论断是错的 |
| [`QUALITY-REVIEW.md`](QUALITY-REVIEW.md) | 已知缺陷 |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | 如何构建、测试与新增 fixture |
| [`README.md`](README.md) | English documentation |

## 许可证

MIT——见 [LICENSE](LICENSE)。
