/**
 * Shared image size utilities for the Web frontend (admin-vue).
 *
 * This module mirrors the logic in @xianzhi/shared-image-utils so that
 * the Web (native JS), mini-program/App (TypeScript via shared package),
 * and backend (Go) all use the same ratio/tier derivation rules.
 *
 * It is loaded as a plain browser global (window.ImageSizeUtils) so
 * canvas.js and smart-canvas.js can call it without bundler imports.
 */
(function (global) {
  "use strict";

  var COMMON_ASPECT_LABELS = {
    "1:1": true,
    "16:9": true,
    "9:16": true,
    "3:2": true,
    "2:3": true,
    "4:3": true,
    "3:4": true,
  };

  function gcd(a, b) {
    a = Math.abs(Math.round(a));
    b = Math.abs(Math.round(b));
    while (b) { var t = b; b = a % b; a = t; }
    return a || 1;
  }

  function parseSize(value) {
    if (typeof value !== "string") return null;
    var m = value.match(/^([1-9]\d*)x([1-9]\d*)$/i);
    if (!m) return null;
    var w = Number(m[1]);
    var h = Number(m[2]);
    if (!isFinite(w) || !isFinite(h) || w <= 0 || h <= 0) return null;
    return { width: w, height: h };
  }

  function isCanonicalImageSize(value) {
    return value === "auto" || Boolean(parseSize(value));
  }

  function deriveRatio(width, height) {
    var d = gcd(width, height);
    return (width / d) + ":" + (height / d);
  }

  function deriveTier(width, height) {
    var pixels = width * height;
    var maxEdge = Math.max(width, height);
    if (pixels <= 1280 * 720 && maxEdge <= 1280) return "720p";
    if (pixels <= 1536 * 1024 && maxEdge <= 1536) return "1K";
    if (pixels <= 2048 * 2048 && maxEdge <= 2048) return "2K";
    return "4K";
  }

  function deriveRatioFromValue(value) {
    if (value === "auto") return undefined;
    var p = parseSize(value);
    if (!p) return undefined;
    return deriveRatio(p.width, p.height);
  }

  function deriveTierFromValue(value) {
    if (value === "auto") return "auto";
    var p = parseSize(value);
    if (!p) return "auto";
    return deriveTier(p.width, p.height);
  }

  function displayImageSizeLabel(value) {
    if (value === "auto") return "auto";
    var p = parseSize(value);
    if (!p) return value;
    var ratio = deriveRatio(p.width, p.height);
    var tier = deriveTier(p.width, p.height);
    if (COMMON_ASPECT_LABELS[ratio]) return tier + " · " + ratio;
    return tier + " · " + p.width + "x" + p.height;
  }

  function groupSizesByRatio(sizeOptions) {
    var groups = {};
    var groupList = [];
    for (var i = 0; i < sizeOptions.length; i++) {
      var value = sizeOptions[i];
      if (value === "auto") continue;
      var p = parseSize(value);
      if (!p) continue;
      var ratio = deriveRatio(p.width, p.height);
      var tier = deriveTier(p.width, p.height);
      if (!groups[ratio]) {
        groups[ratio] = { ratio: ratio, sizes: [] };
        groupList.push(groups[ratio]);
      }
      groups[ratio].sizes.push({ value: value, tier: tier, width: p.width, height: p.height });
    }
    var tierOrder = { "720p": 0, "1K": 1, "2K": 2, "4K": 3, auto: -1 };
    for (var j = 0; j < groupList.length; j++) {
      groupList[j].sizes.sort(function (a, b) { return tierOrder[a.tier] - tierOrder[b.tier]; });
    }
    var ratioOrder = ["1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"];
    groupList.sort(function (a, b) {
      var ai = ratioOrder.indexOf(a.ratio);
      var bi = ratioOrder.indexOf(b.ratio);
      if (ai !== -1 && bi !== -1) return ai - bi;
      if (ai !== -1) return -1;
      if (bi !== -1) return 1;
      return a.ratio < b.ratio ? -1 : a.ratio > b.ratio ? 1 : 0;
    });
    return groupList;
  }

  function findSizeByRatioAndTier(sizeOptions, ratio, tier) {
    for (var i = 0; i < sizeOptions.length; i++) {
      var value = sizeOptions[i];
      if (value === "auto") continue;
      var p = parseSize(value);
      if (!p) continue;
      var r = deriveRatio(p.width, p.height);
      var t = deriveTier(p.width, p.height);
      if (r === ratio && t === tier) return value;
    }
    return undefined;
  }

  function getAvailableRatios(sizeOptions) {
    var groups = groupSizesByRatio(sizeOptions);
    return groups.map(function (g) { return g.ratio; });
  }

  function getAvailableTiersForRatio(sizeOptions, ratio) {
    var tiers = {};
    var tierList = [];
    for (var i = 0; i < sizeOptions.length; i++) {
      var value = sizeOptions[i];
      if (value === "auto") continue;
      var p = parseSize(value);
      if (!p) continue;
      var r = deriveRatio(p.width, p.height);
      if (r === ratio) {
        var t = deriveTier(p.width, p.height);
        if (!tiers[t]) { tiers[t] = true; tierList.push(t); }
      }
    }
    var tierOrder = { "720p": 0, "1K": 1, "2K": 2, "4K": 3, auto: -1 };
    tierList.sort(function (a, b) { return tierOrder[a] - tierOrder[b]; });
    return tierList;
  }

  function hasAutoOption(sizeOptions) {
    return sizeOptions.indexOf("auto") !== -1;
  }

  global.ImageSizeUtils = {
    gcd: gcd,
    parseSize: parseSize,
    isCanonicalImageSize: isCanonicalImageSize,
    deriveRatio: deriveRatio,
    deriveTier: deriveTier,
    deriveRatioFromValue: deriveRatioFromValue,
    deriveTierFromValue: deriveTierFromValue,
    displayImageSizeLabel: displayImageSizeLabel,
    groupSizesByRatio: groupSizesByRatio,
    findSizeByRatioAndTier: findSizeByRatioAndTier,
    getAvailableRatios: getAvailableRatios,
    getAvailableTiersForRatio: getAvailableTiersForRatio,
    hasAutoOption: hasAutoOption,
  };
})(window);
