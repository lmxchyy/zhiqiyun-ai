# Git 分支 / Worktree 治理审计

- 审计日期：2026-08-14（Asia/Shanghai）
- 仓库：E:/code/work/先知AI
- 当前分支：main
- 当前 HEAD：6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c
- 本轮性质：只读 Git 审计；唯一写入是本报告

## 审计口径与限制

1. 本报告使用本地 refs、commit graph、merge-base、main...branch ahead/behind、git cherry patch-equivalence、分支贡献 diff、文件存在性、worktree status，以及传统三方 git merge-tree 只读模拟。
2. 表中的 B/A 表示相对 main 的 behind/ahead，即 main 独有提交数 / 分支独有提交数。文件贡献按 merge-base...branch 统计，避免把主线后续变更误算成分支功能。
3. “等价覆盖”只有两种高确定性证据：分支 tip 已是 main 祖先，或分支所有独有非 merge commit 均被 git cherry 标记为 patch-equivalent。仅有相同 commit message 不视为等价。
4. 远端即时只读校验 git ls-remote origin/gitee 因网络超时失败；未执行会写 refs 的 fetch/prune。因此“远程分支”是本地已有 remote-tracking refs 的完整清单，不保证服务器在审计瞬间没有新增或已删除的 ref。
5. 传统 merge-tree 未产生文本冲突标记不代表业务语义可安全合并。报告同时使用共同修改文件数、add/add、modify/delete、历史漂移和架构分叉判断风险。
6. 未运行构建、测试或部署命令，以避免在 21 个 worktree 中产生构建缓存或其它非报告写入。

## A. Executive Summary

**整体治理状态：高风险；main 本身可作为当前集成基线，但分支和 worktree 数量、长期漂移及并行实现已明显失控。**

| 指标 | 结果 |
|---|---:|
| 本地分支 | 28（含 main） |
| remote-tracking refs | 23（排除 origin/HEAD、gitee/HEAD；origin 12、gitee 11） |
| 远端短名称去重 | 14（含 main） |
| 本地与远端合并后的逻辑分支名 | 31（含 main） |
| worktree | 21 |
| dirty worktree | 1 |
| 非 main 分支建议继续保留 | 13 |
| 高确定性 SAFE_TO_CLEAN_CANDIDATE | 7 |
| 需要进一步人工确认 | 10 |

以上“保留 / 清理候选 / 人工确认”对 30 个非 main 逻辑分支进行互斥计数。人工确认组包含“被另一未验证分支覆盖”或“功能大概率已被 main 后续实现替代、但 patch 不完全等价”的分支。

最主要风险：

1. **身份、安全、支付保护分支严重漂移。** identity 保护分支落后 main 215 commits，仍有 36 个 main 不存在的文件；enterprise 保护分支落后 144 commits，包含双支付 gate 与账务相关代码。直接合并风险不可接受。
2. **Seedance 同一问题存在四套并行实现。** minimal、prod-release、R8、video-artifact-fix 不是祖先链；同源分支之间仍有 8～18 个文件、数百至上千行差异，已形成架构分叉。
3. **SmartVideo 保护分支混入大量产物。** mini 保护分支一次提交 2,562 个文件，其中 artifacts/builds 2,517 个；它基于 2.0.42，而 main 已推进到 2.0.63，并已经合入另一套自动混剪 V1。
4. **21 个 worktree 造成所有权和生命周期不清。** 其中多个指向已合并、重复或后续被覆盖的分支；Safe Area worktree 还存在 2 个未跟踪文件。
5. **双远端镜像不对称且无法即时核验。** origin 与 gitee 的分支集合不同，许多本地保护分支没有 upstream；本轮不能证明服务器端 refs 与本地缓存完全一致。

## B. Branch Inventory

### B1. Ref、拓扑与时间

状态缩写：L=本地，O=origin，G=gitee；WT 编号对应 C 节。HEAD 与 merge-base 使用完整 SHA；B/A 为相对 main 的 behind/ahead。

