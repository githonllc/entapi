# EntDomain — 设计与实现质量报告

> 评审对象：`/Users/simon/workspace/githon/entdomain` @ `7b7effe`
> 评审方式：三个独立评审源（Claude Opus 5 直读 / Fable 5 / GPT-5.6 Sol via Codex CLI），各自只以源码为准，互不共享结论；由主线程逐条裁决去重。
> 验证状态标记：**实测** = 跑代码证实；**读码** = 逐行读源码证实，未执行；**未决** = 证据不足，不采信任何一方。

---

## 一、总体判断

**核心抽象是对的，不需要推倒重来。** `scope 注解 → 生成 HTTP DTO`、`service 层不受 scope 限制直接操作 ent 实体` 这个分层模型健康，也是这个项目真正的价值所在。

问题集中在两个层面，且互为因果：

### 根因 A：唯一的产出物从未被编译过

3,186 行测试全部在断言「字符串拼接出来的片段长得对不对」，**没有任何测试渲染完整模板、更没有编译过一份生成文件**。后果是生成器只对一种很窄的 schema 形态正确（UUID 主键、无 `Nillable`、无 `Immutable`、必有 response 字段、无自定义 GoType），偏一点就吐出编不过的代码，而 CI 全绿。

下表 P0 段的 6 条编译失败，全部是这个根因的直接产物。

### 根因 B：注解合同「收下即忽略」

`DomainField` 上约有 10 个导出旋钮（`Sensitive` / `Searchable` / `Sortable` / `Filterable` / `UniqueLookup` / `RangeLookup` / `Metadata` / `DomainConfig.EntityName` / `ScopeQuery`）**被接受、被存储、然后被完全无视**。消费者无从察觉。

其中 `Sensitive` 的 godoc 明确承诺「should not appear in HTTP responses」而实际会出现——这已经不是「未实现的特性」，是**失效的安全承诺**（P0-1）。

> **接受参数却忽略它，比不提供这个参数糟糕得多。** 前者是无声的错误承诺，后者只是功能缺失。

### 一句话结论

设计良好、实现半成、验证缺失。修复路径清晰，工作量可控，**不需要重构，需要补一个编译门再把合同兑现**。

---

## 二、P0 — 安全 / 崩溃 / 生成物不可编译

| ID | 问题 | 证据 | 触发场景 | 来源 · 验证 |
|---|---|---|---|---|
| **P0-1** | **`AsSensitive()` 不阻止字段进入响应** —— 失效的安全承诺 | `annotations.go:107-109`（godoc 承诺不出现在响应）· `funcs_fields.go:44-53`（`responseFields` 只看 scope，从不读 `Sensitive`）· `dto.tmpl:108-114` | `field.String("password").Annotations(DefaultField().AsSensitive())` 照常生成 `Password string \`json:"password"\`` 并序列化出去 | Codex · **实测**<br>`responseFields -> 1 field(s)`，字段名 `password` |
| **P0-2** | **Windows 上 init 期 panic** —— 纯运行时导入也会崩 | `template_loader.go:18` 用 `filepath.Join` 拼 `embed.FS` 路径；`embed.FS` 只接受正斜杠 | Windows 服务只 import 了 `entdomain.ErrNotFound`，`mustLoadTemplate` 在 `main` 之前 panic。修法：`path.Join` | Codex · **读码**（darwin 无法复现） |
| **P0-3** | 无 `ScopeResponse` 字段的实体 → 引用未定义类型 | `dto.tmpl:102` 的 `{{- if $responseFields }}` 在 `:120` 就闭合，而 `XListResponse`（`:122-129`）在守卫**外面**；`base_handler.tmpl:25`、`base_service.tmpl:304` 也无条件引用 | 一个全部字段用 `InputOnlyField()` 的实体 | Fable + Codex · **读码** |
| **P0-4** | 只开 Handler 不开 Service → 引用未定义函数 | `extension.go:85-96` 两个开关彼此独立；`base_handler.tmpl:25` 调用的 `XEntToResponse` 定义在 `base_service.tmpl:304` | `WithBaseHandler(true)` + `WithBaseService(false)` —— 这是配置组合，不是罕见 schema | Codex · **读码** |
| **P0-5** | `Nillable` + create-required → 类型不匹配 | `dto.tmpl:43` 必填分支发值类型 `T`；`setFieldCallReq`（`funcs_codegen.go:159-164`）见 `Nillable` 就发 `SetNillableX(req.X)`，而它要 `*T` | `field.String("x").Optional().Nillable()` + `.WithRequired(ScopeCreate)` | Fable + Codex · **读码** |
| **P0-6** | `Immutable` + `ScopeUpdate` → 调用不存在的 setter | `ApplyXUpdateRequest`（`base_service.tmpl:285-295`）对 `UpdateOne` 发 `SetX`，而 ent 不为 immutable 字段生成 update setter | `field.String("x").Immutable().Annotations(DefaultField())` —— `DefaultField()` 默认含 `ScopeUpdate` | Fable + Codex · **读码** |
| **P0-7** | 自定义 GoType 的 Optional 字段 → `PtrOrNil` 约束失败 | `base_service.tmpl:314-319` 用 `isComplexFieldType` 分流，而它只认字面量 `[]` / `map[` / `json.` 前缀（`funcs_typechecks.go:75-79`）；`PtrOrNil` 要求 `comparable`（`types.go:60-67`） | `type Tags []string` 作为 GoType，`Type.String()` = `schema.Tags`，不匹配任何前缀 → 走 `PtrOrNil` → 不满足 `comparable`，编译失败 | Codex（表述经我收紧）· **读码** |
| **P0-8** | `ListWithCursor(limit=0)` 下标越界 panic，且 `Validate` 放行 `Size=0` | `base_service.tmpl:240-249`：`Limit(1)` 取回 1 行 → `1 > 0` 成立 → `entities[:0]` → `entities[len-1]` = `entities[-1]`。`types.go:24-44` 的 `Validate()` 允许 `Size=0`，`SetDefaults()` 是分离的可选调用 | handler 把未校验的 `?size=0` 透传，非空表直接崩进程 | Fable(P1) + Codex(P0) · **读码**<br>我裁 **P0**：崩的是进程 |

