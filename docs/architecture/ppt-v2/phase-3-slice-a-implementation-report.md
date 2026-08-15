• # PPT Generation V2 Phase 3 Slice A 实施报告



&#x20; - 日期：2026-08-16

&#x20; - 工作区：E:\\code\\work\\ppt-v2

&#x20; - 分支：codex/ppt-v2

&#x20; - Phase 2 基线：8deed51b1 docs(ppt-v2): record phase 2 readiness

&#x20; - Slice A 主实现：bbfa3627d feat(ppt-v2): add agent outline approval slice

&#x20; - 最终修复：231138dd4 fix(ppt-v2): make outline approval replay idempotent

&#x20; - Slice A 状态：READY



&#x20; 本报告记录 Phase 3 Slice A 的最终实现状态。



&#x20; Phase 1 和 Phase 2 实施报告继续作为历史阶段快照保留，不删除、不覆盖。Slice A 建立在 Phase 1 的 PPT V2 内容内核与 Phase 2 的 Durable GenerationJob 基

&#x20; 础上，最终停止在已批准的 OutlinePlan。



&#x20; Slice A 不生成最终 SlideIR，不执行 Layout/PPTX 渲染，不实现 Preview，也不进入 Slice B、C 或 D。



&#x20; ## 1. 范围与应用边界



&#x20; Slice A 实现以下完整链路：



&#x20; 用户请求 -> IntentSpec -> ResearchPack -> Storyline -> Dynamic OutlinePlan -> 用户审阅与修改 -> Approved OutlinePlan



&#x20; 本阶段继续扩展现有 PPT V2 Application Boundary 和 /api/v1/ppt API family。



&#x20; 没有创建：



&#x20; - /api/v2/ppt；

&#x20; - 第二套 PPT Agent Session Backend；

&#x20; - 第三套 PPT API；

&#x20; - 第三套 PPT Document Model；

&#x20; - 平行的 HTML/Screenshot PPT Pipeline。



&#x20; Slice A 使用 Phase 2 GenerationJob 聚合中的 AGENT\_OUTLINE workflow。



&#x20; 与现有 RENDER workflow 不同，Slice A 的规划任务不包含：



&#x20; - Existing PPT Task；

&#x20; - DeckJob；

&#x20; - SlideJob；

&#x20; - 最终 SlideIR；

&#x20; - LayoutResult；

&#x20; - PPTX Render Bytes；

&#x20; - Private File；

&#x20; - Work Center Asset。



&#x20; 这些内容均不属于 Slice A。



&#x20; ## 2. IntentSpec



&#x20; IntentSpec 是可持久化的独立领域契约，包含：



&#x20; - topic：主题；

&#x20; - goal：目标；

&#x20; - audience：受众；

&#x20; - scenario：使用场景；

&#x20; - language：语言；

&#x20; - pageCount：页数范围及可选首选页数；

&#x20; - professionalStyle：专业风格；

&#x20; - researchRequired：是否需要 Research。



&#x20; Intent 解析遵循以下原则：



&#x20; - 用户自然语言中明确指定的页数优先于表单默认值；

&#x20; - 支持单一页数，例如“做一份 10 页的行业分析”；

&#x20; - 支持页数范围，例如“做一份 8～10 页的趋势分析”；

&#x20; - 没有明确页数时，不强制写死为 8～10 页；

&#x20; - 信息足够时直接继续，不为了制造 Agent 感强制追问；

&#x20; - 只有主题真正缺失或无法识别时才提出必要澄清；

&#x20; - 明确页数超出 Professional Deck 支持范围时要求用户调整。



&#x20; IntentSpec 是后续 Research、Storyline 和 OutlinePlan 的输入权威。前端 Session 或本地缓存不构成 Intent 权威状态。



&#x20; Guide 的幂等身份包含 Intent Request 内容。同一个幂等键如果被用于不同的请求，会返回幂等冲突，而不是复用错误的规划结果。



&#x20; 测试覆盖：



&#x20; - 明确单一页数；

&#x20; - 明确页数范围；

&#x20; - 未指定页数；

&#x20; - 自然语言页数优先于表单默认值；

&#x20; - 信息充分时不强制澄清；

