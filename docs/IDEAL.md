# IDEAL.md — CRUD 密集系统里，这个框架应该省掉多少工作量

> **这是一份设计虚构（design fiction），不是路线图。** 文中代码不能编译——它们是规格。
> 引用今天已存在的东西标 `[已存在]`；估算而非实测的数字标 `[估算]`。
>
> 核心问题只有一个，也是采用 ent + 代码生成的主要动机：
> **在 CRUD 密集的系统中，这个框架能节省、减少多少工作量。**
>
> 方法论固定（本仓库记忆有档）：像 Delphi/VCL 先写 `Button.Text := 'Hello';` 再设计
> VCL 那样，**先写出理想的开发体验，再倒推设计**。

---

## 1. The One Line

```go
// ent/schema/article.go —— 这是全部
type Article struct{ ent.Schema }

func (Article) Fields() []ent.Field {
	return []ent.Field{
		field.String("title"),
		field.Text("body").Optional(),
	}
}

func (Article) Annotations() []schema.Annotation {
	return []schema.Annotation{api.Resource()}     // ←←← 就这一行
}
```

```go
// main.go —— 也是全部
mux := http.NewServeMux()
api.Mount(mux, ent.API(client))
http.ListenAndServe(":8080", mux)
```

```console
$ go generate ./... && go run .

$ curl -s localhost:8080/articles
{"data":[],"total":0,"page":1,"size":20}

$ curl -s -XPOST localhost:8080/articles -d '{"title":"Hello"}'
{"id":"9f1c…","title":"Hello","body":null,"created_at":"2026-08-06T10:12:03Z"}

$ curl -s -XPOST localhost:8080/articles -d '{"body":"no title"}'
{"error":"validation failed: title is required","field":"title"}     # 422

$ curl -s localhost:8080/openapi.yaml | head                          # 一直都在
```

代码库里没有任何其它地方出现过 `articles` 这个词。没有路由表、没有 handler、没有
DTO、没有 `init()`、没有 `Register*`。

---

## 2. 工作量账本

### 2.1 一个 CRUD 实体到底要写多少重复代码

一个典型的 CRUD 密集后端，每个实体的重复性工作清单，与两种世界下"谁来写"：

| # | 重复性工作 | 今天（entdomain v当前） | 理想形态 |
|---|---|---|---|
| 1 | Create/Patch 请求体 + 三态 presence + 校验 | **生成** `[已存在]` | 生成 |
| 2 | Response/Summary + 边展开 + 急加载计划 | **生成** `[已存在]` | 生成 |
| 3 | 过滤器（每算子一参数）/ 排序白名单 / 分页 | **生成** `[已存在]` | 生成 |
| 4 | 每操作一个 wiring 函数（6 个操作） | **生成** `[已存在]` | 生成 |
| 5 | 错误分类（404/409 语义） | **生成**，唯一键谓词需手写 init `[已存在]` | 生成，方言运行期分发 |
| 6 | **schema 上的注解声明** | 手写，每字段一条 3–4 连调用链 | **一行/实体**，字段沉默 |
| 7 | **HTTP handler ×6**（绑参→校验→调 wiring→映射状态码→写 JSON） | **手写** | **生成** |
| 8 | **路由注册** | 手写 | **生成**（Mount） |
| 9 | **OpenAPI**（swaggo 式注释块或手维护 yaml） | 手写 | **副产品** |
| 10 | 行级授权谓词（属主过滤） | 手写在每个 handler/query | **生成**（`OwnedBy`） |

今天的框架消掉了 1–5（中间层），把 6 的声明成本和 7–10 整层留给了开发者。
**理想形态与今天的差距 = 第 6 行的收缩 + 第 7–10 行的接管。**

### 2.2 实测：今天的杠杆率

生成行数取自本仓库 fixtures（`wc -l`，2026-08-06 实测）：

| 实体 | 生成（dto+filter+wiring） | 输入（schema 含注解） |
|---|---|---|
| wiring/article（带边） | 765 行 | ~40 行 |
| wiring/author | 667 行 | ~40 行 |
| wiring/note | 610 行 | ~40 行 |
| basic/widget（最简） | 616 行 | 49 行 |
| query 双实体均值 | ~725 行 | — |

另有 graph 级 `entdomain_errors.go` 57 行 + `entdomain_softdelete.go`（启用时）。

**今天：每实体约 40–50 行 schema 换 610–765 行生成代码，杠杆率 ≈ 1 : 15。**

### 2.3 估算：理想形态的账（N = 30 实体的 CRUD 密集系统）

`[估算]` HTTP 层手写成本按每操作 25–45 行（绑参/校验/调用/错误映射/写响应）×6 操作
+ 路由注册 + swaggo 注释块 ≈ **220–400 行/实体**。未在本仓库实测（仓库无 handler 层），
需一个真实消费者项目校准。