---

## 三、P1 — 行为错误与契约破坏

| ID | 问题 | 证据 | 后果 | 来源 · 验证 |
|---|---|---|---|---|
| **P1-1** | 软删除只写不读 | `hasSoftDelete` 把 Delete 改写成 `SetDeletedAt`（`base_service.tmpl:182-186`），但 `GetByID`（`:124`）和 `ListWithCursor`（`:220`）从不过滤 `deleted_at IS NULL` | 删掉的行照样出现在列表和详情里。**半个特性比没有更糟** | 3/3 · 读码 |
| **P1-2** | `DeleteBatch` 绕过全部 hooks | `base_service.tmpl:197-216`，模板注释自己承认 | 写在 `BeforeDelete` 里的租户鉴权可被批量接口绕过 | 3/3 · 读码 |
| **P1-3** | `GetByID` 不映射 `ErrNotFound` | `base_service.tmpl:124` 裸返回 `s.DB.X.Get(...)`；而 `Update`（`:163`）映射 | 同一个不存在的 UUID，走两条路 `entdomain.IsNotFound(err)` 结果相反 | Codex · 读码 |
| **P1-4** | Service 从不调用生成的 `Validate()` | `base_service.tmpl:130-147` 的 Create 只走 `BeforeCreate → builder → Save` | 生成了校验器却指望调用方记得调；漏调无任何提示 | Codex · 读码 |
| **P1-5** | 非必填非 Optional 字段无条件 `SetX` → **覆盖 ent 的 `Default()`** | `base_service.tmpl:274-276` 的 else 分支 | schema 写了 `Default("pending")`，客户端不传 status，实际写进 `""` | Codex · 读码 |
| **P1-6** | UpdateRequest 无法表达 `null` | 全部字段是裸指针（`dto.tmpl:75`），JSON `null` 与「字段未提供」都变成 `nil` | 无法生成 `ClearX()`，可选字段一旦设值就再也清不掉 | Codex · 读码 |
| **P1-7** | `EntToResponse` 对响应边递归，无环检测 | `base_service.tmpl:325-338` 与 `responseEdges`（`funcs_fields.go:110-134`） | A、B 互持带 Response scope 的 FK 且都预加载 → 无限递归栈溢出 | Fable(P2) + Codex(P1) · 读码<br>我裁 **P1**：崩进程 |
| **P1-8** | 值接收器 builder 只是浅拷贝 → 别名污染 + 全局污染 | `annotations.go:41`（`AllFieldScopes` 是包级 slice）· `:169-172`（`DefaultField` 直接引用它）· `:238-245`（`WithRequired` 只在 nil 时建 map） | 见右侧实测 | Codex · **实测**<br>分叉链：`a.Required` 与 `b.Required` 均为 `map[create:true response:true update:true]`<br>全局：`AllFieldScopes[0]: create -> query` |
| **P1-9** | 游标大整数静默丢精度 | `cursor.go:47-75`，`json.Unmarshal` 到 `any` 先成 `float64` | 分页位置漂移，下一页重复或跳记录 | Codex · **实测**<br>`in=9007199254740993 out=9007199254740992` |
| **P1-10** | `IsConstraintError` 一律映射成 `ErrAlreadyExists` | `base_service.tmpl:140-142` | 外键违规、check 违规被误报为「已存在」 | Fable · 读码 |
| **P1-11** | codegen 与 runtime 共包 | 生成代码 `import "github.com/githonllc/entdomain"`，而该包内含 `extension.go`，依赖 `entc/gen` + `x/tools/imports`，且 init 时 embed 并加载 664 行模板 | 每个消费者的生产二进制都链进整套代码生成机器。修法：`errors.go`/`types.go`/`cursor.go` 拆进 `entdomain/runtime`，`entdomainPkg` 默认指它 | Fable + Codex · 读码 |
| **P1-12** | 格式化失败仍写盘 | `extension.go:167-177`：`imports.Process` 出错只 log 一条 WARNING，然后写未格式化源码并返回 nil | 「生成成功」但源码已坏 | 3/3 · 读码 |
| **P1-13** | 生成物的可编译性实质依赖 goimports 兜底 | `dto.tmpl:9-12` 无条件 import `fmt`（无 create/update 字段时未使用）；enum 字段发 `user.Status` 却不声明该 import | 与 P1-12 叠加：兜底失败 = 把编译错误静默写进用户仓库 | 我 + Fable · 读码 |
| **P1-14** | 陈旧产物不清理 | `extension.go:75` 对无注解节点直接 `continue` | 把实体的 DomainField 全删掉后重新生成，旧的 `user_*.go` 留在原地继续编译 | Codex · 读码 |
| **P1-15** | `ScopeQuery` 是一份没有实现的合同 | `annotations.go:26-30` 承诺「字段会出现在 QueryParams struct 里」；`queryFields`/`searchableFields`/`sortableFields`（`funcs_fields.go:58,73,85`）既未注册进 `templateFuncs()` 也无人调用；不存在 QueryParams 模板 | `.AsSearchable()` 是彻底的空操作 | 我 + Fable · **实测**（grep 全仓确认双向死） |
| **P1-16** | `SetSelf` 的失败模式全是静默的 | `base_service.tmpl:72-82` | 忘调 `SetSelf` → hook 编译通过但永不触发；`BeforeCreat` 拼错 → 不覆盖任何东西也不报错。替代方案见 §五 | Fable · 读码 |

