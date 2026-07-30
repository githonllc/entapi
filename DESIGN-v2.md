# entdomain 设计 v2：把生成物从「根」改成「叶」

> **状态：方向已采纳，实现未开始。修订 3。**
>
> 初稿经过两轮对抗性评审（Fable 5 与 Codex GPT-5.6，各自独立、只读、要求证伪而非认同）。
> 初稿有 **5 处事实错误**，其中 2 处使整节论证失效。全部列在 §6，本稿已按结论重构。
>
> 修订 3 增补 §7：初稿的四个开放问题中有两个已由 spike（PR #22）用真实 ent + 真实
> SQLite 判定，不是靠读码。被推翻的推理一律保留在 §6/§7，不做静默删除——
> **知道哪些直觉在这个代码库里不成立，本身就是设计资料。**
>
> 现状描述见 `ARCHITECTURE.md`，缺陷清单见 `QUALITY-REVIEW.md`。本文只谈设计变更。
> 工作项拆分见 epic #23。

---

## 0. 论点

原型的核心直觉是对的：**一个 CRUD API 的 N 个 DTO 不是 N 个独立类型，而是同一个字段集上的 N 个视图。** 这是全项目唯一的真抽象，保留。

初稿据此提了两项重设计。评审后只剩一项：

- **T3（保留，且范围比初稿小一个量级）**——删除「根」形态的生成物，加固已有的「叶」形态。
- **T2（延后）**——给 scope 加受众维度。论据在评审中塌缩，见 §2。

### 0.1 关键事实：项目里已经同时存在「根」和「叶」

初稿说「生成的代码位于消费者继承链的根」。**这个描述过头了**，`extension.go:24-34,39-45` 显示：

```go
type ExtensionConfig struct {
    GenerateBaseService bool   // 默认 false
    GenerateBaseHandler bool   // 默认 false
}
```

`NewExtension(nil)` 得到零值配置，**两个开关都是关的**。所以：

| 生成物 | 默认 | 形态 |
|---|---|---|
| `{entity}_dto.go` | **开** | 叶——纯数据类型，消费者调用 |
| `{entity}_base_service.go` | 关（opt-in） | 根——消费者内嵌并继承 |
| `{entity}_base_handler.go` | 关（opt-in） | 根 |

**项目已经有了正确的一半。** T3 因此不是「推翻架构」，而是两件小事：

1. 删掉 opt-in 的那一半（它承载了软删除改写、`uuid.UUID` 硬编码、`SetSelf` 静默失败、`DB *Client` 导出这一整串问题）。
2. 把默认开的那一半补齐——目前它只生成结构体，不生成任何映射代码，而映射恰恰是手写必错的部分。

### 0.2 为什么「根」那一半必须删，而不是修

`Base{Entity}Service` 导出 `DB *Client`。任何在该层生成的可见性过滤——软删除、租户隔离——都能被一行 `s.DB.User.Query()` 绕过。**在这个位置生成的策略是建议，不是保证。**

这解释了注解表里那批"没接线"字段的真实性质：它们不是**还没做**，是**在这个落点上做不了**。

策略的正确落点是 ent 自己的扩展点（interceptor / mixin / privacy），绕不过去。见 issue #18。

---

## 1. T3：加固叶子，删除根

### 1.1 判据

**只生成「手写会写错」的部分：** 字段逐个映射、`Set` vs `SetNillable` 的选择、optional/nillable/immutable 的规则差异、边的加载状态判定。这些机械且与 schema 强耦合——人写必错，schema 改了必忘。

**其余全部还给消费者：** 事务边界、鉴权、业务前后置、查询组合。ent 的查询构建器本身就是完整的，在它之上包一层只会更弱。

### 1.2 生成物清单

删除：`Base{Entity}Service`、`Base{Entity}Handler`、`SetSelf`、条件生成的 2–6 个 hook、两个模板、两个配置开关。

新增到 DTO 层：

