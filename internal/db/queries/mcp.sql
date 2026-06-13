-- name: CreateMCPServer :one
insert into mcp_servers (name, transport, endpoint, description, risk_tier, allowed_tools, tenant_id)
values ($1, $2, $3, $4, $5, $6, $7)
returning *;

-- name: ListMCPServers :many
select * from mcp_servers order by created_at desc;

-- name: GetMCPServerByName :one
select * from mcp_servers where name = $1;

-- name: UpdateMCPServerStatus :one
update mcp_servers set status = $2, updated_at = now() where id = $1 returning *;

-- name: DeleteMCPServer :exec
delete from mcp_servers where id = $1;

-- name: CreateMCPGrant :one
insert into mcp_grants (server_id, owner, expires_at, granted_by, tenant_id, status)
values ($1, $2, $3, $4, $5, $6)
returning *;

-- name: ListMCPGrants :many
select g.*, s.name as server_name, s.risk_tier as server_risk_tier
from mcp_grants g
join mcp_servers s on s.id = g.server_id
order by g.created_at desc;

-- name: FindLatestMCPGrant :one
-- Most recent grant for a server+owner in ANY state (to avoid duplicate requests).
select g.* from mcp_grants g
where g.server_id = $1 and lower(g.owner) = lower($2)
order by g.created_at desc
limit 1;

-- name: SetMCPGrantStatus :one
update mcp_grants set status = $2 where id = $1 returning *;

-- name: FindActiveMCPGrant :one
select g.* from mcp_grants g
where g.server_id = $1 and lower(g.owner) = lower($2) and g.status = 'active'
order by g.created_at desc
limit 1;

-- name: RevokeMCPGrant :one
update mcp_grants set status = 'revoked', revoked_at = now()
where id = $1
returning *;
