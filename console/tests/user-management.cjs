const { chromium } = require(process.env.PRISM_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');

// Exercise the built console using synthetic users; never connect to a database.
const root = path.resolve(process.env.PRISM_CONSOLE_DIST || 'dist');
const admin = { id: 1, username: 'qa-admin', role: 'admin', balance: 100, status: 1, created_at: '2026-01-01T00:00:00Z' };
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
    const changes = [];
    let failList = false;
    const member = { ...admin, id: 2, username: 'qa-member', role: 'user' };
    page.on('pageerror', error => errors.push(error.message));
    await page.route('**/api/**', async route => {
      const request = route.request();
      const pathname = new URL(request.url()).pathname;
      let data;
      if (pathname === '/api/auth/settings') data = { registration_enabled: false };
      else if (pathname === '/api/user/me') data = admin;
      else if (pathname === '/api/admin/users') {
        if (failList) {
          await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ message: '测试读取失败' }) });
          return;
        }
        data = [admin, member];
      } else if (pathname === '/api/admin/users/2/status') {
        assert.equal(request.method(), 'PUT');
        const body = request.postDataJSON();
        changes.push(body);
        member.status = body.status;
        data = { updated: true };
      } else {
        throw new Error(`Unexpected API: ${pathname}`);
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, data }) });
    });
    await page.goto(base + '/#/users');
    const adminRow = page.getByRole('row').filter({ hasText: 'qa-admin' });
    const memberRow = page.getByRole('row').filter({ hasText: 'qa-member' });
    await adminRow.waitFor();
    assert.equal(await adminRow.getByRole('button', { name: '设为用户', exact: true }).isDisabled(), true);
    assert.equal(await adminRow.getByRole('button', { name: '停用用户', exact: true }).isDisabled(), true);
    await page.getByLabel('搜索用户', { exact: true }).fill('qa-member');
    assert.equal(await adminRow.count(), 0, 'search must actually filter users');
    await memberRow.getByRole('button', { name: '停用用户', exact: true }).click();
    await page.getByRole('button', { name: '确认停用', exact: true }).click();
    await memberRow.getByRole('button', { name: '恢复用户', exact: true }).waitFor();
    await page.getByLabel('搜索用户', { exact: true }).fill('');
    await page.getByLabel('用户状态', { exact: true }).selectOption('0');
    assert.equal(await adminRow.count(), 0, 'status filter must hide enabled users');
    await memberRow.getByRole('button', { name: '恢复用户', exact: true }).click();
    await page.getByRole('button', { name: '确认恢复', exact: true }).click();
    await page.getByText('没有匹配的用户', { exact: true }).waitFor();
    await page.getByLabel('用户状态', { exact: true }).selectOption('');
    await memberRow.getByRole('button', { name: '停用用户', exact: true }).waitFor();
    assert.deepEqual(changes, [{ status: 0 }, { status: 1 }]);
    failList = true;
    await page.reload();
    await page.getByRole('alert').getByText('测试读取失败', { exact: true }).waitFor();
    failList = false;
    await page.getByRole('button', { name: '重试', exact: true }).click();
    await memberRow.waitFor();
    assert.equal(await page.getByRole('alert').count(), 0);

    const guestContext = await browser.newContext();
    const guest = await guestContext.newPage();
    let enabled = false;
    guest.on('pageerror', error => errors.push(error.message));
    await guest.route('**/api/**', route => route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ code: 0, data: { registration_enabled: enabled } }),
    }));
    await guest.goto(base);
    await guest.getByRole('button', { name: '登录', exact: true }).first().click();
    await guest.getByText('请登录您的账户', { exact: true }).waitFor();
    assert.equal(await guest.getByRole('button', { name: '没有账户? 点击注册', exact: true }).count(), 0);
    enabled = true;
    await guest.reload();
    await guest.getByRole('button', { name: '登录', exact: true }).first().click();
    await guest.getByRole('button', { name: '没有账户? 点击注册', exact: true }).click();
    await guest.getByText('创建新账户', { exact: true }).waitFor();
    assert.deepEqual(errors, []);
    console.log(JSON.stringify({ admin_protection: 'passed', suspend_restore: 'passed', search_and_filter: 'passed', load_retry: 'passed', registration_policy: 'passed', console_errors: 0 }));
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
})().catch(error => {
  console.error(error);
  process.exitCode = 1;
});