| | 今天（entdomain v当前） | 理想形态 |
|---|---|---|
| schema 声明 | ~1,350 行（45×30，含注解链） | **~900 行**（30×30，字段沉默） |
| HTTP 层（handler+路由+OpenAPI） | **~9,000 行手写** `[估算]` | **0**（生成 + 副产品） |
| 中间层（DTO/filter/wiring/errors） | 0（已生成 ~21,000 行） | 0（同左，另加生成 HTTP 层） |
| 唯一键分类 init、软删除注册等仪式 | ~30 个调用点 | **0**（Mount 内自动） |
| **开发者授权总量（CRUD 部分）** | **≈ 10,400 行** | **≈ 900 行 + 偏离** |

**理想形态把 CRUD 部分的授权代码再压一个数量级。**

账本是**度量，不是裁决器**（owner 校正，2026-08-06）：最高标准始终是「找到最好的
开发体验」，行数只是杠杆兑现与否的证据——一个省行数但复活仪式感的方案照样出局，
裁决器是 transcript 对照（§5 第 3 条）。

### 2.4 账本之外，代码生成的三项既得红利（保持，不折腾）

- **生成的代码是你的代码** `[已存在]`：躺在你的 `ent/` 包里的普通 Go 函数，可读、
  可断点、可搜索；删掉文件第一行 marker 就永远归你。凌晨三点排障没有魔法层。
- **OpenAPI 是副产品**：`curl :8080/openapi.yaml` 一直都在，没人维护它；
  `npx openapi-typescript` 让前端类型免费。"接口文档在哪"这个问题从宇宙中消失。
- **矛盾即拒绝** `[已存在]`：注解与 ent schema 矛盾时生成失败并同时报出两个事实。
  （消息附可直接粘贴的修复行是 nice-to-have，加分不承重。）

---

## 3. 方法：沉默即默认，注解只表达偏离

第 6 行（声明成本）怎么从每字段 3–4 连调用链收缩到每实体一行——这是理想形态里
唯一的**声明设计**，其余都是接管。

### 3.1 ent 已经说过的，一个字都不再说

`field.String("title")` 已经写着：string、非 Optional（创建必填）、非 Immutable
（可 PATCH）、无 Default、无 Sensitive。这五个事实**完全决定**它在 HTTP 面上的形状，
所以一个注解都不用写：

```go
field.String("title"),                                    // 全默认：进 create/patch/response
field.String("slug").Unique().Immutable(),                // ent 说了算：创建可给、PATCH 不存在、重复 409
field.Text("body").Optional().Annotations(api.Searchable()), // 偏离：显式解禁子串扫描
field.String("password_hash").Sensitive(),                // ent 原生：永不进响应，任何注解覆写不了
field.String("internal_note").Annotations(api.Hidden()),  // 偏离：只在 service 层存在
```

推导的边界（实测钉死）：ent 把 `MinLen`/`Match`/`Min`/`Max` 编译成闭包，
`gen.Field.Validators` 只是 `int` 计数（`entc/gen/type.go:94`），值在 codegen 前已擦除
——所以数值/模式类约束**推不出来**，它们保留为注解（这也是「注解只表达偏离」的正统
用法：ent 不知道的事实才需要说）。查询三维度（Searchable/Filterable/Sortable）保持
显式标记：子串匹配天生扫表，排序白名单是安全边界（ADR-0005，`[已存在]`），
它们是偏离，不是默认。

### 3.2 新字段的可见性

实体级一行意味着新字段默认进入 API 面。防线放在体验里，不放在门禁里：

```console
$ go generate ./...
  Article: + views (response, filterable)      ← 变化被说出来，不拦截
```

`Sensitive()` 硬挡高危类；其余靠生成时打印 + 生成物/spec 的 PR diff。
（曾考虑过契约文件门禁与 accept 棘轮，按「体验优先 + 治理出界」裁决移除，见 §5。）

### 3.3 偏离的梯度：偏离 = 少调一个函数

```go
// 第 1 级：全默认
api.Mount(mux, ent.API(client))

// 第 2 级：某个操作不合胃口 —— 停止调用生成的，写你自己的。就这样。
//         生成的 DTO、校验、过滤器都还在，继续用；只有偏离的那层换成你的。
func MyCreateArticle(ctx context.Context, db *ent.Client, r *ent.ValidArticleCreateRequest) …

// 第 3 级：只要 wiring，不要 handler                  [已存在，今天就是这层]
page, err := ent.ListArticles(ctx, db, filter, req)

// 第 4 级：只要 DTO                                   [已存在]
resp, err := ent.NewArticleResponse(entity)

// 第 5 级：删掉文件第一行 marker —— 永远归你           [已存在]
```

每一级都是「少调一个函数」，不是「配置一个开关」——开关会让第 N 级的行为取决于
第 N+1 级的配置状态。偏离的成本与偏离的幅度成正比，任何一档都不需要学新概念。

### 3.4 HTTP 层的边界：transport 100%，业务 0%（不是走回 Base 的老路）

初版曾有 `Base{Entity}Handler`/`Base{Entity}Service`，#29 删除。病**不是**"生成了这
两层"，而是三条（`wiring.tmpl:30-36` 的比例论证 + #16/#29 的删除记录）：定制发生在
生成类型**内部**（embed + override + `SetSelf` 自派发，Go 无虚派发所以才需要 hack）；
生成了带业务判断的**厚方法体**（三十行猜测，调用方无法干预）；框架试图当业务逻辑的
**宿主**。理想 HTTP 层三条都不犯：

