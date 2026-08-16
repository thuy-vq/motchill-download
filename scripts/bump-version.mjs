import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(scriptDirectory, '..');
const versionPath = path.join(root, 'VERSION');
const configPath = path.join(root, 'wails-app', 'wails.json');
const goVersionPath = path.join(root, 'wails-app', 'version.go');

const current = fs.readFileSync(versionPath, 'utf8').trim();
const match = /^(\d+)\.(\d+)\.(\d+)$/.exec(current);
if (!match) throw new Error(`Phiên bản không hợp lệ trong VERSION: ${current}`);

const version = `${match[1]}.${match[2]}.${Number(match[3]) + 1}`;
const config = JSON.parse(fs.readFileSync(configPath, 'utf8'));
config.info.productVersion = version;

fs.writeFileSync(versionPath, `${version}\n`);
fs.writeFileSync(configPath, `${JSON.stringify(config, null, 2)}\n`);
fs.writeFileSync(goVersionPath, `package main\n\nconst appVersion = "${version}"\n`);
process.stdout.write(`${version}\n`);