| 分支 | Ref 状态 | WT | 最后提交时间 | HEAD | B/A | Merge-base |
|---|---|---:|---|---|---:|---|
| main | L/O/G | WT01 | 2026-08-14T16:28:50+08:00 | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c | 0/0 | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c |
| codex/agent-invite-apk-production-final | L/G | WT14 | 2026-07-25T21:43:29+08:00 | 27dde3b4e9130eff574b9d2aa48ebc0522b7d36b | 195/1 | 7695c8ae8376fb96599d94b9e643dd605bebd742 |
| codex/agent-invite-apk-production-readiness | O only | — | 2026-07-25T20:06:26+08:00 | 7695c8ae8376fb96599d94b9e643dd605bebd742 | 195/0 | 7695c8ae8376fb96599d94b9e643dd605bebd742 |
| codex/agent-invite-apk-release-blockers | L/O/G | WT13 | 2026-07-25T14:47:15+08:00 | 84bda3b7bb5e7e6114d724c63dece843571f98b7 | 202/2 | 0649ca7b055b77620ebec7daae18ab4a61b868b3 |
| codex/channel-ecosystem-v132-phase3 | O/G only | — | 2026-08-12T05:31:47+08:00 | e0b57e1efcd501854aaba2d6459e412ed679bad2 | 60/0 | e0b57e1efcd501854aaba2d6459e412ed679bad2 |
| codex/enterprise-v1 | L only | — | 2026-07-31T05:08:56+08:00 | 9c5350ed2b074e9175cf837fed52f5ff305157da | 144/1 | dbe6e656107cf4ee855db37ac29a5a8c2016d0ec |
| codex/grok-imagine-1.5-video | L/O/G | — | 2026-08-02T23:16:29+08:00 | 39ea07413ac595b41f4eb8cbc2d547299f4dfa63 | 183/2 | 38a8318b82cc55a7d5bd0f667d1c2230fd743c5a |
| codex/identity-phase2-1-security | L only | WT08 | 2026-07-22T23:03:41+08:00 | dc7efceacd805efa73970fe5a3798697f413005c | 215/1 | 8c3ea79575af70acd8e89244bea0c391935c6993 |
| codex/identity-phase2-2-deployment-gates | L only | — | 2026-07-23T18:40:09+08:00 | 867cf82e807a9ae8c5950171aed9e8985a224f37 | 215/2 | 8c3ea79575af70acd8e89244bea0c391935c6993 |
| codex/identity-phase2-2-release-readiness | L only | WT09 | 2026-07-23T18:40:09+08:00 | 867cf82e807a9ae8c5950171aed9e8985a224f37 | 215/2 | 8c3ea79575af70acd8e89244bea0c391935c6993 |
| codex/login-compliance-2.0.38 | L only | — | 2026-07-31T14:13:39+08:00 | 61ea015127e8ffbc048e4cb9a3a3a0bb38b90166 | 137/4 | dfc70c84535c0d8b28c44c17248f08e0408fb37e |
| codex/miniprogram-agent-invite-autobind-production | L/G | WT04 | 2026-07-30T12:06:09+08:00 | e10726bf279842f7a61fe713eb78d00701ce4a2f | 145/3 | 054a8fcaf4754ca9b3fd5492265685998924835c |
| codex/operation-center-invite-promotion | L/O | WT12 | 2026-08-03T23:03:02+08:00 | 41cdf9a3437695f1a64a02849895f4b4bd71b9e4 | 183/3 | 38a8318b82cc55a7d5bd0f667d1c2230fd743c5a |
| codex/ppt-agent-phase1-regression-integration | L/O/G | WT15 | 2026-08-12T07:42:07+08:00 | df7a9808741dd829b493632bb3abd5e794fd49ac | 42/16 | cfe52c8bf79aaec26a72fab88814bd7163c0cf76 |
| codex/protect-enterprise-v1-20260814 | L only | WT07 | 2026-08-14T01:20:02+08:00 | 931b79d9554666f783abafd2256234c492d76774 | 144/2 | dbe6e656107cf4ee855db37ac29a5a8c2016d0ec |
| codex/protect-identity-phase2-2-deployment-gates-20260814 | L only | WT10 | 2026-08-14T01:20:50+08:00 | 15889e5dbe5b8659db46c0e5a13a882ae7872ee2 | 215/3 | 8c3ea79575af70acd8e89244bea0c391935c6993 |
| codex/protect-main-production-ops-20260814 | L only | — | 2026-08-14T01:26:06+08:00 | fb4fc7cc9347883323142e3b1b5a5bf944e753ad | 29/1 | 4d332631d21796ac90d115df874ca13498e7f8bd |
| codex/protect-seedance-prod-minimal-20260805 | L only | WT02 | 2026-08-14T01:17:10+08:00 | d5de37b03c5f7008a04c416245166b61da2a7ebf | 144/1 | dbe6e656107cf4ee855db37ac29a5a8c2016d0ec |
| codex/protect-seedance-prod-release-20260805 | L only | WT03 | 2026-08-14T01:17:44+08:00 | bad1268190f6f26e975a8927518e26959b7db604 | 144/1 | dbe6e656107cf4ee855db37ac29a5a8c2016d0ec |
| codex/protect-seedance-r8-compliant-20260806 | L only | WT16 | 2026-08-14T01:18:16+08:00 | 21b07c50c3bd2668f260881624541d00514d35dd | 108/1 | 11879fc9097db091b64d2cddee691ee397e5defd |
| codex/protect-seedance-video-artifact-fix-20260814 | L only | WT17 | 2026-08-14T01:21:20+08:00 | a8c518b01108d15c76b7333e1b8ec5a59d1d7acb | 144/1 | dbe6e656107cf4ee855db37ac29a5a8c2016d0ec |
| codex/protect-smartvideo-grok-backend-20260814 | L only | WT19 | 2026-08-14T01:24:27+08:00 | 157b063281c5b11b1e97d792909c24e0c5b386f3 | 183/3 | 38a8318b82cc55a7d5bd0f667d1c2230fd743c5a |
| codex/protect-smartvideo-mini-2.0.42-20260814 | L only | WT18 | 2026-08-14T01:23:05+08:00 | b75fc1f06b10b9a55a68a13bae673f29e498b014 | 183/1 | 38a8318b82cc55a7d5bd0f667d1c2230fd743c5a |
| codex/seedance-video-artifact-fix | L only | — | 2026-07-30T23:38:46+08:00 | dbe6e656107cf4ee855db37ac29a5a8c2016d0ec | 144/0 | dbe6e656107cf4ee855db37ac29a5a8c2016d0ec |
| codex/smartvideo-mini-2.0.42 | L only | — | 2026-08-02T18:35:06+08:00 | 38a8318b82cc55a7d5bd0f667d1c2230fd743c5a | 183/0 | 38a8318b82cc55a7d5bd0f667d1c2230fd743c5a |
| codex/video-compliance-release-tools-20260731 | L only | WT20 | 2026-07-31T18:12:37+08:00 | ad2a02c0448f72c3ecd0e14167f2937e04baa30f | 137/5 | dfc70c84535c0d8b28c44c17248f08e0408fb37e |
| codex/video-contract-2.0.36 | O/G only | — | 2026-07-31T12:39:53+08:00 | dfc70c84535c0d8b28c44c17248f08e0408fb37e | 137/0 | dfc70c84535c0d8b28c44c17248f08e0408fb37e |
| codex/video-model-params-2.0.36 | L/O/G | WT21 | 2026-07-31T14:13:39+08:00 | 61ea015127e8ffbc048e4cb9a3a3a0bb38b90166 | 137/4 | dfc70c84535c0d8b28c44c17248f08e0408fb37e |
| codex/video-ui-login-2.0.39 | L/O/G | WT11 | 2026-08-02T16:53:27+08:00 | f2090a6c0973b1fe1a69ef01de1cbbe6ae13b1d3 | 137/7 | dfc70c84535c0d8b28c44c17248f08e0408fb37e |
| feature/ai-inspiration | L/O/G | WT05 | 2026-08-12T07:57:32+08:00 | 7ef4a4b34e89fda8ef8f1bdf3297b86355934d90 | 42/5 | cfe52c8bf79aaec26a72fab88814bd7163c0cf76 |
| feature/cross-platform-safe-area | L/O | WT06 | 2026-08-14T19:15:21+08:00 | 81a66692fc8c8601007c3b055bfbf27b17d43f46 | 0/4 | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c |