- **HTTP 层可以被生成，恰恰因为它没有判断含量**：绑参由 DTO 形状完全决定，状态码由
  错误分类完全决定，序列化纯机械。生成的 handler 永远是三步体——绑 → 调一个函数 →
  写；长出业务分支即设计违规，且**没有任何 override 点**。
- **框架永远不生成 Service 层**：等着被填充的空 Service 骨架就是 Base 模式还魂。
  纯 CRUD 端点的三步体第 2 步默认调生成的 wiring 自由函数（零业务代码——账本省掉的
  正是这部分）；有业务逻辑的端点，你写**普通函数**，框架看不见它。
- **定制的三扇门，全是组合，没有继承**：
  ① 认证/日志/追踪 → 标准中间件，在 Mount 外面；
  ② 这个操作要业务逻辑 → **换脑**：`api.Mount(mux, ent.API(client).With(
  ent.CreateArticleFn(myCreate)))`——签名由生成的 DTO 固定、编译器检查、整单元替换。
  与 Base+SetSelf 的区别是本质的：生成代码调用你，你从不嵌入它，不存在部分覆写一个
  生成方法体的可能。先例：gRPC 的 protoc 生成 transport + 接口、你实现业务——且
  wiring 比 gRPC 多走一步，默认实现就是完整的，纯 CRUD 连接口都不用实现；
  ③ 端点根本不是 CRUD（上传/流式/按角色裁剪）→ `external`，整个动词归你，
  生成的 DTO/校验照用。

---

## 4. 与今天的差距

| | 今天 | 理想 | 性质 |
|---|---|---|---|
| 中间层生成（账本 1–5 行） | ✅ 每实体 610–765 行实测 | 保留 | `[已存在]` |
| 泛型运行时，0 个 entgo 依赖 | ✅ 实测 | 保留 | `[已存在]` |
| 矛盾即拒绝 + 高质量消息 | ✅ | 保留并扩展到新推导 | `[已存在]` |
| marker 逃生舱 | ✅ | 保留（第 5 级） | `[已存在]` |
| 声明成本 | 每字段 3–4 连链 | 每实体一行，字段沉默 | **需推翻 Scope 模型** |
| 属性推导 | 仅 presence 三判定 | 全面（C3 边界内） | 现有哲学的扩展 |
| HTTP handler / 路由 / Mount | 无 | 生成（三步体：绑→调→写，比例论证不变） | **需新增** |
| OpenAPI | 无 | 副产品 | **需新增** |
| 唯一键/软删除仪式 | 手写 init/Register | Mount 内按 `Driver.Dialect()` 运行期分发 | **需新增** |
| 行级授权 | 无 | `api.Resource().OwnedBy("author")` 生成谓词 | **需新增** |

落地顺序上唯一的裁决：**先让 `.Sensitive()` 从响应里消失**——它是「从 ent 推导」
最小的可证伪切片，纯收窄（只可能移除字段），不可能破坏正确的消费者，只可能修复
一个正在泄漏的。做完它，属性推导就从主张变成已落地的模式。

---

## 5. 边界与已裁决事项（一句话存档，全文见 git 历史与会话记忆）

**Scope 宪章**：这套框架的目的是杠杆——一次性 schema 声明 → 生成大量"猜不错"的代码
→ 大规模简化 CRUD 密集系统的开发。义务到「产出确定性、可 diff、带 spec 的工件」为止。

已裁决（2026-08-06，两轮跨家族评审 + owner 三次校正的净残留）：

1. **API 治理出界**：版本化、废弃遥测、破坏性变更门禁、暴露面审查流程——全部交给
   生态在工件之上完成（先例：protoc 生成，buf 治理）。框架不经营治理流程。
2. **意图必须编译器可验证**：声明走注解（Go 编译器 + `checkGraphConflicts` 双重验证），
   永不引入人工编辑的文本 DSL；纯文本工件只允许机器拥有。
3. **暴露姿态**：实体级一行 + 生成时大声打印，不设门禁（transcript 对照裁决：
   字段级逐一注解会复活「忘写→字段消失→排查」的仪式感，出局）。
4. **C3 事实**：校验器值在 codegen 前被 ent 擦除，数值/模式类约束保留为注解。
5. **查询三维度保持显式**（ADR-0005 延续）：索引不推导契约——DB 调优与公开 API
   解耦，`sort=`/`filter=` 白名单只来自显式标记。
6. **不做**：GraphQL 式深嵌套（Summary 只展开一层）、自研路由/Context、运行时反射、
   生成三十行方法体（比例论证：生成的厚方法体是生成器猜错而调用方无法干预的地方）。

---

## 6. 一句话

> 今天：40 行 schema 换 700 行中间层，HTTP 层的 300 行还得你写。
> 理想：30 行 schema 换整个 CRUD 面——中间层、HTTP 层、spec，全部；
> 业务逻辑住在框架看不见的普通函数里，transport 与业务的边界一刀切干净。
> **最高标准是体验；账本是它兑现与否的证据。**
