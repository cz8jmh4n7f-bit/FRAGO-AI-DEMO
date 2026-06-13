// Domain types mirroring the OPORD backend (internal/models + DB schema).
// Used by mock data now; will back the real API client later.

export type ProviderType = "vsphere" | "proxmox" | "aws" | "azure" | "gcp";
export type AIProviderType = "mock_ai" | "openai" | "anthropic" | "litellm" | "gemini" | "github_copilot" | "cursor";

export type ClusterStatus =
  | "pending"
  | "provisioning"
  | "bootstrapping"
  | "ready"
  | "degraded"
  | "destroying"
  | "destroyed"
  | "failed";

export type JobOperation = "provision" | "bootstrap" | "reconcile" | "destroy";
export type JobStatus = "queued" | "running" | "succeeded" | "failed" | "cancelled";
export type NodeRole = "control_plane" | "worker";

// ProviderHealth is the last persisted connectivity probe (POST /providers/{name}/check).
// status is "" until the first check, then one of the values below.
export interface ProviderHealth {
  status: "" | "ok" | "failed" | "unsupported";
  message?: string;
  latencyMs?: number;
  checkedAt?: string;
}

export interface Provider {
  id: string;
  name: string;
  type: ProviderType;
  server: string;
  datacenter: string;
  region?: string;
  secretRef?: string;
  config?: Record<string, unknown>;
  clusters: number;
  createdAt: string;
  health?: ProviderHealth;
}

export interface AIProvider {
  id: string;
  name: string;
  type: AIProviderType;
  config?: Record<string, unknown>;
  status: string;
  createdAt: string;
  updatedAt: string;
}

// AI org administration (ADR-0022): real org/workspace/role state from a
// governable AI provider (Anthropic via the admin key).
export interface AIOrgUser {
  id: string;
  email: string;
  name?: string;
  role: string;
  addedAt?: string;
}

export interface AIWorkspace {
  id: string;
  name: string;
  createdAt?: string;
  archivedAt?: string;
  archived: boolean;
}

export interface AIAccessReviewItem extends AIInstance {
  flag: string;
  ageDays: number;
}

export interface AIInvite {
  inviteId: string;
  email: string;
  role: string;
  status: string;
  invitedAt?: string;
  expiresAt?: string;
}

export interface AIWorkspaceAccess {
  userId: string;
  email?: string;
  orgRole?: string;
  workspaceRole: string;
  inherited: boolean;
}

// Agent & MCP governance (migration 00022): a registry of approved MCP servers
// teams may use, with per-owner grants and an authorize enforcement check.
export interface MCPServer {
  id: string;
  name: string;
  transport: string;
  endpoint: string;
  description: string;
  riskTier: string;
  allowedTools: string[];
  status: string;
  createdAt: string;
}

export interface MCPGrant {
  id: string;
  server: string;
  riskTier: string;
  owner: string;
  status: string;
  expiresAt?: string;
  grantedBy?: string;
  createdAt: string;
}

