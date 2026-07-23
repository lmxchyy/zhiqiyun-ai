<template>
  <EnterprisePageShell
    :title="pageTitle"
    :action-label="pageActionLabel"
    :action-disabled="submitting"
    :fixed-action="hasFixedAction"
    @action="handlePageAction()"
  >
    <EnterpriseStatePanel v-if="loading" kind="loading" />
    <EnterpriseStatePanel v-else-if="forbidden" kind="forbidden" action-label="返回企业中心" @action="openEntry()" />
    <EnterpriseStatePanel v-else-if="loadError" kind="error" :copy="loadError" action-label="重新加载" @action="load()" />

    <template v-else-if="screen === 'onboarding'">
      <view class="enterprise-hero">
        <view class="enterprise-hero-illustration"><view class="enterprise-hero-icon">企</view></view>
        <text class="enterprise-hero-title">创建你的企业AI工作空间</text>
        <text class="enterprise-hero-copy">统一管理成员、知识库、AI员工、作品和企业算力</text>
      </view>
      <button class="enterprise-primary-button" type="button" hover-class="enterprise-pressed" @click="openPage(pages.create)">创建企业</button>
      <view class="enterprise-button-gap" />
      <button class="enterprise-secondary-button" type="button" hover-class="enterprise-pressed" @click="openPage(pages.join)">输入邀请码加入</button>
      <view class="enterprise-assist-links">
        <text @click="scanInvitationCode()">扫码加入</text><text>·</text><text @click="openPage(pages.join + '?mode=request')">申请加入</text><text>·</text><text @click="contactService()">联系平台客服</text>
      </view>
      <view class="enterprise-section">
        <text class="enterprise-section-title">企业空间可以做什么</text>
        <view class="enterprise-benefit"><text class="enterprise-benefit-icon">人</text><text class="enterprise-benefit-title">成员协作</text><text class="enterprise-benefit-copy">统一组织与权限</text></view>
        <view class="enterprise-benefit"><text class="enterprise-benefit-icon">知</text><text class="enterprise-benefit-title">共享知识</text><text class="enterprise-benefit-copy">企业知识库与AI员工</text></view>
        <view class="enterprise-benefit"><text class="enterprise-benefit-icon">点</text><text class="enterprise-benefit-title">统一算力</text><text class="enterprise-benefit-copy">成员与消费统一管理</text></view>
      </view>
    </template>

    <template v-else-if="screen === 'create'">
      <view class="enterprise-card">
        <text class="enterprise-title">企业信息</text>
        <text class="enterprise-copy">创建成功后，你将成为企业创建者和管理员</text>
      </view>
      <view class="enterprise-section enterprise-form">
        <view><text class="enterprise-field-label">创建类型</text><view class="enterprise-segment"><button :class="{ active: createDraft.type === 'TEAM' }" @click="createDraft.type = 'TEAM'">团队空间</button><button :class="{ active: createDraft.type === 'ENTERPRISE' }" @click="createDraft.type = 'ENTERPRISE'">认证企业</button></view></view>
        <view><text class="enterprise-field-label">企业/团队名称</text><input v-model="createDraft.name" class="enterprise-input" maxlength="80" placeholder="请输入企业或团队名称" /><text v-if="formError" class="enterprise-field-error">{{ formError }}</text></view>
        <view><text class="enterprise-field-label">所属行业</text><input v-model="createDraft.industry" class="enterprise-input" maxlength="40" placeholder="请输入所属行业" /></view>
        <view><text class="enterprise-field-label">企业规模</text><input v-model="createDraft.scale" class="enterprise-input" maxlength="30" placeholder="例如：1—20人" /></view>
        <view><text class="enterprise-field-label">联系人</text><input v-model="createDraft.contact" class="enterprise-input" maxlength="40" placeholder="请输入联系人" /></view>
        <view><text class="enterprise-field-label">联系电话</text><input v-model="createDraft.phone" class="enterprise-input" maxlength="20" type="number" placeholder="请输入联系电话" /></view>
        <view><text class="enterprise-field-label">所在地区</text><input v-model="createDraft.region" class="enterprise-input" maxlength="80" placeholder="请输入所在地区" /></view>
        <text class="enterprise-copy">当前企业V1创建接口先保存企业名称，其余资料可在企业设置和认证中继续完善。</text>
        <text class="enterprise-agreement" @click="openLegalDocument('enterprise-space-service-agreement')">✓ 我已阅读并同意《企业空间服务协议》</text>
      </view>
    </template>

    <template v-else-if="screen === 'join'">
      <view class="enterprise-card">
        <text class="enterprise-title">{{ joinMode === 'invitation' ? '通过邀请码加入企业' : '提交加入申请' }}</text>
        <text class="enterprise-copy">{{ joinMode === 'invitation' ? '邀请码由企业管理员生成，有效期内可使用' : '请填写企业ID，管理员审核通过后即可加入' }}</text>
      </view>
      <view class="enterprise-section enterprise-form">
        <view class="enterprise-segment"><button :class="{ active: joinMode === 'invitation' }" @click="joinMode = 'invitation'">邀请码加入</button><button :class="{ active: joinMode === 'request' }" @click="joinMode = 'request'">申请加入</button></view>
        <template v-if="joinMode === 'invitation'">
          <view><text class="enterprise-field-label">企业邀请码</text><view class="enterprise-search-row"><input v-model="joinDraft.code" class="enterprise-input" maxlength="64" placeholder="请输入邀请码" /><button class="enterprise-inline-button" @click="scanInvitationCode()">扫码</button></view></view>
        </template>
        <template v-else>
          <view><text class="enterprise-field-label">企业ID</text><input v-model="joinDraft.tenantId" class="enterprise-input" maxlength="80" placeholder="请输入企业ID" /></view>
          <view><text class="enterprise-field-label">申请说明</text><textarea v-model="joinDraft.reason" class="enterprise-textarea" maxlength="200" placeholder="例如：我是市场部新成员" /></view>
        </template>
        <text v-if="formError" class="enterprise-field-error">{{ formError }}</text>
      </view>
      <EnterpriseStatePanel v-if="operationState === 'reviewing'" kind="reviewing" title="加入申请已提交" copy="请等待企业管理员审核" />
    </template>

    <template v-else-if="screen === 'switcher'">
      <text class="enterprise-section-note">切换后将使用对应空间的知识库、作品和算力</text>
      <view class="enterprise-section-title">个人空间</view>
      <view class="enterprise-workspace-list">
        <button v-for="context in personalContexts" :key="`${context.type}-${context.tenantId}`" :class="['enterprise-workspace-card', { active: context.current }]" @click="switchWorkspace(context)">
          <view class="enterprise-logo">人</view><view><text class="enterprise-workspace-name">{{ context.tenantName || '个人空间' }}</text><text class="enterprise-workspace-meta">个人数据与企业数据相互独立</text></view><text class="enterprise-workspace-check">{{ context.current ? '●' : '○' }}</text>
        </button>
      </view>
      <view class="enterprise-section"><text class="enterprise-section-title">企业空间</text>
        <view v-if="enterpriseContexts.length" class="enterprise-workspace-list">
          <button v-for="context in enterpriseContexts" :key="context.tenantId" :class="['enterprise-workspace-card', { active: context.current }]" @click="switchWorkspace(context)">
            <view class="enterprise-logo">企</view><view><text class="enterprise-workspace-name">{{ context.tenantName }}</text><text class="enterprise-workspace-meta">{{ roleName(context.currentRole) }} · {{ certificationName(context.certificationStatus) }}</text></view><text class="enterprise-workspace-check">{{ context.current ? '●' : '○' }}</text>
          </button>
        </view>
        <EnterpriseStatePanel v-else kind="empty" title="尚未加入企业" copy="创建或加入企业后会显示在这里" />
      </view>
      <view class="enterprise-section"><button class="enterprise-secondary-button" @click="openPage(pages.onboarding)">＋ 创建或加入其他企业</button></view>
    </template>

    <template v-else-if="screen === 'overview' && overview">
      <view class="enterprise-company-header"><view class="enterprise-logo">企</view><view><text class="enterprise-company-name">{{ overview.tenant.name }}</text><view class="enterprise-company-meta"><text :class="['enterprise-status-text', certificationTone]">{{ certificationName(overview.tenant.certificationStatus) }}</text><text>{{ planName }}</text></view></view></view>
      <view class="enterprise-metrics">
        <EnterpriseMetricCard label="算力余额" :value="`${formatNumber(overview.wallet.pointBalance)}点`" />
        <EnterpriseMetricCard label="冻结算力" :value="`${formatNumber(overview.wallet.frozenPoints)}点`" />
        <EnterpriseMetricCard label="成员" :value="`${overview.activeMembers} / ${overview.memberCount}人`" />
        <EnterpriseMetricCard label="AI员工" :value="`${aiEmployees.length}个`" />
      </view>
      <view class="enterprise-section enterprise-quick-grid">
        <button v-if="can('enterprise.member.invite')" class="enterprise-quick-action" @click="openPage(pages.invitations)"><text class="enterprise-quick-icon">邀</text><text class="enterprise-quick-label">邀请成员</text></button>
        <button v-if="can('enterprise.organization.create')" class="enterprise-quick-action" @click="openPage(pages.organizations + '?create=1')"><text class="enterprise-quick-icon">部</text><text class="enterprise-quick-label">新建部门</text></button>
        <button v-if="canAIManage" class="enterprise-quick-action" @click="openPage(pages.aiEmployeeCreate)"><text class="enterprise-quick-icon">AI</text><text class="enterprise-quick-label">AI员工</text></button>
        <button v-if="can('enterprise.billing.read')" class="enterprise-quick-action" @click="openPage(pages.billing)"><text class="enterprise-quick-icon orange">点</text><text class="enterprise-quick-label">企业算力</text></button>
      </view>
      <view class="enterprise-section"><text class="enterprise-section-title">企业管理</text><view class="enterprise-setting-group">
        <button v-if="can('enterprise.member.read')" class="enterprise-setting-row" @click="openPage(pages.members)"><text class="enterprise-setting-label">成员管理</text><text class="enterprise-setting-value">{{ overview.memberCount }}人</text><text class="enterprise-chevron">›</text></button>
        <button v-if="can('enterprise.organization.read')" class="enterprise-setting-row" @click="openPage(pages.organizations)"><text class="enterprise-setting-label">组织架构</text><text class="enterprise-setting-value">{{ flatOrganizations.length ? `${flatOrganizations.length}个部门` : '' }}</text><text class="enterprise-chevron">›</text></button>
        <button v-if="can('enterprise.role.read')" class="enterprise-setting-row" @click="openPage(pages.roles)"><text class="enterprise-setting-label">角色权限</text><text class="enterprise-setting-value" /><text class="enterprise-chevron">›</text></button>
        <button v-if="can('enterprise.overview.read')" class="enterprise-setting-row" @click="openPage(pages.aiEmployees)"><text class="enterprise-setting-label">企业AI员工</text><text class="enterprise-setting-value">{{ aiEmployees.length }}个</text><text class="enterprise-chevron">›</text></button>
        <button v-if="can('enterprise.billing.read')" class="enterprise-setting-row" @click="openPage(pages.billing)"><text class="enterprise-setting-label">企业算力</text><text class="enterprise-setting-value">{{ formatNumber(overview.wallet.pointBalance) }}点</text><text class="enterprise-chevron">›</text></button>
        <button v-if="can('enterprise.settings.read')" class="enterprise-setting-row" @click="openPage(pages.settings)"><text class="enterprise-setting-label">企业设置</text><text class="enterprise-setting-value" /><text class="enterprise-chevron">›</text></button>
      </view></view>
      <view v-if="overview.tenant.certificationStatus !== 'VERIFIED'" :class="['enterprise-section', 'enterprise-alert', overview.tenant.certificationStatus === 'PENDING' ? 'purple' : '']" @click="can('enterprise.certification.submit') && openPage(pages.certification)"><text>{{ overview.tenant.certificationStatus === 'PENDING' ? '审' : '证' }}</text><text class="enterprise-alert-title">{{ overview.tenant.certificationStatus === 'PENDING' ? '企业认证审核中' : '企业尚未认证' }}</text><text>{{ can('enterprise.certification.submit') ? '›' : '' }}</text></view>
      <view v-if="isSubscriptionExpired" class="enterprise-section enterprise-alert red"><text>期</text><text class="enterprise-alert-title">企业套餐已到期</text><text>›</text></view>
      <view v-else-if="overview.wallet.status && overview.wallet.status !== 'ACTIVE'" class="enterprise-section enterprise-alert"><text>!</text><text class="enterprise-alert-title">企业算力状态异常</text><text>›</text></view>
      <view class="enterprise-section"><text class="enterprise-section-title">最近动态</text><view class="enterprise-card">
        <view v-for="item in auditLogs.slice(0, 3)" :key="item.id" class="enterprise-activity"><text class="enterprise-list-icon">{{ activityIcon(item.action) }}</text><text class="enterprise-activity-copy">{{ activityCopy(item) }}</text><text class="enterprise-activity-time">{{ relativeTime(item.createdAt) }}</text></view>
        <EnterpriseStatePanel v-if="!auditLogs.length" kind="empty" title="暂无企业动态" copy="成员、组织和权限变更会显示在这里" />
      </view></view>
    </template>

    <template v-else-if="screen === 'organizations'">
      <view class="enterprise-card"><text class="enterprise-title">{{ currentEnterprise?.tenantName || '当前企业' }}</text><text class="enterprise-copy">{{ organizationSummary }}</text></view>
      <view class="enterprise-section"><input v-model="searchText" class="enterprise-search" placeholder="搜索部门" /></view>
       <view class="enterprise-section"><text class="enterprise-section-title">组织树</text><text class="enterprise-section-note">多级组织按企业数据范围展示</text><view v-if="filteredOrganizations.length" class="enterprise-org-tree"><EnterpriseOrganizationNode v-for="item in filteredOrganizations" :key="item.id" :item="item" :editable="can('enterprise.organization.create') || can('enterprise.organization.update') || can('enterprise.organization.delete')" @manage="manageOrganization($event)" /></view><EnterpriseStatePanel v-else kind="empty" title="暂无部门" copy="有权限的管理员可以新建部门" /></view>
    </template>

    <template v-else-if="screen === 'members'">
      <input v-model="searchText" class="enterprise-search" placeholder="搜索成员" />
      <view class="enterprise-filters"><button v-for="filter in memberFilters" :key="filter.value" :class="['enterprise-chip', { active: memberFilter === filter.value }]" @click="memberFilter = filter.value">{{ filter.label }}</button></view>
      <view v-if="filteredMembers.length" class="enterprise-list">
        <button v-for="item in filteredMembers" :key="item.id" class="enterprise-list-card" @click="openPage(`${pages.memberDetail}?id=${encodeURIComponent(item.id)}`)"><view class="enterprise-avatar">{{ avatarText(item.name) }}</view><view class="enterprise-list-main"><text class="enterprise-list-title">{{ item.name || item.email || item.userId }}</text><text class="enterprise-list-meta">{{ item.organizationName || '未分配部门' }} · {{ item.email || item.userId }}</text><text class="enterprise-role-tag">{{ roleName(item.roles[0]) }}</text></view><text :class="['enterprise-list-status', { disabled: item.memberStatus !== 'ACTIVE' }]">{{ memberStatusName(item.memberStatus) }}</text></button>
      </view>
      <EnterpriseStatePanel v-else kind="empty" title="暂无成员" copy="邀请成员加入后会显示在这里" />
    </template>

    <template v-else-if="screen === 'member-detail' && selectedMember">
      <view class="enterprise-detail-header"><view class="enterprise-avatar">{{ avatarText(selectedMember.name) }}</view><view><text class="enterprise-detail-title">{{ selectedMember.name || selectedMember.email }}</text><text class="enterprise-detail-meta">{{ selectedMember.email || selectedMember.userId }}</text><text :class="['enterprise-status-text', { red: selectedMember.memberStatus !== 'ACTIVE' }]">{{ memberStatusName(selectedMember.memberStatus) }}</text></view></view>
      <view class="enterprise-section"><text class="enterprise-section-title">组织与角色</text><view class="enterprise-setting-group">
        <button class="enterprise-setting-row" :disabled="!can('enterprise.member.update')" @click="chooseMemberOrganization()"><text class="enterprise-setting-label">所属部门</text><text class="enterprise-setting-value">{{ selectedMember.organizationName || '未分配' }}</text><text class="enterprise-chevron">›</text></button>
        <button class="enterprise-setting-row" :disabled="!can('enterprise.role.assign')" @click="chooseMemberRole()"><text class="enterprise-setting-label">企业角色</text><text class="enterprise-setting-value">{{ selectedMember.roles.map(roleName).join('、') }}</text><text class="enterprise-chevron">›</text></button>
        <button class="enterprise-setting-row" :disabled="!can('enterprise.member.update')" @click="chooseMemberScope()"><text class="enterprise-setting-label">数据范围</text><text class="enterprise-setting-value">{{ scopeName(selectedMember.dataScope) }}</text><text class="enterprise-chevron">›</text></button>
      </view></view>
      <view class="enterprise-section"><text class="enterprise-section-title">基本资料</text><view class="enterprise-setting-group"><view class="enterprise-setting-row"><text class="enterprise-setting-label">加入时间</text><text class="enterprise-setting-value">{{ formatDate(selectedMember.joinedAt) }}</text><text /></view><view class="enterprise-setting-row"><text class="enterprise-setting-label">成员ID</text><text class="enterprise-setting-value">{{ selectedMember.id }}</text><text /></view></view></view>
      <view v-if="can('enterprise.member.disable') || can('enterprise.member.remove')" class="enterprise-section"><button v-if="can('enterprise.member.disable') && selectedMember.memberStatus === 'ACTIVE'" class="enterprise-secondary-button" @click="disableSelectedMember()">禁用成员</button><view v-if="can('enterprise.member.disable') && can('enterprise.member.remove')" class="enterprise-button-gap" /><button v-if="can('enterprise.member.remove')" class="enterprise-danger-button" @click="removeSelectedMember()">移出企业</button></view>
    </template>

    <template v-else-if="screen === 'invitations'">
      <view v-if="can('enterprise.member.invite')" class="enterprise-card"><text class="enterprise-title">选择邀请方式</text><text class="enterprise-copy">使用邮箱或通用邀请码邀请成员</text><view class="enterprise-section enterprise-form"><input v-model="inviteDraft.email" class="enterprise-input" placeholder="成员邮箱（可选）" /><button class="enterprise-primary-button" :disabled="submitting" @click="createInvitation()">{{ submitting ? '生成中...' : '生成企业邀请码' }}</button></view></view>
      <EnterpriseStatePanel v-else kind="forbidden" title="无邀请权限" copy="你可以查看加入申请，但不能创建企业邀请" />
      <view v-if="lastInvitation" class="enterprise-section"><text class="enterprise-section-title">当前企业邀请码</text><view class="enterprise-invite-code"><text class="enterprise-invite-value">{{ lastInvitation.invitationCode }}</text><button class="enterprise-inline-button" @click="copyText(lastInvitation.invitationCode)">复制</button></view><text class="enterprise-section-note">有效期至 {{ formatDate(lastInvitation.expiresAt) }}</text></view>
      <view class="enterprise-section"><text class="enterprise-section-title">待审核申请</text><view v-if="pendingJoinRequests.length" class="enterprise-list"><view v-for="item in pendingJoinRequests" :key="item.id" class="enterprise-card"><view class="enterprise-activity"><view class="enterprise-avatar">{{ avatarText(item.applicantName || item.applicantEmail) }}</view><view><text class="enterprise-list-title">{{ item.applicantName || item.applicantEmail || item.applicantUserId }}</text><text class="enterprise-list-meta">{{ item.reason || '未填写申请说明' }}</text><text class="enterprise-list-meta">{{ formatDate(item.createdAt) }}</text></view></view><view v-if="can('enterprise.member.update')" class="enterprise-review-actions"><button class="reject" @click="reviewJoin(item, false)">拒绝</button><button class="approve" @click="reviewJoin(item, true)">同意</button></view></view></view><EnterpriseStatePanel v-else kind="empty" title="暂无待审核申请" copy="新的加入申请会显示在这里" /></view>
    </template>

    <template v-else-if="screen === 'roles'">
      <view class="enterprise-filters"><button v-for="item in roles" :key="item.code" :class="['enterprise-chip', { active: selectedRoleCode === item.code }]" @click="selectedRoleCode = item.code">{{ roleName(item.code) }}</button></view>
      <view v-if="selectedRole" class="enterprise-card"><text class="enterprise-title">{{ roleName(selectedRole.code) }}</text><text class="enterprise-copy">数据范围由成员角色和组织归属共同决定</text></view>
      <view v-if="selectedRole" class="enterprise-section"><text class="enterprise-section-title">功能权限</text><view class="enterprise-permission-group"><view v-for="permission in selectedRole.permissions" :key="permission" class="enterprise-permission-row"><view><text class="enterprise-permission-code">{{ permissionLabel(permission) }}</text><text class="enterprise-list-meta">{{ permission }}</text></view><view class="enterprise-toggle"><view class="enterprise-toggle-dot" /></view></view></view></view>
      <EnterpriseStatePanel v-if="!roles.length" kind="empty" title="暂无角色配置" copy="角色权限接口未返回数据" />
    </template>

    <template v-else-if="screen === 'ai-employees'">
      <view class="enterprise-card"><text class="enterprise-title">在岗AI员工</text><text class="enterprise-copy">{{ aiEmployees.length }} 个 · 数据归属当前企业空间</text></view>
      <view class="enterprise-section"><input v-model="searchText" class="enterprise-search" placeholder="搜索AI员工" /></view>
      <view v-if="filteredAIEmployees.length" class="enterprise-list"><button v-for="item in filteredAIEmployees" :key="item.id" class="enterprise-ai-card" @click="openPage(`${pages.aiEmployeeDetail}?id=${encodeURIComponent(item.id)}`)"><view class="enterprise-list-icon">AI</view><view><text class="enterprise-list-title">{{ item.name }}</text><text class="enterprise-list-meta">{{ employeePosition(item) }} · {{ item.modelName }}</text><text class="enterprise-list-meta">负责人：{{ employeeOwner(item) }}</text></view><text :class="['enterprise-list-status', { disabled: item.status !== 'ACTIVE' }]">{{ item.status === 'ACTIVE' ? '运行中' : '已停用' }}</text></button></view>
      <EnterpriseStatePanel v-else kind="empty" title="暂无AI员工" copy="创建后可绑定当前企业的知识库和模型" />
    </template>

    <template v-else-if="screen === 'ai-employee-create'">
      <view class="enterprise-card"><text class="enterprise-title">配置岗位与组织</text><text class="enterprise-copy">AI员工数据、知识库和调用记录归属当前Tenant</text></view>
      <view class="enterprise-section enterprise-form"><view><text class="enterprise-field-label">AI员工名称</text><input v-model="aiDraft.name" class="enterprise-input" maxlength="60" placeholder="例如：知识库客服" /></view><view><text class="enterprise-field-label">职责说明</text><textarea v-model="aiDraft.description" class="enterprise-textarea" maxlength="300" placeholder="说明AI员工负责的工作" /></view><view><text class="enterprise-field-label">所属部门</text><picker :range="flatOrganizations" range-key="name" @change="selectAIOrganization($event)"><view class="enterprise-select">{{ selectedAIOrganization?.name || '请选择所属部门' }} ›</view></picker></view><view><text class="enterprise-field-label">岗位</text><input v-model="aiDraft.position" class="enterprise-input" maxlength="40" placeholder="例如：客服专员" /></view><view><text class="enterprise-field-label">负责人</text><input v-model="aiDraft.ownerName" class="enterprise-input" maxlength="40" placeholder="请输入负责人" /></view><view><text class="enterprise-field-label">绑定知识库</text><picker :range="knowledgeBases" range-key="name" @change="selectAIKnowledgeBase($event)"><view class="enterprise-select">{{ selectedAIKnowledgeBase?.name || (knowledgeBases.length ? '请选择知识库（可选）' : '当前企业暂无知识库') }} ›</view></picker></view><view><text class="enterprise-field-label">模型</text><input v-model="aiDraft.modelName" class="enterprise-input" maxlength="80" placeholder="请输入模型标识" /></view><view><text class="enterprise-field-label">月度预算</text><input v-model.number="aiDraft.monthlyBudget" class="enterprise-input" type="number" placeholder="请输入点数" /></view><text v-if="formError" class="enterprise-field-error">{{ formError }}</text></view>
    </template>

    <template v-else-if="screen === 'ai-employee-detail' && selectedAIEmployee">
      <view class="enterprise-detail-header"><view class="enterprise-avatar">AI</view><view><text class="enterprise-detail-title">{{ selectedAIEmployee.name }}</text><text class="enterprise-detail-meta">{{ selectedAIEmployee.description || '未填写职责说明' }}</text><text :class="['enterprise-status-text', { red: selectedAIEmployee.status !== 'ACTIVE' }]">{{ selectedAIEmployee.status === 'ACTIVE' ? '运行中' : '已停用' }}</text></view></view>
      <view class="enterprise-section"><text class="enterprise-section-title">基本资料</text><view class="enterprise-setting-group"><view class="enterprise-setting-row"><text class="enterprise-setting-label">所属部门</text><text class="enterprise-setting-value">{{ employeeOrganization(selectedAIEmployee) }}</text><text /></view><view class="enterprise-setting-row"><text class="enterprise-setting-label">岗位</text><text class="enterprise-setting-value">{{ employeePosition(selectedAIEmployee) }}</text><text /></view><view class="enterprise-setting-row"><text class="enterprise-setting-label">负责人</text><text class="enterprise-setting-value">{{ employeeOwner(selectedAIEmployee) }}</text><text /></view><view class="enterprise-setting-row"><text class="enterprise-setting-label">模型</text><text class="enterprise-setting-value">{{ selectedAIEmployee.modelName }}</text><text /></view><view class="enterprise-setting-row"><text class="enterprise-setting-label">月度预算</text><text class="enterprise-setting-value">{{ formatNumber(employeeBudget(selectedAIEmployee)) }}点</text><text /></view></view></view>
      <view class="enterprise-section"><text class="enterprise-section-title">绑定知识库</text><view class="enterprise-card"><text class="enterprise-copy">已绑定 {{ aiKnowledgeBindingCount }} 个知识库</text></view></view>
    </template>

    <template v-else-if="screen === 'billing' && billing">
      <view class="enterprise-balance-hero"><text class="enterprise-balance-label">企业可用算力</text><text class="enterprise-balance-value">{{ formatNumber(billing.wallet.pointBalance) }}<text style="font-size:14px"> 点</text></text><text class="enterprise-balance-note">冻结算力 {{ formatNumber(billing.wallet.frozenPoints) }} 点 · 状态 {{ billing.wallet.status }}</text></view>
      <view class="enterprise-metrics"><EnterpriseMetricCard label="套餐" :value="billing.subscription.planCode || '--'" /><EnterpriseMetricCard label="套餐状态" :value="subscriptionStatusName(billing.subscription.status)" /><EnterpriseMetricCard label="现金余额" :value="formatCurrency(billing.wallet.cashBalanceCents)" /><EnterpriseMetricCard label="数据范围" :value="scopeName(currentEnterprise?.dataScope)" /></view>
      <view class="enterprise-section"><text class="enterprise-section-title">算力管理</text><view class="enterprise-setting-group"><button class="enterprise-setting-row" @click="openPage(pages.usage)"><text class="enterprise-setting-label">消费明细</text><text class="enterprise-setting-value">查看当前企业记录</text><text class="enterprise-chevron">›</text></button><view class="enterprise-setting-row"><text class="enterprise-setting-label">余额预警</text><text class="enterprise-setting-value">{{ billing.wallet.status }}</text><text /></view></view></view>
    </template>

    <template v-else-if="screen === 'usage'">
      <view v-if="billing" class="enterprise-card"><text class="enterprise-title">企业算力余额</text><text class="enterprise-copy">{{ formatNumber(billing.wallet.pointBalance) }} 点</text></view>
      <EnterpriseStatePanel kind="empty" title="暂无企业消费明细" copy="企业V1接口当前仅返回租户算力汇总，没有返回成员级消费流水" />
    </template>

    <template v-else-if="screen === 'settings' && overview">
      <view class="enterprise-company-header"><view class="enterprise-logo">企</view><view><text class="enterprise-company-name">{{ overview.tenant.name }}</text><view class="enterprise-company-meta"><text :class="['enterprise-status-text', certificationTone]">{{ certificationName(overview.tenant.certificationStatus) }}</text><text>企业ID：{{ overview.tenant.id }}</text></view></view></view>
      <view class="enterprise-section"><text class="enterprise-section-title">企业资料</text><view class="enterprise-setting-group"><view class="enterprise-setting-row"><text class="enterprise-setting-label">基本资料</text><text class="enterprise-setting-value">{{ overview.tenant.name }}</text><text /></view><button v-if="can('enterprise.certification.submit')" class="enterprise-setting-row" @click="openPage(pages.certification)"><text class="enterprise-setting-label">企业认证</text><text :class="['enterprise-setting-value', 'enterprise-status-text', certificationTone]">{{ certificationName(overview.tenant.certificationStatus) }}</text><text class="enterprise-chevron">›</text></button><view v-else class="enterprise-setting-row"><text class="enterprise-setting-label">企业认证</text><text :class="['enterprise-setting-value', 'enterprise-status-text', certificationTone]">{{ certificationName(overview.tenant.certificationStatus) }}</text><text /></view></view></view>
      <view class="enterprise-section"><text class="enterprise-section-title">管理设置</text><view class="enterprise-setting-group"><button v-if="can('enterprise.member.read')" class="enterprise-setting-row" @click="openPage(pages.members)"><text class="enterprise-setting-label">企业管理员</text><text class="enterprise-setting-value">{{ administratorCount }}人</text><text class="enterprise-chevron">›</text></button><button v-if="can('enterprise.connector.read')" class="enterprise-setting-row" @click="openPage(pages.feishu)"><text class="enterprise-setting-label">企业连接 · 飞书</text><text class="enterprise-setting-value">配置机器人</text><text class="enterprise-chevron">›</text></button><button class="enterprise-setting-row" @click="openPage(pages.switcher)"><text class="enterprise-setting-label">默认工作空间</text><text class="enterprise-setting-value">{{ currentEnterprise?.tenantName }}</text><text class="enterprise-chevron">›</text></button><button v-if="can('enterprise.audit.read')" class="enterprise-setting-row" @click="showAuditSummary()"><text class="enterprise-setting-label">操作日志</text><text class="enterprise-setting-value">最近 {{ auditLogs.length }} 条</text><text class="enterprise-chevron">›</text></button></view></view>
      <view class="enterprise-section"><text class="enterprise-section-title" style="color:#f04438">危险操作</text><view class="enterprise-danger-zone"><text class="enterprise-danger-title">转让、退出和解散企业</text><text class="enterprise-danger-copy">企业V1后端未提供对应写接口，本端不会调用个人或渠道接口代替</text></view></view>
    </template>

    <template v-else-if="screen === 'certification'">
      <EnterpriseStatePanel v-if="certificationStatus === 'PENDING' && !certificationResult" kind="reviewing" action-label="返回企业设置" @action="openPage(pages.settings)" />
      <EnterpriseStatePanel v-else-if="certificationStatus === 'VERIFIED' && !certificationResult" kind="success" title="企业已认证" copy="企业认证状态有效" action-label="返回企业设置" @action="openPage(pages.settings)" />
      <template v-else>
        <view :class="['enterprise-alert', { red: certificationStatus === 'REJECTED' }]"><text>证</text><text class="enterprise-alert-title">{{ certificationStatus === 'REJECTED' ? '认证资料未通过，请修改后重新提交' : '企业尚未认证' }}</text><text>›</text></view>
      <view class="enterprise-section enterprise-form"><view><text class="enterprise-field-label">企业名称</text><input v-model="certDraft.legalName" class="enterprise-input" maxlength="100" placeholder="请输入营业执照企业名称" /></view><view><text class="enterprise-field-label">统一社会信用代码</text><input v-model="certDraft.creditCode" class="enterprise-input" maxlength="18" placeholder="请输入18位信用代码" /></view><view><text class="enterprise-field-label">法人姓名</text><input v-model="certDraft.legalRepresentative" class="enterprise-input" maxlength="40" placeholder="请输入法人姓名" /></view><view><text class="enterprise-field-label">联系人手机号</text><input v-model="certDraft.contactPhone" class="enterprise-input" maxlength="20" type="number" placeholder="请输入联系人手机号" /></view><view><text class="enterprise-field-label">营业执照</text><view class="enterprise-upload" @click="chooseLicense()"><image v-if="licensePreview" class="enterprise-license-preview" mode="aspectFit" :src="licensePreview" /><template v-else><text class="enterprise-upload-icon">＋</text><text class="enterprise-upload-copy">上传营业执照</text><text class="enterprise-upload-note">支持 JPG、PNG，文件不超过10MB</text></template></view></view><text class="enterprise-agreement">✓ 我确认以上资料真实有效</text><text v-if="formError" class="enterprise-field-error">{{ formError }}</text></view>
        <EnterpriseStatePanel v-if="certificationResult" kind="success" title="认证资料已提交" copy="预计1—2个工作日完成审核" />
      </template>
    </template>

    <template v-else-if="screen === 'status'">
      <EnterpriseStatePanel :kind="statusKind" :title="statusTitle" :copy="statusCopy" action-label="返回企业中心" @action="openEntry()" />
    </template>

      <EnterpriseStatePanel v-else kind="empty" title="暂无可展示数据" copy="请返回企业中心后重试" action-label="返回企业中心" @action="openEntry()" />

    <template v-if="hasFixedAction" #fixed-action>
      <button v-if="screen === 'create'" class="enterprise-primary-button" :disabled="submitting" @click="createEnterprise()">{{ submitting ? '创建中...' : '创建企业' }}</button>
      <button v-else-if="screen === 'join'" class="enterprise-primary-button" :disabled="submitting || operationState === 'reviewing'" @click="joinEnterprise()">{{ submitting ? '提交中...' : joinMode === 'invitation' ? '加入企业' : '提交加入申请' }}</button>
      <button v-else-if="screen === 'members' && can('enterprise.member.invite')" class="enterprise-primary-button" @click="openPage(pages.invitations)">邀请成员</button>
      <button v-else-if="screen === 'ai-employee-create'" class="enterprise-primary-button" :disabled="submitting" @click="createAIEmployee()">{{ submitting ? '发布中...' : '确认发布' }}</button>
      <button v-else-if="screen === 'certification' && certificationStatus !== 'PENDING' && certificationStatus !== 'VERIFIED'" class="enterprise-primary-button" :disabled="submitting || Boolean(certificationResult)" @click="submitCertification()">{{ submitting ? '提交中...' : certificationStatus === 'REJECTED' ? '重新提交认证' : '提交认证' }}</button>
    </template>
  </EnterprisePageShell>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { onPullDownRefresh } from "@dcloudio/uni-app";
