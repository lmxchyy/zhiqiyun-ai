alter table generation_tasks
  add column if not exists billing_source varchar(30),
  add column if not exists enterprise_member_id uuid references enterprise_members(id),
  add column if not exists billing_transaction_id uuid references enterprise_quota_transactions(id);

alter table enterprise_quota_transactions
  add column if not exists reference_type varchar(50),
  add column if not exists reference_id varchar(100);
