// Isolated browser smoke test. API calls are mocked; no real SMS or user data is used.
const { chromium } = require('playwright');
const { PNG } = require('pngjs');
const JSZip = require('jszip');
const fs = require('node:fs/promises');
const path = require('node:path');
const os = require('node:os');
const assert = require('node:assert/strict');

const base = process.env.QA_BASE_URL || 'http://127.0.0.1:18081';
if (!['127.0.0.1', 'localhost'].includes(new URL(base).hostname)) throw new Error('QA must target loopback');
const output = process.env.QA_OUTPUT || path.join(os.tmpdir(), 'bharatchat-ui-20261001');
const userId = '00000000-0000-4000-8000-000000000001';
const peerId = '00000000-0000-4000-8000-000000000002';
const chatId = '00000000-0000-4000-8000-000000000003';

async function capture(page, name) {
  await page.waitForTimeout(350); // Let Flutter route/dialog transitions settle.
  const file = path.join(output, `${name}.png`);
  const buffer = await page.screenshot({ path: file });
  const png = PNG.sync.read(buffer);
  const colors = new Set();
  for (let i = 0; i < png.data.length; i += 64) colors.add(`${png.data[i]},${png.data[i+1]},${png.data[i+2]}`);
  assert(colors.size > 15, `${name}: blank or unrendered screen`);
  console.log(JSON.stringify({ screenshot: file, width: png.width, height: png.height, colors: colors.size }));
}

async function enterText(page, name, value) {
  const field = page.getByRole('textbox', { name, exact: true });
  await field.waitFor();
  await page.waitForTimeout(350);
  await field.click();
  await page.keyboard.press('ControlOrMeta+A');
  await page.keyboard.type(value, { delay: 30 });
}