import { ApiClientError } from "@xianzhi/api-client";
import EnterpriseMetricCard from "./EnterpriseMetricCard.vue";
import EnterpriseOrganizationNode from "./EnterpriseOrganizationNode.vue";
import EnterprisePageShell from "./EnterprisePageShell.vue";
import EnterpriseStatePanel from "./EnterpriseStatePanel.vue";
import { enterpriseAPI, uploadEnterpriseDocument } from "../../features/enterprise/api";
import { openLegalDocument } from "../../features/legal/navigation";
import type { EnterpriseAIEmployee, EnterpriseCertification, EnterpriseInvitation, EnterpriseJoinRequest, EnterpriseMember, EnterpriseOrganization } from "../../features/enterprise/types";
import { miniProgramEnterprisePages as pages } from "../../config/miniProgramPages";
import { roleLabels } from "../../config/permissions";
import { useEnterpriseStore } from "../../stores/enterprise";
import { useUserStore } from "../../stores/user";
import type { AppRole, EnterpriseContext } from "../../types";

type Screen = "onboarding" | "create" | "join" | "switcher" | "overview" | "organizations" | "members" | "member-detail" | "invitations" | "roles" | "ai-employees" | "ai-employee-create" | "ai-employee-detail" | "billing" | "usage" | "settings" | "certification" | "status";
type StateKind = "loading" | "empty" | "error" | "forbidden" | "disabled" | "reviewing" | "low-balance" | "expired" | "success";

