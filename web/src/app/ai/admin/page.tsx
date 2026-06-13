import Link from "next/link";
import { Building2 } from "lucide-react";
import { InvitesPanel, OrgUsersPanel, WorkspacesPanel } from "@/components/ai-org-admin";
import { EmptyState } from "@/components/empty-state";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui/card";
import { button } from "@/components/ui/button";
import { fetchAIProviders, fetchAIOrgUsers, fetchAIInvites, fetchAIWorkspaces } from "@/lib/api";
import type { AIInvite, AIOrgUser, AIWorkspace } from "@/lib/types";
import { cn } from "@/lib/utils";

export const metadata = { title: "AI Org Admin" };

// Providers that expose org administration (the AdminProvisioner capability,
// ADR-0022). Anthropic today; OpenAI/others fall through to the not-supported note.
const ADMIN_TYPES = new Set(["anthropic", "openai"]);
const TYPE_LABEL: Record<string, string> = { anthropic: "Anthropic", openai: "OpenAI" };

export default async function AIOrgAdminPage({
  searchParams,
}: {
  searchParams: Promise<{ provider?: string }>;
}) {
  const { provider: selected } = await searchParams;
  const providers = await fetchAIProviders();
  const governable = providers.filter((p) => ADMIN_TYPES.has(p.type) && p.status === "active");

  if (governable.length === 0) {
    return (
      <div className="space-y-6">
        <PageHeader title="AI Org Admin" description="Manage organization users, roles, and workspaces directly in your AI provider." />
        <Card>
          <EmptyState
            icon={Building2}
            title="No governable AI provider"
            description="Add an Anthropic provider with an admin key (admin_api_key, sk-ant-admin…) to manage its organization - invite users, set roles, and create workspaces from here."
            action={{ href: "/ai/providers", label: "AI providers" }}
          />
        </Card>
      </div>
    );
  }

  const active = governable.find((p) => p.name === selected) ?? governable[0];

  // Reads run in parallel; each degrades to an error note rather than a blank
  // table so an invalid/non-admin key reads as a real message.
  let users: AIOrgUser[] = [];
  let workspaces: AIWorkspace[] = [];
  let invites: AIInvite[] = [];
  let loadError: string | null = null;
  try {
    [users, workspaces, invites] = await Promise.all([
      fetchAIOrgUsers(active.name),
      fetchAIWorkspaces(active.name),
      fetchAIInvites(active.name),
    ]);
  } catch (err) {
    loadError = err instanceof Error ? err.message : String(err);
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="AI Org Admin"
        description={`Manage the ${active.type} organization for "${active.name}" - users, roles, and workspaces, governed and audited through OPORD.`}
      />

      {governable.length > 1 ? (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm text-muted-foreground">Provider:</span>
          {governable.map((p) => (
            <Link
              key={p.id}
              href={`/ai/admin?provider=${encodeURIComponent(p.name)}`}
              className={cn(button({ variant: p.name === active.name ? "primary" : "outline", size: "sm" }))}
            >
              {p.name}
            </Link>
          ))}
        </div>
      ) : (
        // Only one provider supports org admin, so there's no switcher. Make it
        // clear this page is provider-specific (not a global console) and how to
        // manage another vendor's org (e.g. OpenAI) here too.
        <p className="text-sm text-muted-foreground">
          This is the <span className="font-medium text-foreground">{TYPE_LABEL[active.type] ?? active.type}</span> org
          (<span className="font-medium text-foreground">{active.name}</span>). Org admin is supported for Anthropic and
          OpenAI providers
          {[...ADMIN_TYPES].filter((t) => t !== active.type).length > 0 && (
            <>
              {" "}
              — add{" "}
              {[...ADMIN_TYPES]
                .filter((t) => t !== active.type)
                .map((t) => TYPE_LABEL[t] ?? t)
                .join(" or ")}{" "}
              on the{" "}
              <Link href="/ai/providers" className="font-medium text-foreground underline-offset-4 hover:underline">
                AI Providers
              </Link>{" "}
              page to manage its organization here too
            </>
          )}
          . Models live on the{" "}
          <Link href="/ai/models" className="font-medium text-foreground underline-offset-4 hover:underline">
            AI Models
          </Link>{" "}
          page.
        </p>
      )}

      {loadError ? (
        <Card>
          <EmptyState
            icon={Building2}
            title="Couldn't reach the provider organization"
            description={`${loadError}. Check that "${active.name}" has a valid admin key (admin_api_key) and run Check on the AI Providers page.`}
            action={{ href: "/ai/providers", label: "AI providers" }}
          />
        </Card>
      ) : (
        <>
          <OrgUsersPanel provider={active.name} providerType={active.type} users={users} />
          <InvitesPanel invites={invites} />
          <WorkspacesPanel provider={active.name} providerType={active.type} workspaces={workspaces} users={users} />
        </>
      )}
    </div>
  );
}
