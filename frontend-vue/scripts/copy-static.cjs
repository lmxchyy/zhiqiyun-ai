const fs = require("node:fs");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const source = path.resolve(root, "static");
const target = path.resolve(root, "dist", "build", "h5", "static");

try {
  if (fs.existsSync(source)) {
    copyDirectory(source, target);
  }
} catch (error) {
  console.error(error);
  process.exit(1);
}

function copyDirectory(from, to) {
  fs.mkdirSync(to, { recursive: true });
  for (const entry of fs.readdirSync(from, { withFileTypes: true })) {
    const sourcePath = path.resolve(from, entry.name);
    const targetPath = path.resolve(to, entry.name);
    if (entry.isDirectory()) {
      copyDirectory(sourcePath, targetPath);
    } else if (entry.isFile()) {
      fs.copyFileSync(sourcePath, targetPath);
    }
  }
}