const props = defineProps<{ screen: Screen }>();
const userStore = useUserStore();
const enterpriseStore = useEnterpriseStore();
const loading = ref(false);
const submitting = ref(false);
const forbidden = ref(false);
const loadError = ref("");
const formError = ref("");
const searchText = ref("");
const memberFilter = ref("ALL");
const selectedMember = ref<EnterpriseMember | null>(null);
const selectedAIEmployee = ref<EnterpriseAIEmployee | null>(null);
const aiKnowledgeBindingCount = ref(0);
const lastInvitation = ref<EnterpriseInvitation | null>(null);
const certificationResult = ref<EnterpriseCertification | null>(null);
const licensePreview = ref("");
const licenseDocumentUrl = ref("");
const operationState = ref("");
const selectedRoleCode = ref<AppRole>("ENTERPRISE_ADMIN");
const joinMode = ref<"invitation" | "request">("invitation");

const createDraft = reactive({ type: "TEAM", name: "", industry: "", scale: "", contact: "", phone: "", region: "" });
const joinDraft = reactive({ code: "", tenantId: "", reason: "" });
const inviteDraft = reactive({ email: "" });
const aiDraft = reactive({ name: "", description: "", organizationId: "", knowledgeBaseId: "", position: "", ownerName: "", modelName: "gpt-5.2-chat-latest", monthlyBudget: 0 });
const certDraft = reactive({ legalName: "", creditCode: "", legalRepresentative: "", contactPhone: "" });