---

## 四、P2 — 可维护性、API 卫生、潜伏风险

| ID | 问题 | 证据 | 来源 |
|---|---|---|---|
| **P2-1** | **测试基线是红的**：`TestTemplateFuncs` 断言 `specificMethods`、`setFieldCall` 两个已不存在的函数 | `funcs_test.go:7` | 我 · **实测** |
| **P2-2** | **`make lint` 过不了**：`gofmt -l .` 报 `funcs.go`、`funcs_codegen.go`、`annotations_test.go`、`types_test.go` | `.golangci.yml` 启用了 gofmt/goimports | 我 · **实测** |
| **P2-3** | `templates/model.tmpl` 是 `dto.tmpl` 的死复制（除第 4 行逐字节相同，从未被加载） | `template_index.go` 不引用它；`SKILL.md:266` 还把它当作 DTO 模板 | 我 · 实测 |
| **P2-4** | 8 个注册却无模板引用的函数 + 3 个双向死的选择器 | `generateIdOperation`/`generateSearchCondition`/`searchMethod`/`findByMethod`/`isUniqueField`/`isUUIDType`/`uniqueLookupFields`/`rangeLookupFields`；`queryFields`/`searchableFields`/`sortableFields` | 我 · 实测 |
| **P2-5** | 休眠的字符串生成器若被重新接入，会把解析失败静默变成错误查询 | `funcs_codegen.go:35-41`（int64 解析失败 → 查 ID=0）、`:203-213`（int32 收到 `int64(4294967295)` 强转成 `-1`） | Codex · 读码 |
| **P2-6** | `BaseService` 只支持 UUID 主键，而 `funcs_codegen.go` 还留着整套多主键类型逻辑没人调 | `base_service.tmpl` 全部签名硬编码 `uuid.UUID` | 我 · 读码 |
| **P2-7** | 生成代码里裸调 `IsNotFound` 解析到 **ent 的**同名函数，不是 `entdomain.IsNotFound` | `base_service.tmpl:163`；今天正确纯粹因为文件落在 `package ent` | 我 · 读码 |
| **P2-8** | `gen.Funcs` 先合并、后者覆盖，同名冲突会静默盖掉 ent 内置函数 | `extension.go:181-188` | 我 · 读码 |
| **P2-9** | `DomainConfig.EntityName` 是失效的公共 API：注解了也不改文件名和类型名 | `annotations.go:135-145` vs `extension.go:118,139,160` | 我 + Codex · 读码 |
| **P2-10** | `FieldMetadata` 全套（约 12 个 `With*` 方法）声明即死；`annotations.go:44` 自称 "RESERVED" | grep 全仓（排除测试）只在 `annotations.go` 内出现 | 3/3 · 实测 |
| **P2-11** | **同包内并存两套不兼容的游标格式**：`EncodeCursor`/`DecodeCursor` 是 base64-JSON 且无人使用，`ListWithCursor` 用的是裸 `uuid.Parse` | `cursor.go` vs `base_service.tmpl:223,248`；`ListRequest.Cursor` 的注释描述的是没人用的那套 | 我 · 实测 |
| **P2-12** | `ListRequest` 从不被任何模板引用，消费者得自己接分页入参 | grep 全仓模板 | 我 · 实测 |
| **P2-13** | `funcs.go:9` 注释宣称「只注册模板实际调用的函数」，与 P2-4 直接矛盾 | — | 我 · 实测 |
| **P2-14** | 测试策略投错了地方：3,186 行表驱动测试压在化石层（`searchMethod`/`findByMethod`/`generateIdOperation` 都无人调用），端到端为零 | — | 我 + Fable |
| **P2-15** | `funcs_test.go:188` 的 `convertMapToDomainField` 是 `getDomainFieldAnnotation` map 分支的测试内重复实现 | — | 我 · 实测 |
| **未决** | `responseEdges` 的非 unique 分支是否可达 | Fable 断言不可达（ent 的 edge-field 只存在于 unique 侧）；Codex 去读了 ent 源码的 `Edge.Field()` 但未给结论 | **不采信任何一方**，用到时再查 |

