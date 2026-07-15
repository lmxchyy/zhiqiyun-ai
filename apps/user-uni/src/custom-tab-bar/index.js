const tabItems = [
  {
    pagePath: "/pages/user/UserHomePage",
    route: "pages/user/UserHomePage",
    text: "首页",
    iconPath: "/static/icons/home.svg",
    selectedIconPath: "/static/icons/home-active.svg"
  },
  {
    pagePath: "/pages/user/UserCreationPage",
    route: "pages/user/UserCreationPage",
    text: "创作",
    iconPath: "/static/icons/create.svg",
    selectedIconPath: "/static/icons/create-active.svg"
  },
  {
    pagePath: "/pages/user/UserAssetsPage",
    route: "pages/user/UserAssetsPage",
    text: "作品",
    iconPath: "/static/icons/assets.svg",
    selectedIconPath: "/static/icons/assets-active.svg"
  },
  {
    pagePath: "/pages/user/UserMinePage",
    route: "pages/user/UserMinePage",
    text: "我的",
    iconPath: "/static/icons/profile.svg",
    selectedIconPath: "/static/icons/profile-active.svg"
  }
];

function rememberSelected(selected) {
  const app = getApp();
  app.globalData = app.globalData || {};
  app.globalData.miniProgramTabSelected = selected;
}

function currentRoute() {
  const pages = getCurrentPages();
  const current = pages[pages.length - 1];
  return current && current.route ? current.route : "";
}

Component({
  data: {
    selected: 0,
    switching: false,
    list: tabItems
  },

  lifetimes: {
    attached() {
      this.syncSelected();
      wx.nextTick(() => this.syncSelected());
      setTimeout(() => this.syncSelected(), 120);
    },
    detached() {
      this.clearSwitchingTimer();
    }
  },

  pageLifetimes: {
    show() {
      this.syncSelected();
    }
  },

  methods: {
    clearSwitchingTimer() {
      if (!this.switchingTimer) return;
      clearTimeout(this.switchingTimer);
      this.switchingTimer = null;
    },

    scheduleSwitchingReset() {
      this.clearSwitchingTimer();
      this.switchingTimer = setTimeout(() => this.syncSelected(), 1200);
    },

    syncSelected() {
      const route = currentRoute();
      const routeSelected = tabItems.findIndex((item) => item.route === route);
      const selected = routeSelected >= 0 ? routeSelected : 0;
      rememberSelected(selected);
      if (selected !== this.data.selected || this.data.switching) {
        this.setData({ selected, switching: false });
      }
    },

    switchTab(event) {
      const index = Number(event.currentTarget.dataset.index);
      const item = tabItems[index];
      if (!item) return;
      if (currentRoute() === item.route) {
        this.syncSelected();
        return;
      }
      this.setData({ switching: true });
      this.scheduleSwitchingReset();
      wx.switchTab({
        url: item.pagePath,
        success: () => {
          this.clearSwitchingTimer();
          rememberSelected(index);
          this.setData({ selected: index, switching: false });
          wx.nextTick(() => this.syncSelected());
        },
        fail: () => {
          this.clearSwitchingTimer();
          this.syncSelected();
          wx.showToast({ title: "页面切换失败，请重试", icon: "none" });
        }
      });
    }
  }
});
