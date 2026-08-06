# entdomain 设计 v2：把生成物从「根」改成「叶」

> **状态：方向已采纳，实现未开始。修订 5。**
>
> 初稿经过两轮对抗性评审（Fable 5 与 Codex GPT-5.6，各自独立、只读、要求证伪而非认同）。
> 初稿有 **5 处事实错误**，其中 2 处使整节论证失效。全部列在 §6，本稿已按结论重构。
>
> 修订 3 增补 §7：初稿的四个开放问题中有两个已由 spike（PR #22）用真实 ent + 真实
> SQLite 判定，不是靠读码。被推翻的推理一律保留在 §6/§7，不做静默删除——
> **知道哪些直觉在这个代码库里不成立，本身就是设计资料。**
>
> 修订 4 增补 §8：回答 Q3（生成物生命周期）。它**否决了 Q3 自己提出的 manifest 方案**——
> 「上次生成了什么」是可以从磁盘读出来的事实，不是需要记住的状态。同一惯例：原判断保留，
> 不静默删除。
>
> 修订 5 增补 §9：裁决五项 owner 决策（分页模型 · 注解与 schema 矛盾 · 错误映射位置 ·
> 空注解面 · 兼容策略）。过程中**本文的两条承重断言被证伪**——「游标有双格式」和
> 「ent 错误类型住在框架包里」，两条都记在 §9.0，相关旧表述已就地标注而非删除。
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

- 游标相关符号退出公开 API：`Cursor` / `PageInfo` / `EncodeCursor` / `DecodeCursor` / `ListRequest.Cursor`（§9.1）
- 注解字段删除：`UniqueLookup` / `RangeLookup` / `DomainConfig.EntityName`（§9.4）

两个开关默认关闭，意味着**基类删除对默认配置的消费者不是破坏性的**。仓库内无示例应用、无下游 ent 项目，破坏窗口成本现在最低。

**策略已定，见 §9.5：走 v0，不设弃用窗口，做完打 `v0.2.0` 配迁移说明。** 切 `v1` 的两个触发条件也在那里。

---

## 4. 未解决的问题

> **Q1、Q2 已由 spike 判定，见 §7。Q3 已设计，见 §8。只剩 Q4 开放，且随 T2 一起延后。**

**Q1 — 边选择规则未设计（最大的洞）。** ~~未解决~~ → **已解决**，见 §7.2，实现转 #25。 `edgeQualifiesForResponse`（`funcs_fields.go:126-133`）要求 `edge.Field() != nil`，而该值只对外键落在本实体上的边非 nil，即唯一边。**to-many 边永远无法进入响应**，`dto.tmpl:116` 的 `[]*` 分支是死代码。哪些边该出现在响应里，当前是从 FK 推导的副作用，不是设计出来的。这比"Summary 类型含哪些字段"更根本。

**Q2 — 自引用树。** ~~未解决~~ → **已判定为「默认形态不支持」**，见 §7.3。两级类型在类型系统层面封死深度 1；更深的树需要每层一次往返。显式深度参数仍然可能，但不在本设计范围内，且该代价由测试断言而非文档一笔带过。

**Q3 — 生成物生命周期。** ~~未解决~~ → **已设计，见 §8**，实现转 #11。原文如下，其中「需要生成物 manifest」这一判断**已被 §8.0 否决**：

