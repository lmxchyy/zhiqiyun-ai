package httpserver

import (
	"encoding/json"
	"strings"
)

func (s *postgresStore) ListAdminIdentityConsistency(actorID, actorRole string, filter identityConsistencyFilter) ([]identityConsistencyIssue, error) {
	if strings.TrimSpace(actorID) == "" || !identityChangeAdminRoleAllowed(actorRole) {
		return nil, errIdentityPermission
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	allowed, err := s.roleHasPermission(ctx, strings.ToUpper(strings.TrimSpace(actorRole)), "identity:consistency:view")
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errIdentityPermission
	}
	rows, err := s.db.QueryContext(ctx, identityConsistencySQL, filter.Code, filter.Severity, filter.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]identityConsistencyIssue, 0)
	for rows.Next() {
		var item identityConsistencyIssue
		var raw []byte
		if err := rows.Scan(&item.Code, &item.Severity, &item.UserID, &item.EntityID, &item.Message, &item.SuggestedAction, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &item.Details)
		items = append(items, item)
	}
	return items, rows.Err()
}

const identityConsistencySQL = `
WITH issues(code,severity,user_id,entity_id,message,suggested_action,details) AS (
  SELECT 'ACTIVE_AGENT_WITHOUT_PROFILE','CRITICAL',identity.user_id,identity.id,'有效代理商身份缺少有效档案','通过单用户身份命令补建或终止异常身份','{}'::jsonb
  FROM xz_user_business_identities identity WHERE identity.identity_type='AGENT' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL
    AND NOT EXISTS(SELECT 1 FROM xz_channel_agents profile WHERE profile.user_id=identity.user_id AND upper(coalesce(profile.status,''))='ACTIVE')
  UNION ALL
  SELECT 'ACTIVE_AGENT_WITHOUT_RBAC','CRITICAL',identity.user_id,identity.id,'有效代理商身份缺少AGENT角色','运行单用户RBAC一致性同步','{}'::jsonb
  FROM xz_user_business_identities identity WHERE identity.identity_type='AGENT' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL
    AND NOT EXISTS(SELECT 1 FROM xz_user_roles role WHERE role.user_id=identity.user_id AND role.role='AGENT' AND upper(role.status)='ACTIVE')
  UNION ALL
  SELECT 'ACTIVE_AGENT_WITH_LEGACY_ROLE_ONLY','HIGH',account.id,account.id,'仅legacy role显示为代理商但没有有效商业身份','核对来源后通过身份命令迁移',jsonb_build_object('legacyRole',account.role)
  FROM xz_users account WHERE upper(coalesce(account.role,'')) LIKE 'AGENT%'
    AND NOT EXISTS(SELECT 1 FROM xz_user_business_identities identity WHERE identity.user_id=account.id AND identity.identity_type='AGENT' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL)
  UNION ALL
  SELECT 'ACTIVE_OPERATION_WITHOUT_PROFILE','CRITICAL',identity.user_id,identity.id,'有效运营中心身份缺少有效档案','通过单用户身份命令补建或终止异常身份','{}'::jsonb
  FROM xz_user_business_identities identity WHERE identity.identity_type='OPERATION_CENTER' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL
    AND NOT EXISTS(SELECT 1 FROM xz_operation_centers profile WHERE profile.user_id=identity.user_id AND upper(coalesce(profile.status,''))='ACTIVE')
  UNION ALL
  SELECT 'ACTIVE_OPERATION_WITHOUT_RBAC','CRITICAL',identity.user_id,identity.id,'有效运营中心身份缺少OPERATION角色','运行单用户RBAC一致性同步','{}'::jsonb
  FROM xz_user_business_identities identity WHERE identity.identity_type='OPERATION_CENTER' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL
    AND NOT EXISTS(SELECT 1 FROM xz_user_roles role WHERE role.user_id=identity.user_id AND role.role='OPERATION' AND upper(role.status)='ACTIVE')
  UNION ALL
  SELECT 'TERMINATED_IDENTITY_WITH_ACTIVE_ROLE','CRITICAL',identity.user_id,identity.id,'已终止身份仍保留有效商业角色','停用商业角色并回退当前角色',jsonb_build_object('identityType',identity.identity_type)
  FROM xz_user_business_identities identity JOIN xz_user_roles role ON role.user_id=identity.user_id AND role.role=CASE identity.identity_type WHEN 'AGENT' THEN 'AGENT' WHEN 'OPERATION_CENTER' THEN 'OPERATION' END AND upper(role.status)='ACTIVE'
  WHERE identity.identity_status='TERMINATED' AND NOT EXISTS(
    SELECT 1 FROM xz_user_business_identities current_identity
    WHERE current_identity.user_id=identity.user_id AND current_identity.identity_type=identity.identity_type
      AND current_identity.identity_status='ACTIVE' AND current_identity.ended_at IS NULL
  )
  UNION ALL
  SELECT 'FROZEN_IDENTITY_WITH_ACTIVE_WORKSPACE_ACCESS','CRITICAL',identity.user_id,identity.id,'冻结身份仍具备工作台角色或有效档案','停用档案和RBAC工作台访问',jsonb_build_object('identityType',identity.identity_type)
  FROM xz_user_business_identities identity WHERE identity.identity_status='FROZEN' AND identity.identity_type IN ('AGENT','OPERATION_CENTER') AND identity.ended_at IS NULL AND (
    EXISTS(SELECT 1 FROM xz_user_roles role WHERE role.user_id=identity.user_id AND role.role=CASE identity.identity_type WHEN 'AGENT' THEN 'AGENT' ELSE 'OPERATION' END AND upper(role.status)='ACTIVE') OR
    (identity.identity_type='AGENT' AND EXISTS(SELECT 1 FROM xz_channel_agents profile WHERE profile.user_id=identity.user_id AND upper(profile.status)='ACTIVE')) OR
    (identity.identity_type='OPERATION_CENTER' AND EXISTS(SELECT 1 FROM xz_operation_centers profile WHERE profile.user_id=identity.user_id AND upper(profile.status)='ACTIVE')))
  UNION ALL
  SELECT 'PROFILE_ACTIVE_WITHOUT_BUSINESS_IDENTITY','CRITICAL',profile.user_id,profile.id,'有效代理商档案没有有效商业身份','核对订单后补建身份或终止档案',jsonb_build_object('profileType','AGENT') FROM xz_channel_agents profile
  WHERE upper(coalesce(profile.status,''))='ACTIVE' AND NOT EXISTS(SELECT 1 FROM xz_user_business_identities identity WHERE identity.user_id=profile.user_id AND identity.identity_type='AGENT' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL)
  UNION ALL
  SELECT 'PROFILE_ACTIVE_WITHOUT_BUSINESS_IDENTITY','CRITICAL',profile.user_id,profile.id,'有效运营中心档案没有有效商业身份','核对订单后补建身份或终止档案',jsonb_build_object('profileType','OPERATION_CENTER') FROM xz_operation_centers profile
  WHERE upper(coalesce(profile.status,''))='ACTIVE' AND NOT EXISTS(SELECT 1 FROM xz_user_business_identities identity WHERE identity.user_id=profile.user_id AND identity.identity_type='OPERATION_CENTER' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL)
  UNION ALL
  SELECT 'MULTIPLE_CURRENT_CHANNEL_IDENTITIES','CRITICAL',identity.user_id,identity.user_id,'用户存在多个当前渠道商业身份','人工核对后终止多余身份',jsonb_build_object('count',count(*)) FROM xz_user_business_identities identity
  WHERE identity.identity_type IN ('AGENT','OPERATION_CENTER') AND identity.identity_status IN ('PENDING','ACTIVE','FROZEN') AND identity.ended_at IS NULL GROUP BY identity.user_id HAVING count(*)>1
  UNION ALL
  SELECT 'MULTIPLE_CURRENT_RELATIONSHIPS','CRITICAL',relation.user_id,relation.user_id,'用户存在多个当前关系','结束多余关系并保留历史',jsonb_build_object('count',count(*)) FROM xz_user_relationships relation WHERE relation.status='ACTIVE' AND relation.ended_at IS NULL GROUP BY relation.user_id HAVING count(*)>1
  UNION ALL
  SELECT 'RELATION_PARENT_INACTIVE','HIGH',relation.user_id,relation.id,'当前关系指向无效代理商','选择有效上级并创建新关系','{}'::jsonb FROM xz_user_relationships relation
  WHERE relation.status='ACTIVE' AND relation.ended_at IS NULL AND relation.parent_agent_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM xz_channel_agents profile JOIN xz_user_business_identities identity ON identity.user_id=profile.user_id AND identity.identity_type='AGENT' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL WHERE profile.id=relation.parent_agent_id AND upper(profile.status)='ACTIVE')
  UNION ALL
  SELECT 'RELATION_CENTER_INACTIVE','HIGH',relation.user_id,relation.id,'当前关系指向无效运营中心','选择有效运营中心并创建新关系','{}'::jsonb FROM xz_user_relationships relation
  WHERE relation.status='ACTIVE' AND relation.ended_at IS NULL AND relation.operation_center_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM xz_operation_centers profile JOIN xz_user_business_identities identity ON identity.user_id=profile.user_id AND identity.identity_type='OPERATION_CENTER' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL WHERE profile.id=relation.operation_center_id AND upper(profile.status)='ACTIVE')
  UNION ALL
  SELECT 'RELATION_PARENT_CENTER_MISMATCH','HIGH',relation.user_id,relation.id,'上级代理商与运营中心归属不一致','按上级代理商自动推导运营中心',jsonb_build_object('relationCenterId',relation.operation_center_id,'parentCenterId',coalesce(parent_relation.operation_center_id,parent.operation_center_id))
  FROM xz_user_relationships relation JOIN xz_channel_agents parent ON parent.id=relation.parent_agent_id LEFT JOIN xz_user_relationships parent_relation ON parent_relation.user_id=parent.user_id AND parent_relation.status='ACTIVE' AND parent_relation.ended_at IS NULL
  WHERE relation.status='ACTIVE' AND relation.ended_at IS NULL AND nullif(relation.operation_center_id,'') IS DISTINCT FROM nullif(coalesce(parent_relation.operation_center_id,parent.operation_center_id),'')
  UNION ALL
  SELECT 'CURRENT_ROLE_NOT_ASSIGNED','CRITICAL',context.user_id,context.user_id,'当前工作台角色没有有效绑定','回退USER或重新激活正确角色',jsonb_build_object('currentRole',context.current_role_code) FROM xz_user_role_context context
  WHERE NOT EXISTS(SELECT 1 FROM xz_user_roles role WHERE role.user_id=context.user_id AND role.role=context.current_role_code AND upper(role.status)='ACTIVE')
  UNION ALL
  SELECT 'LEGACY_REFERRED_BY_MISMATCH','MEDIUM',account.id,account.id,'legacy referred_by与当前关系不一致','以xz_user_relationships为准并保留兼容字段',jsonb_build_object('referredBy',account.referred_by,'relationParentAgentId',relation.parent_agent_id)
  FROM xz_users account LEFT JOIN xz_channel_agents legacy_parent ON legacy_parent.user_id=account.referred_by LEFT JOIN xz_user_relationships relation ON relation.user_id=account.id AND relation.status='ACTIVE' AND relation.ended_at IS NULL
  WHERE nullif(account.referred_by,'') IS NOT NULL AND relation.parent_agent_id IS DISTINCT FROM legacy_parent.id
  UNION ALL
  SELECT 'IDENTITY_ORDER_NOT_FOUND','HIGH',identity.user_id,identity.id,'订单来源身份找不到来源订单','核对身份来源和订单号',jsonb_build_object('sourceOrderId',identity.source_order_id) FROM xz_user_business_identities identity
  WHERE identity.source_type IN ('OFFLINE_ORDER','PACKAGE_CONVERSION','COMMERCE_ORDER') AND nullif(identity.source_order_id,'') IS NOT NULL AND NOT EXISTS(SELECT 1 FROM xz_orders orders WHERE orders.id=identity.source_order_id)
  UNION ALL
  SELECT 'IDENTITY_ORDER_NOT_FULFILLED','HIGH',identity.user_id,identity.id,'订单来源身份对应订单尚未履约','核对订单履约状态，禁止重复创建订单',jsonb_build_object('sourceOrderId',identity.source_order_id) FROM xz_user_business_identities identity JOIN xz_orders orders ON orders.id=identity.source_order_id
  WHERE identity.source_type IN ('OFFLINE_ORDER','PACKAGE_CONVERSION','COMMERCE_ORDER') AND upper(coalesce(orders.fulfillment_status,orders.price_snapshot->>'fulfillmentStatus',''))<>'FULFILLED'
)
SELECT code,severity,user_id,coalesce(entity_id,''),message,suggested_action,details FROM issues
WHERE ($1='' OR code=$1) AND ($2='' OR severity=$2) AND ($3='' OR user_id=$3)
ORDER BY CASE severity WHEN 'CRITICAL' THEN 0 WHEN 'HIGH' THEN 1 WHEN 'MEDIUM' THEN 2 ELSE 3 END,code,user_id LIMIT 500`