---

## 五、设计层面的改进意见

### 5.1 `SetSelf` → hooks 结构体

现状是手写的虚函数分发：基类实现自己的 hook 接口（全 no-op），`hooks()` 在 `s.self` 为 nil 时回退到自身。**它的失败模式全是静默的**（P1-16）。

建议换成可空函数字段，构造时赋值：

```go
type UserHooks struct {
    BeforeCreate func(ctx context.Context, req *UserCreateRequest) error
    AfterCreate  func(ctx context.Context, e *User) (*User, error)
    // ...
}
```

- 方法名拼错 → **编译错误**，而不是静默不触发
- 不需要记 `SetSelf` 这个仪式
- 部分实现天然支持（nil 即跳过）

代价：breaking change。攒进一次 minor 发布。保留接口方案的唯一理由是需要 `self` 递归，而当前没有任何 CRUD 方法通过 `hooks()` 互相调用——不需要。

### 5.2 让模板依赖闭包

当前模板的条件生成是「各管各的守卫」，导致 P0-3 / P0-4。三处改动即可闭合：

1. `XListResponse` 移进 `$responseFields` 守卫内（或让 Response 恒含 ID 而始终生成）
2. `XEntToResponse` 从 `base_service.tmpl` 下沉到 `dto.tmpl` —— 它本来就是 DTO 的一部分，这样 Handler 不再隐式依赖 Service 开关
3. 不支持的字段组合（`Immutable`+`ScopeUpdate`、`Nillable`+required）在**生成期显式报错**，而不是产出坏代码

> 生成器的第一原则：**宁可拒绝生成，也不要生成编不过的代码。** 前者报错清晰，后者把问题推给用户的编译器，且被 P1-12 的静默兜底掩盖。

### 5.3 统一 presence 模型

P1-4/5/6 是同一件事的三个切面：DTO 层没有区分「未提供 / 显式 null / 有值」。建议 Create 侧对未提供的非必填字段**不发 setter**（保留 ent 的 `Default()`），Update 侧引入可区分 null 的包装类型以支撑 `ClearX()`。

