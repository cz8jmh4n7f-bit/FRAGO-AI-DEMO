import { Workflow } from "lucide-react";
import { AuthorizeTester, MCPServerActions, MCPGrantActions, RegisterMCPServerButton, RequestMCPAccessButton } from "@/components/mcp-actions";
import { EmptyState } from "@/components/empty-state";
import { PageHeader } from "@/components/page-header";
import { Badge, type BadgeVariant } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { fetchMCPGrants, fetchMCPServers } from "@/lib/api";

export const metadata = { title: "Agents & MCP" };

const riskVariant: Record<string, BadgeVariant> = {
  low: "success",
  medium: "info",
  high: "warning",
  critical: "danger",
};

export default async function MCPGovernancePage() {
  const [servers, grants] = await Promise.all([fetchMCPServers(), fetchMCPGrants()]);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Agents & MCP governance"
        description="Govern which MCP servers and tools your agents may use - register approved servers, grant teams access, and enforce it at connect time."
      >
        <div className="flex gap-2"><RequestMCPAccessButton servers={servers} /><RegisterMCPServerButton /></div>
      </PageHeader>

      {servers.length > 0 && (
        <Card className="p-4">
          <AuthorizeTester servers={servers} />
        </Card>
      )}

      <Card className="overflow-hidden p-0">
        <div className="border-b border-border px-5 py-3">
          <h2 className="text-sm font-semibold">Approved MCP servers ({servers.length})</h2>
        </div>
        {servers.length === 0 ? (
          <EmptyState
            icon={Workflow}
            title="No MCP servers registered"
            description="Register the MCP servers your agents are allowed to connect to. Each gets a risk tier and an optional tool allow-list, enforced by the authorize check."
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
                  <th scope="col" className="px-5 py-3 font-medium">Server</th>
                  <th scope="col" className="px-5 py-3 font-medium">Risk</th>
                  <th scope="col" className="px-5 py-3 font-medium">Transport</th>
                  <th scope="col" className="px-5 py-3 font-medium">Allowed tools</th>
                  <th scope="col" className="px-5 py-3 font-medium">Status</th>
                  <th scope="col" className="px-5 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {servers.map((s) => (
                  <tr key={s.id} className="border-b border-border last:border-0 align-top hover:bg-muted/60">
                    <td className="px-5 py-3">
                      <div className="font-medium text-foreground">{s.name}</div>
                      {s.endpoint && <div className="max-w-xs truncate font-mono text-xs text-muted-foreground">{s.endpoint}</div>}
                    </td>
                    <td className="px-5 py-3"><Badge variant={riskVariant[s.riskTier] ?? "info"}>{s.riskTier}</Badge></td>
                    <td className="px-5 py-3 text-muted-foreground">{s.transport}</td>
                    <td className="px-5 py-3 text-muted-foreground">
                      {s.allowedTools.length ? s.allowedTools.join(", ") : <span className="italic">all tools</span>}
                    </td>
                    <td className="px-5 py-3"><Badge variant={s.status === "active" ? "success" : "default"}>{s.status}</Badge></td>
                    <td className="px-5 py-3"><MCPServerActions server={s} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Card className="overflow-hidden p-0">
        <div className="border-b border-border px-5 py-3">
          <h2 className="text-sm font-semibold">Access grants ({grants.filter((g) => g.status === "active").length} active)</h2>
        </div>
        {grants.length === 0 ? (
          <p className="px-5 py-8 text-center text-sm text-muted-foreground">No grants yet. Use “Grant” on a server to allow a team.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
                  <th scope="col" className="px-5 py-3 font-medium">Owner</th>
                  <th scope="col" className="px-5 py-3 font-medium">Server</th>
                  <th scope="col" className="px-5 py-3 font-medium">Status</th>
                  <th scope="col" className="px-5 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {grants.map((g) => (
                  <tr key={g.id} className="border-b border-border last:border-0 hover:bg-muted/60">
                    <td className="px-5 py-3 font-medium">{g.owner}</td>
                    <td className="px-5 py-3 text-muted-foreground">{g.server}</td>
                    <td className="px-5 py-3"><Badge variant={g.status === "active" ? "success" : "default"}>{g.status}</Badge></td>
                    <td className="px-5 py-3 text-right"><MCPGrantActions grant={g} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