```go
// 校验即解析（parse, don't validate）——见 1.4
func (r *UserCreateRequest) Validate() (*ValidUserCreateRequest, error)
func (r *UserUpdateRequest) Validate() (*ValidUserUpdateRequest, error)

// 只有已校验的类型才能 Apply，类型系统保证顺序
func (v *ValidUserCreateRequest) Apply(b *ent.UserCreate) *ent.UserCreate
func (v *ValidUserUpdateRequest) Apply(b *ent.UserUpdate) *ent.UserUpdate
func (v *ValidUserUpdateRequest) ApplyOne(b *ent.UserUpdateOne) *ent.UserUpdateOne

// 实体 → 响应；返回 error 见 1.5
func NewUserResponse(e *ent.User) (*UserResponse, error)
func NewUserResponses(es []*ent.User) ([]*UserResponse, error)
```

### 1.3 三态 PATCH：生成 unmarshaler，不引入包装类型

当前 `dto.tmpl:75` 对所有 update 字段发射 `*T`，**nil 同时表示"没传"和"传了 null"**，PATCH 语义表达不了。

初稿提议引入泛型 `entdomain.Optional[T]`。**评审否掉了，两家都否**，理由充分：

- 它没有 `MarshalJSON`，字段又全部未导出 → 出站序列化产出 `{"nickname":{}}`，unset/null/value 三态全丢，再反序列化到标量还会报类型错
- 结构体类型用不了 `omitempty`（`omitzero` 是 Go 1.24，go.mod 是 `go 1.23`）
- 破坏 go-playground/validator、form binder、OpenAPI 生成器等一切反射消费者
- 没有构造器，消费者无法在代码里构造 set/null 状态，测试只能绕 JSON

**正确做法：这是代码生成器，用生成解决。** 字段保持 `*T`，为每个请求类型生成 `UnmarshalJSON` 记录出现情况：

```go
type UserUpdateRequest struct {
    Nickname *string `json:"nickname,omitempty"`

    present map[string]bool
}

func (r *UserUpdateRequest) UnmarshalJSON(b []byte) error {
    type alias UserUpdateRequest              // 避免递归
    var raw map[string]json.RawMessage
    if err := json.Unmarshal(b, &raw); err != nil {
        return err
    }
    if err := json.Unmarshal(b, (*alias)(r)); err != nil {
        return err
    }
    r.present = make(map[string]bool, len(raw))
    for k := range raw {
        r.present[k] = true
    }
    return nil
}

func (r *UserUpdateRequest) HasNickname() bool { return r.present["nickname"] }
```

线格式不变，绑定器不破，反射消费者不破，`json.Marshal` 正常工作。初稿 §4 的 Q1（框架绑定）**因此消失**——那不是外部未知，是我自己引入的包装类型造成的。

对应的 Apply：

```go
func (v *ValidUserUpdateRequest) Apply(b *ent.UserUpdate) *ent.UserUpdate {
    if v.HasNickname() {
        if v.Nickname == nil {
            b.ClearNickname()      // 仅当该字段进入 ent 的 MutableFields 且 Optional
        } else {
            b.SetNickname(*v.Nickname)
        }
    }
    return b
}
```

> **`Clear<X>()` 的确切生成条件**（`entgo.io/ent@v0.14.4` `entc/gen/template/builder/setter.tmpl:13-24,68-75` 与 `entc/gen/type.go:552-564`）：必须同时满足——builder 是 updater、字段 `Optional`、且字段进入 `MutableFields`。Immutable 字段与 immutable-edge 的 FK 已被 `MutableFields` 先行排除。
>
> **Immutable 字段**：`Update` 与 `UpdateOne` 都基于 `MutableFields`，因此**都不生成 setter**；而 `Create` 仍遍历普通 `Fields`，**会**为 immutable 字段生成 setter。

### 1.4 校验即解析

初稿写「`Apply` 永不返回 error，错误只住在 `Validate`」。**评审指出这个契约不安全**：消费者可以直接跳过 `Validate` 调 `Apply`，此时显式 null 打到不可空字段、或 immutable 字段出现在 update 里，只能静默忽略。

改为 **parse-don't-validate**：`Validate()` 返回一个**只有它能构造**的 `Valid*Request` 类型，`Apply` 只挂在该类型上。跳过校验不再是纪律问题，而是**编译错误**。

生成器多生成一个类型的成本为零。

> 初稿还写「immutable 字段出现在 update 请求里由 `Validate` 拦下」——**这一条不可执行**。immutable 字段本来就不会出现在 `UpdateRequest` 结构体里（ent 不生成对应 setter），`encoding/json` 会静默丢弃这个未知 key，`Validate()` 无从得知。要拦必须在解码时 `DisallowUnknownFields`，那在 handler 的 decoder 手上，不在 dto 手上。**已从设计中删除**；若要这个能力，需要生成一个拥有严格解码的 `Decode` 辅助函数。