> 输出位置变更 + 零注解节点被直接跳过（`extension.go:74-77`）意味着删注解会遗留旧文件；生成中途失败还会留下「ent 已更新、dto 部分更新」的混合状态。需要**生成物 manifest**，而不是靠文件名猜测清理。`ent/dto` 的目录所有权与消费者既有同名包的冲突策略也未定义。

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
| 6 | Cursor: zero-limit panic, precision, two formats | **拆分**，后再改写 → 见 §9.1 | 零 limit panic 在 `base_service.tmpl` → 随基类删除**消解**。~~`cursor.go` 的 int64 精度丢失 + 双格式 → 存活~~ **「双格式」这一判断已被 §9.0 W1 否决**（spike 从未实现游标分页）。精度缺陷真实但零调用者；整组游标符号按 §9.1 排除出公开 API |
| 7 | Remove dead funcs / duplicated template | **存活并扩大** | 新增死代码一处：`dto.tmpl:116` 的 to-many 分支不可达（Q1） |
| 8 | Codegen fixture harness (generate + compile) | **存活并升级为前置门** | 要重写 DTO 生成就必须先有编译门。**唯一的硬瓶颈** |
| 9 | Guards not dependency-closed | **改写** | `EntToResponse` 移入 dto 后跨守卫引用消失；`ListResponse` 守卫问题随模板重写解决。验收条件保留 |
| 10 | Nillable/immutable/named-GoType 不编译 | **改写** | 目标从 `setFieldCallReq` 改为 `Apply` 生成；immutable 的处理依 §1.3 的 `MutableFields` 规则 |
| 11 | Format failure silent / stale artifacts | **存活并加重** | 输出位置迁移使陈旧产物从卫生问题变成迁移必要条件；~~需要 manifest（Q3）~~ **manifest 已被 §8.0 否决**，改为标记扫描 + 目录所有权，见 §8 |
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
  #13 MapError · #15 运行时拆包 · #11 产物清理（标记扫描，非 manifest）· #6 游标符号退出公开 API
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

**QUALITY-REVIEW P1-7（`EntToResponse` 无环检测）** ~~没有对应的 GitHub issue~~ → **已裁决并归属 #25**（2026-07-30）。Fable 当时指出递归只跟随已加载的边，而 ent 的预加载深度是调用方显式且有限的，因此"崩进程"的路径需要手工缝出环形指针——这个论证正确，但**条件依赖调用方行为**。

**§7.3 的两级响应类型给出了无条件的结论：`Summary` 类型不含边**，所以 `NewUserResponse` 调 `NewPostSummary`，而 `NewPostSummary` 不再调任何东西。**没有第二层可供环闭合**，深度由类型系统封死，不靠运行时计数器或 visited 集合。P1-7 因此是结构性消解，不是概率降低。

但这条同样是**设计断言，尚未验证**——本项目已有两次这类断言被推翻的记录。#25 带一条 fixture 验收：A、B 互持 response scope 的 FK 且都预加载，断言映射终止；以及断言生成的 `Summary` 类型确实不含边。

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

- **游标分页**：只跑了 offset。`cursor.go` 原封未动。
  （**后续修正：** 此处初稿写作「精度丢失与双格式（#6）原封未动」，其中「双格式」是错的——
  spike 根本没有第二套游标编码，见 §9.0 W1。精度缺陷仍属实，处置见 §9.1。）
- **模板本身**：一行都还没写。这是有意的——目标产物先被证明正确，模板才有东西可对照。

---

## 8. Q3 的答案：生成物生命周期

> **状态：设计，未实现。** §4 的 Q3 到此关闭；Q4 仍然开放（且随 T2 一起延后）。
>
> **本节推翻 Q3 自己提出的方案。** Q3 的原话是「需要**生成物 manifest**，而不是靠文件名猜测清理」。
> manifest 被否决，理由见 8.1。按本文惯例保留原判断而不静默删除。

Q3 是三个问题捆在一起：**删注解遗留旧文件**、**生成中途失败留下混合状态**、**`ent/dto` 目录所有权与消费者既有包冲突**。它们看起来都需要「记住上次生成了什么」，所以初判是 manifest。三个都不需要。

### 8.0 判据：不要 manifest

manifest 是**第二份真相**。它必须与磁盘保持同步，而它做不到：消费者 `git revert` 了生成目录、手工删了一个文件、在两个分支间切换——每一种都让 manifest 与实际产物分叉，而分叉的方向恰好是「manifest 说文件在，磁盘上没有」或反过来，两种都会让清理逻辑做错事。

产物文件里**已经**有一个不会分叉的标记，因为它和文件是同一个东西：

