import { ShieldCheck } from "lucide-react";
import { AddAIGovernanceButton } from "@/components/ai-governance-actions";
import { EmptyState } from "@/components/empty-state";
import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { fetchAIPolicies } from "@/lib/api";
import { PolicyRowActions } from "@/components/ai-governance-row-actions";
import { formatDate } from "@/lib/utils";

export const metadata = { title: "AI Policies" };

export default async function AIPoliciesPage() {
  const policies = await fetchAIPolicies();

  return (
    <div className="space-y-6">
      <PageHeader title="AI Policies" description="Request questionnaire and model-access policy records for AI governance.">
        <AddAIGovernanceButton kind="policy" />
      </PageHeader>

      {policies.length === 0 ? (
        <Card>
          <EmptyState icon={ShieldCheck} title="No AI policies" description="Create a policy to document approved models, expiry limits, and justification requirements." />
        </Card>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {policies.map((p) => (
            <Card key={p.id} className="p-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <h2 className="text-sm font-semibold text-foreground">{p.name}</h2>
                  <p className="mt-1 text-xs text-muted-foreground">Updated {formatDate(p.updatedAt)}</p>
                </div>
                <Badge variant={p.status === "active" ? "success" : "default"}>{p.status}</Badge>
              </div>
              <pre className="mt-4 max-h-56 overflow-auto rounded-lg bg-muted p-3 text-xs text-muted-foreground">
                {JSON.stringify(p.rules ?? {}, null, 2)}
              </pre>
              <div className="mt-3 flex justify-end">
                <PolicyRowActions policy={p} />
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