### 1.5 边：必须走 `OrErr()`，`nil` 判断是错的

初稿写「只映射已加载的边（`if e.Edges.Posts != nil`）」。**这是错的**，`entgo.io/ent@v0.14.4` `entc/gen/template/ent.tmpl:69-86` 显示真相在未导出的 `loadedTypes [N]bool` 里：

```go
func (e UserEdges) ProfileOrErr() (*Profile, error) {
    if e.Profile != nil {
        return e.Profile, nil
    } else if e.loadedTypes[0] {
        return nil, &NotFoundError{...}     // 加载了，但关联行不存在
    }
    return nil, &NotLoadedError{edge: "profile"}   // 根本没加载
}
```

对唯一边，`nil` 同时可能是「未加载」和「已加载但不存在」。**`loadedTypes` 未导出，`ent/dto` 子包读不到**——唯一通道是生成的 `<Edge>OrErr()` 配合 `ent.IsNotLoaded` / `ent.IsNotFound`。

而且（见 §6-2）**当前规则下只有唯一边能进响应**，也就是说 `nil` 有歧义的那一类，恰好是唯一会发生的那一类。

**设计决定**：`NewUserResponse` 返回 `(*UserResponse, error)`。

- `NotFound`（已加载、无关联行）→ 边字段置 nil，正常返回
- `NotLoaded`（调用方没 eager-load）→ **返回 error**

理由：响应类型声明了这条边，就意味着调用方承诺加载它。把「响应形状取决于调用方的 eager-load 计划」这件事**变吵，而不是变静默**。这推翻了初稿「纯映射函数永不失败」的说法——那个说法在评审中已经站不住。

### 1.6 输出位置：`ent/dto`

```
handler  →  dto              handler 里一个 ent 类型名都不出现
service  →  dto, ent         service 是被允许知道持久化的那一层
dto      →  ent              单向，无环
```

- **无环，已核实**：ent 自己生成的 `hook` 子包就是同一方向（`entc/gen/template/hook.tmpl:11-25`）。
- **实现缺一步**：当前 writer 用 `os.WriteFile` 直接写（`extension.go:166-175`），**不建目录**，必须显式 `MkdirAll`。
- **前提条件**：service 返回 `*dto.UserResponse` 而非 `*ent.User`。这才是 handler 解不解耦的决定因素。

> `base_handler.tmpl:21` 声称 "handler code never imports the ent package directly"，但 `:7` 生成到 ent 包内、`:25` 参数就是 ent 类型——**目标对，手段无效**。删掉 BaseHandler 后由包放置兑现。

### 1.7 消费者代码

```go
type UserService struct{ db *ent.Client }

func (s *UserService) Create(ctx context.Context, req *dto.UserCreateRequest) (*dto.UserResponse, error) {
    valid, err := req.Validate()
    if err != nil {
        return nil, err
    }
    u, err := valid.Apply(s.db.User.Create()).Save(ctx)
    if err != nil {
        return nil, entdomain.MapError(err)
    }
    return dto.NewUserResponse(u)
}
```

十行，全部可见，全部可改。**生成器不在这段代码的上游，它只是被调用。**

---

## 2. T2（受众维度）：延后

初稿提议给 scope 加受众维度（`Views map[Audience][]FieldScope`），并删除 `Sensitive`/`ReadOnly`/`WriteOnly` 三个"语义重叠的布尔"。

**两轮评审都判它不能按原样推进，理由一致：**