&#x20; - 真正缺少主题时提出澄清。



&#x20; ## 3. ResearchPack 与事实来源追踪



&#x20; Slice A 建立了独立的 ResearchProvider Port。



&#x20; 具体搜索供应商不会耦合进：



&#x20; - GenerationJob；

&#x20; - Storyline；

&#x20; - OutlinePlan；

&#x20; - Outline Approval；

&#x20; - PPT Renderer。



&#x20; 第一版 Research Adapter 使用 Wikipedia Search，并通过有超时限制的 HTTP Client 获取资料。



&#x20; Research 结果不会被拼接成一大段无结构文本，而是进入结构化 ResearchPack。



&#x20; ResearchPack 包含：



&#x20; ### ResearchSource



&#x20; 记录：



&#x20; - 稳定 Source ID；

&#x20; - Provider；

&#x20; - Provider Identity；

&#x20; - 标题；

&#x20; - Source Type；

&#x20; - Source Locator；

&#x20; - RetrievedAt。



&#x20; Source ID 由 Provider 和 Provider Identity 稳定生成。同一来源在不同请求处理阶段不会因为标题或展示顺序变化而获得新的随机 ID。



&#x20; ### ResearchCitation



&#x20; 记录：



&#x20; - Citation ID；

&#x20; - Source ID；

&#x20; - Citation Locator；

&#x20; - RetrievedAt。



&#x20; Citation 必须指向 ResearchPack 中真实存在的 Source。



&#x20; ### ResearchClaim



&#x20; 记录：



&#x20; - Claim ID；

&#x20; - Source ID；

&#x20; - Citation Refs；

&#x20; - Claim Text；

&#x20; - Verification Status。



&#x20; Claim 不允许脱离 Source 存在，也不允许没有 Citation Ref。



&#x20; ### ResearchDataset



&#x20; Contract 支持：



&#x20; - Dataset ID；

&#x20; - Source ID；

&#x20; - 标题；

&#x20; - Locator；

&#x20; - Citation Refs。



&#x20; 当前 Wikipedia Adapter 暂不产生 Dataset，但 ResearchPack 已经为后续真实数据源保留了结构化能力。



&#x20; ### Verification Status



&#x20; ResearchPack 和 Claim 都保存 Verification Status，用于区分：



&#x20; - UNVERIFIED；

&#x20; - SOURCE\_SUPPORTED；

&#x20; - MIXED。



&#x20; Slice A 建立的事实追踪链为：



&#x20; Source -> Citation -> Claim -> evidenceRefs -> SlideObjective



&#x20; 这使后续 Slice B 可以将 evidenceRefs 映射到最终 SlideIR 或 CitationElement，而不需要重新构造事实来源关系。



&#x20; ResearchPack 校验会拒绝：



&#x20; - 不稳定或错误的 Source ID；

&#x20; - 重复 Source；

&#x20; - Claim 指向不存在的 Source；

&#x20; - Claim 缺少 Citation；

&#x20; - Citation 与 Claim 指向不同 Source；

&#x20; - Dataset 指向不存在的 Source 或 Citation；

&#x20; - 重复 Claim、Citation 或 Dataset ID。



&#x20; ## 4. Storyline



&#x20; Storyline 是独立领域对象，不等同于 OutlinePlan。



&#x20; Storyline 保存：



&#x20; - thesis；

&#x20; - audienceTakeaway；

&#x20; - narrativeArc；

&#x20; - sections；

&#x20; - closingAction。



&#x20; Storyline 回答的是：



&#x20; - 整套 PPT 要证明什么；

&#x20; - 受众最终应该获得什么判断；

&#x20; - 论证应该按照什么顺序展开；

&#x20; - 最后一部分要推动什么行动。



&#x20; OutlinePlan 回答的是：



&#x20; - 具体有多少页；

&#x20; - 每一页承担什么目标；

&#x20; - 页面顺序是什么；

&#x20; - 每一页需要哪些事实证据与视觉表达。



&#x20; Storyline 中不存在：



&#x20; - SlideIR；

&#x20; - 页面 Geometry；

&#x20; - LayoutResult；

&#x20; - Renderer 数据；

&#x20; - PPTX 输出结构。



