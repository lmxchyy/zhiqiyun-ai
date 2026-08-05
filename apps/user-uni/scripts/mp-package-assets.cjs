const fs = require("node:fs");
const path = require("node:path");

const neutralizableAssetExtensions = new Set([
  ".avif",
  ".bmp",
  ".gif",
  ".jpeg",
  ".jpg",
  ".png",
  ".svg",
  ".webp"
]);

function listFiles(directory) {
  const files = [];
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const filePath = path.resolve(directory, entry.name);
    if (entry.isDirectory()) files.push(...listFiles(filePath));
    else if (entry.isFile()) files.push(filePath);
  }
  return files;
}

function ignoredByRule(relativePath, rule) {
  const value = String(rule?.value || "");
  switch (rule?.type) {
    case "file":
      return relativePath === value;
    case "folder":
      return relativePath === value || relativePath.startsWith(`${value}/`);
    case "suffix":
      return relativePath.endsWith(value);
    case "prefix":
      return path.posix.basename(relativePath).startsWith(value);
    case "regexp":
      return new RegExp(value).test(relativePath);
    default:
      return false;
  }
}

function neutralizeIgnoredAssets(outputRoot, ignoreRules) {
  const neutralized = [];
  for (const filePath of listFiles(outputRoot)) {
    if (!neutralizableAssetExtensions.has(path.extname(filePath).toLowerCase())) continue;
    const relativePath = path.relative(outputRoot, filePath).split(path.sep).join("/");
    if (!ignoreRules.some((rule) => ignoredByRule(relativePath, rule))) continue;
    if (fs.statSync(filePath).size > 0) fs.writeFileSync(filePath, Buffer.alloc(0));
    neutralized.push(relativePath);
  }
  return neutralized;
}

module.exports = {
  ignoredByRule,
  neutralizeIgnoredAssets
};