1. **核心论据是误读。** `ReadOnly`/`WriteOnly` 不在 `DomainField` 上，它们在 `FieldMetadata`（`annotations.go:46-82`），而该结构体开头写着 "RESERVED … will be used when OpenAPI/Swagger spec generation is implemented"（`:43-45`）。**那是 OpenAPI 规范词汇，不是 scope 词汇。** 只有 `Sensitive` 在 `DomainField:109`。"同一个缺口被从三个方向补了三次"从 3 个证据塌缩成 1 个。
2. **`ReadOnly`/`WriteOnly` 不得删除**——它们承载文档契约，与 scope 不等价。
3. **alias 规则的安全承诺是假的。** 构造函数不能 alias，必须另发 `NewUserPublicResponse` 包装；受众分叉时生成器重新发射包装+结构体，**所有调用点照常编译**。分叉是静默行为变更，不是初稿说的"编译错误精确暴露"。
4. **受众无关的 `Required` 不闭合。** 初稿 §1.2 明确接受了这个代价，但代价被低估了：某受众隐藏一个「创建必填且 schema 无默认值」的字段，该受众的 create **结构上不可能成功**，错误推迟到 ent `Save`。要么 `Required` 也进视图，要么生成期拒绝非闭合投影。
5. **没有安全出口契约。** 受众只是展示投影；运行时如何选受众未设计，调用 admin mapper 依然泄漏。它不能被当作安全机制宣传。
6. **零消费者、零真实用例。** 初稿自己排的顺序（N2 在 N1 前）已经默认了这点。

**结论：T2 从本提案中撤出，单独立案，等一个真实用例再谈。** `Sensitive` 的处置回到 issue #3 独立解决——实现它，或废弃它；不与受众维度绑定。

---

## 3. 兼容性

破坏面：

- 生成物：`Base{Entity}Service` / `Base{Entity}Handler` / `SetSelf` / hooks 删除；`ExtensionConfig` 两个开关删除
- 输出位置：DTO 从 `ent/` 移到 `ent/dto/`
- update 请求：新增生成的 `UnmarshalJSON` 与 `Has<X>()`（线格式不变）
- 新增 `Valid*Request` 中间类型；`Apply` 移到该类型上
- `NewXxxResponse` 返回 `(T, error)`

两个开关默认关闭，意味着**基类删除对默认配置的消费者不是破坏性的**。仓库内无示例应用、无下游 ent 项目，破坏窗口成本现在最低。

---

## 4. 未解决的问题

> **Q1、Q2 已由 spike 判定，见 §7。Q3、Q4 仍然开放。**

**Q1 — 边选择规则未设计（最大的洞）。** ~~未解决~~ → **已解决**，见 §7.2，实现转 #25。 `edgeQualifiesForResponse`（`funcs_fields.go:126-133`）要求 `edge.Field() != nil`，而该值只对外键落在本实体上的边非 nil，即唯一边。**to-many 边永远无法进入响应**，`dto.tmpl:116` 的 `[]*` 分支是死代码。哪些边该出现在响应里，当前是从 FK 推导的副作用，不是设计出来的。这比"Summary 类型含哪些字段"更根本。

**Q2 — 自引用树。** ~~未解决~~ → **已判定为「默认形态不支持」**，见 §7.3。两级类型在类型系统层面封死深度 1；更深的树需要每层一次往返。显式深度参数仍然可能，但不在本设计范围内，且该代价由测试断言而非文档一笔带过。

**Q3 — 生成物生命周期。** 输出位置变更 + 零注解节点被直接跳过（`extension.go:74-77`）意味着删注解会遗留旧文件；生成中途失败还会留下「ent 已更新、dto 部分更新」的混合状态。需要**生成物 manifest**，而不是靠文件名猜测清理。`ent/dto` 的目录所有权与消费者既有同名包的冲突策略也未定义。

**Q4 — 受众在运行时如何选择**（若 T2 将来重启）。这是鉴权层的判断，可能不属于 entdomain 的职责；若不属于，交接点必须写进文档。

---

## 5. 对现有 18 条 issue 的重算（已执行）

> **本节已落地。** 结论驱动了 tracker 重构：epic #23 建立，#20 关闭（completed）、
> #21 关闭（superseded，拆成 #24–#29）。下表保留为决策记录，**当前工作项以 #23 为准**。

判定：**存活**＝问题依旧，issue 不变 · **改写**＝问题在但形态变了 · **消解**＝承载问题的代码不存在了