(async () => {
  await fs.mkdir(output, { recursive: true });
  const archive = new JSZip();
  archive.file('metadata.json', JSON.stringify({ exportVersion: 1 }));
  archive.file('profile.jsonl', JSON.stringify({ id: userId, displayName: 'QA User' }) + '\n');
  const exportBytes = await archive.generateAsync({ type: 'nodebuffer' });
  const browser = await chromium.launch({ channel: 'chrome', headless: true });
  try {
    for (const viewport of [{ width: 390, height: 844 }, { width: 1366, height: 900 }]) {
      const context = await browser.newContext({ viewport });
      const page = await context.newPage();
      page.setDefaultTimeout(30000);
      let created = false;
      let blocked = false;
      let reports = 0;
      let exports = 0;
      let deletions = 0;
      const errors = [];
      page.on('pageerror', error => errors.push(error.message));
      page.on('console', message => {
        if (/overflowed by|RenderFlex overflow/i.test(message.text())) errors.push(message.text());
      });
      await page.route('**/api/v1/**', async route => {
        const request = route.request();
        const endpoint = new URL(request.url()).pathname.replace('/api/v1', '');
        let status = 200;
        let data = {};
        if (endpoint === '/users/me/export') {
          exports++;
          await route.fulfill({ status: 200, contentType: 'application/zip', body: exportBytes,
            headers: { 'Access-Control-Allow-Origin': '*', 'Cache-Control': 'no-store, private' } });
          return;
        }
        if (endpoint === '/auth/otp/request') { status = 202; data = { message: 'OTP sent successfully' }; }
        else if (endpoint === '/auth/otp/verify' || endpoint === '/auth/refresh') data = {
          userId, isNewUser: false, accessToken: 'qa-access', refreshToken: 'qa-refresh',
          accessTokenExpiresAt: new Date(Date.now()+900000).toISOString(),
        };
        else if (endpoint === '/auth/ws-ticket') { status = 403; data = { code: 'FORBIDDEN', message: 'QA uses mocked REST' }; }
        else if (endpoint === '/users/resolve') data = { id: peerId, username: 'asha', displayName: 'Asha Rao' };
        else if (endpoint === '/chats/direct') { created = true; data = { id: chatId }; }
        else if (endpoint === '/chats') data = { chats: created ? [{ id: chatId, type: 'direct', peerUserId: peerId, peerDisplayName: 'Asha Rao', unreadCount: 0, isMuted: false, isPinned: false, peerIsOnline: false, lastActivityAt: new Date().toISOString() }] : [] };
        else if (endpoint.endsWith('/messages')) data = { messages: [] };
        else if (endpoint === `/users/me/blocked/${peerId}`) { blocked = request.method() === 'PUT'; status = 204; }
        else if (endpoint === '/users/me/blocked') data = { users: blocked ? [{ id: peerId, username: 'asha', displayName: 'Asha Rao' }] : [], nextCursor: '' };
        else if (endpoint === '/users/me/reports') { reports++; status = 201; data = { id: 'report-1', status: 'open' }; }
        else if (endpoint === '/users/me' && request.method() === 'DELETE') {
          const body = request.postDataJSON();
          assert.equal(body.confirmation, 'DELETE');
          assert.equal(body.refreshToken, 'qa-refresh');
          deletions++; status = 204;
        }
        else if (endpoint === '/auth/logout') data = { message: 'logged out' };
        else if (endpoint === '/users/me') data = { id: userId, phoneNumber: '+919876500001', displayName: 'QA User', username: 'qatester', privacyLastSeen: 'nobody', privacyAvatar: 'contacts', privacyAbout: 'contacts', privacyPhone: 'nobody', allowGroupAdds: 'contacts', privacyReadReceipts: false, discoverableByPhone: false, shareTypingIndicators: false, securityNotifications: true };
        else if (endpoint === '/privacy/devices') data = { devices: [] };
        else { status = 404; data = { code: 'NOT_FOUND', message: 'QA endpoint not configured' }; }
        await route.fulfill({ status, contentType: 'application/json', body: status === 204 ? '' : JSON.stringify(data), headers: { 'Access-Control-Allow-Origin': '*' } });
      });
      try {
        await page.goto(base, { waitUntil: 'networkidle', timeout: 90000 });
        await page.locator('flt-semantics-placeholder').evaluate(el => el.click());
        await enterText(page, 'Phone number', '9876500001');
        await page.getByRole('button', { name: 'Send OTP' }).click();
        await enterText(page, '6-digit code', '123456');
        await page.getByRole('button', { name: 'Verify', exact: true }).click();
        await page.getByRole('button', { name: 'New chat' }).click();
        await enterText(page, 'Exact username', 'asha');
        await page.getByRole('button', { name: 'Find user' }).click();
        await page.getByRole('button', { name: 'Start chat' }).waitFor();
        await capture(page, `lookup-${viewport.width}`);
        await page.getByRole('button', { name: 'Start chat' }).click();
        await page.getByRole('button', { name: 'Chat safety' }).click();
        await page.getByRole('menuitem', { name: 'Report user', exact: true }).click();
        await capture(page, `report-${viewport.width}`);
        await page.getByRole('button', { name: 'Submit report' }).click();
        await page.getByRole('button', { name: 'Chat safety' }).click();
        await page.getByRole('menuitem', { name: 'Block user', exact: true }).click();
        await page.getByRole('button', { name: 'Block', exact: true }).click();
        await page.getByRole('button', { name: 'Privacy', exact: true }).waitFor();
        assert.equal(blocked, true);
        assert.equal(reports, 1);
        await page.getByRole('button', { name: 'Privacy', exact: true }).click();
        await page.getByRole('button', { name: 'Blocked users', exact: true }).click();
        await page.getByRole('button', { name: 'Unblock', exact: true }).waitFor();
        await capture(page, `blocked-${viewport.width}`);
        await page.getByRole('button', { name: 'Unblock', exact: true }).click();
        await page.getByRole('button', { name: 'Unblock', exact: true }).last().click();
        await page.getByText('No blocked users', { exact: true }).waitFor();
        assert.equal(blocked, false);
        await page.getByRole('button', { name: 'Back', exact: true }).click();
        await page.getByRole('button', { name: 'Export account data', exact: true }).click();
        await page.getByText(/not password-protected/).waitFor();
        assert.equal(exports, 0, 'no personal data download without consent');
        await capture(page, `export-consent-${viewport.width}`);
        const downloadEvent = page.waitForEvent('download');
        await page.getByRole('button', { name: 'Export', exact: true }).click();
        const download = await downloadEvent;
        assert.match(download.suggestedFilename(), /^bharatchat-account-export-\d+\.zip$/);
        const downloaded = await JSZip.loadAsync(await fs.readFile(await download.path()));
        assert.equal(JSON.parse(await downloaded.file('profile.jsonl').async('text')).id, userId);
        assert.equal(exports, 1);
        await page.getByRole('button', { name: 'Delete account', exact: true }).click();
        const deleteButton = page.getByRole('button', { name: 'Delete', exact: true });
        assert.equal(await deleteButton.isEnabled(), false);
        await enterText(page, 'Type DELETE', 'delete');
        assert.equal(await deleteButton.isEnabled(), false);
        await enterText(page, 'Type DELETE', 'DELETE');
        await capture(page, `deletion-consent-${viewport.width}`);
        await deleteButton.click();
        await page.getByRole('button', { name: 'Send OTP', exact: true }).waitFor();
        assert.equal(deletions, 1);
        assert.deepEqual(errors, [], 'browser runtime errors');
        console.log(`PASS ${viewport.width}: lookup, report, block, unblock, export, delete`);
      } catch (error) {
        await capture(page, `failure-${viewport.width}`);
        console.error((await page.locator('body').ariaSnapshot()).slice(0, 8000));
        throw error;
      } finally { await context.close(); }
    }
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
