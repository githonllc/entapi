# 竞对对比：entapi vs lrstanley/entrest

> **Provenance：独立上下文评审（Opus，2026-08-08）**。entapi 侧读
> `DESIGN-v3-final.md` + `CLAUDE.md`；entrest 侧全部来自一手源（GitHub API、
> `config.go`/`constants.go`/`annotations.go`/`schema_filtering.go`/
> `templates/*.tmpl`、docs 站 mdx 原文、issue #57），查不到的标「未查明」，
> "无软删除"的缺席断言带阳性对照。偷师清单的采纳记录见
> `DESIGN-v3-final.md` §5 backlog。

## 1. 注解/配置模型

- **entapi**：实体级 **opt-in**——不写 `api.Resource()` 零产出；字段默认**沉默**，HTTP 形状由 ent 事实推导；偏离词恰五个 + 边的 `Expand()`；无 preset；矛盾组合在生成期**失败并报出两个事实**。
- **entrest**：**opt-out**——注解参考原文「If empty, all operations are generated (unless globally disabled)」，schema 不写任何注解即得完整 CRUD，`WithSkip` 才排除。28 个 `With*` 注解 + 约 24 个全局 config 旋钮。Filter/Sortable/EagerLoad 逐字段 opt-in，字段本身的暴露是 opt-out。
- **判定**：安全默认上 entapi 占优（漏写一个 `WithSkip` 在 entrest 就是一次泄漏，且无等价矛盾拒绝机制）；起步速度与旋钮覆盖面 entrest 占优。

## 2. 生成代码所有权与逃生舱

- **entapi**：marker 首行即所有权契约，删掉即永久接管。handler 恒为三段式 handler、无 override 点，逃生舱是**类型化定制点**（签名逐字相同、编译器校验）。非 CRUD 端点走自定义端点（external），框架永不生成 Service 骨架。
- **entrest**：生成进 `rest` 包；`ServerConfig` 只暴露 `MaskErrors`/`ErrorHandler`/`GetReqID`。未见文档化的单操作覆写机制；可用 `WithHandler(false)` 卸掉路由自己包一层再挂。业务逻辑与授权的官方指引是 ent privacy 层（issue #57）。handler 配置与 auth 两页文档正文都是 `TODO`。
- **判定**：设计上 entapi 明显占优。但 entrest 的 ent-privacy 路线在行级授权上更成熟——entapi v3 把 OwnedBy 划出界只立边界。

## 3. HTTP 层与路由器耦合

- **entapi**：Go 1.22+ stdlib `ServeMux`，`ent.API(client)` 返回 `http.Handler`，另有 `Routes()` 数据清单（§2.5）。零第三方 router 依赖。
- **entrest**：`Handler` 三值 `none/stdlib/chi`（chi ≥ v5.0.12）；`HandlerNone` 可只要 spec。
- **判定**：接近平手，entrest 略优（双后端 + 可关 handler）。

## 4. 查询面

- **entapi**：op-in-value；`_` 前缀保留命名空间恰四参数；`_sort` 多字段 + PK tiebreak；算子集生成期算死为 parse switch；门控违规与非法排序一律 400。
- **entrest**：每算子一参数（`CamelCase(field).predicate`，边形式 `Pet.Name.eq`）；`WithFilterGroup` 字段组 OR；排序**单字段** `sort`+`order` 白名单校验；分页 `page`/`per_page` 默认 10 上限 100，信封带 `last_page`/`is_last_page`。
- **判定**：entapi 赢在多字段排序+PK tiebreak（entrest 单字段排序非唯一列翻页会漏/重）、命名空间隔离、`_q`；entrest 赢在 OpenAPI 类型精度、AND/OR 过滤组、**跨边过滤**（entapi 没有）。

## 5. OpenAPI

- **entapi**：3.1；spec 落盘 commit、marker 所有权、embed 服务同源。设计态零实现。
- **entrest**：3.0.3；`Spec`/`SpecFromPath` 合并、`WithExample`/`WithSchema`/`WithOperationID`/`WithTags`、全局 headers/错误响应、Scalar UI 在 `/docs`、`WithTesting` 生成 `resttest` 包。
- **判定**：entrest 大幅领先；entapi 唯一优势是 3.1 与"spec 进仓库、diff 即审查"工作流。

## 6. 错误处理

- **entapi**：RFC 9457 problem+json 带 `field`；状态码表明确；`DisallowUnknownFields` 默认开；唯一键分类接口探测 + 方言装配，未知方言宁 500 不错答 409；残余如实入档。
- **entrest**：自定义 `ErrorResponse{error, type, code, request_id, timestamp}`；`MaskErrors`、可注入 `ErrorHandler`；`StrictMutate` 默认**关**；ent 错误→状态码映射未查明。
- **判定**：格式与分类严谨度 entapi 占优；entrest 占优在错误注入点与 request_id 开箱。

## 7. 软删除 / hooks

- **entapi**：已实现且有行为证据（`internal/softdeleteproof`，真 SQLite、调用链无生成代码）；v3 计划生成 init 注册（spike 门控）。
- **entrest**：无软删除支持（已核实缺席，带阳性对照）；不生成任何 ent hook。
- **判定**：entapi 占优，且是它今天就有行为证据的部分；但软删除本质在 ent 层，entrest 用户可自配第三方 mixin。

## 8. 成熟度与生态

- **entrest**：41 star / 8 fork / 47 open issue，2024-06 建，2026-08 仍活跃（单一维护者）；文档自述 WIP、expect breaking changes、三页正文 TODO；有两个可跑示例。
- **entapi**：HTTP 层/OpenAPI/新注解模型零行代码；已实现的中间层工程质量扎实（fixture 进仓编译、嵌套模块行为证明、死代码即测试失败、带阳性对照的隔离探针）；外部用户为零。
- **判定**：entrest 压倒性领先。

## 判决

**entapi 设计上最强的三个优势**：① 默认安全 + 生成期拒绝矩阵（架构级差异，补不进 entrest）；② 类型化定制点 + marker 所有权（编译期契约）；③ 查询面命名空间纪律（`_` 前缀、多字段排序 PK tiebreak、400 不静默）。

**entapi 相对 entrest 最弱的三点**：① 还不存在（HTTP 层零行，且 v3 迁移零共存期）；② OpenAPI 与开发者周边几乎空白，op-in-value 的 spec 类型精度天然低；③ 覆盖面缺口（跨边过滤、边端点、行级授权出界、软删除 HTTP 语义 defer）+ 生态为零。

**值得偷师**（采纳状态见 DESIGN-v3-final §5）：边端点 `/users/{id}/pets`；spec 合并 + example/schema 覆写；`WithTesting` 生成测试包；内置 docs UI；`WithFilterGroup`；`ErrorHandler` 注入点（第 3 步、观测/替换两档）；`WithHandler(false)` 生成不挂载第三档逃生舱。

**总评**：今天选 entrest——不是设计更好，而是它存在；代价是默认全暴露要逐 schema 审、3.0.3、非标错误格式、单人维护 WIP。六个月后（entapi 实现完成）：仓库所有者/长期投入方选 entapi（四件架构级优势补不进 entrest）；外部团队仍选 entrest 并把偷师清单反向提 PR。改判信号：entapi 出现首个非作者生产用户、OpenAPI 精修面补齐。