| # | 标题 | 判定 | 理由 |
|---|---|---|---|
| 1 | Quality review remediation（父跟踪） | **改写** | 纳入 T3，子项重排 |
| 2 | Green the build baseline | **存活** | 与设计无关。**最先做** |
| 3 | Sensitive fields emitted into responses | **存活** | T2 撤出后回到独立问题：实现 `Sensitive` 或废弃它。**不得连带删除 `ReadOnly`/`WriteOnly`**（§2-2） |
| 4 | Windows init panic on template lookup | **存活** | `template_loader.go:15`，与设计正交 |
| 5 | Builders copy shallowly / share state | **存活** | T2 延后后不再是前置条件，但缺陷本身仍在 |
| 6 | Cursor: zero-limit panic, precision, two formats | **拆分** | 零 limit panic 在 `base_service.tmpl` → 随基类删除**消解**；`cursor.go` 的 int64 精度丢失 + 双格式 → **存活** |
| 7 | Remove dead funcs / duplicated template | **存活并扩大** | 新增死代码一处：`dto.tmpl:116` 的 to-many 分支不可达（Q1） |
| 8 | Codegen fixture harness (generate + compile) | **存活并升级为前置门** | 要重写 DTO 生成就必须先有编译门。**唯一的硬瓶颈** |
| 9 | Guards not dependency-closed | **改写** | `EntToResponse` 移入 dto 后跨守卫引用消失；`ListResponse` 守卫问题随模板重写解决。验收条件保留 |
| 10 | Nillable/immutable/named-GoType 不编译 | **改写** | 目标从 `setFieldCallReq` 改为 `Apply` 生成；immutable 的处理依 §1.3 的 `MutableFields` 规则 |
| 11 | Format failure silent / stale artifacts | **存活并加重** | 输出位置迁移使陈旧产物从卫生问题变成迁移必要条件；需要 manifest（Q3） |
| 12 | Soft delete write-only / batch bypasses hooks | **消解** | 不再生成 Delete/DeleteBatch；软删整体移交 #18 |
| 13 | CRUD methods disagree on error mapping | **改写并缩小** | 无"多个生成方法"需要对齐。改为运行时提供一个 `entdomain.MapError` |
| 14 | Consistent field presence model | **改写为 T3 主线** | 见 §1.3–1.4。**明确否决 `Optional[T]`**，改用生成的 `UnmarshalJSON` + `Has<X>()` |
| 15 | Split runtime into generator-free subpackage | **存活** | 运行时新增 `MapError`，收益略增 |
| 16 | Replace SetSelf with function fields | **需重新决策** | T3 主张整体删除 hook；但基类是 opt-in 默认关，"函数字段"方案的紧迫性因此下降。**与先前已拍板的"选函数字段"冲突，需重新确认** |
| 17 | Accepted-but-ignored annotation surface | **改写** | 查询类需求由消费者直接用 ent 构建器满足；UUID 硬编码随基类删除消失。**`FieldMetadata` 的字段属预留文档契约，排除在删除范围外** |
| 18 | Generate an ent-layer soft-delete mixin | **存活并加重** | 策略下沉的唯一落点，从可选特性变成 T3 的必要配套。其 Q1（是否需要生成）仍未解 |

### 需要新建

| 新 | 标题 | 说明 |
|---|---|---|
| N1 | Design which edges appear in responses | Q1。当前 to-many 边不可达，规则是 FK 推导的副作用。**T3 的前置设计** |
| N2 | Replace base service/handler with hardened DTO leaves | T3 主体（§1.2–1.7）。依赖 #8、N1 |

### 净结果

18 条中 **2 条消解**（#12，及 #6 的 panic 部分），**6 条改写**，**1 条需重新决策**（#16），其余存活；新增 2 条 → **共 20 条**。

价值转移在于：**#8 编译门从"验证工具"变成"重写的前置条件"**，而 **N1 边选择规则是 T3 之前必须先答的设计问题**。

### 建议顺序

```
第 0 阶段（与设计无关，立即可做）
  #2 绿化基线 → #4 Windows panic → #7 删死代码 → #5 拷贝语义

第 1 阶段（门）
  #8 fixture 编译门   ← 没有它，重写没有任何安全网

第 2 阶段（设计）
  N1 边选择规则       ← T3 无法绕过

第 3 阶段（T3 主体）
  N2 → #14 #10 #9 并入

第 4 阶段（配套）
  #13 MapError · #15 运行时拆包 · #11 产物清理 + manifest · #6 cursor 编解码
  #18 软删 mixin（先答其 Q1）

单独立案
  T2 受众维度（等真实用例）· #3 Sensitive 的独立处置 · #16 重新决策
```

---

