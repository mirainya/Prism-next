const { chromium } = require(process.env.PRISM_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');

// Serve the release build locally and exercise account recovery with synthetic API data.
const root = path.resolve(process.env.PRISM_CONSOLE_DIST || 'dist');
const account = {
  id: 7, user_id: 1, status: 'frozen', currency: 'CREDIT',
  posted_balance: '-20.25', credit_limit: '100', held_amount: '3', available: '76.75',
  state_event_id: 20, state_reason: 'settlement_overrun', state_changed_at: '2026-11-07T05:00:00Z',
};
const user = { id: 1, username: 'qa-admin', role: 'admin', balance: 100, status: 1, created_at: '2026-01-01T00:00:00Z' };
const server = http.createServer((request, response) => {
  const pathname = new URL(request.url, 'http://127.0.0.1').pathname;
  const candidate = path.resolve(root, '.' + decodeURIComponent(pathname));
  if (candidate !== root && !candidate.startsWith(root + path.sep)) {
    response.writeHead(403).end();
    return;
  }
  const file = fs.existsSync(candidate) && fs.statSync(candidate).isFile() ? candidate : path.join(root, 'index.html');
  const types = { '.js': 'application/javascript', '.css': 'text/css', '.html': 'text/html', '.svg': 'image/svg+xml' };
  response.setHeader('Content-Type', types[path.extname(file)] || 'application/octet-stream');
  fs.createReadStream(file).pipe(response);
});

(async () => {
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  let browser;
  try {
    browser = await chromium.launch({ channel: process.env.PRISM_BROWSER_CHANNEL || 'msedge', headless: true });
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    await context.addInitScript(() => localStorage.setItem('prism_token', 'isolated-browser-session'));
    const page = await context.newPage();
    const errors = [];
    const submissions = [];
    let current = { ...account };
    let blocked = false;
    page.on('pageerror', error => errors.push(error.message));
    await page.route('**/api/**', async route => {
      const request = route.request();
      const url = new URL(request.url());
      let data;
      if (url.pathname === '/api/user/me') data = user;
      else if (url.pathname === '/api/admin/users') data = [{ ...user, billing_account: current }];
      else if (url.pathname === '/api/admin/users/1/billing-account') {
        data = {
          account: current, recent_anomalies: [], budget_issues: [],
          can_unfreeze: current.status === 'frozen' && !blocked,
          unfreeze_block_reason: blocked ? '仍有 Key 的当前预算超限，请先处理预算' : '',
        };
      } else if (url.pathname === '/api/admin/users/1/billing-account/unfreeze') {
        assert.equal(request.method(), 'POST');
        submissions.push(request.postDataJSON());
        current = { ...current, status: 'open', state_event_id: 21, state_reason: 'manual_overrun_recovery' };
        data = { updated: true, status: 'open' };
      } else {
        await route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ message: 'Unexpected test API' }) });
        return;
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, data }) });
    });
    await page.goto(base + '/#/users');
    await page.getByRole('button', { name: '已冻结 · 查看与解冻', exact: true }).click();
    let dialog = page.getByRole('dialog', { name: '计费账户 · qa-admin' });
    const confirm = dialog.getByRole('button', { name: '确认解冻', exact: true });
    await dialog.getByText('76.75', { exact: true }).waitFor();
    assert.equal(await confirm.isDisabled(), true, 'a recovery note is required');
    await dialog.getByLabel('解冻处理备注').fill('   ');
    assert.equal(await confirm.isDisabled(), true, 'whitespace is not a recovery note');
    await dialog.getByLabel('解冻处理备注').fill('  已核查预扣差额，可用额度充足  ');
    assert.equal(await confirm.isEnabled(), true);
    await confirm.click();
    await dialog.waitFor({ state: 'hidden' });
    await page.getByRole('button', { name: '正常 · 详情', exact: true }).waitFor();
    assert.deepEqual(submissions, [{ expected_freeze_event_id: 20, note: '已核查预扣差额，可用额度充足' }]);

    // An ineligible account must explain the restriction and prevent recovery.
    current = { ...account };
    blocked = true;
    await page.reload();
    await page.getByRole('button', { name: '已冻结 · 查看与解冻', exact: true }).click();
    dialog = page.getByRole('dialog', { name: '计费账户 · qa-admin' });
    await dialog.getByText('仍有 Key 的当前预算超限，请先处理预算', { exact: true }).waitFor();
    assert.equal(await dialog.getByLabel('解冻处理备注').isDisabled(), true);
    assert.equal(await dialog.getByRole('button', { name: '确认解冻', exact: true }).isDisabled(), true);
    assert.equal(submissions.length, 1, 'a blocked account must not submit recovery');
    assert.deepEqual(errors, []);
    console.log(JSON.stringify({ recovery_flow: 'passed', blocked_recovery: 'passed', console_errors: 0 }));
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
})().catch(error => {
  console.error(error);
  process.exitCode = 1;
});
