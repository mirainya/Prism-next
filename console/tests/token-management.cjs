const { chromium } = require(process.env.PRISM_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');

// Synthetic data only: these tests never contact a deployed gateway.
const root = path.resolve(process.env.PRISM_CONSOLE_DIST || 'dist');
const admin = { id: 1, username: 'qa-admin', role: 'admin', status: 1, created_at: '2026-01-01T00:00:00Z' };
const owner = { ...admin, id: 2, username: 'qa-owner', role: 'user' };
const makeKey = (id, user) => ({ id, user_id: user.id, username: user.username, owner_enabled: true, name: `qa-key-${id}`, key_hint: '****test', state: 'active', auth_version: '1', rate_limit: 60, created_at: admin.created_at, expires_at: null, revoked_at: null, budget: { limit: '50', used: '3', held: '2', remaining: '45' }, xfs_storage: { configured: false } });
const tokens = [makeKey(11, admin), makeKey(22, owner)];
tokens[0].budget = { limit: '100000', used: '4', held: '0.125', remaining: '99995.875' };
for (const [id, state] of [[12, 'paused'], [13, 'revoked'], [14, 'expired']]) {
  tokens.push({ ...makeKey(id, admin), state, revoked_at: state === 'revoked' ? '2026-01-01T00:00:00Z' : null });
}
const events = [];
const server = http.createServer((request, response) => {
  const candidate = path.resolve(root, '.' + decodeURIComponent(new URL(request.url, 'http://127.0.0.1').pathname));
  if (candidate !== root && !candidate.startsWith(root + path.sep)) { response.writeHead(403).end(); return; }
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
    const errors = [], requests = [], changes = [], policies = [], creations = [];
    let actor = admin;
    let rejectNextChange = false;
    page.on('pageerror', error => errors.push(error.message));
    await page.route('**/api/**', async route => {
      const request = route.request(), url = new URL(request.url());
      requests.push(url);
      let data;
      if (url.pathname === '/api/auth/settings') data = { registration_enabled: false };
      else if (url.pathname === '/api/user/me') data = actor;
      else if (url.pathname === '/api/user/billing-account') data = null;
      else if (url.pathname === '/api/tokens/rate-limit-settings') data = { enabled: true, requests_per_min: 60 };
      else if (url.pathname === '/api/admin/users') data = [admin, owner];
      else if (url.pathname === '/api/tokens' && request.method() === 'POST') {
        const body = request.postDataJSON();
        creations.push(body);
        data = { id: 33, key: 'synthetic-created-key', balance: '0' };
      }
      else if (url.pathname === '/api/tokens/management') {
        const user = url.searchParams.get('user_id'), token = url.searchParams.get('token_id'), state = url.searchParams.get('state'), search = url.searchParams.get('search');
        data = { items: tokens.filter(t => (!user || t.user_id === Number(user)) && (!token || t.id === Number(token)) && (!state || t.state === state) && (!search || t.name.includes(search)) && (actor.role === 'admin' || t.user_id === actor.id)), total: 0, page: 1, page_size: 20 };
        data.total = data.items.length;
      } else if (/\/tokens\/\d+\/policy$/.test(url.pathname)) {
        const body = request.postDataJSON();
        policies.push(body);
        const token = tokens.find(t => t.id === Number(url.pathname.split('/')[3]));
        assert.equal(body.expected_version, token.auth_version);
        token.auth_version = String(Number(token.auth_version) + 1);
        token.rate_limit = body.rate_limit;
        token.expires_at = body.expires_at;
        data = { updated: true };
      } else if (/\/tokens\/\d+\/access$/.test(url.pathname)) {
        const body = request.postDataJSON();
        changes.push(body);
        if (rejectNextChange) {
          rejectNextChange = false;
          await route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ message: 'Key 状态已变化，请刷新后重新操作' }) });
          return;
        }
        const token = tokens.find(t => t.id === Number(url.pathname.split('/')[3]));
        assert.equal(body.expected_version, token.auth_version);
        token.auth_version = String(Number(token.auth_version) + 1);
        token.state = { pause: 'paused', resume: 'active', revoke: 'revoked' }[body.action];
        if (body.action === 'revoke') token.revoked_at = '2026-11-07T00:00:00Z';
        events.unshift({ id: events.length + 1, auth_version: token.auth_version, old_status: body.action === 'resume' ? 0 : 1, new_status: body.action === 'resume' ? 1 : 0, reason_code: `admin_${body.action}`, actor_user_id: 1, created_at: '2026-11-07T00:00:00Z', note: body.note });
        data = { updated: true };
      } else if (/\/tokens\/\d+\/access-history$/.test(url.pathname)) data = { items: events, total: events.length, page: 1, page_size: 10 };
      else if (url.pathname === '/api/calls/token-options') data = [];
      else if (url.pathname === '/api/calls') data = { items: [], total: 0, snapshot_at: '2026-11-07T00:00:00Z' };
      else if (url.pathname === '/api/admin/unified-gateway/channels') data = { items: [], total: 0 };
      else if (url.pathname.startsWith('/api/observability/')) data = { items: [], total: 0, page: 1, page_size: 20, snapshot_id: 0 };
      else { errors.push(`Unexpected API: ${url.pathname}`); data = {}; }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 0, data }) });
    });

    const openManagement = async name => {
      await page.getByRole('button', { name: `管理 ${name}`, exact: true }).click();
      const modal = page.getByRole('dialog', { name: `管理 API Key · ${name}`, exact: true });
      await modal.waitFor();
      return modal;
    };
    const chooseStatus = async (current, next) => {
      await page.getByRole('button', { name: current, exact: true }).click();
      await page.getByRole('option', { name: next, exact: true }).click();
    };
    await page.goto(base + '/#/tokens');
    const ownRow = page.getByRole('row').filter({ hasText: 'qa-key-11' });
    await ownRow.getByText('99,995.88', { exact: true }).waitFor();
    const table = page.getByRole('table', { name: 'API Key 列表', exact: true });
    assert.equal(await table.getByRole('row').count(), 2);
    assert.equal(await table.getByRole('link').count(), 0);
    assert.equal(await ownRow.getByRole('button').count(), 1);
    assert(requests.some(url => url.pathname === '/api/tokens/management' && url.searchParams.get('user_id') === '1' && url.searchParams.get('state') === 'active'));
    for (const id of [12, 13, 14, 22]) assert.equal(await page.getByRole('button', { name: `管理 qa-key-${id}`, exact: true }).count(), 0);
    if (process.env.PRISM_BROWSER_SCREENSHOT) await page.screenshot({ path: process.env.PRISM_BROWSER_SCREENSHOT, fullPage: true });
    await page.getByRole('button', { name: '我的 Key', exact: true }).click();
    await page.getByRole('option', { name: '全部用户', exact: true }).click();
    let row = page.getByRole('row').filter({ hasText: 'qa-key-22' });
    await row.getByText('qa-owner', { exact: true }).waitFor();
    let manager = await openManagement('qa-key-22');
    assert.equal(await manager.getByRole('button', { name: '增加额度', exact: true }).count(), 0);
    await manager.getByRole('button', { name: '关闭', exact: true }).last().click();
    // Policy values are saved as numbers and UTC timestamps; clearing the
    // date sends an explicit null instead of silently keeping the old value.
    manager = await openManagement('qa-key-11');
    await manager.getByRole('button', { name: '调用设置', exact: true }).click();
    let policyDialog = page.getByRole('dialog', { name: '调用设置 · qa-key-11', exact: true });
    assert.equal(await policyDialog.getByRole('button', { name: '保存设置', exact: true }).isDisabled(), true);
    await policyDialog.getByLabel('每分钟调用上限', { exact: true }).fill('1.5');
    await policyDialog.getByLabel('操作备注', { exact: true }).fill('限制测试请求');
    assert.equal(await policyDialog.getByRole('button', { name: '保存设置', exact: true }).isDisabled(), true);
    await policyDialog.getByLabel('每分钟调用上限', { exact: true }).fill('5');
    const futureInput = new Date(Date.now() + 86400000).toISOString().slice(0, 19);
    await policyDialog.getByLabel('到期时间', { exact: true }).fill(futureInput);
    const expectedExpiration = await page.evaluate(value => new Date(value).toISOString(), futureInput);
    await policyDialog.getByRole('button', { name: '保存设置', exact: true }).click();
    await policyDialog.waitFor({ state: 'hidden' });
    manager = await openManagement('qa-key-11');
    await manager.getByText('每分钟最多调用 5 次', { exact: true }).waitFor();
    assert.deepEqual(policies[0], { expected_version: '1', rate_limit: 5, expires_at: expectedExpiration, note: '限制测试请求' });
    await manager.getByRole('button', { name: '调用设置', exact: true }).click();
    await policyDialog.getByLabel('每分钟调用上限', { exact: true }).fill('120');
    await policyDialog.getByLabel('到期时间', { exact: true }).fill('');
    await policyDialog.getByLabel('操作备注', { exact: true }).fill('改为长期有效');
    await policyDialog.getByRole('button', { name: '保存设置', exact: true }).click();
    await policyDialog.waitFor({ state: 'hidden' });
    manager = await openManagement('qa-key-11');
    await manager.getByText('每分钟最多调用 60 次', { exact: true }).waitFor();
    await manager.getByRole('button', { name: '关闭', exact: true }).last().click();
    assert.equal(policies[1].expected_version, '2');
    assert.equal(policies[1].expires_at, null);

    await page.getByRole('button', { name: '创建 API Key', exact: true }).click();
    const creationDialog = page.getByRole('dialog', { name: '创建 API Key', exact: true });
    await creationDialog.getByPlaceholder('如: 生产环境、测试项目', { exact: true }).fill('temporary-key');
    await creationDialog.getByLabel('每分钟调用上限', { exact: true }).fill('4');
    await creationDialog.getByLabel('到期时间', { exact: true }).fill(futureInput);
    await creationDialog.getByRole('button', { name: '创建 API Key', exact: true }).click();
    await creationDialog.getByRole('button', { name: '我已保存，关闭', exact: true }).click();
    assert.equal(creations[0].rate_limit, 4);
    assert.equal(creations[0].expires_at, expectedExpiration);

    manager = await openManagement('qa-key-22');
    await manager.getByRole('button', { name: '停用', exact: true }).click();
    let dialog = page.getByRole('dialog', { name: '停用 Key', exact: true });
    assert.equal(await dialog.getByRole('button', { name: '确认停用', exact: true }).isDisabled(), true);
    await dialog.getByLabel('操作备注', { exact: true }).fill('核查异常调用');
    await dialog.getByRole('button', { name: '确认停用', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    await row.waitFor({ state: 'hidden' });
    await chooseStatus('使用中', '已停用');
    await row.getByText('已停用', { exact: true }).waitFor();
    assert.equal(changes[0].expected_version, '1');

    manager = await openManagement('qa-key-22');
    await manager.getByRole('button', { name: '启用', exact: true }).click();
    dialog = page.getByRole('dialog', { name: '启用 Key', exact: true });
    await dialog.getByLabel('操作备注', { exact: true }).fill('核查完成');
    rejectNextChange = true;
    await dialog.getByRole('button', { name: '确认启用', exact: true }).click();
    await dialog.getByRole('alert').waitFor();
    assert.equal(await dialog.getByRole('button', { name: '确认启用', exact: true }).isDisabled(), true);
    await dialog.getByRole('button', { name: '关闭', exact: true }).last().click();
    manager = await openManagement('qa-key-22');
    await manager.getByRole('button', { name: '启用', exact: true }).click();
    await dialog.getByLabel('操作备注', { exact: true }).fill('重新核查后启用');
    await dialog.getByRole('button', { name: '确认启用', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    await row.waitFor({ state: 'hidden' });
    await chooseStatus('已停用', '使用中');
    await row.getByText('使用中', { exact: true }).waitFor();

    manager = await openManagement('qa-key-22');
    await manager.getByRole('button', { name: '作废', exact: true }).click();
    dialog = page.getByRole('dialog', { name: '作废 Key', exact: true });
    await dialog.getByLabel('操作备注', { exact: true }).fill('永久停用');
    await dialog.getByRole('button', { name: '确认作废', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    await row.waitFor({ state: 'hidden' });
    await chooseStatus('使用中', '已作废');
    await row.getByText('已作废', { exact: true }).waitFor();
    manager = await openManagement('qa-key-22');
    assert.equal(await manager.getByRole('button', { name: '启用', exact: true }).count(), 0);
    assert.equal(await manager.getByRole('button', { name: '调用设置', exact: true }).count(), 0);
    assert.equal(changes.at(-1).expected_version, '3');
    await manager.getByRole('button', { name: '变更记录', exact: true }).click();
    dialog = page.getByRole('dialog', { name: '变更记录 · qa-key-22', exact: true });
    await dialog.getByText('管理员作废', { exact: true }).waitFor();
    await dialog.getByText('永久停用', { exact: true }).waitFor();
    await dialog.getByRole('button', { name: '关闭', exact: true }).last().click();

    manager = await openManagement('qa-key-22');
    await manager.getByRole('link', { name: '查看调用记录', exact: true }).click();
    await page.getByText('当前范围：用户 #2 · Key #22', { exact: true }).waitFor();
    await page.getByText('Key #22（暂无调用记录）', { exact: true }).waitFor();
    assert(requests.some(url => url.pathname === '/api/calls' && url.searchParams.get('user_id') === '2' && url.searchParams.get('token_id') === '22'));
    await page.goto(base + '/#/tokens?user_id=2&token_id=22');
    row = page.getByRole('row').filter({ hasText: 'qa-key-22' });
    await row.getByText('已作废', { exact: true }).waitFor();
    assert.equal(await page.getByRole('row').filter({ hasText: 'qa-key-11' }).count(), 0);
    assert.equal(await table.getByRole('link').count(), 0);

    // A non-admin follows the same links with a token filter and server-side ownership scope.
    actor = owner;
    await page.reload();
    await page.goto(base + '/#/tokens');
    row = page.getByRole('row').filter({ hasText: 'qa-key-22' });
    await page.getByText('没有使用中的 API Key', { exact: true }).waitFor();
    await chooseStatus('使用中', '已作废');
    await row.getByText('已作废', { exact: true }).waitFor();
    assert.equal(await page.getByLabel('所属用户 ID', { exact: true }).count(), 0);
    assert.equal(await page.getByRole('button', { name: '全部用户', exact: true }).count(), 0);
    manager = await openManagement('qa-key-22');
    await manager.getByRole('link', { name: '查看调用记录', exact: true }).click();
    await page.getByText('当前范围：用户 #2 · Key #22', { exact: true }).waitFor();
    assert(requests.some(url => url.pathname === '/api/calls' && url.searchParams.get('token_id') === '22' && !url.searchParams.has('user_id')));
    assert.deepEqual(errors, []);
    console.log(JSON.stringify({ own_active_default: 'passed', historical_keys_hidden: 'passed', compact_actions: 'passed', all_owners: 'passed', pause_resume_revoke: 'passed', stale_version: 'passed', retained_history: 'passed', owner_scope: 'passed', policy_and_creation: 'passed', console_errors: 0 }));
  } finally {
    if (browser) await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
