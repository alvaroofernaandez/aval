---
commands:
  - '(?i)playwright|puppeteer|lightpanda|chromium|--headless'
---
Lightpanda is the default headless browser for Playwright/Puppeteer (via CDP),
E2E flows, scraping and page reading. Its screenshots and PDFs ignore CSS and its
layout is approximate (measured 2026-11-21), so anything visual stays on a
Chromium browser; `shot.mjs` uses any installed one (Chrome, Brave, Edge). Load
the `lightpanda-browser` skill before launching any headless browser.