## 6. 评审记录：初稿中被证伪的断言

保留此节，因为**被推翻的推理链本身是设计资料**——它记录了哪些直觉在这个代码库里不成立。

| # | 初稿断言 | 判定 | 证据 |
|---|---|---|---|
| 1 | `ReadOnly`/`WriteOnly` 与 `Sensitive` 是同层的三个重叠布尔，说明代数少一维 | **误读，两家均指出** | 前两者在 `FieldMetadata`（`annotations.go:46-82`），明确预留给 OpenAPI（`:43-45`）；只有 `Sensitive` 在 `DomainField:109`。T2 的核心论据因此塌缩 |
| 2 | 例：`UserResponse.Posts []*PostSummary` | **该形态无法生成** | `edgeQualifiesForResponse` 要求 `edge.Field() != nil`（`funcs_fields.go:126-129`），只有唯一边满足。to-many 边永不入响应 |
| 3 | 生成物位于消费者继承链的根 | **过头** | 基类是 opt-in 且默认关（`extension.go:24-34,39-45`）。项目已有正确的叶子一半 |
| 4 | `if e.Edges.X != nil` 可判断边是否已加载 | **错** | 真相在未导出的 `loadedTypes`；唯一边的 nil 三义（`ent.tmpl:69-86`）。必须走 `OrErr()` |
| 5 | 引入 `entdomain.Optional[T]` 表达三态 | **错误的工具** | 无 `MarshalJSON` + 字段未导出 → 出站产出 `{}`，三态全丢；破坏反射消费者；`omitzero` 需 Go 1.24 而 go.mod 是 1.23。生成器应当用生成解决 |
| 6 | `Apply` 永不失败，错误只住在 `Validate` | **契约不安全** | 消费者可跳过 `Validate`。改为 parse-don't-validate |
| 7 | immutable 字段出现在 update 请求里由 `Validate` 拦下 | **不可执行** | 该字段根本不在结构体里，`encoding/json` 静默丢弃未知 key |
| 8 | 受众 alias 分叉时"编译错误精确暴露" | **假** | 构造函数不能 alias；生成的包装函数使调用点照常编译 |
| 9 | §5 净结果 17 条 | **算错** | 完全消解 3 条、新增 3 条 → 18。本稿已按新判定重算 |
| 10 | 「六个固定 hook 点」 | **不准确（单源，未独立核实）** | Codex 指出 Create/Update hook 按字段存在性条件生成，实际 2–6 个 |

**QUALITY-REVIEW P1-7（`EntToResponse` 无环检测）的定级需要重估。** Fable 指出递归只跟随已加载的边，而 ent 的预加载深度是调用方显式且有限的，因此"崩进程"的路径需要手工缝出环形指针。该发现目前**没有对应的 GitHub issue**（建 issue 时的遗漏）。建议等 #8 编译门建起来后用真实 fixture 判定，而不是靠读码改定级。

### 评审方法

两个评审并行、独立、只读，指令要求**证伪而非认同**，且明令不得采信本文档的事实断言、必须回源码抽查。裁决规则事先声明：两家都提的直接采纳；单家提出的由 orchestrator 回源码独立核实；冲突则标为未决。

上表 1–5 中，第 1、2、3、4 条由 orchestrator 独立核实过（读 `annotations.go:40-114`、`funcs_fields.go:105-146`、`extension.go:18-77`、`ent.tmpl:54-90`）。第 10 条为单源、未独立核实，已如实标注。

两家的最终评级表面冲突（Fable：T3 adopt-with-changes；Codex：T3 reject），但 Codex 的正文写明"保留删除基类、只生成叶子的方向"——**实质结论一致：方向成立，初稿规格不成立。**


---

## 7. 验证记录：spike（PR #22）判定了什么

§1 的形态没有直接写成模板，而是**先手写成目标产物、编译、跑通**，再决定要不要教模板去产出它。理由在 §6 已经写明：现有模板正是反着做的产物。

fixture 是**独立 Go module**，SQLite 驱动进不了库的依赖图。40 个子测试在 `-count=2` 下通过。

### 7.1 Layer 1 / Layer 2 的边界成立

判据落成一句话：**schema 能唯一决定的，生成；只是长得像的，不生成。**

