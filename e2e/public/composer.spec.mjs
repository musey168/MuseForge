import {test,expect} from '@playwright/test';
import {password as adminPassword, signInUnconfigured, appReady, switchProject, chat, openHome, openExecutorSetup} from './helpers.mjs';

// Credential UI must never enter a Playwright trace or screenshot artifact.
test.use({trace:'off',screenshot:'off'});

test('a user without model credentials can draft, configure credentials and return without losing text',async({page})=>{
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await signInUnconfigured(page);
  const current=await page.request.get('/api/auth/current').then(r=>r.json());
  const path=`/api/users/${current.user.id}/credentials`;
  // This is an inert value for a UI state transition. No model request is sent.
  const marker='e2e-placeholder-credential-not-for-model-use';
  const draft='保留这份尚未发送的需求草稿';
  let clearCredential;
  try {
    let input=await openHome(page);
    await input.fill(draft);
    const chooser=await openExecutorSetup(page);
    if(chooser)await page.getByRole('listbox',{name:'新增执行器',exact:true}).getByRole('option',{name:'ZCode',exact:true}).click();
    const wizard=page.locator('#credentialWizard');
    const pane=wizard.locator('.credential-pane:not(.is-leaving)');
    const next=wizard.locator('[data-wizard-next]');
    if(!chooser)await expect(wizard.locator('[data-wizard-step]')).toHaveText('2 / 3');
    await expect(pane.locator('input[name="secret"]')).toBeFocused();
    await expect(next).toBeDisabled();
    clearCredential=chooser?{agentCliCredentials:{zcode:{apiKeyClear:true}}}:{codeBuddyApiKeyClear:true};
    await pane.locator('input[name="secret"]').fill(marker);
    await expect(next).toBeEnabled();
    await next.click();
    if(chooser){
      await expect(wizard).toBeHidden();
    }else{
      await expect(wizard.locator('[data-wizard-step]')).toHaveText('3 / 3');
      await expect(pane).not.toContainText(marker);
      await next.click();
      await expect(wizard.locator('[data-wizard-done]')).toBeVisible({timeout:20_000});
      await wizard.locator('[data-wizard-done]').click();
    }
    const users=await page.request.get('/api/users').then(r=>r.json());
    const saved=users.find(u=>u.id===current.user.id);
    expect(chooser?saved.agentCliCredentials?.zcode?.apiKeyConfigured:saved.codeBuddyApiKeyConfigured).toBe(true);
    expect(JSON.stringify(saved)).not.toContain(marker);
    input=await openHome(page);
    await expect(input).toHaveValue(draft);
    await expect(chat(page).locator('.credential-notice')).toHaveCount(0);
    expect(errors).toEqual([]);
  } finally {
    if(clearCredential){
      const cleared=await page.request.put(path,{data:clearCredential});
      expect(cleared.ok()).toBe(true);
    }
  }
});