```go
// Code generated by entdomain extension from schema "User" (…). DO NOT EDIT.
```

`entdomain extension` 这个串足够特异——ent 自己的产物头不含它。**「上次生成了什么」是可以从磁盘读出来的事实，不是需要记住的状态。**

### 8.1 目录所有权：把长期问题变成一次性问题

§1.6 把产物从 `ent/` 移到 `ent/dto/`。这个决定当时的理由是依赖方向（handler 不再见到 ent 类型），但它顺带解决了 Q3 的两个问题——因为 **entdomain 独占 `ent/dto/`**，而它从来不独占 `ent/`。

生成流程因此变成：

1. 扫描 `ent/dto/*.go`
2. 读每个文件的头，按上述标记分成「我的」和「不是我的」
3. **存在「不是我的」→ 拒绝生成**，报错列出冲突文件（问题 3）
4. 渲染本次输出集合，写入
5. 「是我的」但不在本次输出集合里 → **删除**（问题 1）

第 3 步就是目录所有权的执行。消费者若已有 `ent/dto` 包，得到的是一条指名道姓的生成期错误，不是被悄悄覆盖的源码——这是 `ent/` 混杂布局下**做不到**的检查，因为那里满是不属于 entdomain 的合法文件。

第 5 步让「删掉一个实体的全部注解」有正确后果：该实体不再产出文件（`extension.go:74-77` 直接跳过它），旧文件因此不在输出集合里，被删除。今天它会永远留在磁盘上，继续编译，继续引用可能已经改名的 schema 字段。

**这一步不需要知道过去。** 输出集合是本次 schema 的纯函数，磁盘状态直接可读，差集当场算出来。

### 8.2 唯一需要「知道过去」的地方，且有到期日

`ent/` 下的 v1 残留（`{entity}_dto.go`、`_base_service.go`、`_base_handler.go`）**不能**用目录扫描清理——那个目录里绝大多数文件是 ent 的，误删就是灾难。但它们带着同一个标记。

所以迁移期加一段有明确寿命的代码：生成时额外扫描 `ent/*.go`（**只**这一层，不递归），删除带 entdomain 标记的文件，并打印一条一次性迁移说明。这段代码在下一个版本删除。

它是本设计里唯一「按标记删别人目录里的文件」的操作，所以它必须是有到期日的，而不是一个长期机制。

### 8.3 原子性：不需要事务，需要「失败即失败」

「ent 已更新、dto 部分更新」的混合状态，真实成因不是缺少事务，是 **`writeFile` 明知产物是坏的还报告成功**：

```go
formatted, err := imports.Process(path, content, nil)
if err != nil {
    log.Printf("WARNING: goimports formatting failed for %s: %v (writing unformatted)", path, err)
    formatted = content          // extension.go:168-172
}
```

goimports 失败几乎总是意味着模板产出了**语法上不合法的 Go 代码**。那是生成器的 bug，不是可以容忍的格式瑕疵。当前行为把生成器的 bug 变成消费者仓库里一个来路不明的编译错误。

- **改为返回 error。** 这是 QUALITY-REVIEW P1-12，也是 Q3「混合状态」的实际来源。
- **目录级原子性**：渲染进 `ent/.dto.tmp/`，全部成功后再 `RemoveAll` + `Rename`。同一文件系统上 rename 是原子的，所以 `ent/dto/` 要么是上一次的完整产物，要么是这一次的完整产物。

**剩下的窗口不消除，写进文档。** hook 在 `next.Generate(g)` 之后运行（`extension.go:66-70`），所以「ent 已重新生成、entdomain 硬失败」这个状态一定存在。它不需要事务，因为 **ent 的生成是幂等的**：修掉 schema 或模板重跑一次，两边都对。造一个假的跨阶段事务，代价远大于写清楚这一句。

### 8.4 由此新增的两件小事

