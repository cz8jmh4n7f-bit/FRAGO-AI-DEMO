import { ShieldCheck } from "lucide-react";
import { AIInstanceActions } from "@/components/ai-instance-actions";
import { RecertifyButton } from "@/components/ai-recertify-button";
import { EmptyState } from "@/components/empty-state";
import { PageHeader } from "@/components/page-header";
import { StatCard } from "@/components/stat-card";
import { Badge, type BadgeVariant } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { fetchAIAccessReview } from "@/lib/api";

export const metadata = { title: "Access Review" };

const flagVariant: Record<string, BadgeVariant> = {
  ok: "success",
  expiring_soon: "warning",
  long_lived: "warning",
  no_expiry: "danger",
  overdue: "danger",
};
const flagLabel: Record<string, string> = {
  ok: "ok",
  expiring_soon: "expiring soon",
  long_lived: "long-lived",
  no_expiry: "no expiry",
  overdue: "overdue",
};

export default async function AIAccessReviewPage() {
  const items = await fetchAIAccessReview();
  const flagged = items.filter((i) => i.flag !== "ok");
  const noExpiry = items.filter((i) => i.flag === "no_expiry").length;
  const overdue = items.filter((i) => i.flag === "overdue").length;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Access Review"
        description="Periodic recertification of standing AI access (SOC 2 / ISO 27001). Recertify to re-justify and extend, or revoke."
      />

      <div className="grid gap-3 sm:grid-cols-3">
        <StatCard icon={ShieldCheck} label="Active grants" value={items.length} hint="Under review" />
        <StatCard icon={ShieldCheck} label="No expiry" value={noExpiry} hint="Standing access - needs a TTL" accent="bg-danger/10 text-danger" />
        <StatCard icon={ShieldCheck} label="Overdue" value={overdue} hint="Past expiry (reaper gap)" accent="bg-warning/10 text-warning" />
      </div>

      {items.length === 0 ? (
        <Card>
          <EmptyState icon={ShieldCheck} title="No active AI access" description="Granted AI access appears here for periodic review." />
        </Card>
      ) : (
        <Card className="overflow-hidden p-0">
          <div className="border-b border-border px-5 py-3">
            <h2 className="text-sm font-semibold">
              {flagged.length} of {items.length} grants need attention
            </h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs uppercase tracking-wide text-muted-foreground">
                  <th scope="col" className="px-5 py-3 font-medium">Service</th>
                  <th scope="col" className="px-5 py-3 font-medium">Owner</th>
                  <th scope="col" className="px-5 py-3 font-medium">Workspace</th>
                  <th scope="col" className="px-5 py-3 font-medium">Age</th>
                  <th scope="col" className="px-5 py-3 font-medium">Expires</th>
                  <th scope="col" className="px-5 py-3 font-medium">Review</th>
                  <th scope="col" className="px-5 py-3 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {items.map((i) => (
                  <tr key={i.id} className="border-b border-border last:border-0 align-top hover:bg-muted/60">
                    <td className="px-5 py-3">
                      <div className="font-medium">{i.serviceName || i.serviceSlug}</div>
                      <div className="text-xs text-muted-foreground">{i.providerName}</div>
                    </td>
                    <td className="px-5 py-3 text-muted-foreground">{i.owner}</td>
                    <td className="px-5 py-3 text-muted-foreground">{i.workspace}</td>
                    <td className="px-5 py-3 tabular-nums text-muted-foreground">{i.ageDays}d</td>
                    <td className="px-5 py-3 text-muted-foreground">{i.expiresAt ? i.expiresAt.slice(0, 10) : "never"}</td>
                    <td className="px-5 py-3"><Badge variant={flagVariant[i.flag] ?? "default"}>{flagLabel[i.flag] ?? i.flag}</Badge></td>
                    <td className="px-5 py-3">
                      <div className="flex items-center justify-end gap-2">
                        <RecertifyButton id={i.id} />
                        <AIInstanceActions id={i.id} status={i.status} />
                      </div>
                    </td>
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
