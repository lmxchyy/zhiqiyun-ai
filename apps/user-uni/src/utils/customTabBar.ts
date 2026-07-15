type CustomTabBarInstance = {
  setData: (data: { selected: number; switching: boolean }) => void;
};

type PageWithCustomTabBar = {
  getTabBar?: () => CustomTabBarInstance | null;
};

export function syncCustomTabBar(selected: number) {
  const pages = getCurrentPages();
  const page = pages[pages.length - 1] as PageWithCustomTabBar | undefined;
  const tabBar = page?.getTabBar?.();
  tabBar?.setData({ selected, switching: false });
}
