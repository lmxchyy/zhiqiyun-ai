export type SidebarPointSources = {
  account?: Record<string, unknown> | null;
  summary?: Record<string, unknown> | null;
  moduleData?: Record<string, unknown> | null;
};

export type SidebarPointDisplay = {
  available: number;
  total: number;
  percent: number;
};

/** Lifetime-aware available/total for the user console sidebar. */
export function resolveSidebarPlanPoints(sources: SidebarPointSources): SidebarPointDisplay {
  const summary = (sources.summary || {}) as Record<string, unknown>;
  const account = (sources.account || {}) as Record<string, unknown>;
  const moduleData = (sources.moduleData || {}) as Record<string, unknown>;

  const rawAvailable = Number(
    account.available ??
      summary.availablePoints ??
      summary.pointsAvailable ??
      moduleData.availablePoints ??
      moduleData.pointsAvailable ??
      0
  );
  const frozen = Number(account.frozen || 0);
  const used = Number(account.totalUsed ?? account.totalConsumed ?? 0);
  const rawTotal = Number(
    account.totalGranted ?? account.total ?? summary.totalPoints ?? summary.pointsTotal ?? 0
  );
  const total = Math.max(rawAvailable + frozen + used, rawTotal || rawAvailable + frozen + used);
  const available = Math.max(0, rawAvailable);
  return {
    available,
    total: Math.max(available, total),
    percent: Math.min(100, Math.max(4, Math.round((available / Math.max(1, total)) * 100)))
  };
}
