# 知启云AI渠道生态中心 V1.3.2 运营中心一期上线前操作手册

## 1. 上线边界

- 本批只接入运营中心技术服务费订单的支付后审核入口。
- 会员、代理、Legacy、Shadow、Canary保持原流程。
- 运营中心仍禁止加入Canary和真实白名单。
- 微信虚拟支付不支持主动退款时必须进入人工退款，不得伪造自动退款成功。
- 三类后台调度默认关闭，生产上线前不得自动打开。

## 2. 迁移与备份

1. 分别备份生产数据库结构和业务数据。
2. 在隔离测试数据库验证schema.sql与002至096全量迁移。
3. 在生产同版本备份副本验证095至096升级。
4. 核对089至096的运营中心服务、审核、推荐资格、奖励、钱包、释放、冲正和退款任务约束。
5. 生产迁移必须单独审批，本批不执行生产迁移。

## 3. 运行时配置

生产使用XIANZHI_PRODUCTION_OPERATION_CENTER_前缀，测试使用XIANZHI_TEST_OPERATION_CENTER_前缀，禁止混用。

初始安全值：

- REFUND_RETRY_SCHEDULER_ENABLED=false
- REFUND_VERIFICATION_SCHEDULER_ENABLED=false
- REFERRAL_REWARD_RELEASE_SCHEDULER_ENABLED=false
- MANUAL_REFUND_AUTO_APPROVAL=false
- DRY_RUN=false

必须配置并复核批量大小、最大尝试次数、租约、临时失败重试间隔、UNKNOWN安全等待期、查询间隔和三个调度轮询间隔。缺失Provider映射时，启用退款或核验调度必须启动失败。

## 4. 首单门禁

1. 保持所有调度关闭。
2. 使用非生产租户完成一笔运营中心技术服务费支付。
3. 确认订单为PAID且履约为REVIEW_REQUIRED。
4. 确认未激活身份、档案和RBAC，未生成推荐奖励及钱包流水。
5. 使用运营审核账号批准，确认ACTIVE、RBAC、推荐资格、冻结奖励、冻结钱包流水和释放任务同事务完成。
6. 重复支付回调和重复审核，确认所有记录均不重复。

## 5. 人工退款双人复核

1. WECHAT_VIRTUAL主动退款返回UNSUPPORTED后，任务必须进入MANUAL_REQUIRED。
2. financial_submitter提交渠道流水号、凭证引用和文件哈希。
3. financial_approver使用另一账号审批。
4. 提交人不得审批，审批权限不得继承运营审核权限。
5. SUCCEEDED必须保留人工退款证据和两名操作人的审计记录。

## 6. 调度启用顺序

1. 先保持全部关闭，确认不会领取任务。
2. 使用Dry-run观察统计，确认不改状态、不持有租约、不调用Provider。
3. 先启用UNKNOWN核验调度。
4. 稳定后启用退款重试调度。
5. 最后启用奖励释放调度。
6. 任一异常立即关闭对应开关；已领取任务依租约安全结束或过期恢复。

## 7. 监控与停机阈值

- 监控REVIEW_REQUIRED积压、REFUND_RETRYABLE积压、UNKNOWN_VERIFYING时长、MANUAL_REQUIRED积压和释放失败数。
- 监控同一Provider退款号重复响应、钱包负余额、资金守恒差异、RBAC撤销失败和Legacy投影写入。
- 出现重复退款、已退款未撤权、奖励部分冲正、钱包不守恒或规则快照漂移时，立即关闭全部调度并停止运营中心新订单入口。
- 关闭调度只影响新任务领取；已经固化的历史规则、订单快照和退款号不得重建。

## 8. 回滚

- 应用回滚不得把已进入V1.3.2运营中心流程的订单迁回Legacy。
- 关闭运营中心新入口、退款重试、UNKNOWN核验和奖励释放开关。
- 保留REVIEW_REQUIRED、REVOKED、退款任务、奖励和钱包审计数据，禁止人工删除。
- 已撤权运营中心不得因Provider失败恢复ACTIVE。
- 恢复前必须完成数据库、状态、资金、权限和Provider证据核验。