- **`MkdirAll`。** 当前 writer 直接 `os.WriteFile`（`extension.go:173`），全仓无 `MkdirAll`。迁到子目录后**首次生成必然失败**。这是 §1.6 已经点过的「实现缺一步」，在此确认为 Q3 的一部分。
- **一个配置项 `OutputPackage`，默认 `"dto"`。** 8.1 第 3 步在冲突时拒绝生成，消费者需要一个逃生阀而不是一堵墙。

> 加这个开关与 §1.2「删掉两个配置开关」不矛盾，但值得说清楚：被删的两个开关**改变产物形态**（生成不生成基类），每一种取值都是一条要维护、要测试、要保证依赖闭合的代码路径——#9 正是死在这上面。`OutputPackage` 只改变一个目录名，产物形态不变，路径不分叉。**它是 Never break userspace 的成本，不是可配置性的成本。**

### 8.5 代价，以及尚未验证的部分

- **entdomain 会删除消费者磁盘上的文件。** 这是新行为。护栏是标记 + 目录所有权 + 拒绝生成，但护栏失效的后果是删掉用户的代码。**这一条必须在 #8 的 harness 里有专门的用例**，包括「目录里有一个手写文件」和「标记被手工改坏」两种。
- **未验证：** 标记扫描的实际实现、rename 在 Windows 上的行为（`os.Rename` 覆盖已存在目录在 Windows 上会失败，需要先 `RemoveAll`——这与 #4 的 Windows 路径问题是同一类，一起验）。本节全部是设计，一行代码都没写，判定应在 #8 建成后用真实 fixture 做。

---

## 9. 五项 owner 决策（已裁决）

> **状态：已拍板，未实现。** 本节关闭此前散落在 #6 / #10 / #13 / #15 / #17 上的
> `blocked:owner-decision`。
>
> **方法：** 跨模型家族二次意见，裁决规则在看到结果之前声明——两家一致即采纳；
> 单家有实质意见则由 orchestrator 亲自回源码核实；两家相反则停下问 owner。
> 实际到场两家：**fable（Anthropic）** 与 **agy（Gemini 3.1 Pro high）**；
> codex（OpenAI）与 grok（xAI）本轮**额度耗尽**，属真不可用，按顺位规则跳过。
> D 项两家相反，按预声明规则交 owner 拍板，owner 选 D2。

### 9.0 先记两条被推翻的事实

裁决过程中，**本文档与提交给评审的决策书各有一条承重断言被证伪**。按本文惯例保留而不静默修改。

**W1 —「`cursor.go` 与 spike 各有一套游标编码，双格式二选一」是错的。**
`git show origin/spike/layer2-fixture:query.go | grep "ursor"` → **零匹配**；
`git diff --stat main origin/spike/layer2-fixture -- cursor.go` → **空**，两分支该文件逐字节相同。
spike 的 `ListPage` 走的是 `Limit().Offset()`，压根没实现游标分页。
**真正的问题不是「两种游标编码选一个」，而是「offset 与 keyset 两套分页模型选一个」。**
§5 表中 #6 行、§7.5、§9.1 已按此改写。

**W2 —「ent 的错误类型住在框架包里，所以一个独立子包 import 它即可」是错的。**
`ent.NotFoundError` / `ent.ConstraintError` 由 `entc/gen/template/base.tmpl:142` 与 `:209`
**生成到每个消费者自己的 ent 包**；框架里的 `sqlgraph.NotFoundError` / `sqlgraph.ConstraintError`
（`dialect/sql/sqlgraph/graph.go:864` / `:53`）是**另一对类型**。
所以「独立子包只 import 框架」的方案**不可实现**，不是成本高。详见 9.3。

### 9.1 分页模型：只发 offset，keyset 推迟（原 #6）

**先记事实：`EncodeCursor` / `DecodeCursor` 有零个非测试调用者。** 整套 base64(json)
keyset 编解码器是死代码。模板里唯一真实的游标分页在 `base_service.tmpl:218-251`，
用的是裸 `entities[len(entities)-1].ID.String()`，而 #29 要删掉整个 base service。
`types.go` 的 `ListRequest` 同时声明 `Page`/`Size` 与 `Cursor`，注释承诺
「When Cursor is set, keyset pagination is used」——**没有任何代码实现这个分支**。