### B2. 功能、覆盖、冲突与建议

“独有码”表示 main 中是否仍不存在可识别的有效内容；“冲突面”包含共同修改文件和只读 merge-tree 的结构冲突。SAFE_TO_CLEAN_CANDIDATE 仅是候选标签，本轮没有执行清理。

| 分支 | 主要功能 / 模块范围 | 当前状态 | 独有码 | 重复 / 覆盖关系 | 漂移与潜在冲突 | 保留与下一步 | 风险 |
|---|---|---|---|---|---|---|---|
| main | 当前集成主线；Go 后端、Vue/uni-app、计费、视频、SmartVideo、AI 生图 | 已合并 / 主线 | 不适用 | 与本地 origin/main、gitee/main 同 SHA | 无本地漂移；实时远端未核验 | 保留；作为所有后续审计基线 | LOW |
| codex/agent-invite-apk-production-final | 邀请注册页 navigationStyle；1 文件 1 行 | 已被 main 等价覆盖 | 否 | 唯一 commit 被 git cherry 标记等价；main 有 efe5ba73c | B195；1/1 文件与 main 重叠；无模拟冲突 | SAFE_TO_CLEAN_CANDIDATE；人工批准后先移除 WT14，再清 refs | LOW |
| codex/agent-invite-apk-production-readiness | APK 生产准备 / Alpine 镜像 | 已合并 | 否 | tip 本身是 main 祖先 | B195/A0 | SAFE_TO_CLEAN_CANDIDATE；仅远端候选 | LOW |
| codex/agent-invite-apk-release-blockers | APK 注册、邀请码、后端 API、迁移 077、部署；37 文件 | 已被其他实现覆盖，需确认 | 未确认；无 branch-only 文件，但 2 commits patch-unique | main 有 4a20d7114 邀请发布实现；所有 37 文件均被 main 后续改过 | B202；37/37 重叠；InviteRegisterPage.vue 与 agent_invite_apk_api.go 为 add/add | 暂不直接合并；对比 main 当前邀请链路和迁移后再决定清理 | MEDIUM |
| codex/channel-ecosystem-v132-phase3 | 渠道生态 / 作品中心 montage 类型修复 | 已合并 | 否 | tip 是 main 祖先 | B60/A0 | SAFE_TO_CLEAN_CANDIDATE；origin、gitee 远端候选 | LOW |
| codex/enterprise-v1 | Enterprise V1 feature flags、API contract、pricing health；14 文件 | 已被后续分支覆盖 | 有，但全部包含于 protect-enterprise | protect-enterprise 是其直接后继 | B144；4/14 文件重叠；模拟无结构冲突 | 不单独继续；先审计保护分支，确认后列条件清理 | HIGH |
| codex/grok-imagine-1.5-video | Grok 1.5 视频、估算、计费历史；16 文件 | 已被 main 后续实现大体覆盖 | 仅旧计划和一份计费历史测试明确不在 main；生产独有价值未确认 | main 8/10～8/11 已加入 per-second Grok、Preview、默认模型与价格排序；也是 operation/smartvideo 旧基线 | B183；13/16 重叠；模拟无结构冲突，语义冲突高 | 不直接合并；确认测试是否仍有价值后清理候选 | MEDIUM |
| codex/identity-phase2-1-security | 商业身份安全、客户资料保护、迁移 072；48 文件 | 已被后续保护分支覆盖 | 有；8 个 main 不存在文件 | 是 phase2.2 和 protect-identity 的祖先 | B215；22/48 重叠 | 仅作历史基线；保护分支验证前不得清理，之后条件清理 | HIGH |
| codex/identity-phase2-2-deployment-gates | 身份 release readiness、环境校验、迁移 072/073；80 文件 | 已被后续保护分支覆盖 | 有；24 个 main 不存在文件 | 与 release-readiness 完全同 HEAD；又被 protect-identity 完整包含 | B215；35/80 重叠 | 不继续双线维护；保护分支审计后条件清理 | HIGH |
| codex/identity-phase2-2-release-readiness | 同上 | 已被后续保护分支覆盖 | 同上 | 与 deployment-gates 完全同 HEAD | B215；35/80 重叠 | WT09 无独立价值；待保护分支验证后条件清理 | HIGH |
| codex/login-compliance-2.0.38 | 实际是视频动态参数 contract；19 文件 | 已被 main 后续实现覆盖，需确认 | 无 branch-only 文件 | 与 video-model-params 完全同 HEAD；3/4 commits patch-equivalent，main 有同主题集成 | B137；19/19 重叠；3 个 add/add | 不继续；与 video-model 一起做一次范围核对后条件清理 | MEDIUM |
| codex/miniprogram-agent-invite-autobind-production | 小程序扫码自动绑定、kill switch、H5/小程序 gate；14 文件 | 已完成待验证 / 待同步 | 是；独有测试和 3 个 patch-unique commits | 与旧 APK 邀请分支相关但不是等价实现 | B145；7/14 重叠；模拟无结构冲突 | 保留；先基于 main 做行为审计与回归，再决定同步/拆分合并 | HIGH |
| codex/operation-center-invite-promotion | 运营中心邀请归因、推广入口、RBAC；另继承旧 Grok commits | 部分已合并 / 待拆分 | 是；11 个 branch-only 文件，核心为 invite_attribution 与 OperationPromotionPage | Grok 两 commits 被继承；main 已有其它推广实现 | B183；24/41 重叠；模拟无结构冲突但语义冲突高 | 保留；只审计 41cdf9a 的运营增量，不整体合并旧 Grok 历史 | HIGH |
| codex/ppt-agent-phase1-regression-integration | PostgreSQL PPT Agent、会话/租约、管理端、DeepSeek 计费、迁移 106/107；81 文件 | 开发中 / 已完成待验证 | 是；31 个 branch-only 文件，14 个 patch-unique commits | tag ppt-deepseek-billing-29d2a8f6f 指向该未合并历史 | B42；9/81 重叠；ppt_model_fallback_test.go modify/delete；.gitignore 出现二进制合并警告 | 保留；先完整测试与安全审计，再同步 main；禁止整体盲合 | HIGH |
| codex/protect-enterprise-v1-20260814 | Enterprise V1 + 双支付 gate、payment service、验证；24 文件 | 开发中 / WIP 保护 | 是；10 个 branch-only 文件 | 完整包含 enterprise-v1，并新增 10 文件 559+/15- | B144；7/24 重叠；模拟无结构冲突，但支付语义高风险 | 保留；优先人工审计账务幂等、gate 和现行支付架构，再同步测试 | HIGH |
| codex/protect-identity-phase2-2-deployment-gates-20260814 | Identity 2.1/2.2 + shared rate limit、CI、HAProxy、发布脚本；96 文件 | 开发中 / WIP 保护 | 是；36 个 branch-only 文件 | 完整覆盖三个旧 identity 分支 | B215；39/96 重叠；虽无结构冲突，auth/config/store 语义风险极高 | 保留；下一轮第一优先级，先安全/迁移审计，禁止直接合并 | CRITICAL |
| codex/protect-main-production-ops-20260814 | 磁盘监控、队列 guardrail、部署文档；8 文件 | 已被 main 等价覆盖 | 否 | 唯一 commit 被 git cherry 标记等价；main 有 6a4df4475 | B29；8/8 重叠；无独有码 | SAFE_TO_CLEAN_CANDIDATE | LOW |
| codex/protect-seedance-prod-minimal-20260805 | 视频 artifact admission、storage、connector/provider；10 文件 | 开发中 / 并行分叉 | 是；1 个 branch-only admission 测试，且重叠文件有大量独有逻辑 | 与 prod-release、R8、artifact-fix 同功能但非祖先链 | B144；7/10 重叠；对 prod-release 仍差 8 文件 941+/208- | 保留到四分支差异审计完成；不得直接合并 | HIGH |
| codex/protect-seedance-prod-release-20260805 | 生产视频 artifact/storage release 实现；9 文件 | 开发中 / 并行分叉 | 是；1 个 branch-only admission 测试及大量重叠文件逻辑 | 与 minimal、artifact-fix 同源分叉 | B144；6/9 重叠；对 artifact-fix 差 16 文件 785+/272- | 保留到四分支差异审计完成；建立能力矩阵后选定唯一基线 | HIGH |
| codex/protect-seedance-r8-compliant-20260806 | Seedance R8、飞书 Connector、计费/参数/合规；23 文件 | 开发中 / 并行分叉 | 是；2 个 branch-only 合规测试，另有大量重叠逻辑 | 起点晚于其它三支，但不是其后继 | B108；15/23 重叠；video_generation_estimate.go 和 test 为 add/add | 保留；单独审计 Connector 隔离、计费和 R8 约束 | HIGH |
| codex/protect-seedance-video-artifact-fix-20260814 | 视频 artifact、fail-closed billing、thumbnail、合规；18 文件 | 开发中 / 并行分叉 | 是；4 个 branch-only 文件 | 与 minimal/release 同源 sibling，不是线性升级 | B144；12/18 重叠；与 minimal 差 18 文件 | 保留；作为候选实现之一进行逐能力比较，不得宣称已覆盖其它分支 | HIGH |
| codex/protect-smartvideo-grok-backend-20260814 | 旧 Grok 基线 + SmartVideo storyboard/render dispatcher/media provider/OBS 工具；70 文件 | 部分被 main 替代 / 架构分叉 | 是；26 个 branch-only 文件，但含 exe、验收图片和运维产物 | main 已合入另一套 AI 自动混剪 V1；本支继承旧 Grok 两 commits | B183；40/70 重叠；render_worker_test add/add，smoke_renderer 两个 modify/delete | 保留到架构差异审计完成；只择取仍需要的能力，禁止整体合并 | CRITICAL |
| codex/protect-smartvideo-mini-2.0.42-20260814 | 小程序 SmartVideo 2.0.42 源码、发布工具与编译产物 | 疑似废弃 / 需人工确认 | 混合；约 37 个源码/测试/工具文件可能有价值，但 2,517 个为 build 产物 | main 已到 2.0.63 且已合入自动混剪 V1 | B183；2,562 文件；主线仅重叠 6 文件；模拟无结构冲突不代表可合并 | 保留 ref 直到源代码逐项核对；严禁整体合并，核对后倾向清理 | CRITICAL |
| codex/seedance-video-artifact-fix | 指向 2026-07-30 主线历史节点的基线分支 | 已合并 | 否 | tip 是 main 祖先 | B144/A0 | SAFE_TO_CLEAN_CANDIDATE | LOW |
| codex/smartvideo-mini-2.0.42 | 指向 2.0.41 上传节点的基线分支 | 已合并 | 否 | tip 是 main 祖先 | B183/A0 | SAFE_TO_CLEAN_CANDIDATE | LOW |
| codex/video-compliance-release-tools-20260731 | 动态视频参数 + rollback-image 工具/契约测试；22 文件 | 部分已合并 | 是；rollback-image.sh 与其测试不在 main | video-model 是其祖先；前 3 commits patch-equivalent，后 2 commits unique | B137；20/22 重叠；3 个 add/add | 保留；单独审计回滚工具是否仍符合现行部署，再选择性迁移 | HIGH |
| codex/video-contract-2.0.36 | 微信包限制历史基线 | 已合并 | 否 | tip 是 main 祖先 | B137/A0 | SAFE_TO_CLEAN_CANDIDATE；origin/gitee 远端候选 | LOW |
| codex/video-model-params-2.0.36 | 视频动态参数与估算；19 文件 | 已被 main 后续实现覆盖，需确认 | 无 branch-only 文件 | 与 login-compliance 完全同 HEAD；3 commits 等价、最后一 commit 被 main 手工/后续集成替代 | B137；19/19 重叠；3 个 add/add | 与 login-compliance 合并审计一次；确认现行参数回归绿后条件清理 | MEDIUM |
| codex/video-ui-login-2.0.39 | 视频参数 + 小程序合规 override、游客浏览、登录；31 文件 | 已被 main 后续实现覆盖，需确认 | 无 branch-only 文件；3 commits patch-unique、4 commits equivalent | main 8/2 有同主题逐提交和 exact-scope integration | B137；31/31 重叠；5 个 add/add | 核对 protected surfaces M1/M2/M3/M6 后列清理候选；不直接合并 | MEDIUM |
| feature/ai-inspiration | 灵感模板 definition/compose、上传表单、迁移 108；39 文件 | 开发中 / 已完成待验证 | 是；18 个 branch-only 文件 | main 仅有较早“旧照片修复”流程，本支是后续模板扩展 | B42；5/39 重叠；旧 photo restoration 测试为 modify/delete | 保留；先恢复/保留旧回归测试，再同步 main 并做端到端验证 | HIGH |
| feature/cross-platform-safe-area | 目前仅设计与实施计划文档；2 文件 | 开发中 | 提交中只有文档；WT06 另有 2 个未跟踪文件 | main 已有 7/25 小程序 safe-area 修复，本支目标是跨平台方案 | B0/A4；无 committed 冲突；dirty worktree 是主要风险 | 保留；先人工确认未跟踪 Harmony 测试与基线文档归属，再继续开发 | MEDIUM |

