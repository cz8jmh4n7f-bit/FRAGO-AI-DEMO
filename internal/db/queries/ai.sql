-- name: CreateAIProvider :one
insert into ai_providers (name, type, config, tenant_id)
values ($1, $2, $3, $4)
returning *;

-- name: ListAIProviders :many
select * from ai_providers order by created_at desc;

-- name: GetAIProvider :one
select * from ai_providers where id = $1;

-- name: GetAIProviderByName :one
select * from ai_providers where name = $1;

-- name: UpdateAIProvider :one
update ai_providers
set name = $2, type = $3, config = $4, status = $5, updated_at = now()
where id = $1
returning *;

-- name: CreateAIProviderCredential :one
insert into ai_provider_credentials (provider_id, secret_ref, scopes)
values ($1, $2, $3)
returning *;

-- name: ListAIProviderCredentials :many
select * from ai_provider_credentials order by created_at desc;

-- name: GetAIProviderCredentialByProvider :one
select * from ai_provider_credentials
where provider_id = $1
order by created_at desc
limit 1;

-- name: CreateAIService :one
insert into ai_services (provider_id, name, slug, category, description, request_schema, default_expiration_days, requires_approval)
values ($1, $2, $3, $4, $5, $6, $7, $8)
returning *;

-- name: ListAIServices :many
select
    s.*,
    p.name as provider_name,
    p.type as provider_type
from ai_services s
join ai_providers p on p.id = s.provider_id
order by s.created_at desc;

-- name: GetAIService :one
select
    s.*,
    p.name as provider_name,
    p.type as provider_type
from ai_services s
join ai_providers p on p.id = s.provider_id
where s.id = $1;

-- name: GetAIServiceBySlug :one
select
    s.*,
    p.name as provider_name,
    p.type as provider_type
from ai_services s
join ai_providers p on p.id = s.provider_id
where s.slug = $1;

-- name: CreateAIServiceInstance :one
insert into ai_service_instances (
    service_id, request_id, provider_access_id, owner, tenant_id, workspace,
    status, spec, observed, provisioned_at, expires_at
)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
returning *;

-- name: GetAIServiceInstance :one
select * from ai_service_instances where id = $1;

-- name: ListAIServiceInstances :many
select
    i.*,
    s.name as service_name,
    s.slug as service_slug,
    p.name as provider_name,
    p.type as provider_type
from ai_service_instances i
join ai_services s on s.id = i.service_id
join ai_providers p on p.id = s.provider_id
order by i.created_at desc;

-- name: RevokeAIServiceInstance :one
update ai_service_instances
set status = 'revoked', revoked_at = now(), updated_at = now()
where id = $1
returning *;

-- name: UpdateAIServiceInstanceStatus :one
update ai_service_instances
set status = $2, observed = $3, updated_at = now()
where id = $1
returning *;

-- name: RecertifyAIServiceInstance :one
update ai_service_instances
set expires_at = $2, updated_at = now()
where id = $1
returning *;

-- name: CreateAIUsageRecord :one
insert into ai_usage_records (
    instance_id, provider_id, period_start, period_end, metric, quantity, unit, cost_usd, raw
)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
returning *;

-- name: FindAIUsageRecordByImportKey :one
select *
from ai_usage_records
where provider_id = $1
  and period_start = $2
  and period_end = $3
  and metric = $4
  and raw->>'import_key' = sqlc.arg(import_key)::text
limit 1;

-- name: FindAIKeySpendRecord :one
-- The single cumulative-spend row for a LiteLLM virtual key (one per instance);
-- ImportLiteLLMSpend updates it in place so budgets count the live total once.
select *
from ai_usage_records
where instance_id = $1
  and metric = 'cost_usd'
  and raw->>'source' = 'litellm_spend'
limit 1;

-- name: UpdateAIUsageRecordCost :exec
-- The LiteLLM cumulative row is the live lifetime total for a key; bump BOTH
-- period_start and period_end to now() so it always falls inside the current
-- budget/quota period (otherwise the original-period row drops out next period
-- and the spend silently stops counting).
update ai_usage_records
set cost_usd = $2, quantity = $2, period_start = now(), period_end = now()
where id = $1;