&#x20; 测试验证：



&#x20; - Thesis 存在；

&#x20; - Audience Takeaway 存在；

&#x20; - Narrative Arc 存在且有序；

&#x20; - Sections 存在且顺序明确；

&#x20; - Closing Action 存在；

&#x20; - Storyline 与 OutlinePlan 是两个独立对象。



&#x20; ## 5. Dynamic OutlinePlan 与动态页数



&#x20; Slice A 将 Professional Deck 的 OutlinePlan 页数范围定义为：



&#x20; 6～12 页



&#x20; 该范围替代 Phase 1 固定两页的规划限制，但不会修改 Phase 1 历史报告中的阶段事实。



&#x20; 页数决策规则：



&#x20; - 用户明确指定合法页数时优先尊重；

&#x20; - 用户指定范围时保留范围，并在范围内确定首选页数；

&#x20; - 用户未指定页数时，根据 Storyline Sections 和内容密度决定；

&#x20; - 不把 8～10 页写死为业务规则；

&#x20; - 不允许少于 6 页或多于 12 页。



&#x20; 每一个 SlideObjective 包含：



&#x20; - slideId；

&#x20; - title；

&#x20; - purpose；

&#x20; - keyMessage；

&#x20; - evidenceRefs；

&#x20; - visualIntent；

&#x20; - expectedElementTypes。



&#x20; OutlinePlan 还包含：



&#x20; - 稳定 Outline ID；

&#x20; - 当前 Revision；

&#x20; - Topic；

&#x20; - Page Count；

&#x20; - Next Slide Sequence；

&#x20; - 有序 SlideObjective 列表；

&#x20; - CreatedAt；

&#x20; - 可选 ApprovedAt。



&#x20; Slide ID 通过稳定 Sequence 生成。



&#x20; 删除或移动页面不会回收旧 Slide ID。新增页面使用新的 Sequence，因此能够为后续 Revision、局部重新生成和 SlideIR 映射提供稳定身份。



&#x20; ## 6. Outline 修改操作



&#x20; Slice A 支持以下 Outline EditCommand：



&#x20; - ADD\_SLIDE；

&#x20; - DELETE\_SLIDE；

&#x20; - MOVE\_SLIDE；

&#x20; - UPDATE\_SLIDE\_OBJECTIVE。



&#x20; 这些命令修改的是 OutlinePlan，而不是最终 SlideIR。



&#x20; 每批成功执行的命令都会：



&#x20; - 创建新的 Outline Revision；

&#x20; - 保持已有页面的稳定 Slide ID；

&#x20; - 为新增页面分配新的稳定 Slide ID；

&#x20; - 更新 Page Count；

&#x20; - 维持 6～12 页约束；

&#x20; - 验证所有 evidenceRefs；

&#x20; - 保持操作结果确定性。



&#x20; 具体行为：



&#x20; ### ADD\_SLIDE



&#x20; - 支持在指定页面之后插入；

&#x20; - 未指定位置时添加到末尾；

&#x20; - 页面总数达到 12 页时拒绝继续添加；

&#x20; - 新页面必须包含有效 Objective 数据。



&#x20; ### DELETE\_SLIDE



&#x20; - 按稳定 Slide ID 删除；

&#x20; - 页面总数不能低于 6 页；

&#x20; - 不修改其他页面的 Slide ID。



&#x20; ### MOVE\_SLIDE



&#x20; - 按 Slide ID 和目标位置移动；

&#x20; - 使用一基序号作为命令位置；

&#x20; - 不创建新的 Slide ID；

&#x20; - 操作结果确定。



&#x20; ### UPDATE\_SLIDE\_OBJECTIVE



&#x20; 支持更新：



&#x20; - 标题；

&#x20; - Purpose；

&#x20; - Key Message；

&#x20; - Evidence Refs；

&#x20; - Visual Intent；

&#x20; - Expected Element Types。



&#x20; 命令执行后会重新验证 Evidence Ref 是否存在于 ResearchPack 中。



&#x20; 自然语言不能直接执行 arbitrary JSON patch。受控命令是 OutlinePlan 修改的唯一应用边界。



&#x20; ## 7. WAITING\_FOR\_OUTLINE\_APPROVAL