### 5.4 拆包

`errors.go` / `types.go` / `cursor.go` → `entdomain/runtime`，`entdomainPkg` 默认值改指它，根包保留兼容 facade。这一步同时解决 P1-11 和 P0-2 的爆炸半径。

---

## 六、修复路线图

分阶段，每阶段可独立发布。**排序依据是「阻塞关系 + 用户可见伤害」，不是难度。**

### 阶段 0 — 止血（数小时，无依赖）

| 动作 | 对应 | 理由 |
|---|---|---|
| `responseFields` 排除 `Sensitive` 字段 | P0-1 | 已发布模块的数据泄露，godoc 已明确承诺 |
| `filepath.Join` → `path.Join` | P0-2 | 一个字的改动，避免 Windows 用户在 `main` 之前崩溃 |
| 修 `TestTemplateFuncs`、跑 `make fmt` | P2-1, P2-2 | `make check` 现在就是红的，后续任何改动都无基线可比 |

这三项加起来不到 10 行，且**不依赖阶段 1**。

### 阶段 1 — 建编译门（这是所有后续工作的前提）

建 `testdata/` fixture 模块，放一组刻意刁钻的 schema，测试里跑 `entc.Generate` 生成到临时目录，然后 **`go build` 它**。

必须覆盖的形态：

- 全 `InputOnlyField()` 的实体（→ P0-3）
- `WithBaseHandler(true)` + `WithBaseService(false)`（→ P0-4）
- `Optional().Nillable()` + create-required（→ P0-5）
- `Immutable()` + `DefaultField()`（→ P0-6）
- 自定义 GoType（`type Tags []string`）的 Optional 字段（→ P0-7）
- enum 字段、JSON/map 字段（→ P1-13）
- 双向边（→ P1-7）
- 非 UUID 主键（→ P2-6，先确认是拒绝生成还是支持）

**一个测试抓住全部 8 条 P0。** 在这之前修的任何一处都无法验证——你只是在用眼睛当编译器。

### 阶段 2 — 修 P0

阶段 1 会把它们全部变红。逐条修绿。同时把 `writeFile` 改成硬失败（P1-12）并清理陈旧产物（P1-14）——否则阶段 1 的门会被静默兜底绕过。

### 阶段 3 — 行为正确性

P1-1（软删读路径）、P1-2（批量走 hooks）、P1-3（统一 sentinel 映射）、P1-8（builder copy-on-write + `AllFieldScopes` 改成返回不可变副本的函数）、P1-9（游标改 `json.Number` 或显式类型编码）、P1-10（区分约束类型）。

P1-8 和 P1-9 已有实测复现用例，可直接转成回归测试。

### 阶段 4 — 结构与 API 收口（breaking，攒一个 minor）

拆 runtime 子包（P1-11）· hooks 结构体替换 `SetSelf`（P1-16）· 统一 presence 模型（P1-4/5/6）· 删除全部死代码（P2-3/4/5/11/15）· 对「收下即忽略」的旋钮做决断（P1-15, P2-9, P2-10）。

**决断规则**：每个旋钮只有两条路——实现它，或者在 godoc 标 deprecated 并在下个 minor 删除。**保持现状（接受参数但忽略）是三个选项里最坏的一个**，因为消费者无法察觉。

---

## 七、评审方法说明

三个评审源独立进行，互不可见对方结论：

| 源 | 工具面 | 独有贡献 |
|---|---|---|
| Claude Opus 5（主线程） | 全量读源码 + 执行 | P2-1/2/3/4/6/7/8/11/12/13/15，以及根因 A 的定性 |
| Fable 5 | Read/Grep/Glob | P0-3/5/6、P1-10/15/16、`SetSelf` 替代设计、测试策略批评 |
| GPT-5.6 Sol（Codex CLI） | 全量读源码 + 执行，另读了 ent 自身源码 | P0-1/2/4/7、P1-3/4/5/6/8/9/14，以及根因 B 的定性 |

**裁决规则**：
1. 两方以上命中 → 直接采信
2. 单方命中 → 主线程独立复核（实测优先，否则逐行读码）才入册
3. 双方冲突 → 单列为「未决」，不采信任何一方

实际结果：**零硬冲突**，仅一处严重度分歧（P1-7，Fable 判 P2 / Codex 判 P1，我裁 P1 因为后果是进程崩溃）。

四条「实测」结论由一份临时测试文件产出，证毕即删，工作区未留残留。