const screen = computed(() => props.screen);
const overview = computed(() => enterpriseStore.overview);
const members = computed(() => enterpriseStore.members);
const organizations = computed(() => enterpriseStore.organizations);
const roles = computed(() => enterpriseStore.roles);
const billing = computed(() => enterpriseStore.billing);
const auditLogs = computed(() => enterpriseStore.auditLogs);
const aiEmployees = computed(() => enterpriseStore.aiEmployees);
const knowledgeBases = computed(() => enterpriseStore.knowledgeBases);
const enterpriseContexts = computed(() => userStore.enterpriseContexts.filter(item => item.type === "ENTERPRISE"));
const personalContexts = computed(() => userStore.enterpriseContexts.filter(item => item.type === "PERSONAL"));
const currentEnterprise = computed(() => userStore.currentContext?.type === "ENTERPRISE" ? userStore.currentContext : enterpriseContexts.value.find(item => item.current) || null);
const certificationStatus = computed(() => String(currentEnterprise.value?.certificationStatus || overview.value?.tenant.certificationStatus || "UNVERIFIED").toUpperCase());
const certificationTone = computed(() => certificationStatus.value === "VERIFIED" ? "" : certificationStatus.value === "PENDING" ? "orange" : "orange");
const planName = computed(() => overview.value?.subscription.planCode || "企业套餐");
const isSubscriptionExpired = computed(() => ["EXPIRED", "CANCELLED", "SUSPENDED"].includes(String(overview.value?.subscription.status || "").toUpperCase()));
const canAIManage = computed(() => can("ai:admin") || can("enterprise.settings.update"));
const pendingJoinRequests = computed(() => enterpriseStore.joinRequests.filter(item => item.status === "PENDING"));
const flatOrganizations = computed(() => flattenOrganizations(organizations.value));
const selectedAIOrganization = computed(() => flatOrganizations.value.find(item => item.id === aiDraft.organizationId));
const selectedAIKnowledgeBase = computed(() => knowledgeBases.value.find(item => item.id === aiDraft.knowledgeBaseId));
const selectedRole = computed(() => roles.value.find(item => item.code === selectedRoleCode.value) || roles.value[0] || null);
const administratorCount = computed(() => members.value.filter(item => item.roles.includes("ENTERPRISE_ADMIN")).length);
const organizationSummary = computed(() => `${flatOrganizations.value.length}个部门 · ${members.value.length}名成员`);
const memberFilters = [{ label: "全部", value: "ALL" }, { label: "已启用", value: "ACTIVE" }, { label: "已禁用", value: "DISABLED" }, { label: "管理员", value: "ADMIN" }];
const filteredMembers = computed(() => members.value.filter(item => {
  const query = searchText.value.trim().toLowerCase();
  const matchesSearch = !query || [item.name, item.email, item.organizationName, item.userId].some(value => String(value || "").toLowerCase().includes(query));
  const matchesFilter = memberFilter.value === "ALL" || item.memberStatus === memberFilter.value || (memberFilter.value === "ADMIN" && item.roles.includes("ENTERPRISE_ADMIN"));
  return matchesSearch && matchesFilter;
}));
const filteredOrganizations = computed(() => {
  const query = searchText.value.trim().toLowerCase();
  return query ? flatOrganizations.value.filter(item => item.name.toLowerCase().includes(query)) : organizations.value;
});
const filteredAIEmployees = computed(() => {
  const query = searchText.value.trim().toLowerCase();
  return aiEmployees.value.filter(item => !query || [item.name, item.description, item.modelName, employeePosition(item)].some(value => String(value || "").toLowerCase().includes(query)));
});

