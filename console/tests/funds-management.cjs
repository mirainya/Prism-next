const { chromium } = require(process.env.PRISM_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');

// All API responses are synthetic; this test never credits a real account.
const root = path.resolve(process.env.PRISM_CONSOLE_DIST || 'dist');
const user = { id: 1, username: 'qa-admin', role: 'admin', balance: '99', status: 1, created_at: '2026-01-01T00:00:00Z' };
const account = { id: 7, user_id: 1, status: 'open', currency: 'CREDIT', posted_balance: '100', held_amount: '3', credit_limit: '2', available: '99', state_event_id: 1, state_reason: 'account_opened', state_changed_at: null };
const token = { id: 11, name: 'qa-key', key: '****test', status: 1, balance: '45', total_used: '3', budget: { limit: '50', used: '3', held: '2', remaining: '45' } };
const server = http.createServer((request, response) => {
  const candidate = path.resolve(root, '.' + decodeURIComponent(new URL(request.url, 'http://127.0.0.1').pathname));
  if (candidate !== root && !candidate.startsWith(root + path.sep)) {
    response.writeHead(403).end();
    return;
  }
  const file = fs.existsSync(candidate) && fs.statSync(candidate).isFile() ? candidate : path.join(root, 'index.html');
  response.setHeader('Content-Type', ({ '.js': 'application/javascript', '.css': 'text/css', '.html': 'text/html', '.svg': 'image/svg+xml' })[path.extname(file)] || 'application/octet-stream');
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
    const errors = [], credits = [], budgets = [];
    const applied = new Set();
    let loseFirstResponse = true;
    page.on('pageerror', error => errors.push(error.message));
    await page.route('**/api/**', async route => {
      const request = route.request();
      const url = new URL(request.url());
      let data;
      if (url.pathname === '/api/auth/settings') data = { registration_enabled: false };
      else if (url.pathname === '/api/user/me') data = user;
      else if (url.pathname === '/api/user/billing-account') data = account;
      else if (url.pathname === '/api/tokens/rate-limit-settings') data = { enabled: true, requests_per_min: 60 };
      else if (url.pathname === '/api/admin/users') data = [{ ...user, billing_account: account }];
      else if (url.pathname === '/api/tokens') data = [token];
      else if (url.pathname === '/api/tokens/management') data = { items: [{ ...token, key_hint: token.key, state: 'active', auth_version: '1', user_id: 1, username: user.username, owner_enabled: true, created_at: user.created_at, expires_at: null, revoked_at: null }], total: 1, page: 1, page_size: 20 };
      else if (url.pathname === '/api/admin/users/1/recharge') {
        const body = request.postDataJSON();
        credits.push(body);
        applied.add(body.request_id);
        if (loseFirstResponse) {
          loseFirstResponse = false;
          await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ message: '测试响应中断' }) });
          return;
        }
        data = { recharged: true };
      } else if (url.pathname === '/api/tokens/11/budget-adjustments') {
        budgets.push(request.postDataJSON());
        data = { id: 11, balance: '45', total_used: '3' };
      } else if (url.pathname === '/api/observability/funds-events') {
        const isBudget = url.searchParams.get('account_type') === 'token';
        data = { items: [{ id: 1, account_type: isBudget ? 'token' : 'user', account_id: isBudget ? 11 : 7, user_id: 1, username: user.username, token_id: isBudget ? 11 : 0, token_name: isBudget ? token.name : '', direction: isBudget ? 'credit' : 'hold', category: isBudget ? 'budget_adjustment' : 'reservation_created', amount: '2', currency: 'CREDIT', call_id: '', request_id: '', note: '隔离测试流水', actor_user_id: 1, before: null, after: null, created_at: '2026-11-07T00:00:00Z' }], total: 1, page: 1, page_size: 20, snapshot_id: 1 };
      } else if (url.pathname.startsWith('/api/observability/')) data = { items: [], total: 0, page: 1, page_size: 20, snapshot_id: 0 };
      else {
        errors.push(`Unexpected API: ${url.pathname}`);
        data = {};
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, data }) });
    });
    await page.goto(base + '/#/users');
    const adminRow = page.getByRole('row').filter({ hasText: user.username });
    await adminRow.getByText('可用 99 CREDIT', { exact: true }).waitFor();
    await adminRow.getByRole('button', { name: '账户充值', exact: true }).click();
    let dialog = page.getByRole('dialog', { name: '账户资金充值', exact: true });
    assert.equal(await dialog.getByRole('button', { name: '确认提交', exact: true }).isDisabled(), true);
    const exactAmount = '0.123456789012345678';
    await dialog.getByLabel('充值金额', { exact: true }).fill(exactAmount);
    await dialog.getByLabel('操作备注', { exact: true }).fill('精确金额与重试测试');
    await dialog.getByRole('button', { name: '确认提交', exact: true }).click();
    await dialog.getByRole('alert').getByText('测试响应中断', { exact: true }).waitFor();
    await page.reload();
    await adminRow.getByRole('button', { name: '账户充值', exact: true }).click();
    dialog = page.getByRole('dialog', { name: '账户资金充值', exact: true });
    assert.equal(await dialog.getByLabel('充值金额', { exact: true }).inputValue(), exactAmount);
    assert.equal(await dialog.getByLabel('充值金额', { exact: true }).isDisabled(), true);
    await dialog.getByRole('button', { name: '重试原操作', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    assert.equal(credits.length, 2);
    assert.deepEqual(credits[0], credits[1]);
    assert.equal(applied.size, 1);
    assert.equal(credits[0].amount, exactAmount);

    await page.goto(base + '/#/tokens');
    await page.getByText('99.00', { exact: true }).waitFor();
    await page.getByRole('button', { name: '管理 qa-key', exact: true }).click();
    await page.getByRole('dialog', { name: '管理 API Key · qa-key', exact: true }).getByRole('button', { name: '增加额度', exact: true }).click();
    dialog = page.getByRole('dialog', { name: '增加 Key 额度', exact: true });
    await dialog.getByText('增加这把 Key 的消费上限，不改变账户余额。', { exact: true }).waitFor();
    await dialog.getByLabel('增加额度', { exact: true }).fill('2.5');
    await dialog.getByLabel('操作备注', { exact: true }).fill('调整测试 Key 的消费限额');
    await dialog.getByRole('button', { name: '确认提交', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    assert.equal(budgets.length, 1);
    assert.equal(budgets[0].amount, '2.5');
    assert.equal(credits.length, 2, 'budget adjustment must not call account funding');

    await page.goto(base + '/#/observability');
    await page.getByRole('tab', { name: '资金记录', exact: true }).click();
    await page.getByText('占用可用资金', { exact: true }).waitFor();
    await page.getByText('隔离测试流水', { exact: true }).waitFor();
    assert.equal(await page.getByText('-2.00000000 CREDIT', { exact: true }).count(), 0, 'a hold is not a posted debit');
    await page.getByRole('button', { name: '账户资金', exact: true }).click();
    await page.getByRole('option', { name: 'Key 额度调整', exact: true }).click();
    await page.getByRole('button', { name: '查询', exact: true }).click();
    await page.getByText('增加限额', { exact: true }).waitFor();
    assert.deepEqual(errors, []);
    console.log(JSON.stringify({ exact_decimal: 'passed', retry_after_reload: 'passed', separate_account_and_budget: 'passed', current_funds_history: 'passed', console_errors: 0 }));
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
})().catch(error => {
  console.error(error);
  process.exitCode = 1;
});
