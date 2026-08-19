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

  var COMMON_ASPECTS = [
    { label: "1:1", width: 1, height: 1 },
    { label: "16:9", width: 16, height: 9 },
    { label: "9:16", width: 9, height: 16 },
    { label: "4:3", width: 4, height: 3 },
    { label: "3:4", width: 3, height: 4 },
    { label: "3:2", width: 3, height: 2 },
    { label: "2:3", width: 2, height: 3 },
  ];
  var COMMON_ASPECT_LABELS = {};
  for (var ci = 0; ci < COMMON_ASPECTS.length; ci++) {
    COMMON_ASPECT_LABELS[COMMON_ASPECTS[ci].label] = true;
  }
  var VISIBLE_RESOLUTION_TIERS = { "1K": true, "2K": true, "4K": true };
  var TIER_ORDER = { "720p": 0, "1K": 1, "2K": 2, "4K": 3, auto: -1 };

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

  function aspectLogDistance(width, height, ratioLabel) {
    var target = null;
    for (var i = 0; i < COMMON_ASPECTS.length; i++) {
      if (COMMON_ASPECTS[i].label === ratioLabel) {
        target = COMMON_ASPECTS[i];
        break;
      }
    }
    if (!target) return Infinity;
    return Math.abs(Math.log(width / height) - Math.log(target.width / target.height));
  }

  function classifyCommonAspectRatio(width, height) {
    var exact = deriveRatio(width, height);
    if (COMMON_ASPECT_LABELS[exact]) return exact;
    var bestLabel = COMMON_ASPECTS[0].label;
    var bestDist = Infinity;
    for (var i = 0; i < COMMON_ASPECTS.length; i++) {
      var dist = aspectLogDistance(width, height, COMMON_ASPECTS[i].label);
      if (dist < bestDist) {
        bestDist = dist;
        bestLabel = COMMON_ASPECTS[i].label;
      }
    }
    return bestLabel;
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
    return classifyCommonAspectRatio(p.width, p.height);
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
      var ratio = classifyCommonAspectRatio(p.width, p.height);
      var tier = deriveTier(p.width, p.height);
      if (!groups[ratio]) {
        groups[ratio] = { ratio: ratio, sizes: [] };
        groupList.push(groups[ratio]);
      }
      groups[ratio].sizes.push({ value: value, tier: tier, width: p.width, height: p.height });
    }
    for (var j = 0; j < groupList.length; j++) {
      groupList[j].sizes.sort(function (a, b) { return TIER_ORDER[a.tier] - TIER_ORDER[b.tier]; });
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
    var best;
    var bestDist = Infinity;
    for (var i = 0; i < sizeOptions.length; i++) {
      var value = sizeOptions[i];
      if (value === "auto") continue;
      var p = parseSize(value);
      if (!p) continue;
      var r = classifyCommonAspectRatio(p.width, p.height);
      var t = deriveTier(p.width, p.height);
      if (r !== ratio || t !== tier) continue;
      var dist = aspectLogDistance(p.width, p.height, ratio);
      if (dist < bestDist) {
        bestDist = dist;
        best = value;
      }
    }
    return best;
  }

  function getAvailableRatios(sizeOptions) {
    return groupSizesByRatio(sizeOptions)
      .map(function (g) { return g.ratio; })
      .filter(function (ratio) { return COMMON_ASPECT_LABELS[ratio]; });
  }

  function getAvailableTiersForRatio(sizeOptions, ratio) {
    var tiers = {};
    var tierList = [];
    for (var i = 0; i < sizeOptions.length; i++) {
      var value = sizeOptions[i];
      if (value === "auto") continue;
      var p = parseSize(value);
      if (!p) continue;
      var r = classifyCommonAspectRatio(p.width, p.height);
      if (r === ratio) {
        var t = deriveTier(p.width, p.height);
        if (!tiers[t]) { tiers[t] = true; tierList.push(t); }
      }
    }
    tierList.sort(function (a, b) { return TIER_ORDER[a] - TIER_ORDER[b]; });
    return tierList;
  }

  function getVisibleTiersForRatio(sizeOptions, ratio) {
    return getAvailableTiersForRatio(sizeOptions, ratio).filter(function (tier) {
      return VISIBLE_RESOLUTION_TIERS[tier];
    });
  }

  function hasAutoOption(sizeOptions) {
    return sizeOptions.indexOf("auto") !== -1;
  }

  global.ImageSizeUtils = {
    gcd: gcd,
    parseSize: parseSize,
    isCanonicalImageSize: isCanonicalImageSize,
    deriveRatio: deriveRatio,
    classifyCommonAspectRatio: classifyCommonAspectRatio,
    deriveTier: deriveTier,
    deriveRatioFromValue: deriveRatioFromValue,
    deriveTierFromValue: deriveTierFromValue,
    displayImageSizeLabel: displayImageSizeLabel,
    groupSizesByRatio: groupSizesByRatio,
    findSizeByRatioAndTier: findSizeByRatioAndTier,
    getAvailableRatios: getAvailableRatios,
    getAvailableTiersForRatio: getAvailableTiersForRatio,
    getVisibleTiersForRatio: getVisibleTiersForRatio,
    hasAutoOption: hasAutoOption,
  };
})(window);
