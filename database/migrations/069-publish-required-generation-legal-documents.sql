-- Publish the minimum legal documents required before mini-program generation.
-- Content can later be versioned and replaced from the compliance admin page.
INSERT INTO xz_legal_documents (
  id, code, title, version, content, status, published_at, created_at, updated_at
)
VALUES
  (
    'legal_user_agreement_20260722',
    'user-agreement',
    '用户服务协议',
    '2026-07-22',
    E'欢迎使用知启云AI。使用本服务前，请确认您具有相应民事行为能力，并遵守法律法规及平台规则。您应对输入内容、上传素材及生成结果的合法使用负责，不得利用本服务制作、传播违法违规、侵权、虚假或危害他人权益的内容。平台会依据产品规则提供人工智能创作、作品保存及相关服务，并可能为保障安全、计费和服务质量记录必要的操作信息。具体功能、费用和权益以页面展示及实际订单为准。',
    'PUBLISHED', now(), now(), now()
  ),
  (
    'legal_privacy_policy_20260722',
    'privacy-policy',
    '隐私政策',
    '2026-07-22',
    E'知启云AI仅在实现登录、身份识别、内容生成、作品管理、安全审核和客户服务所必需的范围内处理个人信息。可能处理的信息包括微信登录标识、您授权的头像昵称或手机号、设备与网络信息、创作输入和上传素材。未经您的授权或法律法规要求，平台不会向无关第三方披露个人信息。您可以在“我的-设置”中查询账号信息、管理授权或申请注销账号。请勿在提示词或素材中提交不必要的敏感个人信息。',
    'PUBLISHED', now(), now(), now()
  ),
  (
    'legal_ai_content_rules_20260722',
    'ai-content-rules',
    'AI生成内容使用规范',
    '2026-07-22',
    E'使用AI生图、视频、PPT及其他生成能力时，不得输入或生成违反法律法规、侵害知识产权和人格权益、泄露个人隐私、冒充他人、实施欺诈或误导公众的内容。请确保上传素材具有合法来源和使用授权。AI生成结果可能存在错误或偏差，发布、商用或用于重要决策前应由您核验。平台将依法进行内容安全审核，并对生成内容展示AI标识；违规内容可能被拒绝生成、限制展示或删除。',
    'PUBLISHED', now(), now(), now()
  )
ON CONFLICT (code, version) DO UPDATE SET
  title = EXCLUDED.title,
  content = EXCLUDED.content,
  status = 'PUBLISHED',
  published_at = COALESCE(xz_legal_documents.published_at, now()),
  updated_at = now();