## C. Worktree Inventory

所有 worktree 路径均存在；没有 detached、locked 或被 Git 标记 prunable 的 worktree。

| ID | 路径 | 关联分支 | Dirty | 使用判断 | 建议 |
|---:|---|---|---:|---|---|
| WT01 | E:/code/work/先知AI | main | 0 | 当前主工作区 | 保留 |
| WT02 | E:/code/work/seedance-prod-minimal-20260805-222018 | codex/protect-seedance-prod-minimal-20260805 | 0 | 仍承载未审计分叉 | 保留到 Seedance 对比完成 |
| WT03 | E:/code/work/seedance-prod-release-20260805-2202 | codex/protect-seedance-prod-release-20260805 | 0 | 仍承载未审计分叉 | 同上 |
| WT04 | E:/code/work/先知AI-agent-invite-autobind-prod | codex/miniprogram-agent-invite-autobind-production | 0 | 有独有业务代码 | 保留 |
| WT05 | E:/code/work/先知AI-ai-inspiration | feature/ai-inspiration | 0 | 有独有业务代码 | 保留 |
| WT06 | E:/code/work/先知AI-cross-platform-safe-area | feature/cross-platform-safe-area | **2** | 正在规划/验证；唯一 dirty worktree | 立即人工保护未跟踪文件；本轮不处理 |
| WT07 | E:/code/work/先知AI-enterprise-v1 | codex/protect-enterprise-v1-20260814 | 0 | 支付 gate WIP | 保留 |
| WT08 | E:/code/work/先知AI-identity-phase2-1 | codex/identity-phase2-1-security | 0 | 已被 WT10 分支覆盖 | WT10 验证后列清理候选 |
| WT09 | E:/code/work/先知AI-identity-phase2-2 | codex/identity-phase2-2-release-readiness | 0 | 与另一分支同 HEAD，且被 WT10 覆盖 | WT10 验证后列清理候选 |
| WT10 | E:/code/work/先知AI-identity-phase2-2-deploy | codex/protect-identity-phase2-2-deployment-gates-20260814 | 0 | 关键未合并安全工作 | 保留，下一轮优先 |
| WT11 | E:/code/work/先知AI-login-compliance-2.0.38 | codex/video-ui-login-2.0.39 | 0 | 大概率已由 main 覆盖 | protected-surface 核对后列清理候选 |
| WT12 | E:/code/work/先知AI-oc-invite | codex/operation-center-invite-promotion | 0 | 有独有业务代码 | 保留 |
| WT13 | E:/code/work/先知AI-phase1.6 | codex/agent-invite-apk-release-blockers | 0 | main 有替代实现，仍需核对 | 人工确认后清理 |
| WT14 | E:/code/work/先知AI-phase2.1A | codex/agent-invite-apk-production-final | 0 | commit 已等价进入 main | SAFE_TO_CLEAN_CANDIDATE |
| WT15 | E:/code/work/先知AI-ppt-agent-phase1-integration-20260806 | codex/ppt-agent-phase1-regression-integration | 0 | 大量独有 PPT Agent 代码 | 保留 |
| WT16 | E:/code/work/先知AI-seedance-r8-compliant-20260806 | codex/protect-seedance-r8-compliant-20260806 | 0 | 仍承载未审计分叉 | 保留 |
| WT17 | E:/code/work/先知AI-seedance-video-artifact-fix | codex/protect-seedance-video-artifact-fix-20260814 | 0 | 仍承载未审计分叉 | 保留 |
| WT18 | E:/code/work/先知AI-smartvideo-2.0.42 | codex/protect-smartvideo-mini-2.0.42-20260814 | 0 | 源码与 2,517 个构建文件混合 | 暂保留 ref；源码核对后倾向清理 |
| WT19 | E:/code/work/先知AI-video-bypass-release-f4b023e | codex/protect-smartvideo-grok-backend-20260814 | 0 | 与 main SmartVideo 架构分叉 | 保留到人工对比完成 |
| WT20 | E:/code/work/先知AI-video-compliance-release-tools | codex/video-compliance-release-tools-20260731 | 0 | 有独有回滚工具 | 暂保留 |
| WT21 | E:/code/work/先知AI-video-contract-2.0.36 | codex/video-model-params-2.0.36 | 0 | 功能大概率已由 main 覆盖 | 回归核对后条件清理 |