&#x20; 完成 Intent、Research、Storyline 和 OutlinePlan 持久化后，GenerationJob 进入：



&#x20; - Workflow：AGENT\_OUTLINE；

&#x20; - Status：WAITING\_FOR\_OUTLINE\_APPROVAL；

&#x20; - Stage：OUTLINE\_PLANNED；

&#x20; - Completed Work Units：3/3。



&#x20; 该状态是真实的 Durable Approval Gate。



&#x20; GenerationJob Claim 操作遇到该状态时返回：



&#x20; ErrGenerationJobAwaitingOutlineApproval



&#x20; 因此在用户明确批准前：



&#x20; - Renderer 不能继续；

&#x20; - 后续 Slide Worker 不能继续；

&#x20; - 不会创建 DeckJob；

&#x20; - 不会创建 SlideJob；

&#x20; - 不会生成最终 SlideIR；

&#x20; - 不会生成 LayoutResult；

&#x20; - 不会生成 PPTX；

&#x20; - 不会创建 Private Artifact。



&#x20; 用户批准后，Job 进入合法交接状态：



&#x20; - Status：QUEUED；

&#x20; - Stage：OUTLINE\_APPROVED。



&#x20; 该状态仅表示未来可以由 Slice B 消费。



&#x20; Slice A 没有注册或调用 Slice B Consumer，因此批准后不会自动开始页面生成。



&#x20; ## 8. PostgreSQL 持久化与 Durable Recovery



&#x20; PostgreSQL Planning Store 持久化：



&#x20; - IntentSpec；

&#x20; - ResearchPack；

&#x20; - Storyline；

&#x20; - Current Outline Revision；

&#x20; - Approved Outline Revision；

&#x20; - Research Execution Count；

&#x20; - 每个 Outline Revision 的完整 Snapshot。



&#x20; 规划阶段的持久化操作会验证 Phase 2 已有的：



&#x20; - Lease Owner；

&#x20; - Lease Expiration；

&#x20; - Fencing Token；

&#x20; - Job Status；

&#x20; - Expected Stage。



&#x20; 初始规划和后续 Outline 操作都在 PostgreSQL Transaction 中完成。



&#x20; ### Restart Recovery



&#x20; 真实 PostgreSQL Integration Test 验证以下恢复流程：



&#x20; 1. 创建 Agent Planning Job；

&#x20; 2. 生成 OutlinePlan；

&#x20; 3. Job 进入 WAITING\_FOR\_OUTLINE\_APPROVAL；

&#x20; 4. 销毁原 Store/Service 实例；

&#x20; 5. 创建新的 Store/Service 实例模拟服务重启；

&#x20; 6. 使用相同幂等请求重新进入；

&#x20; 7. 恢复相同 GenerationJob；

&#x20; 8. 恢复相同 OutlinePlan；

&#x20; 9. 保持相同 Revision 和 Slide ID；

&#x20; 10. 不重新执行 Research；

&#x20; 11. 不重新生成 Storyline；

&#x20; 12. 不重新规划 Outline；

&#x20; 13. 用户继续修改和批准。



&#x20; 恢复后：



&#x20; - Job ID 不变；

&#x20; - Outline ID 不变；

&#x20; - Outline Revision 不变；

&#x20; - Research Execution Count 不增加；

&#x20; - Research Provider 不再次调用；

&#x20; - 前端能够继续显示同一个待确认 Outline。



&#x20; 前端只缓存：



&#x20; xianzhi:ppt:v2:agent-job-id



&#x20; OutlinePlan 本身不会作为权威状态保存在前端 Local Storage 中。



&#x20; ## 9. Approval Revision 与 Optimistic Locking



&#x20; Outline Update 和 Outline Approval 都必须提交：



&#x20; expectedRevision



&#x20; PostgreSQL 在操作过程中：



&#x20; 1. 锁定 GenerationJob Row；

&#x20; 2. 锁定 Agent Plan Row；

&#x20; 3. 读取 Current Outline Revision；

&#x20; 4. 比较 expectedRevision；

&#x20; 5. 不一致时返回 ErrStaleOutlineRevision。



&#x20; 第一次合法批准会：



