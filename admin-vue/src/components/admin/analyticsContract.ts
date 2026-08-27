export function analyticsMetricValue(record: Record<string, unknown>, key: string): number {
  const value = record[key];
  return typeof value === "number" ? value : 0;
}

export function normalizeAnalyticsModelsResponse<T>(response: { models?: T[] }): T[] {
  return Array.isArray(response.models) ? response.models : [];
}

export function normalizeAnalyticsProvidersResponse<T>(response: { providers?: T[] }): T[] {
  return Array.isArray(response.providers) ? response.providers : [];
}