**裁决：#24 只发布 offset 分页**（spike 已验证的 `Page[R]{Data,Total,Page,Size}` 形状），
并把下列符号**排除在公开 API 之外**：

`Cursor` · `PageInfo` · `EncodeCursor` · `DecodeCursor` · `ListRequest.Cursor`

决定性失败场景：消费者按注释设了 `ListRequest.Cursor`，没有代码分支处理它，于是
**永远静默拿到 offset 第一页**——一个被冻进契约的错误结果。事后删这个字段是破坏性变更，
事后加回来是增量变更。**两者不对称，所以往小了发。**

`cursor.go` 的 64-bit 精度缺陷（`ID any` 过 JSON 变 `float64`，而
`normalizeJSONNumber` 的判据 `f == float64(int64(f))` 对已失真的值同样成立，无法检测）
**真实存在但当前伤不到任何人**——因为零调用者。它随这些符号一起离开公开 API，
真要做 keyset 时重新设计，不要复活。

**没有裁决的：** 深翻页的 O(n) 代价、大表上 `OFFSET 100000` 的已知风险、并发写入时跨页
丢行/重复。这些是 offset 分页的固有代价，**接受，并写进 #24 的文档**。等真实消费者提出
深翻页需求，再设计 keyset——那时它是加法。

### 9.2 注解与 schema 矛盾：生成期硬失败（原 #10）

**先记事实：`Immutable` 与 `MutableFields` 在本仓 `.go`/`.tmpl` 中零出现。**
这不是「处理得差」，是**完全没处理**。`updateFields()`（`funcs_fields.go:31-41`）
只看 `hasDomainScope(field, ScopeUpdate)`。而 ent 自己早就算好了：
`MutableFields()`（`entc/gen/type.go:553`）跳过 `f.Immutable`，并额外跳过 edge 为 immutable 的字段。

**裁决：检测到矛盾就让 `entc generate` 失败**，错误信息点名 schema、字段、以及互相矛盾的两个事实。

决定性失败场景：静默忽略（把 update 字段与 `MutableFields` 求交集）会让消费者显式写下的
`ScopeUpdate` 无声消失，字段不出现在 PATCH 请求里，**由 API 客户端在生产环境发现**——
这比今天那个编译错误严格更糟。「没有逃生阀」正是要点：修复动作是删掉一个注解。

**这是一条通用政策，不是一次修补：注解与 ent schema 矛盾 → 生成失败并同时报出两个事实。**
后续每个生成切片都按此办理。

### 9.3 错误映射：手写在 runtime，模板只生成一行接线（原 #13 / #15）

由 W2，方案空间比原设想窄。更关键的是一条不对称性：

| 生成的类型 | 形状（`base.tmpl`） | 可判定性 |
|---|---|---|
| `NotFoundError` | `struct{ label string }`，无 `Unwrap`，除 `Error()` 外无方法 | **只能靠消费者包里的 `ent.IsNotFound`**；框架侧无通道 |
| `ConstraintError` | `struct{ msg string; wrap error }` + `Unwrap()` | 可对 `*sqlgraph.ConstraintError` 用 `errors.As` |

**not-found 与 constraint 的可判定性根本不对称**，任何方案都必须分开回答这两类，不能一刀切。

**裁决：`MapError` 手写在 runtime，把判定函数作为参数收进来**，签名形如
`MapError(err error, isNotFound, isConstraint func(error) bool) error`；
模板只生成**一行接线**：

```go
entdomain.NewErrorMapper(ent.IsNotFound, ent.IsConstraintError)
```

已核实 `base.tmpl:152` 与 `:225` 确实生成了 `IsNotFound` 与 `IsConstraintError` 这两个函数，
所以这一行是可写的，且**模板里零逻辑**——满足「生成 schema 决定的，手写只是重复的」。

映射范围仍按 #13：not-found → `ErrNotFound`，constraint → `ErrAlreadyExists`，**其余不猜**。
约束错误不总是唯一性冲突；再往下猜，就会在真实原因是外键时告诉用户「已存在」。