&#x20; 1. 为当前 Outline Revision 写入 approved\_at；

&#x20; 2. 在 Agent Plan 上记录 approved\_outline\_revision；

&#x20; 3. 将 GenerationJob Stage 推进到 OUTLINE\_APPROVED；

&#x20; 4. 写入一条 Approval Transition；

&#x20; 5. 提交 Transaction；

&#x20; 6. 重新读取并返回持久化权威状态。



&#x20; 批准后继续修改 OutlinePlan 会返回：



&#x20; ErrOutlinePlanApproved



&#x20; 如果批准请求针对不同 Revision，则返回 stale/conflict，不会作为幂等成功处理。



&#x20; ## 10. Duplicate Approval Idempotency



&#x20; Commit 231138dd4 修复了 Slice A 最后一个 CI Failure。



&#x20; ### 根因



&#x20; 第一次 Approve 完成后，旧实现直接返回内存中的 Approval State。



&#x20; 其中：



&#x20; - 内存 ApprovedAt 为纳秒精度；

&#x20; - PostgreSQL timestamptz Round-trip 为微秒精度。



&#x20; 第二次对相同 Revision 执行 Duplicate Approve 时，返回的是从 PostgreSQL 重新读取的 State。



&#x20; 重复请求本身没有：



&#x20; - 再次写 Approval；

&#x20; - 新增 Revision；

&#x20; - 新增 Transition；

&#x20; - 再次推进 Job。



&#x20; 但第一次和第二次返回的 ApprovedAt 不完全相等，因此两次 State 不具备真正的语义等价性。



&#x20; ### 修复



&#x20; 第一次 Approve Transaction Commit 后，不再直接返回内存 State，而是重新读取 PostgreSQL 中的权威 State。



&#x20; 因此第一次批准和后续 Replay 都返回同一持久化表达。



&#x20; ### 最终幂等语义



&#x20; 如果同一个 Outline Revision 已经批准，再收到相同 Approve 请求：



&#x20; - 不新增 Revision；

&#x20; - 不修改 ApprovedAt；

&#x20; - 不重新生成 ApprovedOutline Snapshot；

&#x20; - 不新增 OUTLINE\_APPROVED Transition；

&#x20; - 不再次推进 GenerationJob；

&#x20; - 返回与第一次批准后语义等价的持久化 State。



&#x20; 如果请求针对不同 Revision：



&#x20; - 返回 stale/conflict；

&#x20; - 不作为幂等成功处理。



&#x20; 真实 PostgreSQL 测试验证：



&#x20; - 第一次和 Replay 的完整 State 相等；

&#x20; - ApprovedAt 不变；

&#x20; - Outline Revision 数不变；

&#x20; - Approval Transition 数不变。



&#x20; ## 11. Tenant 与 Owner Isolation



&#x20; Planning Get、Update 和 Approve 都使用：



&#x20; - tenant\_id；

&#x20; - user\_id。



&#x20; 作为联合 Scope。



&#x20; 不属于当前 Tenant/Owner 的 Job 会被返回为 Not Found。



&#x20; 这同时避免：



&#x20; - 跨租户读取；

&#x20; - 跨租户修改；

&#x20; - 跨租户批准；

&#x20; - 通过错误差异泄露 Job 是否存在。



&#x20; Memory Test 和 PostgreSQL Integration Test 都覆盖：



&#x20; - Cross-tenant Read Denied；

&#x20; - Cross-tenant Update Denied；

&#x20; - Cross-tenant Approve Denied。



&#x20; HTTP Handler 不接受请求体提供的 Tenant ID 或 Owner ID。



&#x20; Tenant 和 Owner 均从服务端认证用户上下文中获得。



&#x20; ## 12. Migration 110



&#x20; Slice A 最终 Migration 为：



&#x20; database/migrations/110-ppt-v2-agent-outline-approval.sql



&#x20; Migration 110：



&#x20; - 为 xz\_ppt\_v2\_generation\_jobs 增加 workflow\_type；

&#x20; - 为历史和现有任务保留默认 RENDER Workflow；

&#x20; - 允许 AGENT\_OUTLINE Job 不关联 Existing Task；

&#x20; - 允许 AGENT\_OUTLINE Job 不创建 DeckJob；