const pageTitles: Record<Screen, string> = { onboarding: "企业中心", create: "创建企业", join: "加入企业", switcher: "切换工作空间", overview: "企业中心", organizations: "组织架构", members: "成员管理", "member-detail": "成员详情", invitations: "邀请与审核", roles: "角色权限", "ai-employees": "AI员工", "ai-employee-create": "创建AI员工", "ai-employee-detail": "AI员工详情", billing: "企业算力", usage: "消费明细", settings: "企业设置", certification: "企业认证", status: "企业中心状态" };
const pageTitle = computed(() => pageTitles[screen.value]);
const pageActionLabel = computed(() => ({ overview: "切换", organizations: can("enterprise.organization.create") ? "新建" : "", members: can("enterprise.member.invite") ? "邀请" : "", "ai-employees": canAIManage.value ? "创建" : "", billing: can("enterprise.billing.read") ? "明细" : "" } as Partial<Record<Screen, string>>)[screen.value] || "");
const hasFixedAction = computed(() => ["create", "join", "ai-employee-create"].includes(screen.value) || (screen.value === "members" && can("enterprise.member.invite")) || (screen.value === "certification" && can("enterprise.certification.submit") && !["PENDING", "VERIFIED"].includes(certificationStatus.value)));

const statusKind = computed<StateKind>(() => {
  const value = routeOption("state").toLowerCase();
  if (["disabled", "forbidden", "reviewing", "low-balance", "expired", "success", "error"].includes(value)) return value as StateKind;
  return "error";
});
const statusTitle = computed(() => routeOption("title") || undefined);
const statusCopy = computed(() => routeOption("reason") || routeOption("copy") || undefined);

