const path = require("node:path");
const { spawnSync } = require("node:child_process");

const appRoot = path.resolve(__dirname, "..");
const productionApiBaseURL = String(
  process.env.VITE_API_BASE_URL || "https://ai.zs-kjhn.cn",
).trim().replace(/\/+$/, "");

if (!/^https:\/\/[^/]+/i.test(productionApiBaseURL)) {
  throw new Error(
    `Refusing to build a release mini-program with a non-HTTPS API base URL: ${productionApiBaseURL || "(empty)"}`,
  );
}

const env = {
  ...process.env,
  VITE_API_BASE_URL: productionApiBaseURL,
};

function run(command, args) {
  const result = spawnSync(command, args, {
    cwd: appRoot,
    env,
    stdio: "inherit",
    shell: false,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}

const uniCli = path.join(appRoot, "node_modules", ".bin", process.platform === "win32" ? "uni.cmd" : "uni");
if (process.platform === "win32") {
  run(process.env.ComSpec || "cmd.exe", [
    "/d",
    "/s",
    "/c",
    "node_modules\\.bin\\uni.cmd build -p mp-weixin --mode production",
  ]);
} else {
  run(uniCli, ["build", "-p", "mp-weixin", "--mode", "production"]);
}

run(process.execPath, [path.join(__dirname, "patch-mp-native-login.cjs")]);
run(process.execPath, [
  "--test",
  path.join(appRoot, "tests", "wallet-subpackage-output.test.mjs"),
  path.join(appRoot, "tests", "mp-startup-runtime.test.mjs"),
]);
run(process.execPath, [path.join(__dirname, "check-mp-package-size.cjs")]);
console.log(`Release API base URL: ${productionApiBaseURL}`);
