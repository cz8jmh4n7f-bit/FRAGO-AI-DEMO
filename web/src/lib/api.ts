import { cookies } from "next/headers";
import type { Account, AIAccessPolicy, AIAuditEvent, AIBudget, AIInstance, AIInvite, AIModelCatalogItem, AIOrgUser, AIProvider, AIQuota, AIService, AIUsageRecord, AIWorkspace, AIWorkspaceAccess, APIGateway, Blueprint, Cache, CDN, Cert, Cluster, ClusterNode, ComplianceScorecard, CostReport, Database, DNSZone, Environment, FinOpsReport, FunctionResource, Job, LoadBalancer, MCPGrant, MCPServer, Project, Provider, Queue, QueueJob, Request, S3Bucket, Secret, Stack, Table, VM } from "./types";

// Base URL of the OPORD API. Server components read OPORD_API_URL; the browser
// can use NEXT_PUBLIC_API_URL. Falls back to the dev default.
const API_BASE =
  process.env.OPORD_API_URL ?? process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export type ClusterDetail = Cluster & { nodes: ClusterNode[]; jobs: Job[] };

// Server-side auth header: forward the `opord_key` cookie (set by /login) as a
// Bearer token so authenticated page fetches pass OPORD's API-key middleware.
// cookies() throws outside a request scope (e.g. build-time prerender), so the
// try/catch degrades to an unauthenticated request there.
async function authHeader(): Promise<Record<string, string>> {
  try {
    const key = (await cookies()).get("opord_key")?.value;
    return key ? { Authorization: `Bearer ${key}` } : {};
  } catch {
    return {};
  }
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    cache: "no-store",
    headers: await authHeader(),
  });
  if (!res.ok) throw new Error(`API ${path} -> ${res.status}`);
  return (await res.json()) as T;
}

