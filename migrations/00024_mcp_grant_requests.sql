-- +goose Up
-- MCP access can now be REQUESTED (self-service) and approved, not only granted
-- directly by an operator. A 'requested' grant does not authorize (AuthorizeMCP
-- only honors 'active'); an operator approves it to 'active' or rejects it.

alter table mcp_grants drop constraint if exists mcp_grants_status_check;
alter table mcp_grants add constraint mcp_grants_status_check
    check (status in ('requested', 'active', 'revoked', 'expired', 'rejected'));

-- +goose Down
alter table mcp_grants drop constraint if exists mcp_grants_status_check;
alter table mcp_grants add constraint mcp_grants_status_check
    check (status in ('active', 'revoked', 'expired'));