重复 ≠ 可推导。`getOne`/`getMany`/`update`/`delete` 确实重复，但 schema 里没有它们需要的信息（鉴权、事务、租户、副作用），所以生成器只能猜——**而每一次猜都会变成使用者逃不掉的约束**。

这不是理论。现有设计里每一条难受的约束，都恰好落在"生成器不得不猜"的位置：

| 约束 | 猜的是什么 |
|---|---|
| `uuid.UUID` 硬编码 | 主键类型 |
| `deleted_at` 字面量 | 软删除的存在与命名 |
| `DB *Client` 导出 | 使用者需要逃生口——而这恰好废掉了策略层自己 |
| 2–6 个 hook 点 | 业务想在哪里插手 |
| 错误映射不一致 | 什么算 not-found、什么算冲突 |

DTO 那一半一条都没有。同样不是巧合。

**经验证实**：手写一次的泛型 `ListPage` 驱动真实 `*ent.UserQuery`，**调用点类型推断全自动**。自引用约束 `Query[Q,P,O,E]` 成立，因为 ent 的链式方法返回具体 builder 类型（`entc/gen/template/builder/query.tmpl:43-68`）。**主键成为类型参数**，`uuid.UUID` 硬编码结构性消失——它当初存在纯粹因为 `text/template` 写不出泛型。

### 7.2 边的契约（初稿 Q1）

四个子问题各有答案，各有通过的测试：

| 子问题 | 结论 | 证据 |
|---|---|---|
| **选择** | 边必须有**自己的**注解。旧规则从外键位置推导，使 to-many 边永远不可达，且把"暴露标量"和"暴露嵌套对象"绑成一个开关 | `gen.Edge.Annotations`（`entc/gen/type.go:143-145`）、`schema/edge/edge.go:131,215` |
| **存在性** | 只能走 `<Edge>OrErr()`。`loadedTypes` 未导出，nil 指针区分不了「没加载」和「加载了但无关联行」 | `entc/gen/template/ent.tmpl:69-86` |
| **加载** | eager-load 计划由响应类型的边集合生成，使"未加载即报错"零成本——生成的接线里永不触发，只抓手写查询 | ent 的单实体快捷方法不加载任何边 |
| **深度** | 两级类型在**类型系统**封死，不用运行时计数器。Summary 无边字段，环构造不出来 | `TestTwoTierBoundsDepthAndWhatThatCosts` |

**决定**：加载了但不存在 → 显式 `null`；未加载 → **返回 error**。两者不可合并——客户端必须能区分「没有关联行」和「没人去查」。

### 7.3 顺带发现的两个坑

**① 自引用边的注解会静默挂错。** 链式写法

```go
edge.To("children", X.Type).From("parent").Unique().Field("parent_id").Annotations(a)
```

只把注解挂到**反向边**上，正向边什么也没拿到，**没有任何报错**，那条边就是永远不出现在响应里。已拆成 #30 要求生成期报错。

**② 注解到达 codegen 时是 `map[string]interface{}`**，不是写下去的那个 Go 类型。reader 里的 JSON 归一化是承重的——与字段注解完全相同的坑，`CLAUDE.md` 里已经记着，但边注解是新写的，差点重犯。

### 7.4 被 spike 推翻的初稿设计（续 §6）

| 初稿断言 | 判定 |
|---|---|
| 运算符集合需要一张自定义的裁剪表 | **不需要**。ent 自己就有（`entc/gen/predicate.go:67-71` + `func.go:122`），并以 `$field.Ops` 暴露给模板。生成物对账：enum 恰好 4 个谓词、string 13 个、optional int 追加 `IsNil/NotNil` |
| 应生成裁剪后的默认运算符子集 | **改为全量生成**。生成多少行不花人力，事后补运算符却要改模板、重生成、可能破坏兼容。唯一合并：`IsNil`/`NotNil` 收成一个 `*bool`——可空性是一个问题，拆两个参数会允许自相矛盾的请求 |
| `NewXxxResponse` 可以是永不失败的纯映射 | **不成立**。边的未加载态必须响亮失败，否则响应静默降级 |

### 7.5 尚未验证

- **游标分页**：只跑了 offset。`cursor.go` 的精度丢失与双格式（#6）原封未动。
- **模板本身**：一行都还没写。这是有意的——目标产物先被证明正确，模板才有东西可对照。
