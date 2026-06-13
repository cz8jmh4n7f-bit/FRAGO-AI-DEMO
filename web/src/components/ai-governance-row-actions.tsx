"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { createPortal } from "react-dom";
import { Loader2, Pencil, Trash2 } from "lucide-react";
import type { AIAccessPolicy, AIBudget, AIQuota } from "@/lib/types";
import { authHeaders } from "@/lib/client-auth";
import { button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { useToast } from "@/components/ui/toast";
import { cn } from "@/lib/utils";

const API = "/bff";
const inputCls =
  "h-9 w-full rounded-lg border border-input bg-card px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center p-4" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />
      <div className="relative w-full max-w-sm rounded-xl border border-border bg-card p-5 shadow-xl">
        <h2 className="mb-4 text-base font-semibold text-foreground">{title}</h2>
        {children}
      </div>
    </div>
  );
}

// useRowOps wraps a PATCH/DELETE with busy + toast + refresh.
function useRowOps(label: string) {
  const router = useRouter();
  const { toast } = useToast();
  const [busy, setBusy] = useState(false);
  async function run(fn: () => Promise<Response>): Promise<boolean> {
    setBusy(true);
    try {
      const res = await fn();
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        toast({ variant: "error", title: `${label} failed`, description: data.error ?? `(${res.status})` });
        return false;
      }
      toast({ variant: "success", title: `${label} done` });
      router.refresh();
      return true;
    } catch (err) {
      toast({ variant: "error", title: `${label} failed`, description: String(err) });
      return false;
    } finally {
      setBusy(false);
    }
  }
  return { busy, run };
}

