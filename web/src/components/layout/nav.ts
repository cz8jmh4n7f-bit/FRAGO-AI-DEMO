import {
  Activity,
  Blocks,
  Bot,
  Boxes,
  BrainCircuit,
  Building2,
  Archive,
  ChartNoAxesCombined,
  CircleDollarSign,
  Cpu,
  Database,
  Gauge,
  History,
  Inbox,
  KeyRound,
  Layers,
  LayoutDashboard,
  LayoutGrid,
  Lock,
  MessageSquarePlus,
  Server,
  Sparkles,
  ShieldCheck,
  SlidersHorizontal,
  Table2,
  Workflow,
  Zap,
  type LucideIcon,
} from "lucide-react";

export type NavItem = { href: string; label: string; icon: LucideIcon };
export type NavSection = { title: string; items: NavItem[] };

// Primary navigation, grouped into scannable sections (single source of truth for
// the desktop sidebar + mobile drawer). Replaced the previous flat 19-item list -
// related concepts are now chunked so the eye doesn't parse all of it every time.
export const navSections: NavSection[] = [
  {
    title: "Marketplace",
    items: [
      { href: "/", label: "Home", icon: LayoutDashboard },
      { href: "/catalog", label: "Catalog", icon: LayoutGrid },
      { href: "/environments", label: "Environments", icon: Layers },
    ],
  },
  {
    title: "Resources · Compute",
    items: [
      { href: "/vms", label: "Virtual machines", icon: Cpu },
      { href: "/clusters", label: "Clusters", icon: Boxes },
    ],
  },
  {
    title: "Resources · Data",
    items: [
      { href: "/databases", label: "Databases", icon: Database },
      { href: "/s3", label: "Object storage", icon: Archive },
      { href: "/caches", label: "Caches", icon: Gauge },
      { href: "/tables", label: "Tables", icon: Table2 },
    ],
  },
  {
    title: "Resources · App services",
    items: [
      { href: "/functions", label: "Functions", icon: Zap },
      { href: "/queues", label: "Queues", icon: Inbox },
    ],
  },
  {
    title: "Resources · Security",
    items: [
      { href: "/secrets", label: "Secrets", icon: Lock },
      { href: "/access", label: "Access requests", icon: KeyRound },
    ],
  },
  {
    title: "Governance",
    items: [
      { href: "/projects", label: "Projects", icon: KeyRound },
      { href: "/finops", label: "Cost & usage", icon: ChartNoAxesCombined },
      { href: "/jobs", label: "Activity / audit", icon: Activity },
      { href: "/compliance", label: "Policies", icon: ShieldCheck },
    ],
  },
  {
    title: "AI Governance",
    items: [
      { href: "/ai/overview", label: "Overview", icon: LayoutDashboard },
      { href: "/ai/catalog", label: "AI Services", icon: Sparkles },
      { href: "/ai/requests", label: "AI Requests", icon: MessageSquarePlus },
      { href: "/ai/instances", label: "AI Access", icon: Bot },
      { href: "/ai/admin", label: "Org Admin", icon: Building2 },
      { href: "/ai/mcp", label: "Agents & MCP", icon: Workflow },
      { href: "/ai/usage", label: "AI Usage", icon: BrainCircuit },
      { href: "/ai/budgets", label: "AI Budgets", icon: CircleDollarSign },
      { href: "/ai/quotas", label: "AI Quotas", icon: SlidersHorizontal },
      { href: "/ai/policies", label: "AI Policies", icon: ShieldCheck },
      { href: "/ai/models", label: "AI Models", icon: Cpu },
      { href: "/ai/renewals", label: "AI Renewals", icon: Workflow },
      { href: "/ai/access-review", label: "Access Review", icon: ShieldCheck },
      { href: "/ai/gateway", label: "AI Gateway", icon: Zap },
      { href: "/ai/providers", label: "AI Providers", icon: Server },
      { href: "/ai/audit", label: "AI Audit", icon: History },
    ],
  },
  {
    title: "Operator / Admin",
    items: [
      { href: "/stacks", label: "Blueprints / stacks", icon: Blocks },
      { href: "/providers", label: "Providers", icon: Server },
      { href: "/accounts", label: "Accounts", icon: Building2 },
    ],
  },
];

// Flattened list for consumers that need every nav link (e.g. active-route lookup).
export const navItems: NavItem[] = navSections.flatMap((s) => s.items);

// isActive matches the dashboard exactly and other routes by prefix.
export function isActive(pathname: string, href: string): boolean {
  return href === "/" ? pathname === "/" : pathname.startsWith(href);
}

// One product, two workspaces: the infrastructure catalog and the AI-governance
// catalog. The nav follows the ROUTE (see lib/ai-mode.ts) - inside /ai/* only the
// AI section shows; everywhere else the infra sections show. The topbar AI sign
// navigates between the two. Single source of truth so the sidebar and mobile
// drawer filter identically.
export const AI_SECTION_TITLE = "AI Governance";

export function sectionsFor(aiMode: boolean): NavSection[] {
  return aiMode
    ? navSections.filter((s) => s.title === AI_SECTION_TITLE)
    : navSections.filter((s) => s.title !== AI_SECTION_TITLE);
}
