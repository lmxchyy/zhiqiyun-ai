-- Publish the commercial agreements linked from user purchase and enterprise flows.
-- Later revisions remain editable and versioned through the compliance admin page.
INSERT INTO xz_legal_documents (
  id, code, title, version, content, status, published_at, created_at, updated_at
)
VALUES
  (
    'legal_member_service_agreement_20260723',
    'member-service-agreement',
    '知启云AI会员服务协议',
    '2026-07-23',
    E'欢迎开通知启云AI会员服务。会员商品的价格、有效期、赠送点数和具体权益以订单确认页展示及服务端订单快照为准。支付成功且服务端确认后，相应会员权益将发放至当前账号。会员权益仅限账号本人依照平台规则使用，不得转售、出租、共享账号或用于违法违规活动。因用户主动取消、账号违规处置或法律法规另有要求产生的退款与权益处理，按照页面说明、平台退款规则及适用法律执行。AI生成结果可能存在误差，用户在发布或商用前应自行核验并确保拥有必要权利。',
    'PUBLISHED', now(), now(), now()
  ),
  (
    'legal_agent_service_agreement_20260723',
    'agent-service-agreement',
    '知启云AI代理商服务协议',
    '2026-07-23',
    E'开通知启云AI代理商服务前，请确认您具备相应民事行为能力和经营资质。代理身份、有效期、推广权限、返佣范围及结算条件以订单页面、代理商规则和服务端记录为准。代理商不得作虚假承诺、擅自变更平台价格与权益、冒用平台名义签约，或通过刷单、诱导、侵权等方式获取收益。返佣仅对符合规则且最终有效的业务订单计算，退款、撤销、风控订单及违规交易不计入或依法冲回。代理商应依法处理客户信息并承担其推广行为产生的责任。',
    'PUBLISHED', now(), now(), now()
  ),
  (
    'legal_enterprise_space_service_agreement_20260723',
    'enterprise-space-service-agreement',
    '企业空间服务协议',
    '2026-07-23',
    E'创建或加入企业空间即表示您同意按照企业授权和平台规则使用相关服务。企业创建者和管理员负责成员邀请、角色权限、企业资料及空间内数据管理，并应确保已取得上传、共享和处理相关内容所需的合法授权。企业空间与个人空间的数据和权限相互隔离；成员退出、被移除或企业服务终止后，其访问权限将依规则调整。企业不得利用空间从事违法违规、侵权、泄密或超越授权范围的活动。',
    'PUBLISHED', now(), now(), now()
  ),
  (
    'legal_recharge_service_agreement_20260723',
    'recharge-service-agreement',
    '点数充值服务协议',
    '2026-07-23',
    E'点数是用于兑换知启云AI平台内指定服务的虚拟权益，不属于法定货币，不得提现、转让或用于平台外交易。充值金额、到账点数、有效期和适用范围以订单确认页及服务端商品配置为准。支付成功且服务端确认后点数发放至当前账号；实际消耗以具体模型、规格和生成任务页面公示的计费规则为准。因系统故障导致重复扣减或未到账的，平台将依据订单和账本记录核查处理。',
    'PUBLISHED', now(), now(), now()
  )
ON CONFLICT (code, version) DO UPDATE SET
  title = EXCLUDED.title,
  content = EXCLUDED.content,
  status = 'PUBLISHED',
  published_at = COALESCE(xz_legal_documents.published_at, now()),
  updated_at = now();