function DeleteButton({ onDelete, busy }: { onDelete: () => void; busy: boolean }) {
  return (
    <button type="button" onClick={onDelete} disabled={busy} className={cn(button({ variant: "outline", size: "sm" }), "text-danger")}>
      {busy ? <Loader2 className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
      Delete
    </button>
  );
}

export function BudgetRowActions({ budget }: { budget: AIBudget }) {
  const { busy, run } = useRowOps("Budget");
  const { confirm } = useConfirm();
  const [open, setOpen] = useState(false);
  const [limit, setLimit] = useState(String(budget.limitUsd));
  const [period, setPeriod] = useState(budget.period);
  const [soft, setSoft] = useState(String(budget.softThresholdPct));
  const [hard, setHard] = useState(String(budget.hardThresholdPct));

  async function save(e: React.FormEvent) {
    e.preventDefault();
    const ok = await run(() =>
      fetch(`${API}/api/v1/ai/budgets/${budget.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ scope: budget.scope, scopeRef: budget.scopeRef, limitUsd: Number(limit), period, softThresholdPct: Number(soft), hardThresholdPct: Number(hard) }),
      }),
    );
    if (ok) setOpen(false);
  }
  async function del() {
    if (!(await confirm({ title: `Delete ${budget.scope} budget?`, message: `Removes the ${budget.scope} budget${budget.scopeRef ? ` (${budget.scopeRef})` : ""}.`, danger: true, confirmLabel: "Delete" }))) return;
    run(() => fetch(`${API}/api/v1/ai/budgets/${budget.id}`, { method: "DELETE", headers: authHeaders() }));
  }

  return (
    <div className="flex justify-end gap-2">
      <button type="button" onClick={() => setOpen(true)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>
        <Pencil className="size-4" /> Edit
      </button>
      <DeleteButton onDelete={del} busy={busy} />
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <Modal title={`Edit ${budget.scope} budget`} onClose={() => !busy && setOpen(false)}>
            <form onSubmit={save} className="space-y-3">
              <label className="flex flex-col gap-1"><span className="text-xs font-medium text-muted-foreground">Limit (USD)</span>
                <input className={inputCls} type="number" step="0.01" min="0" required value={limit} onChange={(e) => setLimit(e.target.value)} /></label>
              <label className="flex flex-col gap-1"><span className="text-xs font-medium text-muted-foreground">Period</span>
                <select className={inputCls} value={period} onChange={(e) => setPeriod(e.target.value)}>
                  <option value="monthly">monthly</option><option value="daily">daily</option><option value="yearly">yearly</option>
                </select></label>
              <div className="grid grid-cols-2 gap-2">
                <label className="flex flex-col gap-1"><span className="text-xs font-medium text-muted-foreground">Soft %</span>
                  <input className={inputCls} type="number" min="1" max="100" value={soft} onChange={(e) => setSoft(e.target.value)} /></label>
                <label className="flex flex-col gap-1"><span className="text-xs font-medium text-muted-foreground">Hard %</span>
                  <input className={inputCls} type="number" min="1" max="200" value={hard} onChange={(e) => setHard(e.target.value)} /></label>
              </div>
              <div className="flex justify-end gap-2 pt-1">
                <button type="button" onClick={() => setOpen(false)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>Cancel</button>
                <button type="submit" disabled={busy} className={button({ size: "sm" })}>{busy && <Loader2 className="size-4 animate-spin" />} Save</button>
              </div>
            </form>
          </Modal>,
          document.body,
        )}
    </div>
  );
}

export function QuotaRowActions({ quota }: { quota: AIQuota }) {
  const { busy, run } = useRowOps("Quota");
  const { confirm } = useConfirm();
  const [open, setOpen] = useState(false);
  const [limit, setLimit] = useState(String(quota.limitQuantity));
  const [period, setPeriod] = useState(quota.period);
  const [enforcement, setEnforcement] = useState(quota.enforcement);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    const ok = await run(() =>
      fetch(`${API}/api/v1/ai/quotas/${quota.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ limitQuantity: Number(limit), period, enforcement }),
      }),
    );
    if (ok) setOpen(false);
  }
  async function del() {
    if (!(await confirm({ title: "Delete quota?", message: `Removes the ${quota.metric} quota.`, danger: true, confirmLabel: "Delete" }))) return;
    run(() => fetch(`${API}/api/v1/ai/quotas/${quota.id}`, { method: "DELETE", headers: authHeaders() }));
  }

  return (
    <div className="flex justify-end gap-2">
      <button type="button" onClick={() => setOpen(true)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>
        <Pencil className="size-4" /> Edit
      </button>
      <DeleteButton onDelete={del} busy={busy} />
      {open &&
        typeof document !== "undefined" &&
        createPortal(
          <Modal title={`Edit ${quota.metric} quota`} onClose={() => !busy && setOpen(false)}>
            <form onSubmit={save} className="space-y-3">
              <label className="flex flex-col gap-1"><span className="text-xs font-medium text-muted-foreground">Limit ({quota.metric})</span>
                <input className={inputCls} type="number" min="0" step="any" required value={limit} onChange={(e) => setLimit(e.target.value)} /></label>
              <label className="flex flex-col gap-1"><span className="text-xs font-medium text-muted-foreground">Period</span>
                <select className={inputCls} value={period} onChange={(e) => setPeriod(e.target.value)}>
                  <option value="monthly">monthly</option><option value="daily">daily</option><option value="yearly">yearly</option>
                </select></label>
              <label className="flex flex-col gap-1"><span className="text-xs font-medium text-muted-foreground">Enforcement</span>
                <select className={inputCls} value={enforcement} onChange={(e) => setEnforcement(e.target.value)}>
                  <option value="warn">warn (alert only)</option><option value="block">block (hard stop)</option>
                </select></label>
              <div className="flex justify-end gap-2 pt-1">
                <button type="button" onClick={() => setOpen(false)} disabled={busy} className={button({ variant: "outline", size: "sm" })}>Cancel</button>
                <button type="submit" disabled={busy} className={button({ size: "sm" })}>{busy && <Loader2 className="size-4 animate-spin" />} Save</button>
              </div>
            </form>
          </Modal>,
          document.body,
        )}
    </div>
  );
}

export function PolicyRowActions({ policy }: { policy: AIAccessPolicy }) {
  const { busy, run } = useRowOps("Policy");
  const { confirm } = useConfirm();
  const nextStatus = policy.status === "active" ? "disabled" : "active";

  async function toggle() {
    run(() =>
      fetch(`${API}/api/v1/ai/policies/${policy.id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", ...authHeaders() },
        body: JSON.stringify({ rules: policy.rules, status: nextStatus }),
      }),
    );
  }
  async function del() {
    if (!(await confirm({ title: `Delete policy “${policy.name}”?`, message: "Removes the access policy.", danger: true, confirmLabel: "Delete" }))) return;
    run(() => fetch(`${API}/api/v1/ai/policies/${policy.id}`, { method: "DELETE", headers: authHeaders() }));
  }

  return (
    <div className="flex justify-end gap-2">
      <button type="button" onClick={toggle} disabled={busy} className={button({ variant: "outline", size: "sm" })}>
        {busy ? <Loader2 className="size-4 animate-spin" /> : null}
        {policy.status === "active" ? "Disable" : "Enable"}
      </button>
      <DeleteButton onDelete={del} busy={busy} />
    </div>
  );
}
