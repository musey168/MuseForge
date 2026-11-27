import test from 'node:test';
import assert from 'node:assert/strict';
import { loadApp, settle } from '../lib/load-app.mjs';

// The home page is the requirement list. The chat workbench is gone: there is
// no conversation frame, no composer and no assistant preference page.
test('the home page shows the requirement list, not a chat composer', async (t) => {
  const app = await loadApp();
  t.after(app.close);
  await settle(3);

  assert.equal(app.document.querySelector('#chatNextFrame'), null, 'the chat iframe must be gone');
  assert.equal(app.document.querySelector('#chatNextHost'), null, 'the chat host must be gone');
  assert.equal(app.document.querySelector('#collaborationView'), null, 'the chat view must be gone');
  assert.equal(app.document.querySelector('#assistantView'), null, 'the assistant preference view must be gone');
  assert.equal(app.document.querySelector('#reqCreateChat'), null, 'the create-and-chat button must be gone');
  assert.equal(app.document.querySelector('[data-view="collaboration"]'), null, 'the chat rail entry must be gone');

  assert.ok(app.document.querySelector('#issuesView'), 'the requirement list must still be there');
  assert.equal(app.evaluate('activeViewName()'), 'issues', 'the default route is the requirement list');
  assert.equal(app.evaluate('restoredView()'), 'issues');
});