function can(permission: string) { return userStore.permissions.includes(permission); }
function routeOption(key: string) {
  const current = getCurrentPages().slice(-1)[0] as { options?: Record<string, unknown>; $page?: { options?: Record<string, unknown> } } | undefined;
  const value = current?.options?.[key] ?? current?.$page?.options?.[key];
  return decodeURIComponent(String(value || ""));
}
function openPage(url: string) { uni.navigateTo({ url, fail: () => uni.redirectTo({ url, fail: () => uni.reLaunch({ url }) }) }); }
function replacePage(url: string) { uni.redirectTo({ url, fail: () => uni.reLaunch({ url }) }); }
function openEntry() { replacePage(pages.entry); }

async function ensureContext() {
  await userStore.loadProfile(true);
  const payload = await userStore.loadEnterpriseContexts();
  const active = payload.contexts.filter(item => item.type === "ENTERPRISE");
  let current = payload.current?.type === "ENTERPRISE" ? payload.current : active.find(item => item.current) || null;
  if (!current && active.length === 1 && !["onboarding", "create", "join", "switcher", "status"].includes(screen.value)) {
    current = await userStore.switchContext({ type: "ENTERPRISE", tenantId: active[0].tenantId, organizationId: active[0].organizationId, role: active[0].currentRole });
  }
  if (!current && !["onboarding", "create", "join", "switcher", "status"].includes(screen.value)) {
    replacePage(active.length > 1 ? pages.switcher : pages.onboarding);
    return null;
  }
  if (current) enterpriseStore.useTenant(current.tenantId);
  return current;
}

async function load() {
  if (loading.value) return;
  loading.value = true; forbidden.value = false; loadError.value = "";
  try {
    const context = await ensureContext();
    if (!context && !["onboarding", "create", "join", "switcher", "status"].includes(screen.value)) return;
    if (context?.memberStatus && context.memberStatus !== "ACTIVE") {
      replacePage(`${pages.status}?state=disabled&reason=${encodeURIComponent("当前企业成员状态已停用")}`);
      return;
    }
    if (screen.value === "ai-employee-create" && !canAIManage.value) { forbidden.value = true; return; }
    if (screen.value === "join" && routeOption("mode") === "request") joinMode.value = "request";
    if (screen.value === "overview") await Promise.all([enterpriseStore.loadOverview(true), enterpriseStore.loadOrganizations(true).catch(() => []), enterpriseStore.loadAIEmployees(true).catch(() => []), can("enterprise.audit.read") ? enterpriseStore.loadAuditLogs(true) : Promise.resolve([])]);
    if (screen.value === "organizations") { await Promise.all([enterpriseStore.loadOrganizations(true), can("enterprise.member.read") ? enterpriseStore.loadMembers(true) : Promise.resolve([])]); if (routeOption("create") === "1" && can("enterprise.organization.create")) setTimeout(() => createOrganization(), 80); }
    if (screen.value === "members") await enterpriseStore.loadMembers(true);
    if (screen.value === "member-detail") { selectedMember.value = await enterpriseAPI.member(routeOption("id")); await Promise.all([enterpriseStore.loadOrganizations(true), can("enterprise.role.read") ? enterpriseStore.loadRoles(true) : Promise.resolve([])]); }
    if (screen.value === "invitations") await Promise.all([enterpriseStore.loadJoinRequests(true), enterpriseStore.loadOrganizations(true)]);
    if (screen.value === "roles") { await enterpriseStore.loadRoles(true); if (roles.value[0]) selectedRoleCode.value = roles.value[0].code; }
    if (screen.value === "ai-employees") await enterpriseStore.loadAIEmployees(true);
    if (screen.value === "ai-employee-create") await Promise.all([enterpriseStore.loadOrganizations(true), enterpriseStore.loadKnowledgeBases(true)]);
    if (screen.value === "ai-employee-detail") { const value = await enterpriseAPI.aiEmployee(routeOption("id")); selectedAIEmployee.value = value.agent; aiKnowledgeBindingCount.value = Array.isArray(value.knowledgeBindings) ? value.knowledgeBindings.length : 0; }
    if (screen.value === "billing" || screen.value === "usage") await enterpriseStore.loadBilling(true);
    if (screen.value === "settings") await Promise.all([enterpriseStore.loadOverview(true), can("enterprise.member.read") ? enterpriseStore.loadMembers(true) : Promise.resolve([]), can("enterprise.audit.read") ? enterpriseStore.loadAuditLogs(true) : Promise.resolve([])]);
    if (screen.value === "certification") {
      await enterpriseStore.loadOverview(true);
      if (overview.value?.tenant.name) certDraft.legalName = overview.value.tenant.name;
    }
  } catch (error) {
    if (error instanceof ApiClientError && error.statusCode === 403) forbidden.value = true;
    else loadError.value = error instanceof Error ? error.message : "企业数据加载失败";
  } finally { loading.value = false; uni.stopPullDownRefresh(); }
}

function handlePageAction() {
  if (screen.value === "overview") openPage(pages.switcher);
  else if (screen.value === "organizations") createOrganization();
  else if (screen.value === "members") openPage(pages.invitations);
  else if (screen.value === "ai-employees") openPage(pages.aiEmployeeCreate);
  else if (screen.value === "billing") openPage(pages.usage);
}

async function createEnterprise() {
  formError.value = "";
  if (!createDraft.name.trim()) { formError.value = "请输入企业或团队名称"; return; }
  submitting.value = true;
  try {
    const created = await enterpriseAPI.create(createDraft.name.trim());
    await userStore.loadEnterpriseContexts();
    await userStore.switchContext({ type: "ENTERPRISE", tenantId: created.tenant.id, organizationId: created.organization.id, role: created.context.currentRole });
    uni.showToast({ title: "企业创建成功", icon: "success" });
    replacePage(pages.overview);
  } catch (error) { formError.value = apiErrorMessage(error, "企业创建失败"); } finally { submitting.value = false; }
}

