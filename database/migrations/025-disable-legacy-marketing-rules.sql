-- Disable obsolete pre-L0-L5 marketing rules while preserving audit history.
update xz_marketing_commission_rules
set status = 'DISABLED',
    updated_at = now()
where id in ('rule_center_referral', 'rule_sales_direct', 'rule_sales_indirect');