WT06 未跟踪文件：

- apps/user-uni/tests/harmony-output-path-contract.test.mjs
- docs/verification/safe-area/harmony-baseline-81a66692fc.md

## D. Functional Branch Map

| 功能域 | 分支 | 事实关系 |
|---|---|---|
| 邀请 / APK / 登录 | agent-invite-apk-production-readiness、release-blockers、production-final、miniprogram-agent-invite-autobind-production、video-ui-login | readiness 已合并；production-final patch-equivalent；release-blockers 被 main 替代但需确认；autobind 仍有独有工作；video-ui-login 大概率已被 8/2 主线集成覆盖 |
| Enterprise / 支付 | enterprise-v1、protect-enterprise-v1-20260814 | protect 是 enterprise-v1 的直接后继，新增双支付 gate；main 未包含其 10 个关键文件 |
| Identity / 安全 / 发布 | identity-phase2-1-security、identity-phase2-2-deployment-gates、identity-phase2-2-release-readiness、protect-identity... | phase2.2 两名称同 HEAD；protect 完整覆盖前序并新增 26 文件，是唯一应继续审计的主候选 |
| 视频模型 / Grok / 合规 | video-contract、video-model-params、login-compliance、video-ui-login、video-compliance-release-tools、grok-imagine-1.5-video | 多条链大部分已由 main 8/2～8/11 后续实现覆盖；rollback-image 工具仍独有 |
| Seedance / 视频产物 | protect-seedance-prod-minimal、prod-release、R8、video-artifact-fix | 四套并行实现，不存在单一后继；主要冲突集中在 generation_storage、openai_compatible、connector_generation、计费/估算 |
| SmartVideo / 自动混剪 | smartvideo-mini-2.0.42、protect-smartvideo-mini、protect-smartvideo-grok-backend | 基线已合并；两个保护分支分别是旧小程序产物快照和另一套后端架构，均不能整体进入 main |
| PPT Agent | ppt-agent-phase1-regression-integration | 14 个 patch-unique commits、31 个 branch-only 文件、迁移 106/107；tag 仍指向未合并历史 |
| AI 灵感 | feature/ai-inspiration | 在 main 的旧照片修复基础上扩展模板定义/组合与上传，仍有 18 个 branch-only 文件 |
| 运营中心 / 推广 | operation-center-invite-promotion、channel-ecosystem-v132-phase3 | channel 分支已合并；operation 分支有独有邀请归因与页面，但继承了旧 Grok commits |
| Safe Area | feature/cross-platform-safe-area | committed 内容目前仅两份设计/计划；实际 Harmony 验证仍在未跟踪文件 |
| 生产运维 | protect-main-production-ops-20260814 | patch 已等价进入 main，可列清理候选 |

