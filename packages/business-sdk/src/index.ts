import type { ApiClient } from "@xianzhi/api-client";
import { createAgentsSdk } from "./agents";
import { createAssetsSdk } from "./assets";
import { createAuthSdk } from "./auth";
import { createBillingSdk } from "./billing";
import { createDashboardSdk } from "./dashboard";
import { createGenerationSdk } from "./generation";
import { createMembershipSdk } from "./membership";
import { createModelsSdk } from "./models";
import { createPointsSdk } from "./points";
import { createRoleWorkbenchSdk } from "./role-workbench";
import type { BusinessSdk } from "./types";

export type {
  BillingOrderInput,
  BillingOrderResponse,
  BusinessSdk,
  HomeOverview,
  ItemsResponse,
  MemberProfileResponse,
  OperationProfileResponse,
  RechargeOrderInput,
  RoleWalletResponse,
  SubscriptionOrderInput
} from "./types";

export function createBusinessSdk(api: ApiClient): BusinessSdk {
  return {
    auth: createAuthSdk(api),
    dashboard: createDashboardSdk(api),
    generation: createGenerationSdk(api),
    assets: createAssetsSdk(api),
    models: createModelsSdk(api),
    points: createPointsSdk(api),
    membership: createMembershipSdk(api),
    billing: createBillingSdk(api),
    agents: createAgentsSdk(api),
    roleWorkbench: createRoleWorkbenchSdk(api)
  };
}