async function joinEnterprise() {
  formError.value = ""; submitting.value = true;
  try {
    if (joinMode.value === "invitation") {
      if (!joinDraft.code.trim()) throw new Error("请输入企业邀请码");
      const context = await enterpriseAPI.acceptInvitation(joinDraft.code.trim());
      await userStore.loadEnterpriseContexts();
      await userStore.switchContext({ type: "ENTERPRISE", tenantId: context.tenantId, organizationId: context.organizationId, role: context.currentRole });
      uni.showToast({ title: "已加入企业", icon: "success" }); replacePage(pages.overview);
    } else {
      if (!joinDraft.tenantId.trim()) throw new Error("请输入企业ID");
      await enterpriseAPI.createJoinRequest({ tenantId: joinDraft.tenantId.trim(), reason: joinDraft.reason.trim() });
      operationState.value = "reviewing";
    }
  } catch (error) { formError.value = apiErrorMessage(error, "加入企业失败"); } finally { submitting.value = false; }
}

function scanInvitationCode() {
  uni.scanCode({ scanType: ["qrCode"], success: result => { joinDraft.code = String(result.result || "").trim(); if (screen.value !== "join") openPage(`${pages.join}?code=${encodeURIComponent(joinDraft.code)}`); }, fail: () => uni.showToast({ title: "未识别到邀请码", icon: "none" }) });
}

async function switchWorkspace(context: EnterpriseContext) {
  if (context.current || submitting.value) return;
  submitting.value = true;
  try {
    await userStore.switchContext({ type: context.type, tenantId: context.tenantId, organizationId: context.organizationId, role: context.currentRole });
    if (context.type === "PERSONAL") uni.switchTab({ url: "/pages/user/UserMinePage", fail: () => uni.reLaunch({ url: "/pages/user/UserMinePage" }) });
    else replacePage(pages.overview);
  } catch (error) { uni.showToast({ title: apiErrorMessage(error, "工作空间切换失败"), icon: "none" }); } finally { submitting.value = false; }
}

function createOrganization(parentId = "") {
  if (!can("enterprise.organization.create")) return;
  uni.showModal({ title: parentId ? "新建下级部门" : "新建部门", editable: true, placeholderText: "请输入部门名称", success: async result => {
    if (!result.confirm || !String(result.content || "").trim()) return;
    try { await enterpriseAPI.createOrganization({ parentId, organizationType: "DEPARTMENT", name: String(result.content).trim(), metadata: {} }); await enterpriseStore.loadOrganizations(true); uni.showToast({ title: "部门已创建", icon: "success" }); } catch (error) { uni.showToast({ title: apiErrorMessage(error, "部门创建失败"), icon: "none" }); }
  } });
}

function manageOrganization(item: EnterpriseOrganization) {
  const actions: Array<{ label: string; run: () => void }> = [];
  if (can("enterprise.organization.create")) actions.push({ label: "新建下级部门", run: () => createOrganization(item.id) });
  if (can("enterprise.organization.update")) actions.push({ label: "重命名部门", run: () => renameOrganization(item) }, { label: "移动部门", run: () => moveOrganization(item) });
  if (can("enterprise.organization.delete")) actions.push({ label: "删除部门", run: () => deleteOrganization(item) });
  if (!actions.length) return;
  uni.showActionSheet({ itemList: actions.map(action => action.label), success: result => actions[result.tapIndex]?.run() });
}

function renameOrganization(item: EnterpriseOrganization) {
  uni.showModal({ title: "编辑部门", editable: true, content: item.name, placeholderText: "请输入部门名称", success: async result => {
    const name = String(result.content || "").trim(); if (!result.confirm || !name || name === item.name) return;
    try { await enterpriseAPI.updateOrganization(item.id, { name, organizationType: item.organizationType, status: item.status, metadata: item.metadata || {} }); await enterpriseStore.loadOrganizations(true); } catch (error) { uni.showToast({ title: apiErrorMessage(error, "部门更新失败"), icon: "none" }); }
  } });
}

function moveOrganization(item: EnterpriseOrganization) {
  const targets = flatOrganizations.value.filter(target => target.id !== item.id);
  if (!targets.length) return;
  uni.showActionSheet({ itemList: targets.map(target => target.name), success: async result => { const target = targets[result.tapIndex]; if (!target) return; try { await enterpriseAPI.moveOrganization(item.id, target.id); await enterpriseStore.loadOrganizations(true); } catch (error) { uni.showToast({ title: apiErrorMessage(error, "部门移动失败"), icon: "none" }); } } });
}

function deleteOrganization(item: EnterpriseOrganization) {
  uni.showModal({ title: "删除部门", content: "有关联成员、下级部门或资源时将无法删除。确认继续？", confirmText: "删除", confirmColor: "#F04438", success: async result => { if (!result.confirm) return; try { await enterpriseAPI.deleteOrganization(item.id); await enterpriseStore.loadOrganizations(true); } catch (error) { uni.showToast({ title: apiErrorMessage(error, "部门删除失败，请先处理关联成员或资源"), icon: "none" }); } } });
}

function chooseMemberOrganization() { if (!selectedMember.value) return; const items = flatOrganizations.value; uni.showActionSheet({ itemList: items.map(item => item.name), success: result => { const item = items[result.tapIndex]; if (item) updateSelectedMember({ primaryOrganizationId: item.id }); } }); }
function chooseMemberRole() { if (!selectedMember.value) return; const items = roles.value.filter(item => item.assignable); uni.showActionSheet({ itemList: items.map(item => roleName(item.code)), success: result => { const item = items[result.tapIndex]; if (item) updateSelectedMember({ roles: [item.code] }); } }); }
function chooseMemberScope() { const values = ["TENANT_ALL", "ORG_AND_CHILDREN", "ORG_SELF", "SELF"]; uni.showActionSheet({ itemList: values.map(scopeName), success: result => updateSelectedMember({ dataScope: values[result.tapIndex] }) }); }
async function updateSelectedMember(input: { primaryOrganizationId?: string; roles?: AppRole[]; dataScope?: string }) { if (!selectedMember.value) return; try { selectedMember.value = await enterpriseAPI.updateMember(selectedMember.value.id, input); enterpriseStore.members = enterpriseStore.members.map(item => item.id === selectedMember.value?.id ? selectedMember.value : item) as EnterpriseMember[]; } catch (error) { uni.showToast({ title: apiErrorMessage(error, "成员更新失败"), icon: "none" }); } }
function disableSelectedMember() { if (!selectedMember.value) return; uni.showModal({ title: "禁用成员", content: "禁用后该成员将无法使用企业数据和算力。", confirmText: "禁用", success: async result => { if (!result.confirm || !selectedMember.value) return; try { selectedMember.value = await enterpriseAPI.disableMember(selectedMember.value.id); } catch (error) { uni.showToast({ title: apiErrorMessage(error, "成员禁用失败"), icon: "none" }); } } }); }
function removeSelectedMember() { if (!selectedMember.value) return; uni.showModal({ title: "移出企业", content: "成员的企业权限将被立即收回，确认继续？", confirmText: "移出", confirmColor: "#F04438", success: async result => { if (!result.confirm || !selectedMember.value) return; try { await enterpriseAPI.removeMember(selectedMember.value.id); uni.showToast({ title: "成员已移出", icon: "success" }); replacePage(pages.members); } catch (error) { uni.showToast({ title: apiErrorMessage(error, "成员移出失败"), icon: "none" }); } } }); }

async function createInvitation() { submitting.value = true; try { lastInvitation.value = await enterpriseAPI.createInvitation({ invitedEmail: inviteDraft.email.trim() || undefined, defaultOrganizationId: currentEnterprise.value?.organizationId, defaultRole: "ENTERPRISE_MEMBER", expiresInHours: 168 }); uni.showToast({ title: "邀请码已生成", icon: "success" }); } catch (error) { uni.showToast({ title: apiErrorMessage(error, "邀请创建失败"), icon: "none" }); } finally { submitting.value = false; } }
async function reviewJoin(item: EnterpriseJoinRequest, approved: boolean) { try { await enterpriseAPI.reviewJoinRequest(item.id, approved); await enterpriseStore.loadJoinRequests(true); uni.showToast({ title: approved ? "已同意申请" : "已拒绝申请", icon: "success" }); } catch (error) { uni.showToast({ title: apiErrorMessage(error, "审核失败"), icon: "none" }); } }