## E. Duplicate / Superseded Branches

### 完全重复

1. codex/identity-phase2-2-deployment-gates 与 codex/identity-phase2-2-release-readiness：HEAD 都是 867cf82e...。
2. codex/login-compliance-2.0.38 与 codex/video-model-params-2.0.36：HEAD 都是 61ea0151...；前者名称与实际视频参数功能不一致。

### 被后续分支完整包含

1. enterprise-v1 → protect-enterprise-v1-20260814。
2. identity-phase2-1-security → 两个 phase2.2 同 HEAD 分支 → protect-identity...。
3. video-model-params → video-compliance-release-tools；后者只额外修改 DEPLOY.md、rollback-image.sh 和回滚契约测试。
4. grok-imagine-1.5-video → operation-center-invite-promotion，以及 → protect-smartvideo-grok-backend；两个后继分支错误继承了与自身主要功能无关的旧 Grok commits。

### 已完整进入 main 的高确定性候选

| 分支 | 证据 | 标签 |
|---|---|---|
| agent-invite-apk-production-readiness | tip 是 main 祖先 | SAFE_TO_CLEAN_CANDIDATE |
| channel-ecosystem-v132-phase3 | tip 是 main 祖先 | SAFE_TO_CLEAN_CANDIDATE |
| seedance-video-artifact-fix | tip 是 main 祖先 | SAFE_TO_CLEAN_CANDIDATE |
| smartvideo-mini-2.0.42 | tip 是 main 祖先 | SAFE_TO_CLEAN_CANDIDATE |
| video-contract-2.0.36 | tip 是 main 祖先 | SAFE_TO_CLEAN_CANDIDATE |
| agent-invite-apk-production-final | 唯一 commit patch-equivalent 于 main | SAFE_TO_CLEAN_CANDIDATE |
| protect-main-production-ops-20260814 | 唯一 commit patch-equivalent 于 main | SAFE_TO_CLEAN_CANDIDATE |