&#x20; - 增加 WAITING\_FOR\_OUTLINE\_APPROVAL Status；

&#x20; - 增加 Intent、Research、Storyline、Outline 与 Approval Stage；

&#x20; - 约束 Agent Outline 页数为 6～12；

&#x20; - 约束 Agent Planning Work Units 为 3；

&#x20; - 创建 xz\_ppt\_v2\_agent\_plans；

&#x20; - 创建 xz\_ppt\_v2\_outline\_revisions；

&#x20; - 记录 Current Outline Revision；

&#x20; - 记录 Approved Outline Revision；

&#x20; - 记录 Research Execution Count；

&#x20; - 对 (generation\_job\_id, revision) 建立唯一约束；

&#x20; - 约束 Approved Revision 必须等于 Current Revision。



&#x20; Migration 109 继续作为 Phase 2 Durable Generation Migration 保留。



&#x20; Migration 110 只扩展 Phase 3 Slice A 能力，不改写 Phase 1 或 Phase 2 历史报告。



&#x20; ## 13. /api/v1/ppt API 扩展



&#x20; Slice A 在现有 /api/v1/ppt family 中增加四个操作：



&#x20; - POST /api/v1/ppt/agent/guide；

&#x20; - GET /api/v1/ppt/agent/jobs/:jobId；

&#x20; - PATCH /api/v1/ppt/agent/jobs/:jobId/outline；

&#x20; - POST /api/v1/ppt/agent/jobs/:jobId/outline/approve。



&#x20; 没有创建 /api/v2/ppt。



&#x20; ### Guide



&#x20; Guide 请求包含：



&#x20; - Idempotency Key；

&#x20; - 用户自然语言请求；

&#x20; - 可选 Audience；

&#x20; - 可选 Scenario；

&#x20; - 可选 Language；

&#x20; - 可选 Professional Style；

&#x20; - 可选 Page Count；

&#x20; - 可选 Research Required。



&#x20; Guide 请求经过：



&#x20; - 现有认证边界；

&#x20; - 服务端 Tenant 解析；

&#x20; - 现有文本安全检查。



&#x20; ### Get Current State



&#x20; 按 Job ID 返回持久化的：



&#x20; - GenerationJob Planning State；

&#x20; - IntentSpec；

&#x20; - ResearchPack；

&#x20; - Storyline；

&#x20; - Current OutlinePlan；

&#x20; - Approved OutlinePlan；

&#x20; - Research Execution Count。



&#x20; ### Update Outline



&#x20; 接收：



&#x20; - Expected Revision；

&#x20; - 受控 Outline EditCommand 列表。



&#x20; 发生 Revision Conflict 时返回 Conflict。



&#x20; ### Approve Outline



&#x20; 接收：



&#x20; - Expected Revision。



&#x20; 同 Revision Duplicate Approve 返回幂等成功，不同 Revision 返回 Stale Conflict。



&#x20; 生产 Planning Service 只在 PostgreSQL Store 可用时配置。



&#x20; Memory Store 只用于确定性测试，不是生产环境的持久化 Fallback。



&#x20; ## 14. Outline Review UI



&#x20; 现有 PPT V2 工作台已接入 Slice A Outline Approval Flow。



&#x20; 用户可以：



&#x20; - 输入 PPT 需求；

&#x20; - 触发 Intent、Research 和 Storyline 规划；

&#x20; - 查看真实持久化 Planning State；

&#x20; - 查看 Dynamic OutlinePlan；

&#x20; - 查看页数和 Revision；

&#x20; - 修改页面标题；

&#x20; - 修改页面 Purpose；

&#x20; - 添加页面；

&#x20; - 删除页面；

&#x20; - 调整页面顺序；

&#x20; - 处理 Revision Conflict 后重新加载服务端 State；

&#x20; - 点击“确认大纲并继续”；

&#x20; - 查看已持久化的 Approved OutlinePlan Revision。



&#x20; 前端只缓存 Job ID，不缓存 OutlinePlan 权威 Snapshot。



&#x20; 每次修改成功后，前端使用服务端返回的新 Revision 更新页面。



&#x20; 如果修改失败或发生 Stale Revision：



