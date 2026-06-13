import { CircleDollarSign, Gauge, WalletCards } from "lucide-react";
import { AddAIGovernanceButton, ImportOpenAIUsageButton } from "@/components/ai-governance-actions";
import { EmptyState } from "@/components/empty-state";
import { PageHeader } from "@/components/page-header";
import { StatCard } from "@/components/stat-card";
import { Badge, type BadgeVariant } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { fetchAIBudgets } from "@/lib/api";
import { BudgetRowActions } from "@/components/ai-governance-row-actions";

export const metadata = { title: "AI Budgets" };

const statusVariant: Record<string, BadgeVariant> = {
  ok: "success",
  warning: "warning",
  hard_limit: "danger",
};

function usd(n: number): string {
  return "$" + n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

export default async function AIBudgetsPage() {
  const budgets = await fetchAIBudgets();
  const totalLimit = budgets.reduce((sum, b) => sum + b.limitUsd, 0);
  const totalActual = budgets.reduce((sum, b) => sum + b.actualUsd, 0);

  return (
    <div className="space-y-6">
      <PageHeader title="AI Budgets" description="Budget control and showback for AI providers, owners, and workspaces.">
        <div className="flex flex-wrap gap-2">
          <ImportOpenAIUsageButton />
          <AddAIGovernanceButton kind="budget" />
        </div>
      </PageHeader>

      <div className="grid gap-3 sm:grid-cols-3">
        <StatCard icon={WalletCards} label="Budgets" value={budgets.length} hint="Configured controls" />
        <StatCard icon={CircleDollarSign} label="Actual" value={usd(totalActual)} hint="Current period usage" accent="bg-info/10 text-info" />
        <StatCard icon={Gauge} label="Limit" value={usd(totalLimit)} hint="Current period budget" accent="bg-success/10 text-success" />
      </div>

      {budgets.length === 0 ? (
        <Card>
          <EmptyState icon={CircleDollarSign} title="No AI budgets" description="Create a global, provider, owner, or workspace budget to start showback." />
        </Card>
      ) : (
        <Card className="overflow-hidden p-0">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
                  <th scope="col" className="px-5 py-3 font-medium">Scope</th>
                  <th scope="col" className="px-5 py-3 font-medium">Period</th>
                  <th scope="col" className="px-5 py-3 font-medium text-right">Actual</th>
                  <th scope="col" className="px-5 py-3 font-medium text-right">Limit</th>
                  <th scope="col" className="px-5 py-3 font-medium text-right">Used</th>
                  <th scope="col" className="px-5 py-3 font-medium">Status</th>
                  <th scope="col" className="px-5 py-3 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {budgets.map((b) => (
                  <tr key={b.id} className="border-b border-border last:border-0 hover:bg-muted/60">
                    <td className="px-5 py-3">
                      <div className="font-medium">{b.scope}</div>
                      <div className="text-xs text-muted-foreground">{b.scopeRef || "all"}</div>
                    </td>
                    <td className="px-5 py-3 text-muted-foreground">{b.period}</td>
                    <td className="px-5 py-3 text-right tabular-nums">{usd(b.actualUsd)}</td>
                    <td className="px-5 py-3 text-right tabular-nums">{usd(b.limitUsd)}</td>
                    <td className="px-5 py-3 text-right tabular-nums">{b.usagePct.toFixed(1)}%</td>
                    <td className="px-5 py-3"><Badge variant={statusVariant[b.status] ?? "default"}>{b.status}</Badge></td>
                    <td className="px-5 py-3"><BudgetRowActions budget={b} /></td>
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
