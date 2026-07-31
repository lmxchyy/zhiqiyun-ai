const fs = require("node:fs");
const path = require("node:path");

const outputRoot = path.resolve(__dirname, "..", "dist", "build", "mp-weixin");
const maxPackageBytes = 2 * 1024 * 1024;
const maxMainQualityBytes = 1.5 * 1024 * 1024;
const maxTotalBytes = 30 * 1024 * 1024;

function readJson(fileName) {
  const filePath = path.resolve(outputRoot, fileName);
  if (!fs.existsSync(filePath)) {
    throw new Error(`Missing ${fileName}; run the mp-weixin build first`);
  }
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function listFiles(directory) {
  const files = [];
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const filePath = path.resolve(directory, entry.name);
    if (entry.isDirectory()) files.push(...listFiles(filePath));
    else if (entry.isFile()) files.push(filePath);
  }
  return files;
}

function normalizedRelative(filePath) {
  return path.relative(outputRoot, filePath).split(path.sep).join("/");
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

function formatMB(bytes) {
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
}

function verifyRelativeRequires(files) {
  for (const filePath of files.filter((item) => path.extname(item) === ".js")) {
    const source = fs.readFileSync(filePath, "utf8");
    for (const match of source.matchAll(/require\(["'](\.[^"']+)["']\)/g)) {
      const request = match[1];
      const resolved = path.resolve(path.dirname(filePath), request);
      const candidates = [resolved, `${resolved}.js`, path.resolve(resolved, "index.js")];
      if (!candidates.some((candidate) => fs.existsSync(candidate))) {
        throw new Error(
          `Broken generated require in ${normalizedRelative(filePath)}: ${request}`
        );
      }
    }
  }
}

const appJson = readJson("app.json");
const projectConfig = readJson("project.config.json");
const ignoreRules = Array.isArray(projectConfig.packOptions?.ignore)
  ? projectConfig.packOptions.ignore
  : [];
const sourceFiles = listFiles(outputRoot);
const includedFiles = sourceFiles.filter((filePath) => {
  const relativePath = normalizedRelative(filePath);
  return !ignoreRules.some((rule) => ignoredByRule(relativePath, rule));
});
verifyRelativeRequires(includedFiles);
const subPackages = Array.isArray(appJson.subPackages) ? appJson.subPackages : [];
const subPackageRoots = subPackages.map((item) => String(item.root || "")).filter(Boolean);
const packageRows = [
  {
    name: "MAIN",
    pages: Array.isArray(appJson.pages) ? appJson.pages.length : 0,
    files: includedFiles.filter((filePath) => {
      const relativePath = normalizedRelative(filePath);
      return !subPackageRoots.some((root) => relativePath.startsWith(`${root}/`));
    })
  },
  ...subPackages.map((item) => ({
    name: item.root,
    pages: Array.isArray(item.pages) ? item.pages.length : 0,
    files: includedFiles.filter((filePath) =>
      normalizedRelative(filePath).startsWith(`${item.root}/`)
    )
  }))
].map((item) => ({
  ...item,
  bytes: item.files.reduce((sum, filePath) => sum + fs.statSync(filePath).size, 0)
}));
const sourcePackageRows = [
  {
    name: "MAIN",
    files: sourceFiles.filter((filePath) => {
      const relativePath = normalizedRelative(filePath);
      return !subPackageRoots.some((root) => relativePath.startsWith(`${root}/`));
    })
  },
  ...subPackages.map((item) => ({
    name: item.root,
    files: sourceFiles.filter((filePath) =>
      normalizedRelative(filePath).startsWith(`${item.root}/`)
    )
  }))
].map((item) => ({
  ...item,
  bytes: item.files.reduce((sum, filePath) => sum + fs.statSync(filePath).size, 0)
}));

for (const item of packageRows) {
  console.log(`${item.name}: ${formatMB(item.bytes)}, ${item.pages} pages`);
}
for (const item of sourcePackageRows) {
  console.log(`SOURCE ${item.name}: ${formatMB(item.bytes)}`);
}

const oversized = packageRows.filter((item) => item.bytes > maxPackageBytes);
const oversizedSourcePackages = sourcePackageRows.filter((item) => item.bytes > maxPackageBytes);
const mainPackage = packageRows.find((item) => item.name === "MAIN");
const totalBytes = includedFiles.reduce((sum, filePath) => sum + fs.statSync(filePath).size, 0);
console.log(`TOTAL: ${formatMB(totalBytes)}`);
if (oversized.length > 0) {
  throw new Error(
    `WeChat package size exceeds 2 MB: ${oversized
      .map((item) => `${item.name}=${formatMB(item.bytes)}`)
      .join(", ")}`
  );
}
if (oversizedSourcePackages.length > 0) {
  throw new Error(
    `WeChat source package size exceeds 2 MB before upload filtering: ${oversizedSourcePackages
      .map((item) => `${item.name}=${formatMB(item.bytes)}`)
      .join(", ")}`
  );
}
if (mainPackage && mainPackage.bytes > maxMainQualityBytes) {
  throw new Error(
    `WeChat main package exceeds the 1.5 MB code-quality target: ${formatMB(mainPackage.bytes)}`
  );
}
if (totalBytes > maxTotalBytes) {
  throw new Error(`WeChat total package size exceeds 30 MB: ${formatMB(totalBytes)}`);
}