&#x20; - 保留原错误；

&#x20; - 重新获取服务端权威 State；

&#x20; - 不用本地状态覆盖服务端。



&#x20; 批准后 UI：



&#x20; - 显示“大纲已确认”；

&#x20; - 显示 Approved OutlinePlan Revision；

&#x20; - 禁用继续修改；

&#x20; - 不跳转生成进度；

&#x20; - 不创建 PPT Task；

&#x20; - 明确提示尚未生成页面或 PPTX。



&#x20; 现有 Phase 1 PPT Task Detail、Preview 和 Export 页面继续保留，用于已经生成的历史任务，但 Slice A Approval Flow 不会调用这些能力。



&#x20; ## 15. PostgreSQL Integration Tests 与 CI Gate



&#x20; GitHub Actions PostgreSQL Gate 会先枚举并强制要求以下四个测试名称存在，然后对真实 PostgreSQL 执行完整的：



&#x20; TestPostgresGenerationJob\*



&#x20; 四个 Gate 的最终结果：



&#x20; - TestPostgresGenerationJobLeaseFencingRestartCancelAndIsolation：PASS；

&#x20; - TestPostgresGenerationJobArtifactConstraintsAndTransactionRollback：PASS；

&#x20; - TestPostgresGenerationJobRetryAndAtomicTaskRelation：PASS；

&#x20; - TestPostgresGenerationJobAgentOutlineApprovalRecoveryAndIsolation：PASS。



&#x20; Slice A PostgreSQL Test 覆盖：



&#x20; - Durable Waiting；

&#x20; - Approval 前 Claim 被拒绝；

&#x20; - Restart Recovery；

&#x20; - 相同 OutlinePlan 恢复；

&#x20; - Research 不重复；

&#x20; - Storyline 不重复生成；

&#x20; - Outline 不重复规划；

&#x20; - Cross-tenant Isolation；

&#x20; - Optimistic Outline Update；

&#x20; - Stale Approval Rejection；

&#x20; - Approved Snapshot Immutability；

&#x20; - Duplicate Approval Replay；

&#x20; - ApprovedAt 不变；

&#x20; - Revision 数不变；

&#x20; - Approval Transition 数不变；

&#x20; - 不创建 DeckJob；

&#x20; - 不创建 SlideJob。



&#x20; 最终验证 Commit：



&#x20; 231138dd4 fix(ppt-v2): make outline approval replay idempotent



&#x20; 最终 GitHub Actions：



&#x20; - user-core / backend-go：GREEN；

&#x20; - user-core / user-core：GREEN。



&#x20; Slice A PostgreSQL Gate 是在 GitHub Actions 的真实 PostgreSQL Service 中通过的，不是基于本地 Skip 判定 READY。



&#x20; ## 16. Golden 1、OfficeCLI 与 Phase 1/2 回归



&#x20; Slice A 没有修改：



&#x20; - SlideIR；

&#x20; - LayoutResult；

&#x20; - PptxGenJS Renderer；

&#x20; - PPTX 输出语义；

&#x20; - Private Artifact Transaction；

&#x20; - Phase 2 Lease；

&#x20; - Phase 2 Fencing；

&#x20; - Phase 2 Existing Task Relation。



&#x20; 最终 GitHub Actions 结果确认：



&#x20; - Golden 1 Regression：PASS；

&#x20; - OfficeCLI PPTX Integrity Validation：PASS，未 Skip；

&#x20; - Phase 2 三个 PostgreSQL Gate：PASS；

&#x20; - Backend Go Regression：GREEN；

&#x20; - User-core Regression：GREEN。



&#x20; 因此：



&#x20; - Phase 1 两页 Renderer Kernel 保持有效；

&#x20; - Phase 1 Golden 1 语义和 Geometry 回归保持有效；

&#x20; - Phase 2 Durable Generation 保持有效；

&#x20; - Phase 2 Artifact Effectively-once 保持有效；

&#x20; - Phase 2 Lease/Fencing 保持有效；

&#x20; - Phase 2 PostgreSQL Transaction 与 Task Relation 保持有效；

&#x20; - OfficeCLI Integrity Gate 保持有效。



&#x20; ## 17. Known Limitations