export interface AIService {
  id: string;
  providerId: string;
  providerName: string;
  providerType: AIProviderType;
  name: string;
  slug: string;
  category: string;
  description: string;
  requestSchema?: Record<string, unknown>;
  defaultExpirationDays: number;
  requiresApproval: boolean;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface AIInstance {
  id: string;
  serviceId: string;
  serviceName: string;
  serviceSlug: string;
  providerName: string;
  providerType: AIProviderType;
  requestId?: string;
  providerAccessId: string;
  owner: string;
  workspace: string;
  status: string;
  spec?: Record<string, unknown>;
  observed?: Record<string, unknown>;
  provisionedAt?: string;
  expiresAt?: string;
  revokedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AIUsageRecord {
  id: string;
  instanceId?: string;
  providerId: string;
  providerName: string;
  owner?: string;
  workspace?: string;
  periodStart: string;
  periodEnd: string;
  metric: string;
  quantity: number;
  unit: string;
  costUsd: number;
  raw?: Record<string, unknown>;
  createdAt: string;
}

export interface AIAuditEvent {
  id: string;
  actor: string;
  subjectType: string;
  subjectId?: string;
  action: string;
  message: string;
  fields?: Record<string, unknown>;
  createdAt: string;
}

export interface AIBudget {
  id: string;
  scope: string;
  scopeRef: string;
  limitUsd: number;
  period: string;
  softThresholdPct: number;
  hardThresholdPct: number;
  actualUsd: number;
  remainingUsd: number;
  usagePct: number;
  status: "ok" | "warning" | "hard_limit" | string;
  createdAt: string;
  updatedAt: string;
}

export interface AIQuota {
  id: string;
  serviceId?: string;
  metric: string;
  limitQuantity: number;
  period: string;
  enforcement: "warn" | "block" | string;
  createdAt: string;
}

export interface AIAccessPolicy {
  id: string;
  name: string;
  rules: Record<string, unknown>;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface AIModelCatalogItem {
  id: string;
  providerId: string;
  providerName: string;
  providerType: AIProviderType;
  model: string;
  displayName: string;
  modality: string;
  status: string;
  metadata?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface ProviderReadinessCheck {
  id: string;
  label: string;
  status: "ok" | "warn" | "failed";
  message: string;
}

export interface ProviderReadiness {
  provider: string;
  type: ProviderType;
  status: "ok" | "warn" | "failed";
  checks: ProviderReadinessCheck[];
  nextActions: string[];
}

export interface ClusterNode {
  name: string;
  role: NodeRole;
  ip: string;
  status: "pending" | "provisioned" | "ready" | "failed";
}

export interface Cluster {
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  kubernetesVersion: string;
  cni: string;
  controlPlanes: number;
  workers: number;
  endpoint: string;
  managed: boolean;
  kubeconfigRef?: string;
  // Finding E: provision-failure reason when status === "failed".
  lastError?: string;
  createdAt: string;
  updatedAt: string;
  nodes?: ClusterNode[];
}

export interface Job {
  id: string;
  cluster: string;
  operation: JobOperation;
  status: JobStatus;
  startedAt: string | null;
  finishedAt: string | null;
  durationSecs?: number | null;
  createdAt?: string;
  error: string | null;
}

export interface BlueprintComponent {
  name: string;
  kind: string;
}

export interface Blueprint {
  id: string;
  name: string;
  description: string;
  components: BlueprintComponent[];
}

export interface EnvComponent {
  name: string;
  kind: string;
  resource: string;
  status: string;
}

export interface Environment {
  id: string;
  name: string;
  environment: string;
  blueprint: string;
  provider: string;
  status: ClusterStatus;
  components: EnvComponent[];
  createdAt: string;
  updatedAt: string;
}

export interface CostLine {
  name: string;
  kind: string;
  provider: string;
  environment: string;
  status: string;
  monthlyUsd: number;
  owner?: string;
  project?: string;
  costCenter?: string;
  ttlHours?: number;
  riskFlags?: string[];
}

export interface CostReport {
  lines: CostLine[];
  totalUsd: number;
}

export interface FinOpsSpendBreakdown {
  name: string;
  monthlyUsd: number;
}

export interface FocusGuide {
  cloud: string;
  providerType: string;
  focusVersion: string;
  status: string;
  export: string;
  analytics: string;
  opordReadiness: string;
  url: string;
}

export interface FinOpsPhase {
  name: string;
  description: string;
  actions: string[];
}

export interface FinOpsAllocationCoverage {
  resources: number;
  ownerTagged: number;
  projectTagged: number;
  costCenterTagged: number;
  ttlProtected: number;
  coveragePct: number;
}

export interface FinOpsBudget {
  scope: string;
  name: string;
  limitUsd: number;
  actualUsd: number;
  remainingUsd: number;
  usagePct: number;
  status: "ok" | "risk" | "over";
}

export interface FinOpsGuardrail {
  severity: "info" | "warn" | "blocker";
  resource: string;
  kind: string;
  message: string;
  action: string;
}

export interface FinOpsSavingsOpportunity {
  resource: string;
  kind: string;
  provider: string;
  monthlyUsd: number;
  savingsUsd: number;
  confidence: string;
  action: string;
}

export interface FinOpsUnitMetric {
  name: string;
  resources: number;
  monthlyUsd: number;
  avgUsd: number;
}

// --- Real cloud-spend actuals (AWS Cost Explorer) ---
// NOTE: the nested actuals object uses snake_case on the wire (it serializes the
// providers.CostActuals Go struct), unlike the camelCase top-level report fields.

export interface CostBucket {
  key: string;
  name?: string;
  usd: number;
}

export interface CostPoint {
  date: string; // YYYY-MM-DD
  usd: number;
}

export interface CostAnomaly {
  date: string;
  usd: number;
  baselineUsd: number;
  factor: number; // usd / baseline
}

export interface CostAccountRef {
  id: string;
  name?: string;
}

export interface FinOpsCloud {
  name: string;
  type: string;
  usd: number;
  available: boolean;
  error?: string;
}

export interface CostActuals {
  currency: string;
  windowDays: number;
  totalUsd: number;
  mtdUsd: number; // month-to-date actual
  forecastUsd: number; // run-rate end-of-month projection
  dailyRunRate: number; // recent avg daily spend
  byCloud?: CostBucket[]; // spend per provider/cloud (multi-cloud merge)
  byAccount: CostBucket[];
  byService: CostBucket[];
  daily: CostPoint[];
  anomalies: CostAnomaly[];
  accounts: CostAccountRef[];
}

export interface FinOpsReport {
  totalUsd: number;
  projectedMonthlyUsd: number;
  dailyRunRateUsd: number;
  activeResources: number;
  providerSpend: FinOpsSpendBreakdown[];
  environmentSpend: FinOpsSpendBreakdown[];
  kindSpend: FinOpsSpendBreakdown[];
  allocationCoverage: FinOpsAllocationCoverage;
  budgets: FinOpsBudget[];
  guardrails: FinOpsGuardrail[];
  savingsOpportunities: FinOpsSavingsOpportunity[];
  unitMetrics: FinOpsUnitMetric[];
  focusGuides: FocusGuide[];
  phases: FinOpsPhase[];
  recommendations: string[];
  // Real billed spend from AWS Cost Explorer (present only when CE is reachable).
  actuals?: CostActuals;
  actualsSource?: string; // e.g. "aws_cost_explorer"
  actualsError?: string; // why actuals are unavailable (drives the "connect Cost Explorer" banner)
  clouds?: FinOpsCloud[]; // all cost-capable clouds (available + unavailable) - drives the stable tiles
  provider?: string; // the cloud (provider name) tile selected ("" = all clouds)
  account?: string; // the linked-account filter applied ("" = all)
  windowDays?: number; // trailing window for actuals
}

export interface Request {
  id: string;
  name: string;
  environment: string;
  requester: string;
  kind: string;
  provider: string;
  blueprint?: string;
  status: string;
  ticketRef?: string;
  resourceRef?: string;
  decidedBy?: string;
  reason?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Stack {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  moduleDir: string;
  variables?: Record<string, unknown>;
  outputs?: Record<string, unknown>;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Database {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  engine: string;
  version?: string;
  instanceClass?: string;
  storageGb: number;
  dbName: string;
  endpoint?: string;
  port?: number;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Table {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  hashKey: string;
  hashKeyType?: string;
  rangeKey?: string;
  billingMode: string;
  arn?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface S3Bucket {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  bucketName: string;
  bucketArn?: string;
  domainName?: string;
  versioning: boolean;
  blockPublicAccess: boolean;
  kmsKeyArn?: string;
  lifecycleGlacierDays?: number;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

// Expose-layer primitives (ADR-0016) - AWS-only: DNS zone (Route 53), TLS
// certificate (ACM), load balancer (ALB), HTTP API (API Gateway), CDN (CloudFront).
export interface DNSZone {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  zoneName?: string;
  nameServers?: string[];
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Cert {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  domain: string;
  certStatus?: string;
  arn?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface LoadBalancer {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  dnsName?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface APIGateway {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  endpoint?: string;
  apiId?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CDN {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  domainName?: string;
  distributionId?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Secret {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  secretName: string;
  secretArn?: string;
  uri?: string;
  description?: string;
  kmsKeyArn?: string;
  recoveryWindowDays?: number;
  rotationDays?: number;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Queue {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  queueName: string;
  queueArn?: string;
  queueUrl?: string;
  fifo: boolean;
  dlqEnabled: boolean;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Cache {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  cacheName: string;
  primaryEndpoint?: string;
  port?: number;
  engineVersion?: string;
  nodeType?: string;
  numCacheNodes?: number;
  inTransitEncryption: boolean;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

// Named FunctionResource (not Function) to avoid shadowing the built-in TS type.
export interface FunctionResource {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  runtime: string;
  handler?: string;
  memoryMb?: number;
  arn?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

// Access-vending project: an IAM Identity Center group bound to a permission set
// on an existing account, with members.
export interface Project {
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  accountId: string;
  members: string[];
  groupName?: string;
  groupId?: string;
  permissionSetArn?: string;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

// A provisioned member AWS account (Organizations CreateAccount + baseline layers).
export interface Account {
  id: string;
  name: string;
  environment: string;
  provider: string;
  status: ClusterStatus;
  csaId: string;
  cloudName: string;
  accountId?: string;
  createVpc: boolean;
  layers?: Record<string, string>;
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

// QueueJob is a row from the durable River queue (/api/v1/queue).
export interface QueueJob {
  id: number;
  kind: string;
  queue: string;
  state: string;
  attempt: number;
  maxAttempts: number;
  createdAt: string;
  finalizedAt: string | null;
  error?: string;
}

export interface VM {
  targetAccount?: string;
  id: string;
  name: string;
  environment: string;
  provider: string;
  kind: string;
  status: ClusterStatus;
  template: string;
  count: number;
  cpu: number;
  memoryMb: number;
  diskGb: number;
  instanceType?: string;
  ipStart?: string;
  publicIp?: boolean;
  ttlHours?: number;
  // Actual IPs assigned by the provider on the last successful provision.
  publicIps?: string[];
  privateIps?: string[];
  // Finding E: provision-failure reason when status === "failed".
  lastError?: string;
  createdAt: string;
  updatedAt: string;
}

// --- Compliance / guardrails (ADR-0014) ---

export type ComplianceSeverity = "critical" | "warning" | "info";
export type ComplianceStatus = "pass" | "fail" | "skip";

export interface ComplianceCategoryScore {
  category: string;
  passed: number;
  failed: number;
  score: number;
}

export interface ComplianceGroupScore {
  name: string;
  passed: number;
  failed: number;
  score: number;
}

export interface ComplianceCheckSummary {
  id: string;
  title: string;
  category: string;
  severity: ComplianceSeverity;
  passed: number;
  failed: number;
}

export interface ComplianceFailure {
  checkId: string;
  title: string;
  category: string;
  severity: ComplianceSeverity;
  remediation?: string;
  subject: string;
  kind: string;
  provider: string;
  account?: string;
  environment?: string;
  status: ComplianceStatus;
  message: string;
}

export interface ComplianceScorecard {
  score: number;
  evaluated: number;
  passed: number;
  failed: number;
  criticalFailing: number;
  warningFailing: number;
  byCategory: ComplianceCategoryScore[];
  byAccount: ComplianceGroupScore[];
  byEnvironment: ComplianceGroupScore[];
  checks: ComplianceCheckSummary[];
  failures: ComplianceFailure[];
}