### 大概率被替代、但删除前必须确认

- agent-invite-apk-release-blockers：main 已有邀请发布实现，但 2 commits 不是 patch-equivalent，且存在两个 add/add 文件。
- grok-imagine-1.5-video：main 已有更新的 per-second、Preview、默认选择、定价排序实现；旧分支仍有一份测试/计划未进入 main。
- video-model-params、login-compliance、video-ui-login：文件范围 100% 被 main 后续修改，且 main 有同主题提交；仍需按 protected surfaces 做行为验证。
- protect-smartvideo-mini：大部分独有文件是编译/验收产物，源码属于旧 2.0.42 实现；删除前必须把约 37 个源码/测试/工具文件与 main 当前 SmartVideo 逐项核对。

## F. Unmerged Valuable Work

以下为仍有业务或工程价值、且尚未完整进入 main 的工作，按优先级列出；不能用“branch commit 更多”代替内容判断。

| 优先级 | 分支 / 分支组 | main 缺失的有效内容 | 下一步 |
|---:|---|---|---|
| P0 | protect-identity-phase2-2-deployment-gates-20260814 | 36 个 branch-only 文件：迁移 072/073、共享登录限流、release readiness、CI、HAProxy、环境校验 | 先做安全与迁移审计；再在受控分支同步 main、跑身份/发布门禁；未批准前不得 merge |
| P0 | protect-enterprise-v1-20260814 | enterprise contract、pricing health、双支付 gate、payment service tests，共 10 个 branch-only 文件 | 核对现行微信虚拟支付与统一 GrantOrderEntitlements 约束；先审计幂等/账本/事务，再测试 |
| P0 | 四个 protect-seedance 分支 | artifact admission、storage、Connector、R8、fail-closed billing、thumbnail 等并行实现 | 建立“能力 × 分支”矩阵，逐文件选择唯一实现；禁止四支互相直接合并 |
| P0 | protect-smartvideo-grok-backend-20260814 | storyboard、render dispatcher、media provider、OBS 运维工具等 26 个 branch-only 文件 | 与 main 的 auto-montage V1 做架构对照；剔除 exe/验收图片后只选择仍缺能力 |
| P0 | protect-smartvideo-mini-2.0.42-20260814 | 旧小程序 SmartVideo 源码/测试/发布工具与大量产物混合 | 先分离约 37 个源码/工具候选和 2,517 个 build 文件；只审计前者 |
| P1 | ppt-agent-phase1-regression-integration | 31 个 branch-only 文件、14 commits；Postgres Agent、租约、管理端、迁移 106/107、DeepSeek 计费 | 先验证 tag 对应版本、数据库迁移、计费和 Connector 隔离；再同步 main |
| P1 | feature/ai-inspiration | 18 个 branch-only 文件；template definition/compose、上传表单、迁移 108 | 恢复旧 photo-restoration 测试保护面后做端到端验证，再同步 |
| P1 | miniprogram-agent-invite-autobind-production | 扫码绑定、kill switch、H5/小程序 gate、独有 referral test | 对照 main 当前邀请/登录实现做行为审计，避免重复注册或错误归因 |
| P1 | operation-center-invite-promotion | invite_attribution、OperationPromotionPage、RBAC/推广测试 | 从旧 Grok 基线上拆出 41cdf9a 的运营增量，单独审计 |
| P2 | video-compliance-release-tools-20260731 | rollback-image.sh 与 tests/release/rollback-image-contract.ps1 | 判断是否仍适配现行 deploy.sh / 镜像发布方式；若适用再选择性迁移 |
| P2 | feature/cross-platform-safe-area | 已提交的跨平台设计/计划；WT06 中未跟踪 Harmony test/baseline | 先人工保护并确认未跟踪文件，再判断是否进入正式实现 |

## G. Main Health Review

### 结论

main 是当前最合理的新功能基线，理由如下：

1. 本地 main、origin/main、gitee/main 的 cached ref 完全一致，均为 6ee5b36f...；主 worktree 在报告写入前干净。
2. main 已包含较新的自动混剪、Grok/Seedance 定价与模型选择、视频下载 mp4、邀请发布、AI 生图和生产磁盘 guardrail；不能因为旧功能分支的 commit 数更多就判定 main 落后。
3. 当前未发现一个“已完成、已完整验证、且可以无条件整体合并”的重要分支。缺失工作大多带有 WIP、未验证迁移、账务/身份高风险或并行架构分叉。

### main 明确缺少但尚不能直接接入的内容

- Identity 2.1/2.2 安全与发布 gates。
- Enterprise V1 双支付 gate。
- PPT Agent Phase 1。
- AI 灵感模板定义扩展。
- 邀请自动绑定与运营中心邀请归因。
- Seedance 与 SmartVideo 分支中的若干候选能力。

### 冲突热点

在建议保留的 13 个分支中，最常被多个分支修改的文件：

| 分支命中数 | 文件 |
|---:|---|
| 8 | backend-go/internal/httpserver/postgres_store.go |
| 7 | backend-go/internal/provider/video/openai_compatible.go |
| 7 | backend-go/internal/httpserver/api.go |
| 7 | backend-go/internal/httpserver/server.go |
| 6 | backend-go/internal/httpserver/generation_storage.go / generation_storage_test.go |
| 5 | backend-go/internal/httpserver/connector_generation.go |
| 5 | backend-go/internal/httpserver/ai_capability.go |
| 5 | backend-go/internal/httpserver/video_generation_estimate.go |
| 5 | backend-go/internal/httpserver/video_generation_validation.go |
| 5 | apps/user-uni/src/components/MiniProgramRoleWorkbench.vue |