&#x20; - Research MVP 当前只使用 Wikipedia Search。

&#x20; - Research 已结构化且可追踪，但还不是 Multi-provider Autonomous Research Agent。

&#x20; - Storyline 和 OutlinePlan 当前使用确定性的 Professional Deck 规划规则，还没有接入 LLM Planning Adapter。

&#x20; - Guide HTTP 请求同步完成当前短规划流程；Durable Stage 是权威状态，但前端暂时没有接收中间 Checkpoint 的实时事件流。

&#x20; - ResearchPack Contract 支持 Dataset，但 Wikipedia Adapter 当前不产生 Dataset。

&#x20; - Citation Provenance 当前停止在 SlideObjective 的 evidenceRefs。

&#x20; - Citation 到最终 SlideIR/CitationElement 的映射属于 Slice B。

&#x20; - OUTLINE\_APPROVED 已建立合法交接状态，但本阶段没有 Slice B Consumer。

&#x20; - Slice A 不生成最终 SlideIR。

&#x20; - Slice A 不实现 Image Generation。

&#x20; - Slice A 不实现 Chart。

&#x20; - Slice A 不实现最终 LayoutResult。

&#x20; - Slice A 不实现 PreviewRenderer。

&#x20; - Slice A 不修改 PPTX Export。

&#x20; - Slice A 不实现针对 SlideIR 的 Agent EditCommand。

&#x20; - Slice A 不实现 Revision UI。

&#x20; - Slice A 不实现 Undo。

&#x20; - Slice A 不实现 Local Slide Regeneration。

&#x20; - Slice A 不实现 Creative Mode。

&#x20; - Slice A 不扩展企业审批、多人协作或企业知识库治理。



&#x20; ## 18. Slice A Exit Gate



&#x20;  Exit Gate                        最终证据                                               结果

&#x20; ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━

&#x20;  IntentSpec 可持久化              Domain Contract 与 PostgreSQL Planning Row             PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  ResearchPack 结构化              Source/Citation/Claim/Dataset Contract 与测试          PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Source → Claim 可追踪            Stable Source ID 与 Evidence Validation                PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Storyline 独立存在               Thesis、Takeaway、Arc、Sections、Closing Action        PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  6～12 页 Dynamic OutlinePlan     单页数、范围与未指定页数测试                           PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  用户明确页数得到尊重             Intent 与 Dynamic Outline Tests                        PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Stable Slide IDs                 Stable Sequence 与 Command Tests                       PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Outline 可增删改排序             四个 Outline EditCommand                               PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Durable Approval Gate            WAITING\_FOR\_OUTLINE\_APPROVAL 与 Claim Rejection        PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Restart 后恢复待审批状态         Real PostgreSQL Slice A Gate                           PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Research 不重复                  Restart Integration Assertion                          PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Storyline/Outline 不重复规划     Restart Integration Assertion                          PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Optimistic Locking               Expected Revision 与 Row Lock                          PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Stale Revision Rejected          Unit、HTTP、PostgreSQL Tests                           PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Duplicate Approval Idempotent    Persisted-state Replay Fix 与 PostgreSQL Assertions    PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Approved Revision Immutable      Update-after-approval Rejection                        PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Tenant/Owner Isolation           Cross-tenant Read/Update/Approve Tests                 PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Migration 110                    Migration Contract 与 CI Apply                         PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  /api/v1/ppt API 扩展             Guide/Get/Update/Approve                               PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Outline Review UI                Server-authoritative Review and Approval               PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Phase 1/2 Regression             PostgreSQL Gates、Golden 1、OfficeCLI                  PASS

&#x20; ───────────────────────────────  ─────────────────────────────────────────────────────  ───────

&#x20;  Final GitHub Checks              Backend-go 与 User-core                                GREEN



&#x20; Slice A 的全部 Exit Gate 已满足。



&#x20; 该结论只确认 Phase 3 Slice A 里程碑完成：



&#x20; - 不合并当前 PR；

&#x20; - 不自动进入 Slice B；

&#x20; - 不实现 Slice C；

&#x20; - 不实现 Slice D。



&#x20; ## 最终结论



&#x20; SLICE A STATUS: READY

