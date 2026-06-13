import { Gauge, SlidersHorizontal } from "lucide-react";
import { AddAIGovernanceButton } from "@/components/ai-governance-actions";
import { EmptyState } from "@/components/empty-state";
import { PageHeader } from "@/components/page-header";
import { StatCard } from "@/components/stat-card";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { fetchAIQuotas } from "@/lib/api";
import { QuotaRowActions } from "@/components/ai-governance-row-actions";
import { formatDate } from "@/lib/utils";

export const metadata = { title: "AI Quotas" };

export default async function AIQuotasPage() {
  const quotas = await fetchAIQuotas();
  const blocking = quotas.filter((q) => q.enforcement === "block").length;

  return (
    <div className="space-y-6">
      <PageHeader title="AI Quotas" description="Token, request, and service-level AI guardrails.">
        <AddAIGovernanceButton kind="quota" />
      </PageHeader>

      <div className="grid gap-3 sm:grid-cols-2">
        <StatCard icon={SlidersHorizontal} label="Quotas" value={quotas.length} hint="Configured limits" />
        <StatCard icon={Gauge} label="Blocking" value={blocking} hint="Hard enforcement policies" accent="bg-warning/10 text-warning" />
      </div>

      {quotas.length === 0 ? (
        <Card>
          <EmptyState icon={SlidersHorizontal} title="No AI quotas" description="Add quota guardrails for tokens, requests, seats, or workspace usage." />
        </Card>
      ) : (
        <Card className="overflow-hidden p-0">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
                  <th scope="col" className="px-5 py-3 font-medium">Metric</th>
                  <th scope="col" className="px-5 py-3 font-medium text-right">Limit</th>
                  <th scope="col" className="px-5 py-3 font-medium">Period</th>
                  <th scope="col" className="px-5 py-3 font-medium">Enforcement</th>
                  <th scope="col" className="px-5 py-3 font-medium">Created</th>
                  <th scope="col" className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {quotas.map((q) => (
                  <tr key={q.id} className="border-b border-border last:border-0 hover:bg-muted/60">
                    <td className="px-5 py-3">
                      <div className="font-medium">{q.metric}</div>
                      <div className="font-mono text-xs text-muted-foreground">{q.serviceId || "all services"}</div>
                    </td>
                    <td className="px-5 py-3 text-right tabular-nums">{q.limitQuantity.toLocaleString()}</td>
                    <td className="px-5 py-3 text-muted-foreground">{q.period}</td>
                    <td className="px-5 py-3"><Badge variant={q.enforcement === "block" ? "danger" : "warning"}>{q.enforcement}</Badge></td>
                    <td className="px-5 py-3 text-muted-foreground">{formatDate(q.createdAt)}</td>
                    <td className="px-5 py-3"><QuotaRowActions quota={q} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </div>
  );
}