// checkApi reports whether the OPORD API is reachable. The root layout uses it to
// show an honest "API offline" banner instead of letting pages look empty. The
// /healthz endpoint needs no auth.
export async function checkApi(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/healthz`, { cache: "no-store" });
    return res.ok;
  } catch {
    return false;
  }
}

// Fetchers degrade to EMPTY data when the API is unreachable - never fabricated
// demo data. The UI then shows honest empty states (behind the offline banner),
// and `next build` still works without a backend running.

export async function fetchProviders(): Promise<Provider[]> {
  try {
    return await get<Provider[]>("/api/v1/providers");
  } catch {
    return [];
  }
}

export async function fetchAIProviders(): Promise<AIProvider[]> {
  try {
    return await get<AIProvider[]>("/api/v1/ai/providers");
  } catch {
    return [];
  }
}

export async function fetchAIServices(): Promise<AIService[]> {
  try {
    return await get<AIService[]>("/api/v1/ai/services");
  } catch {
    return [];
  }
}

// Agent & MCP governance (migration 00022).
export async function fetchMCPServers(): Promise<MCPServer[]> {
  try {
    return await get<MCPServer[]>("/api/v1/ai/mcp/servers");
  } catch {
    return [];
  }
}

export async function fetchMCPGrants(): Promise<MCPGrant[]> {
  try {
    return await get<MCPGrant[]>("/api/v1/ai/mcp/grants");
  } catch {
    return [];
  }
}

// AI org administration (ADR-0022). Parameterized by the governable provider name.
// These deliberately do NOT swallow errors (unlike list fetchers) - the admin page
// surfaces a real "admin not available / key invalid" message instead of an empty
// table, so a 401 from a non-admin key reads correctly.
export async function fetchAIOrgUsers(provider: string): Promise<AIOrgUser[]> {
  return get<AIOrgUser[]>(`/api/v1/ai/admin/${encodeURIComponent(provider)}/users`);
}

export async function fetchAIWorkspaces(provider: string): Promise<AIWorkspace[]> {
  return get<AIWorkspace[]>(`/api/v1/ai/admin/${encodeURIComponent(provider)}/workspaces`);
}

export async function fetchAIInvites(provider: string): Promise<AIInvite[]> {
  return get<AIInvite[]>(`/api/v1/ai/admin/${encodeURIComponent(provider)}/invites`);
}

export async function fetchAIWorkspaceAccess(provider: string, workspaceID: string): Promise<AIWorkspaceAccess[]> {
  return get<AIWorkspaceAccess[]>(`/api/v1/ai/admin/${encodeURIComponent(provider)}/workspaces/${encodeURIComponent(workspaceID)}/access`);
}

export async function fetchAIRequests(): Promise<Request[]> {
  try {
    return await get<Request[]>("/api/v1/ai/requests");
  } catch {
    return [];
  }
}

export async function fetchAIInstances(): Promise<AIInstance[]> {
  try {
    return await get<AIInstance[]>("/api/v1/ai/instances");
  } catch {
    return [];
  }
}

export async function fetchAIUsage(): Promise<AIUsageRecord[]> {
  try {
    return await get<AIUsageRecord[]>("/api/v1/ai/usage");
  } catch {
    return [];
  }
}

export async function fetchAIAudit(): Promise<AIAuditEvent[]> {
  try {
    return await get<AIAuditEvent[]>("/api/v1/ai/audit");
  } catch {
    return [];
  }
}

export async function fetchAIBudgets(): Promise<AIBudget[]> {
  try {
    return await get<AIBudget[]>("/api/v1/ai/budgets");
  } catch {
    return [];
  }
}

export async function fetchAIQuotas(): Promise<AIQuota[]> {
  try {
    return await get<AIQuota[]>("/api/v1/ai/quotas");
  } catch {
    return [];
  }
}

export async function fetchAIPolicies(): Promise<AIAccessPolicy[]> {
  try {
    return await get<AIAccessPolicy[]>("/api/v1/ai/policies");
  } catch {
    return [];
  }
}

export async function fetchAIModels(): Promise<AIModelCatalogItem[]> {
  try {
    return await get<AIModelCatalogItem[]>("/api/v1/ai/models");
  } catch {
    return [];
  }
}

export async function fetchAIAccessReview(): Promise<import("./types").AIAccessReviewItem[]> {
  try {
    return await get<import("./types").AIAccessReviewItem[]>("/api/v1/ai/access-review");
  } catch {
    return [];
  }
}

export async function fetchAIRenewals(): Promise<AIInstance[]> {
  try {
    return await get<AIInstance[]>("/api/v1/ai/renewals?days=30");
  } catch {
    return [];
  }
}

export async function fetchClusters(): Promise<Cluster[]> {
  try {
    return await get<Cluster[]>("/api/v1/clusters");
  } catch {
    return [];
  }
}

export async function fetchCluster(name: string, env = "dev"): Promise<ClusterDetail | undefined> {
  try {
    return await get<ClusterDetail>(
      `/api/v1/clusters/${encodeURIComponent(name)}?env=${encodeURIComponent(env)}`,
    );
  } catch {
    return undefined;
  }
}

export async function fetchVMs(): Promise<VM[]> {
  try {
    return await get<VM[]>("/api/v1/vms");
  } catch {
    return [];
  }
}

export async function fetchQueue(): Promise<QueueJob[]> {
  try {
    return await get<QueueJob[]>("/api/v1/queue");
  } catch {
    return [];
  }
}

export async function fetchBlueprints(): Promise<Blueprint[]> {
  try {
    return await get<Blueprint[]>("/api/v1/blueprints");
  } catch {
    return [];
  }
}

export async function fetchEnvironments(): Promise<Environment[]> {
  try {
    return await get<Environment[]>("/api/v1/environments");
  } catch {
    return [];
  }
}

export async function fetchEnvironment(name: string, env = "dev"): Promise<Environment | undefined> {
  try {
    return await get<Environment>(
      `/api/v1/environments/${encodeURIComponent(name)}?env=${encodeURIComponent(env)}`,
    );
  } catch {
    return undefined;
  }
}

export async function fetchStacks(): Promise<Stack[]> {
  try {
    return await get<Stack[]>("/api/v1/stacks");
  } catch {
    return [];
  }
}

export async function fetchDatabases(): Promise<Database[]> {
  try {
    return await get<Database[]>("/api/v1/databases");
  } catch {
    return [];
  }
}

export async function fetchTables(): Promise<Table[]> {
  try {
    return await get<Table[]>("/api/v1/tables");
  } catch {
    return [];
  }
}

export async function fetchS3Buckets(): Promise<S3Bucket[]> {
  try {
    return await get<S3Bucket[]>("/api/v1/s3");
  } catch {
    return [];
  }
}

export async function fetchDNS(): Promise<DNSZone[]> {
  try {
    return await get<DNSZone[]>("/api/v1/dns");
  } catch {
    return [];
  }
}

export async function fetchCerts(): Promise<Cert[]> {
  try {
    return await get<Cert[]>("/api/v1/certs");
  } catch {
    return [];
  }
}

export async function fetchLoadBalancers(): Promise<LoadBalancer[]> {
  try {
    return await get<LoadBalancer[]>("/api/v1/loadbalancers");
  } catch {
    return [];
  }
}

export async function fetchAPIGateways(): Promise<APIGateway[]> {
  try {
    return await get<APIGateway[]>("/api/v1/apigateways");
  } catch {
    return [];
  }
}

export async function fetchCDNs(): Promise<CDN[]> {
  try {
    return await get<CDN[]>("/api/v1/cdns");
  } catch {
    return [];
  }
}

export async function fetchSecrets(): Promise<Secret[]> {
  try {
    return await get<Secret[]>("/api/v1/secrets");
  } catch {
    return [];
  }
}

export async function fetchQueues(): Promise<Queue[]> {
  try {
    return await get<Queue[]>("/api/v1/queues");
  } catch {
    return [];
  }
}

export async function fetchCaches(): Promise<Cache[]> {
  try {
    return await get<Cache[]>("/api/v1/caches");
  } catch {
    return [];
  }
}

export async function fetchFunctions(): Promise<FunctionResource[]> {
  try {
    return await get<FunctionResource[]>("/api/v1/functions");
  } catch {
    return [];
  }
}

export async function fetchProjects(): Promise<Project[]> {
  try {
    return await get<Project[]>("/api/v1/projects");
  } catch {
    return [];
  }
}

export async function fetchAccounts(): Promise<Account[]> {
  try {
    return await get<Account[]>("/api/v1/accounts");
  } catch {
    return [];
  }
}

export async function fetchStack(name: string, env = "dev"): Promise<Stack | undefined> {
  try {
    return await get<Stack>(`/api/v1/stacks/${encodeURIComponent(name)}?env=${encodeURIComponent(env)}`);
  } catch {
    return undefined;
  }
}

export async function fetchRequests(): Promise<Request[]> {
  try {
    return await get<Request[]>("/api/v1/requests");
  } catch {
    return [];
  }
}

export async function fetchRequest(name: string, env = "dev"): Promise<Request | undefined> {
  try {
    return await get<Request>(`/api/v1/requests/${encodeURIComponent(name)}?env=${encodeURIComponent(env)}`);
  } catch {
    return undefined;
  }
}

export async function fetchCost(): Promise<CostReport> {
  try {
    return await get<CostReport>("/api/v1/cost");
  } catch {
    return { lines: [], totalUsd: 0 };
  }
}

export async function fetchFinOps(provider?: string, account?: string, days?: number): Promise<FinOpsReport> {
  const params = new URLSearchParams();
  if (provider) params.set("provider", provider);
  if (account) params.set("account", account);
  if (days) params.set("days", String(days));
  const qs = params.toString();
  try {
    return await get<FinOpsReport>(`/api/v1/finops${qs ? `?${qs}` : ""}`);
  } catch {
    return {
      totalUsd: 0,
      projectedMonthlyUsd: 0,
      dailyRunRateUsd: 0,
      activeResources: 0,
      providerSpend: [],
      environmentSpend: [],
      kindSpend: [],
      allocationCoverage: {
        resources: 0,
        ownerTagged: 0,
        projectTagged: 0,
        costCenterTagged: 0,
        ttlProtected: 0,
        coveragePct: 0,
      },
      budgets: [],
      guardrails: [],
      savingsOpportunities: [],
      unitMetrics: [],
      focusGuides: [
        {
          cloud: "AWS",
          providerType: "aws",
          focusVersion: "v1.2",
          status: "setup guide",
          export: "Billing and Cost Management FOCUS export or CloudFormation automation",
          analytics: "Athena table, CID CLI, and FOCUS SQL use cases",
          opordReadiness: "Connect exported S3/Athena dataset, then map account, service, region, tags, and OPORD resource metadata.",
          url: "https://focus.finops.org/get-started/aws/",
        },
        {
          cloud: "Microsoft Azure",
          providerType: "azure",
          focusVersion: "v1.2",
          status: "setup guide",
          export: "Cost Management export using the FOCUS template",
          analytics: "Power BI reports or Microsoft Fabric ingestion",
          opordReadiness: "Connect exported storage/Fabric dataset, then align subscription, resource group, tags, and OPORD environments.",
          url: "https://focus.finops.org/get-started/microsoft/",
        },
        {
          cloud: "Google Cloud",
          providerType: "gcp",
          focusVersion: "v1.0",
          status: "setup guide",
          export: "Detailed Billing Export and Price Export to BigQuery",
          analytics: "FOCUS BigQuery view or Looker template",
          opordReadiness: "Future provider: connect BigQuery billing data and map projects, labels, SKUs, and environments.",
          url: "https://focus.finops.org/get-started/google-cloud/",
        },
      ],
      phases: [
        {
          name: "Inform",
          description: "Make cost and usage visible by owner, provider, environment, service, and resource.",
          actions: [
            "Keep OPORD resource metadata aligned to owners, environments, and projects.",
            "Use FOCUS exports as the normalized billing source across clouds.",
            "Compare OPORD estimates with billed FOCUS cost once ingestion is connected.",
          ],
        },
        {
          name: "Optimize",
          description: "Turn visibility into concrete resource and rate improvements.",
          actions: [
            "Find idle or oversized resources from OPORD inventory and billed usage.",
            "Prefer managed services and TTLs for temporary workloads.",
            "Track savings opportunities by resource, team, and environment.",
          ],
        },
        {
          name: "Operate",
          description: "Make cost ownership part of day-to-day cloud operations.",
          actions: [
            "Set account/subscription budgets and alerts before provisioning.",
            "Expose cost guardrails in catalog forms and approval workflows.",
            "Review anomalies and budget drift as operational work, not monthly archaeology.",
          ],
        },
      ],
      recommendations: [
        "Add owner, project, cost-center, and environment tags to every catalog resource.",
        "Create a FOCUS data connection per cloud and normalize spend into one queryable dataset.",
        "Promote the Cost page from estimates to estimate-vs-actual once FOCUS ingestion is available.",
        "Add policy checks for TTL, monthly budget, public exposure, and missing allocation tags.",
      ],
      // Actuals are unavailable when the API is unreachable; the page falls back to estimates.
      actuals: undefined,
      actualsSource: undefined,
      actualsError: undefined,
      provider: provider ?? "",
      account: account ?? "",
      windowDays: days ?? 0,
    };
  }
}

export async function fetchCompliance(): Promise<ComplianceScorecard> {
  try {
    return await get<ComplianceScorecard>("/api/v1/compliance");
  } catch {
    return {
      score: 100,
      evaluated: 0,
      passed: 0,
      failed: 0,
      criticalFailing: 0,
      warningFailing: 0,
      byCategory: [],
      byAccount: [],
      byEnvironment: [],
      checks: [],
      failures: [],
    };
  }
}

export async function fetchJobs(): Promise<Job[]> {
  try {
    const clusters = await get<Cluster[]>("/api/v1/clusters");
    const details = await Promise.all(
      clusters.map((c) =>
        get<ClusterDetail>(
          `/api/v1/clusters/${encodeURIComponent(c.name)}?env=${encodeURIComponent(c.environment)}`,
        ).catch(() => null),
      ),
    );
    return details.flatMap((d) => d?.jobs ?? []);
  } catch {
    return [];
  }
}