-- name: ListAIUsageRecords :many
select
    u.*,
    p.name as provider_name,
    i.owner as owner,
    i.workspace as workspace,
    i.tenant_id as tenant_id
from ai_usage_records u
join ai_providers p on p.id = u.provider_id
left join ai_service_instances i on i.id = u.instance_id
order by u.period_start desc, u.created_at desc;

-- name: CreateAIBudget :one
insert into ai_budgets (tenant_id, scope, scope_ref, limit_usd, period, soft_threshold_pct, hard_threshold_pct)
values ($1, $2, $3, $4, $5, $6, $7)
returning *;

-- name: ListAIBudgets :many
select * from ai_budgets order by created_at desc;

-- name: GetAIBudget :one
select * from ai_budgets where id = $1;

-- name: UpdateAIBudget :one
update ai_budgets
set scope = $2, scope_ref = $3, limit_usd = $4, period = $5, soft_threshold_pct = $6, hard_threshold_pct = $7, updated_at = now()
where id = $1
returning *;

-- name: DeleteAIBudget :exec
delete from ai_budgets where id = $1;

-- name: CreateAIQuota :one
insert into ai_quotas (service_id, tenant_id, metric, limit_quantity, period, enforcement)
values ($1, $2, $3, $4, $5, $6)
returning *;

-- name: ListAIQuotas :many
select * from ai_quotas order by created_at desc;

-- name: GetAIQuota :one
select * from ai_quotas where id = $1;

-- name: UpdateAIQuota :one
update ai_quotas
set limit_quantity = $2, period = $3, enforcement = $4
where id = $1
returning *;

-- name: DeleteAIQuota :exec
delete from ai_quotas where id = $1;

-- name: CreateAIAccessPolicy :one
insert into ai_access_policies (name, tenant_id, rules, status)
values ($1, $2, $3, $4)
returning *;

-- name: UpdateAIAccessPolicy :one
update ai_access_policies
set rules = $2, status = $3, updated_at = now()
where id = $1
returning *;

-- name: DeleteAIAccessPolicy :exec
delete from ai_access_policies where id = $1;

-- name: ListAIAccessPolicies :many
select * from ai_access_policies order by created_at desc;

-- name: GetAIAccessPolicy :one
select * from ai_access_policies where id = $1;

-- name: UpsertAIModelCatalog :one
insert into ai_model_catalog (provider_id, model, display_name, modality, status, metadata)
values ($1, $2, $3, $4, $5, $6)
on conflict (provider_id, model) do update
set display_name = excluded.display_name,
    modality = excluded.modality,
    status = excluded.status,
    metadata = excluded.metadata,
    updated_at = now()
returning *;

-- name: ListAIModelCatalog :many
select
    m.*,
    p.name as provider_name,
    p.type as provider_type
from ai_model_catalog m
join ai_providers p on p.id = m.provider_id
order by p.name, m.model;

-- name: ListAIExpiringInstances :many
select
    i.*,
    s.name as service_name,
    s.slug as service_slug,
    p.name as provider_name,
    p.type as provider_type
from ai_service_instances i
join ai_services s on s.id = i.service_id
join ai_providers p on p.id = s.provider_id
where i.status in ('active', 'suspended')
  and i.expires_at is not null
  and i.expires_at <= now() + ($1::int * interval '1 day')
order by i.expires_at asc;

-- name: CreateAIAuditEvent :one
insert into ai_audit_events (actor, tenant_id, subject_type, subject_id, action, message, fields)
values ($1, $2, $3, $4, $5, $6, $7)
returning *;

-- name: ListAIAuditEvents :many
select * from ai_audit_events order by created_at desc limit $1;

-- name: CountActiveAIInstancesByProvider :one
select count(*) from ai_service_instances i
join ai_services s on s.id = i.service_id
where s.provider_id = $1 and i.status in ('active', 'suspended');

-- name: DeleteAIServiceInstancesByProvider :exec
delete from ai_service_instances
where service_id in (select id from ai_services where provider_id = $1);

-- name: DeleteAIProvider :exec
delete from ai_providers where id = $1;