由此 **#24「runtime 不 import 任何 ent 包」的验收标准得以保持**：runtime 收的是
两个 `func(error) bool`，它不认识 ent。

### 9.4 空注解面：删三个，留 `ScopeQuery`（原 #17）

| 字段 | 现状（本轮 grep 核实） | 处置 |
|---|---|---|
| `UniqueLookup` / `RangeLookup` | `uniqueLookupFields`/`rangeLookupFields` 注册在 `funcs.go:30-31`，但**无任何模板引用** | **删除**。与 #27 从 `$field.Ops` 导出的运算符表重复，是可推导之物的手写副本 |
| `DomainConfig.EntityName` | `annotations.go:139-140` + 测试，**零个非测试读者** | **删除**。无后继 |
| `ScopeQuery` | 唯一读者 `queryFields()`（`funcs_fields.go:64`）**本身未注册进 `templateFuncs()`**，模板零引用 → 当前无可达路径 | **保留** |

顺带记录一个更大的发现：`queryFields`、`searchableFields`、`sortableFields`
**三个函数都不在 `funcs.go` 的注册表里**，所以 `Searchable` / `Sortable` 同样没有可达消费者。
这扩大了 #7 与 #17 的范围，**不改变本决策**。

保留 `ScopeQuery` 的理由是一个成本事实而非偏好：它不只是被读，还被
`annotations.go:41`（`AllFieldScopes`）、`:197`（`OutputOnlyField`）、`:209`（`CreateOnlyField`）
**三处写入**。删它要改 preset，#27 再加回来就是改两次。而 #27（生成过滤结构与谓词）
就在同一个 arc 里，是它的真实消费者。

**两家评审在此相反**（一家主张三个全删，理由是 `Sensitive` 的先例应一视同仁），
按预声明的裁决规则交 owner 拍板，**owner 选择保留**。附加的硬约束：

- `ScopeQuery` 在 #27 落地前**不得进入任何 tagged release**
- **#27 的验收标准必须包含「`ScopeQuery` 有可达消费者」**

「声明了却静默忽略」的担心由 §9.5 消化：v0 阶段没有对外承诺。

### 9.5 兼容策略：v0，自由破坏，不设弃用窗口

`git tag` → **0**。从未发布过任何版本，唯一已知消费者是同一 owner 的 monorepo。

**裁决：走 Go 自己的约定，`v0.x` 不承诺任何东西。** 本 arc 的删除（`Sensitive`、基类与
handler #29、三个注解字段 #17、产物布局与输出目录 #11、9.1 的游标符号）**不设弃用窗口**，
做完后打 `v0.2.0`，配一份迁移说明。

决定性失败场景：保留 `// Deprecated:` 的 no-op 注解，等于把「注解声明了却什么都不做」这个
**正在被修复的缺陷**又留半年，而新代码会照着复制它。生成期诊断器同样是一段只服务于
owner 自己 monorepo、且必须择日删除的代码——写迁移说明更便宜。

**切 `v1` 的触发条件**（两条都满足才切，因为它们是事后改不动的契约）：

1. monorepo 消费者在新布局 / 新 API 上重新生成并全绿（#11 / #27 / #29 完成）
2. 分页模型定稿——即 9.1 的排除清单执行完毕，且若届时已做 keyset，其线格式已冻结

### 9.6 本节没有裁决的

- **谁来写迁移说明、写到哪里**（README 的 Known limitations？单独 MIGRATION.md？）——留给 #17 / #23 收口时定。
- **9.1 的排除清单是「从公开 API 移除」还是「整个文件删除」**：`cursor.go` 若整文件删除，
  `cursor_test.go` 一并删；若只是转为 internal，则要有 internal 包。**实现时定，#24 的第一步。**
- **9.2 的硬失败如何与 #8 的 harness 协作**：harness 需要一个「预期生成失败」的用例形态，
  这在 #8 当前设计里没有。