function selectAIOrganization(event: { detail: { value: string | number } }) { const item = flatOrganizations.value[Number(event.detail.value)]; aiDraft.organizationId = item?.id || ""; }
function selectAIKnowledgeBase(event: { detail: { value: string | number } }) { const item = knowledgeBases.value[Number(event.detail.value)]; aiDraft.knowledgeBaseId = item?.id || ""; }
async function createAIEmployee() { formError.value = ""; if (!aiDraft.name.trim()) { formError.value = "请输入AI员工名称"; return; } submitting.value = true; try { const employee = await enterpriseAPI.createAIEmployee({ name: aiDraft.name.trim(), description: aiDraft.description.trim(), modelName: aiDraft.modelName.trim() || "gpt-5.2-chat-latest", systemPrompt: aiDraft.description.trim() || `你是${aiDraft.name.trim()}，请按照企业岗位职责完成工作。`, status: "ACTIVE", config: { organizationId: aiDraft.organizationId, organizationName: selectedAIOrganization.value?.name || "", position: aiDraft.position.trim(), ownerName: aiDraft.ownerName.trim(), monthlyBudget: Number(aiDraft.monthlyBudget) || 0 } }); let bindingFailed = false; if (aiDraft.knowledgeBaseId) { try { await enterpriseAPI.replaceAIEmployeeBindings(employee.id, [aiDraft.knowledgeBaseId]); } catch { bindingFailed = true; } } enterpriseStore.aiEmployees = [employee, ...enterpriseStore.aiEmployees]; uni.showToast({ title: bindingFailed ? "已发布，知识库绑定失败" : "AI员工已发布", icon: bindingFailed ? "none" : "success" }); replacePage(`${pages.aiEmployeeDetail}?id=${encodeURIComponent(employee.id)}`); } catch (error) { formError.value = apiErrorMessage(error, "AI员工创建失败"); } finally { submitting.value = false; } }

function chooseLicense() { uni.chooseImage({ count: 1, sizeType: ["compressed"], sourceType: ["album", "camera"], success: async result => { const path = result.tempFilePaths[0]; const size = (result as unknown as { tempFiles?: Array<{ size?: number }> }).tempFiles?.[0]?.size || 0; if (!path) return; if (size > 10 * 1024 * 1024) { uni.showToast({ title: "营业执照不能超过10MB", icon: "none" }); return; } licensePreview.value = path; submitting.value = true; try { licenseDocumentUrl.value = await uploadEnterpriseDocument(path); uni.showToast({ title: "营业执照已上传", icon: "success" }); } catch (error) { licensePreview.value = ""; uni.showToast({ title: apiErrorMessage(error, "营业执照上传失败"), icon: "none" }); } finally { submitting.value = false; } } }); }
async function submitCertification() { formError.value = ""; if (!certDraft.legalName.trim()) { formError.value = "请输入企业名称"; return; } if (!/^[0-9A-Z]{18}$/.test(certDraft.creditCode.trim().toUpperCase())) { formError.value = "请输入18位统一社会信用代码"; return; } if (!licenseDocumentUrl.value) { formError.value = "请先上传营业执照"; return; } submitting.value = true; try { certificationResult.value = await enterpriseAPI.submitCertification({ legalName: certDraft.legalName.trim(), unifiedSocialCreditCode: certDraft.creditCode.trim().toUpperCase(), legalRepresentativeName: certDraft.legalRepresentative.trim(), documentUrls: [licenseDocumentUrl.value], metadata: { contactPhone: certDraft.contactPhone.trim() } }); await userStore.loadEnterpriseContexts(); uni.showToast({ title: "认证资料已提交", icon: "success" }); } catch (error) { formError.value = apiErrorMessage(error, "认证提交失败"); } finally { submitting.value = false; } }

function flattenOrganizations(items: EnterpriseOrganization[]): EnterpriseOrganization[] { return items.flatMap(item => [item, ...flattenOrganizations(item.children || [])]); }
function roleName(role?: AppRole | string) { return role && roleLabels[role as AppRole] ? roleLabels[role as AppRole] : String(role || "企业成员"); }
function certificationName(value?: string) { const names: Record<string, string> = { VERIFIED: "已认证", PENDING: "审核中", REJECTED: "认证驳回", UNVERIFIED: "未认证", NOT_REQUIRED: "无需认证" }; return names[String(value || "UNVERIFIED").toUpperCase()] || String(value || "未认证"); }
function memberStatusName(value?: string) { return String(value || "").toUpperCase() === "ACTIVE" ? "已启用" : "已禁用"; }
function subscriptionStatusName(value?: string) { const names: Record<string, string> = { ACTIVE: "生效中", TRIALING: "试用中", EXPIRED: "已到期", SUSPENDED: "已暂停", CANCELLED: "已取消" }; return names[String(value || "").toUpperCase()] || String(value || "--"); }
function scopeName(value?: string) { const names: Record<string, string> = { TENANT_ALL: "整个企业", ORG_AND_CHILDREN: "本部门及下级", ORG_SELF: "本部门", OWNER: "负责人数据", SELF: "仅本人" }; return names[String(value || "").toUpperCase()] || String(value || "--"); }
function avatarText(value?: string) { return Array.from(String(value || "企").trim())[0] || "企"; }
function formatNumber(value: unknown) { const number = Number(value); return Number.isFinite(number) ? number.toLocaleString("zh-CN") : "0"; }
function formatCurrency(value: unknown) { const number = Number(value); return Number.isFinite(number) ? `¥${(number / 100).toFixed(2)}` : "¥0.00"; }
function formatDate(value?: string) { if (!value) return "--"; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`; }
function relativeTime(value?: string) { if (!value) return ""; const date = new Date(value); const seconds = Math.max(0, (Date.now() - date.getTime()) / 1000); if (seconds < 3600) return `${Math.max(1, Math.floor(seconds / 60))}分钟前`; if (seconds < 86400) return `${Math.floor(seconds / 3600)}小时前`; return `${Math.floor(seconds / 86400)}天前`; }
function activityIcon(action: string) { if (action.includes("member")) return "人"; if (action.includes("organization")) return "部"; if (action.includes("certification")) return "证"; return "企"; }
function activityCopy(item: { action: string; resourceType: string; status: string }) { const labels: Record<string, string> = { "enterprise.organization.create": "新建了部门", "enterprise.organization.update": "更新了部门", "enterprise.organization.move": "移动了部门", "enterprise.organization.delete": "删除了部门", "enterprise.member.update": "更新了成员权限", "enterprise.member.disable": "禁用了成员", "enterprise.member.remove": "移出了成员", "enterprise.invitation.create": "创建了企业邀请", "enterprise.certification.submit": "提交了企业认证" }; return labels[item.action] || `${item.action} · ${item.status}`; }
function permissionLabel(permission: string) { const group = permission.split(".")[1] || permission.split(":")[0]; const names: Record<string, string> = { overview: "企业概览", organization: "组织架构", member: "成员管理", role: "角色权限", billing: "算力与账单", settings: "企业设置", certification: "企业认证", audit: "审计日志", ai: "AI能力管理" }; return names[group] || permission; }
function employeeConfig(item: EnterpriseAIEmployee, key: string) { return item.config && typeof item.config[key] !== "undefined" ? item.config[key] : ""; }
function employeePosition(item: EnterpriseAIEmployee) { return String(employeeConfig(item, "position") || "企业AI员工"); }
function employeeOwner(item: EnterpriseAIEmployee) { return String(employeeConfig(item, "ownerName") || "未指定"); }
function employeeOrganization(item: EnterpriseAIEmployee) { return String(employeeConfig(item, "organizationName") || "未分配部门"); }
function employeeBudget(item: EnterpriseAIEmployee) { return Number(employeeConfig(item, "monthlyBudget")) || 0; }
function copyText(value: string) { uni.setClipboardData({ data: value, success: () => uni.showToast({ title: "已复制", icon: "success" }) }); }
function contactService() { uni.showModal({ title: "联系平台客服", content: "请通过知启云AI官方客服渠道提交企业名称和账号信息。", showCancel: false }); }
function showAuditSummary() { uni.showModal({ title: "企业操作日志", content: auditLogs.value.slice(0, 8).map(item => `${formatDate(item.createdAt)} ${activityCopy(item)}`).join("\n") || "暂无操作日志", showCancel: false }); }
function apiErrorMessage(error: unknown, fallback: string) { return error instanceof Error && error.message ? error.message : fallback; }

onMounted(() => { if (screen.value === "join" && routeOption("code")) joinDraft.code = routeOption("code"); void load(); });
onPullDownRefresh(() => { void load(); });
</script>

<style src="../../styles/enterprise-center.css"></style>
