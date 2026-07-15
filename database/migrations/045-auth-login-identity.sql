-- 知启云AI：小程序统一登录身份约束。
-- 手机号是注册主身份；UnionID 在可用时用于跨微信应用身份归并。

BEGIN;

ALTER TABLE xz_users
  ADD COLUMN IF NOT EXISTS mobile TEXT,
  ADD COLUMN IF NOT EXISTS wechat_union_id TEXT;

UPDATE xz_users
SET mobile = NULLIF(BTRIM(raw ->> 'mobile'), '')
WHERE NULLIF(BTRIM(mobile), '') IS NULL
  AND NULLIF(BTRIM(raw ->> 'mobile'), '') IS NOT NULL;

UPDATE xz_users
SET wechat_union_id = NULLIF(BTRIM(raw ->> 'wechatUnionId'), '')
WHERE NULLIF(BTRIM(wechat_union_id), '') IS NULL
  AND NULLIF(BTRIM(raw ->> 'wechatUnionId'), '') IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS xz_users_mobile_unique
  ON xz_users (mobile)
  WHERE NULLIF(BTRIM(mobile), '') IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS xz_users_wechat_union_id_unique
  ON xz_users (wechat_union_id)
  WHERE NULLIF(BTRIM(wechat_union_id), '') IS NOT NULL;

COMMENT ON COLUMN xz_users.mobile IS
  'Normalized mainland China mobile used as the primary mini-program login identity.';
COMMENT ON COLUMN xz_users.wechat_union_id IS
  'WeChat UnionID used for cross-application identity reconciliation when available.';

COMMIT;
