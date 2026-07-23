-- Keep business product codes independent from the product IDs published in WeChat.
-- Migration 064 renamed the business codes, but the published WeChat goods retain
-- their original IDs. Re-running this migration is safe.

UPDATE xz_wechat_virtual_product_mappings
SET wechat_product_id = CASE plan_id
      WHEN 'plan_ai_creator_996' THEN 'MEMBER_YEAR_996'
      WHEN 'plan_agent_join_996' THEN 'AGENT_JOIN_996'
    END,
    updated_at = now()
WHERE plan_id IN ('plan_ai_creator_996', 'plan_agent_join_996')
  AND wechat_product_id IS DISTINCT FROM CASE plan_id
        WHEN 'plan_ai_creator_996' THEN 'MEMBER_YEAR_996'
        WHEN 'plan_agent_join_996' THEN 'AGENT_JOIN_996'
      END;
