#!/usr/bin/env node
/**
 * RENDER evidence: full-page screenshots at 390, 768 and 1440 px, plus the
 * content-overflow audit, in one command.
 *
 * Why it exists: about one user message in five was a screenshot saying a
 * "fixed" visual bug was not fixed, because the fix had been verified by reading
 * code. Before claiming a visual fix or a finished screen, run this and quote
 * what it prints.
 *
 *   node ~/.claude/skills/design-gates/scripts/shot.mjs <url | file.html> [--widths=390,768,1440] [--out=dir]
 *
 * Screenshots land in `.design/shots/` of the project (git toplevel of cwd, else
 * cwd) as `<name>-<width>.png`; keep `.design/` in .gitignore. The audit is
 * verify_overflow.mjs's own function, read from that file so there is one source
 * of truth, and it also reports the page's scrollWidth against the viewport.
 *
 * Needs Playwright with Chromium (Lightpanda has no rendering engine). When it
 * cannot render it exits 1: a missing browser must never read as a pass.
 * Exit 1 as well on horizontal overflow, clipped text or overlapping controls.
 */
import { readFileSync, mkdirSync, existsSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { dirname, join, resolve, basename } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));
const argv = process.argv.slice(2);
const target = argv.find(a => !a.startsWith('--'));
const opt = (name, dflt) => (argv.find(a => a.startsWith(`--${name}=`)) || '').split('=').slice(1).join('=') || dflt;

if (!target) {
  console.log('usage: node ~/.claude/skills/design-gates/scripts/shot.mjs <url | file.html> [--widths=390,768,1440] [--out=dir]');
  process.exit(1);
}
const widths = opt('widths', '390,768,1440').split(',').map(Number).filter(Boolean);

const fail = (msg) => { console.log(`shot: FAIL - ${msg}`); process.exit(1); };

// --- Playwright: next to this script, else in the project, else not at all ----
const spec = process.env.SHOT_PLAYWRIGHT || 'playwright';
let chromium;
try { ({ chromium } = await import(spec)); }
catch {
  try {
    const req = createRequire(join(process.cwd(), 'noop.js'));
    ({ chromium } = await import(pathToFileURL(req.resolve(spec)).href));
  } catch {
    fail(`playwright not installed (${spec}) - cannot render, so this is NOT a pass. npm i -D playwright && npx playwright install chromium`);
  }
}

// --- the audit: verify_overflow.mjs's own function -----------------------------
let AUDIT_SRC;
try {
  const src = readFileSync(join(HERE, 'verify_overflow.mjs'), 'utf8');
  const a = src.indexOf('const AUDIT = ');
  const b = src.indexOf('\nconst browser', a);
  if (a < 0 || b < 0) throw new Error('AUDIT not found');
  AUDIT_SRC = src.slice(a + 'const AUDIT = '.length, b).trim().replace(/;$/, '');
} catch (e) {
  fail(`cannot load the overflow audit from verify_overflow.mjs (${e.message})`);
}

// --- where do the shots go ------------------------------------------------------
let root;
try { root = execFileSync('git', ['rev-parse', '--show-toplevel'], { stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim(); }
catch { root = process.cwd(); }
const outDir = resolve(opt('out', join(root, '.design', 'shots')));
mkdirSync(outDir, { recursive: true });

const isUrl = /^[a-z][a-z0-9+.-]*:\/\//i.test(target);
let url = target;
if (!isUrl) {
  const abs = resolve(target);
  if (!existsSync(abs)) fail(`file not found: ${abs}`);
  url = pathToFileURL(abs).href;
}
const slug = (isUrl ? new URL(url).host + new URL(url).pathname : basename(target).replace(/\.[^.]+$/, ''))
  .replace(/[^a-z0-9]+/gi, '-').replace(/^-|-$/g, '').toLowerCase() || 'page';

// Playwright's own Chromium is pinned per Playwright version, so a project on
// another version finds no browser. Any installed Chromium-based browser renders
// the same pixels, so try those before giving up. SHOT_BROWSER overrides.
const INSTALLED = [
  process.env.SHOT_BROWSER,
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/Applications/Brave Browser.app/Contents/MacOS/Brave Browser',
  '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge',
  '/Applications/Chromium.app/Contents/MacOS/Chromium',
  '/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser', '/usr/bin/brave-browser',
].filter((p) => p && existsSync(p));

async function launchAny() {
  const chrome = await chromium.launch({ channel: 'chrome' }).catch(() => null);
  if (chrome) return chrome;
  const attempts = [() => chromium.launch(),
    ...INSTALLED.map((executablePath) => () => chromium.launch({ executablePath }))];
  let last;
  for (const attempt of attempts) {
    try { return await attempt(); } catch (e) { last = e; }
  }
  throw last;
}

let browser;
try { browser = await launchAny(); }
catch (e) {
  fail(`Chromium could not start (${String(e.message).split('\n')[0]}) - cannot render, so this is NOT a pass. npx playwright install chromium`);
}

const problems = [];
console.log(`shot: ${url}`);
for (const w of widths) {
  const page = await browser.newPage({ viewport: { width: w, height: 900 } });
  try { await page.goto(url, { waitUntil: 'load', timeout: 30000 }); }
  catch (e) { await browser.close(); fail(`cannot load ${url} (${String(e.message).split('\n')[0]})`); }
  await page.waitForTimeout(150);
  const path = join(outDir, `${slug}-${w}.png`);
  await page.screenshot({ path, fullPage: true });
  const m = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    height: document.documentElement.scrollHeight,
  }));
  const { clipped, overlaps } = await page.evaluate(`(${AUDIT_SRC})()`);
  const sideways = m.scrollWidth - w;
  console.log(`  ${w}px  ${path}  scrollWidth=${m.scrollWidth} height=${m.height} clipped=${clipped.length} overlaps=${overlaps.length}`);
  if (sideways > 1) problems.push(`@${w}px  horizontal overflow: scrollWidth ${m.scrollWidth} > viewport ${w} (+${sideways}px)`);
  for (const c of clipped.slice(0, 5)) problems.push(`@${w}px  [clipped] ${c}`);
  for (const o of overlaps.slice(0, 5)) problems.push(`@${w}px  [overlap] ${o}`);
  await page.close();
}
await browser.close();

if (problems.length) {
  console.log('shot: FAIL - layout does not survive these widths');
  for (const p of problems) console.log('  x ' + p);
  process.exit(1);
}
console.log(`shot: OK - ${widths.length} screenshots in ${outDir}; no horizontal overflow, clipped text or overlapping controls. Look at them before claiming the fix.`);