main 自 2026-07-22 以来的高频变更文件也高度重合：MiniProgramRoleWorkbench.vue 38 次、ai_capability.go 21 次、api.go 21 次、server.go 16 次、postgres_store.go 16 次、business-sdk mappers 15 次。这些是后续同步和重构的真实热点。

### 历史与发布标记

仓库只有 3 个 tag：

- ai-workbench-20260813：已在 main。
- v1.3.2-rc1：已在 main。
- ppt-deepseek-billing-29d2a8f6f：不在 main，指向 PPT 分支的 29d2a8f6...；相对 main 为 B108/A10。

因此当前 tag 同时承担“发布标记”和“保护未合并历史”两种职责，语义不统一。

## H. Repository Optimization Opportunities

本节只提出建议，不实施。

1. **拆分后端冲突热点。** api.go、server.go、postgres_store.go、store.go 同时承载路由、依赖装配、数据访问和多业务注册。后续按 capability / bounded context 拆注册器与 store，优先从 Identity、PPT、Connector、Video Artifact 四个域开始。
2. **收敛视频能力契约。** openai_compatible.go、generation_storage.go、video_generation_estimate/validation.go 被多套 Seedance/Grok/SmartVideo 实现重复修改。应先确定一个统一模型 capability、artifact admission 与 billing fail-closed 契约，再接 Provider。
3. **降低 MiniProgramRoleWorkbench.vue 冲突密度。** 该文件近期 38 次变更且被 5 个保留分支修改。后续把导航/入口、视频、灵感、PPT、SmartVideo 拆成独立 feature components，但必须保持 protected surfaces。
4. **禁止把构建产物混入功能分支。** artifacts/builds、wechat-upload、runtime evidence 应进入制品库或受控 release archive；源码分支只保留可复现脚本、manifest 和必要小型证据索引。
5. **建立分支生命周期字段。** 每个分支至少登记 owner、功能域、base SHA、创建时间、最后同步时间、验证状态、替代分支、计划清理日期。
6. **建立 worktree 台账。** 一个活跃功能只保留一个主 worktree；同 HEAD worktree、已合并分支 worktree、无 owner worktree进入周期性人工清理列表。
7. **双远端治理。** 明确 origin 与 gitee 谁是 source of truth；镜像任务应检测 ref 差异，而不是让本地分支分别跟踪不同远端。
8. **CI 自动只读审计。** 周期性输出 branch age、ahead/behind、merged ancestor、patch-equivalent、worktree dirty、无 upstream、构建产物数量和 tag-not-in-main；自动报告，不自动删除。
9. **发布/tag 规范。** release tag 只指向已批准的集成/发布 commit；若要保护未合并里程碑，使用明确的 archive/ 或 checkpoint/ tag 命名，避免与发布版本混淆。
10. **分支命名反映真实功能。** login-compliance 实际指向 video-model HEAD，是明显治理异味；protect-* 只应是短期抢救分支，完成审计后转为清晰功能分支或清理。

## 治理顺序

### 立即处理

本轮仍不执行，以下动作都等待人工批准：

1. **保护 WT06 的两个未跟踪文件。** 先确认归属和价值，避免后续清理 Safe Area worktree 时丢失。
2. **将 7 个高确定性分支列入清理工单。** production-readiness、channel-ecosystem、seedance-video-artifact-fix 基线、smartvideo-mini-2.0.42 基线、video-contract、production-final、protect-main-production-ops。
3. **冻结整体合并。** protect-smartvideo-mini 与四个 protect-seedance 分支在完成人工差异审计前不得整体 merge/cherry-pick。
4. **记录远端校验缺口。** 下一轮如获批准，先执行不会 prune 的 fetch/remote inventory，再确认远端清理目标仍存在。

### 近期处理

1. 第一优先：protect-identity...；做安全、迁移、限流和部署 gate 审计。
2. 第二优先：四个 Seedance 分支；建立能力矩阵并选出唯一实现。
3. 第三优先：protect-enterprise；对齐微信虚拟支付、账本、事务和幂等约束。
4. 第四优先：PPT Agent；验证 14 commits、迁移 106/107、计费、Connector 会话隔离和 tag。
5. 第五优先：SmartVideo 两个保护分支；把源码能力与构建/验收产物分开。
6. 随后处理 AI 灵感、邀请 autobind、运营中心归因、video rollback 工具。
7. 对 video-model/login/video-ui 与 agent-invite-release-blockers 做 protected-surface / 行为核对，确认后再转清理候选。

### 长期优化

1. 分支最长生命周期和最大 behind 阈值；超过阈值自动告警。
2. worktree owner、用途、最后活动时间和清理审批规范。
3. 功能域分支命名及禁止同 HEAD 多名称规则。
4. release tag、archive tag 与生产部署 SHA 的一致性策略。
5. 定期只读 CI 审计与双远端镜像差异报告。
6. 对 Go HTTP 装配/store、视频 Provider/Artifact/Billing、uni-app 大组件实施按域重构。

## 明确结论

**BRANCH GOVERNANCE STATUS: HIGH_RISK**

下一轮推荐动作：

1. **先处理 codex/protect-identity-phase2-2-deployment-gates-20260814。**
2. 原因：它落后 main 215 commits，涉及身份、鉴权、限流、数据库迁移和部署 gates，且仍有 36 个 main 不存在的文件；错误处理的影响面高于其它分支。
3. 下一步应先做只读代码/迁移/安全审计；人工批准后再创建受控同步方案和测试清单。当前不建议直接 merge。
4. fetch、同步 main、rebase/merge/cherry-pick、运行可能写缓存的测试、删除任何分支、删除任何 worktree、调整远端或 tag，全部必须等待人工明确批准。

本轮到此结束，不进入清理、同步、测试或合并阶段。
