package web

import (
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCredentialBrandAssetsEmbedded(t *testing.T) {
	for _, name := range []string{"gongfeng.svg", "codebuddy.svg", "codex.svg", "claude.svg"} {
		data, err := fs.ReadFile(Files, "images/credentials/"+name)
		if err != nil {
			t.Fatalf("credential logo %s is not embedded: %v", name, err)
		}
		if !strings.Contains(string(data), "<svg") {
			t.Fatalf("credential logo %s is not SVG", name)
		}
	}
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"https://github.com/settings/tokens", "https://www.codebuddy.ai/profile/keys", "https://copilot.tencent.com/profile/", "credential-description", "git-token-verify"} {
		if !strings.Contains(string(app), want) {
			t.Fatalf("credential UI is missing %q", want)
		}
	}
}

func TestAppJSParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping app.js parse check")
	}
	for _, name := range appBundleFiles(t) {
		out, err := exec.Command(node, "--check", name).CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed to parse: %v\n%s", name, err, out)
		}
	}
}

func TestMCPSetupInstructionsAreEmbedded(t *testing.T) {
	setup, err := os.ReadFile("mcp-setup.txt")
	if err != nil {
		t.Fatal(err)
	}
	text := string(setup)
	for _, want := range []string{"Cursor", "Claude Code", "CodeBuddy", "Codex", "API_TOKEN", "ENDPOINT", "tools/list", "失败排查", "汇报", "时间戳的备份"} {
		if !strings.Contains(text, want) {
			t.Fatalf("MCP setup instructions are missing %q", want)
		}
	}
	if strings.Contains(text, "AgentFlow") {
		t.Fatal("MCP setup instructions should name the product LoopForge, not AgentFlow")
	}
	embedSource, err := os.ReadFile("embed.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(embedSource), "mcp-setup.txt") {
		t.Fatal("mcp-setup.txt is not embedded in the web bundle")
	}
}

// The access-key page is gone: the personal build has one account and no
// external access keys.
func TestAccessKeyPageIsGone(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, unwanted := range []string{`id="guideView"`, `id="accessBody"`, `<h1>访问密钥</h1>`, `data-view="guide"`, `data-view="access"`} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("the access-key page is still in the interface: %q", unwanted)
		}
	}
	if strings.Contains(page, `data-access-tab=`) {
		t.Fatal("接入指南 collapsed into 访问密钥; MCP/能力/API tabs must not come back")
	}
	if strings.Contains(page, `data-access-tab="stdio"`) {
		t.Fatal("stdio bridge must not be advertised in the onboarding UI")
	}
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	for _, unwanted := range []string{"renderGuide", "renderAccessTokens", "loadAccessTokens", "createPersonalToken", "revokePersonalToken", "/api/auth/tokens"} {
		if strings.Contains(code, unwanted) {
			t.Fatalf("the access-key page is still wired into the app: %q", unwanted)
		}
	}
}

func TestRunnerBindCommandBootstrapsWithProtocolFallback(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, want := range []string{"https://", "http://", "bindHttpsOrigin", "bindHttpOrigin", "--server", "bindServerHost"} {
		if !strings.Contains(page, want) {
			t.Fatalf("runner bind command is missing %q", want)
		}
	}
}

func TestRenderUserBadgeScope(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping check")
	}
	script := `
const fs = require("fs");
const appCode = fs.readFileSync("` + appBundlePath(t) + `", "utf8");
global.window = { location: { href: "http://loopforge.example.com/", pathname: "/", search: "", hash: "", hostname: "loopforge.example.com" } };
global.sessionStorage = { _s: {}, getItem(k){return this._s[k]||null;}, setItem(k,v){this._s[k]=String(v);}, removeItem(k){delete this._s[k];} };
const makeEl = () => ({
  classList: { add(){}, remove(){} },
  title: "",
  textContent: "",
  innerHTML: "",
  onclick: null,
  querySelector(){ return makeEl(); },
  querySelectorAll(){ return []; }
});
const elements = {};
global.document = {
  querySelector(sel) {
    if (!elements[sel]) elements[sel] = makeEl();
    return elements[sel];
  },
  querySelectorAll() { return []; }
};
global.$ = s => global.document.querySelector(s);
global.escapeHTML = s => String(s);
global.state = { currentUser: { isGuest: true }, loginUrl: "" };
const fnStart = appCode.indexOf("function renderUserProfilePopover()");
const fnEnd = appCode.indexOf("function renderDashboard()");
// The theme preference lives in app.js, outside the sliced profile popover.
const themeCode = appCode.match(/function readThemePreference\(\)\{[\s\S]*?catch\{return'light'\}\}/);
eval(themeCode[0]);
eval(appCode.slice(fnStart, fnEnd));

// Guest mode
renderUserBadge();
sessionStorage.setItem("agentflow_auth_err", "exchange_failed");
renderUserBadge();

// Authenticated mode
state.currentUser = { isGuest: false, chineseName: "测试员工", loginName: "testuser", staffId: 1001, deptName: "技术架构部" };
renderUserBadge();
`
	cmd := exec.Command(node, "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("renderUserBadge failed: %v\n%s", err, out)
	}
}

func TestLocalhostDoesNotAutoSSORedirect(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping check")
	}
	script := `
const fs = require("fs");
const appCode = fs.readFileSync("` + appBundlePath(t) + `", "utf8");
const start = appCode.indexOf("function isLoopbackHost(");
const end = appCode.indexOf("async function boot()");
if (start < 0 || end < 0) process.exit(2);
eval(appCode.slice(start, end));
const loginUrl = "https://sso.example.com/signin";
const cases = [
  [{isGuest:true, hostname:"localhost", loginUrl}, false],
  [{isGuest:true, hostname:"127.0.0.1", loginUrl}, false],
  [{isGuest:true, hostname:"[::1]", loginUrl}, false],
  [{isGuest:true, hostname:"loopforge.example.com", loginUrl}, false],
  [{isGuest:true, hostname:"loopforge.example.com", loginUrl}, false],
  [{isGuest:true, hostname:"loopforge.example.com", loginUrl, bypassAutoAuth:true}, false],
  [{isGuest:true, hostname:"loopforge.example.com", loginUrl, authErr:"exchange_failed"}, false],
  [{isGuest:false, hostname:"loopforge.example.com", loginUrl}, false],
];
for (const [input, want] of cases) {
  const got = shouldAutoSSORedirect(input);
  if (got !== want) {
    console.error(JSON.stringify(input), "got", got, "want", want);
    process.exit(1);
  }
}
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("localhost SSO redirect check failed: %v\n%s", err, out)
	}
}

func TestCredentialWizardCSSKeepsToastsAnimating(t *testing.T) {
	cssBytes, err := os.ReadFile("platform.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssBytes)
	if strings.Contains(css, "body.credential-wizard-open") {
		t.Fatal("wizard pause must not hang off body: it restyles the whole page (and freezes toasts) on open")
	}
	if !strings.Contains(css, ".credential-wizard-open .hui-lamp::after,.credential-wizard-open .credential-spin{animation-play-state:paused}") {
		t.Fatal("wizard should pause only the looping credential animations under #personalSettingsContent")
	}
	code, err := os.ReadFile("app/workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(code), "$('#personalSettingsContent')?.classList.add('credential-wizard-open')") {
		t.Fatal("openCredentialWizard should put the pause class on #personalSettingsContent")
	}
	parts := strings.SplitN(css, ".credential-scrim::before{", 2)
	if len(parts) < 2 {
		t.Fatal("missing .credential-scrim::before rule")
	}
	before := strings.SplitN(parts[1], "}", 2)[0]
	if strings.Contains(before, "transition") {
		t.Fatal(".credential-scrim::before should not fade; a full-viewport opacity transition drops the entrance frame rate")
	}
}

func TestCredentialListAndWizard(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping credential UI check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const user = {id: 1, gitHubCredentialConfigured: true, codexApiKeyConfigured: true, codexBaseUrl: 'https://proxy.example.com/v1'};
const state = {currentUser: {id: 1}, data: {users: [user]}};
const userById = id => state.data.users.find(u => u.id === id);
const escapeHTML = v => String(v), toast = () => {};
const calls = [];
const api = async (path, options = {}) => {
  calls.push({path, method: options.method || 'GET', body: options.body && JSON.parse(options.body)});
  return path.endsWith('/credentials') ? {...user, ...state.nextUser} : {valid: false, error: '令牌无效或已过期'};
};
const $ = () => null, setTimeout = () => {}, location = {origin: 'http://localhost'};
const document = {activeElement: null, querySelectorAll: () => [], addEventListener: () => {}};
eval(code.slice(code.indexOf('const CREDENTIAL_GROUPS='), code.indexOf('function renderUserProfilePopover')) +
  ';globalThis.cred={CREDENTIAL_KINDS,CREDENTIAL_GROUPS,kind:credentialKind,get wizard(){return credentialWizard},set wizard(v){credentialWizard=v}};');
const count = (html, needle) => html.split(needle).length - 1;

// Groups list credentials in a fixed order.
assert.deepEqual(cred.CREDENTIAL_GROUPS.map(g => g.id), ['platform', 'model']);
assert.deepEqual(cred.CREDENTIAL_KINDS.filter(k => k.group === 'platform').map(k => k.key), ['gitToken', 'gitSshKey', 'gitHttpCredentials']);
assert.deepEqual(cred.CREDENTIAL_KINDS.filter(k => k.group === 'model').map(k => k.key), ['codeBuddyApiKey', 'codexApiKey', 'claudeCodeApiKey', 'agentCli.opencode', 'agentCli.cursor', 'agentCli.agy', 'agentCli.kimi', 'agentCli.pi', 'agentCli.dsh', 'agentCli.zcode']);

// Existing credentials are rows. The add controls own the available types.
const html = credentialPanelHTML(user);
assert.equal(count(html, 'data-credential-row='), 2);
assert.equal(count(html, 'data-credential-add-menu='), 2);
assert.match(html, /新增 Git 配置.*<option value="gitToken"[^>]*>GitHub<\/option>/s);
assert.match(html, /新增执行器.*<option value="claudeCodeApiKey"[^>]*>Claude Code<\/option>/s);
assert.ok(!html.includes('<option value="codexApiKey"'));
assert.ok(html.includes('data-credential-row="codexApiKey"'));
assert.equal(count(html, 'credential-editor-form'), 2);
assert.equal(count(html, 'data-credential-act="delete"'), 2);
assert.equal(count(credentialPanelHTML({}), 'data-credential-row='), 0);
// The personal settings carry no assistant preferences: the panel is the
// credential list alone.
assert.ok(!html.includes('data-credential-defaults'));
assert.ok(!html.includes('AI 默认设置'));
assert.ok(credentialRowHTML(cred.kind('gitToken'), user).includes('git-token-verify'), 'Git row keeps 重新校验');

// Each kind saves and deletes only its own fields.
assert.deepEqual(credentialSavePayload(cred.kind('gitToken'), {secret: ' ghp_1 '}), {gitToken: 'ghp_1'});
const sshRow = credentialRowHTML(cred.kind('gitSshKey'), {...user, gitSshKeyConfigured: true});
assert.match(sshRow, /aria-haspopup="dialog">管理密钥/);
assert.ok(!sshRow.includes('credential-editor-form'));
const httpsRow = credentialRowHTML(cred.kind('gitHttpCredentials'), {...user, gitHttpCredentialsConfigured: true});
assert.match(httpsRow, /aria-haspopup="dialog">管理令牌/);
assert.ok(!httpsRow.includes('credential-editor-form'));
assert.deepEqual(credentialSavePayload(cred.kind('codeBuddyApiKey'), {secret: '', extra: 'ioa'}), {codeBuddyApiKey: '', codeBuddyInternetEnvironment: 'ioa'});
assert.deepEqual(credentialSavePayload(cred.kind('codeBuddyApiKey'), {secret: '', extra: ''}), {codeBuddyApiKey: '', codeBuddyInternetEnvironment: ''});
assert.deepEqual(credentialSavePayload(cred.kind('codexApiKey'), {secret: 'sk-1'}), {codexApiKey: 'sk-1'});
assert.deepEqual(credentialSavePayload(cred.kind('claudeCodeApiKey'), {secret: '', extra: '', models: [{id: ' hy4 ', name: '混元 4'}, {id: 'glm-5', name: 'glm-5'}]}), {claudeCodeApiKey: '', claudeCodeBaseUrl: '', claudeCodeModels: [{id: 'hy4', name: '混元 4'}, {id: 'glm-5'}]});
assert.deepEqual(credentialSavePayload(cred.kind('codexApiKey'), {secret: '', extra: '', models: []}), {codexApiKey: '', codexBaseUrl: '', codexModels: []}, '恢复默认 sends an empty catalog');
assert.deepEqual(credentialSavePayload(cred.kind('agentCli.opencode'), {secret: 'k', extra: 'https://gw.example/v1', protocol: 'messages', models: [{id: 'a', name: 'A'}]}), {agentCliCredentials: {opencode: {apiKey: 'k', baseUrl: 'https://gw.example/v1', protocol: 'messages', models: [{id: 'a', name: 'A'}]}}});
assert.equal(cred.kind('agentCli.cursor').models.engine, 'cursor', 'Cursor keeps a per-account model catalog');
assert.equal(cred.kind('agentCli.zcode').models.engine, 'zcode');
assert.ok(credentialSubline(cred.kind('codexApiKey'), {codexApiKeyConfigured: true, codexModels: [{id: 'hy4'}, {id: 'gpt-5.5'}]}).includes('2 个自定义模型'));
assert.deepEqual(credentialClearPayload(cred.kind('codexApiKey')), {codexApiKeyClear: true, codexBaseUrl: '', codexModels: []});
assert.deepEqual(credentialClearPayload(cred.kind('claudeCodeApiKey')), {claudeCodeApiKeyClear: true, claudeCodeBaseUrl: '', claudeCodeModels: []});
assert.deepEqual(credentialClearPayload(cred.kind('agentCli.opencode')), {agentCliCredentials: {opencode: {apiKeyClear: true, baseUrl: '', protocol: '', models: []}}});
assert.deepEqual(credentialClearPayload(cred.kind('agentCli.cursor')), {agentCliCredentials: {cursor: {apiKeyClear: true, baseUrl: '', protocol: '', models: []}}}, 'deleting the Cursor key drops the model catalog of that account');
const catalogUser = {codexApiKeyConfigured: true, codexBaseUrl: 'https://gw.example/v1', codexModels: [{id: 'hy4', name: '混元 4'}, {id: 'gpt-5.5'}]};
const catalogHTML = credentialCatalogHTML(cred.kind('codexApiKey'), catalogUser);
assert.equal(count(catalogHTML, 'data-catalog-row'), 2);
assert.ok(catalogHTML.indexOf('value="hy4"') < catalogHTML.indexOf('value="gpt-5.5"') && catalogHTML.includes('value="混元 4"'));
assert.ok(catalogHTML.includes('data-catalog-act="probe"') && catalogHTML.includes('data-catalog-act="add"') && catalogHTML.includes('data-catalog-grip'));
assert.ok(catalogHTML.includes('data-catalog-source="https://gw.example/v1"'));
assert.equal(credentialCatalogSource(' https://gw.example/v1/ '), credentialCatalogSource('https://gw.example/v1'));
assert.ok(catalogHTML.includes('data-catalog-empty hidden') && catalogHTML.includes('2 个模型') && catalogHTML.includes('data-catalog-body>') && !catalogHTML.includes('data-catalog-body hidden'));
const emptyCatalog = credentialCatalogHTML(cred.kind('claudeCodeApiKey'), {claudeCodeBaseUrl: 'https://gw.example'});
assert.ok(emptyCatalog.includes('跟随服务发布的模型') && emptyCatalog.includes('/v1/models') && emptyCatalog.includes('data-catalog-foot hidden') && emptyCatalog.includes('未自定义'));
const officialCatalog = credentialCatalogHTML(cred.kind('codexApiKey'), {codexModels: [{id: 'hy4'}]});
assert.ok(officialCatalog.includes('value="hy4"') && officialCatalog.includes('data-catalog-act="probe"') && officialCatalog.includes('改回使用自带的模型') && !officialCatalog.includes('data-catalog-body hidden'), 'an official key keeps its custom catalog editable');
const officialEmpty = credentialCatalogHTML(cred.kind('codexApiKey'), {});
assert.ok(officialEmpty.includes('使用 Codex 自带的模型') && officialEmpty.includes('data-catalog-act="add"') && !officialEmpty.includes('data-catalog-body hidden'));
const agyKind = cred.kind('agentCli.agy');
const geminiOfficial = credentialCatalogHTML(agyKind, {agentCliCredentials: {agy: {protocol: 'gemini', models: [{id: 'gemini-4-neon-high'}]}}});
assert.match(geminiOfficial, /data-catalog-mode="official"/);
assert.ok(geminiOfficial.includes('value="gemini-4-neon-high"') && geminiOfficial.includes('使用 Antigravity 自带的模型') && !geminiOfficial.includes('data-catalog-body hidden'), 'a saved catalog stays editable on the official endpoint');
const geminiGateway = credentialCatalogHTML(agyKind, {agentCliCredentials: {agy: {baseUrl: 'https://gw.example/v1', protocol: 'gemini'}}});
assert.match(geminiGateway, /data-catalog-mode="remote"/);
assert.ok(geminiGateway.includes('跟随服务发布的模型') && geminiGateway.includes('data-catalog-endpoint>/v1beta/models') && geminiGateway.includes('data-catalog-act="probe"'), 'a Gemini gateway uses its own model listing');
assert.ok(credentialEditorHTML(cred.kind('codexApiKey'), catalogUser).includes('data-credential-catalog') && credentialEditorHTML(cred.kind('codexApiKey'), catalogUser).includes('has-catalog'));
assert.ok(credentialEditorHTML(cred.kind('agentCli.cursor'), {agentCliCredentials: {cursor: {apiKeyConfigured: true}}}).includes('data-credential-catalog'), 'Cursor can fetch the models of its account');
assert.equal(credentialCatalogKey([{id: 'a', name: 'a'}, {id: '', name: ''}]), credentialCatalogKey([{id: 'a'}]));
assert.equal(credentialCatalogError([{id: 'a'}, {id: 'b', name: 'B'}]), null);
assert.deepEqual(credentialCatalogError([{id: 'a'}, {id: '', name: 'x'}]), {index: 1, field: 'id', error: '模型 ID 不能为空'});
assert.ok(credentialCatalogError([{id: 'a b'}]).error.includes('空格'));
assert.ok(credentialCatalogError([{id: 'a'}, {id: 'a'}]).error.includes('重复'));
assert.equal(credentialCatalogError([{id: 'a', name: 'n'.repeat(65)}]).field, 'name');
assert.deepEqual(credentialClearPayload(cred.kind('codeBuddyApiKey')), {codeBuddyApiKeyClear: true, codeBuddyModels: []});
assert.deepEqual(credentialClearPayload(cred.kind('gitToken')), {gitTokenClear: true});
assert.equal(credentialSecretError(cred.kind('gitToken'), 'github_pat_1'), '');
assert.ok(credentialExtraError(cred.kind('codexApiKey'), 'ftp://proxy'));
assert.equal(credentialExtraError(cred.kind('codexApiKey'), ''), '');

// CodeBuddy: the network environment is a fixed choice and the key link follows it.
// The CLI gets the stored text as-is, so only exact values are recognized; aliases only guess what an old value meant.
assert.deepEqual(['', ' internal ', 'ioa'].map(v => codeBuddyEnvironment(v)?.value), ['', 'internal', 'ioa']);
assert.deepEqual(['public', 'INTL', 'iOA', '国内站', 'china', 'cn', 'private-net'].map(v => codeBuddyEnvironment(v)), [null, null, null, null, null, null, null]);
assert.deepEqual(['', 'public', 'INTL', ' iOA ', 'internal', '国内站', 'china'].map(v => codeBuddyEnvironmentIntent(v)?.value), ['', '', '', 'ioa', 'internal', 'internal', 'internal']);
assert.equal(codeBuddyEnvironmentIntent('private-net'), null);
assert.deepEqual(['', 'internal', 'china', 'iOA', 'private-net'].map(codeBuddyEnvironmentLegacy), [false, false, true, true, true]);
assert.equal(codeBuddyKeyURL(''), 'https://www.codebuddy.ai/profile/keys');
assert.equal(codeBuddyKeyURL('internal'), 'https://copilot.tencent.com/profile/');
assert.equal(codeBuddyKeyURL('ioa'), 'https://tencent.sso.copilot.tencent.com/profile/keys');
assert.equal(codeBuddyKeyURL('typo'), 'https://www.codebuddy.ai/profile/keys');
const cbKind = cred.kind('codeBuddyApiKey');
assert.equal(credentialExtraValue(cbKind, 'iOA'), 'ioa');
assert.equal(credentialExtraValue(cbKind, 'private-net'), '', 'an unknown value shows as 国际站, which is what the CLI uses');
assert.equal(credentialExtraValue(cred.kind('codexApiKey'), 'https://p.example'), 'https://p.example');
const cbEditor = credentialEditorHTML(cbKind, {codeBuddyApiKeyConfigured: true, codeBuddyInternetEnvironment: 'internal'});
assert.ok(cbEditor.includes('<select name="extra">') && !cbEditor.includes('<input name="extra"'));
assert.ok(cbEditor.includes('data-original="internal"') && cbEditor.includes('<option value="internal" selected>'));
assert.ok(cbEditor.includes('href="https://copilot.tencent.com/profile/"') && cbEditor.includes('前往国内站获取 API Key'));
const legacyEditor = credentialEditorHTML(cbKind, {codeBuddyApiKeyConfigured: true, codeBuddyInternetEnvironment: 'private-net'});
assert.ok(legacyEditor.includes('data-original=""') && legacyEditor.includes('<option value="" selected>') && legacyEditor.includes('「private-net」无法识别'));
assert.ok(legacyEditor.includes('data-legacy="true"') && !legacyEditor.includes('data-credential-act="save" disabled'), 'a legacy value can be saved as 国际站 without retyping the key');
assert.ok(!cbEditor.includes('data-legacy') && cbEditor.includes('data-credential-act="save" disabled'));
// An old alias preselects what it meant and stays savable: the CLI would otherwise keep connecting to 国际站.
const chinaEditor = credentialEditorHTML(cbKind, {codeBuddyApiKeyConfigured: true, codeBuddyInternetEnvironment: 'china'});
assert.ok(chinaEditor.includes('data-original="internal"') && chinaEditor.includes('<option value="internal" selected>') && chinaEditor.includes('「china」无法识别'));
assert.ok(chinaEditor.includes('data-legacy="true"') && !chinaEditor.includes('data-credential-act="save" disabled'));
assert.ok(credentialSubline(cbKind, {codeBuddyApiKeyConfigured: true, codeBuddyInternetEnvironment: 'china'}).includes('「china」无法识别'));
assert.ok(credentialSubline(cbKind, {codeBuddyApiKeyConfigured: true, codeBuddyInternetEnvironment: 'ioa'}).includes('>iOA<'));
assert.ok(credentialSubline(cbKind, {codeBuddyApiKeyConfigured: true}).includes('>国际站<'));
assert.ok(credentialSubline(cbKind, {codeBuddyApiKeyConfigured: true, codeBuddyInternetEnvironment: 'private-net'}).includes('无法识别'));
// Third-party keys are flagged but never blocked.
assert.equal(credentialSecretWarning(cbKind, 'ck_abc'), '');
assert.match(credentialSecretWarning(cbKind, 'sk-abc'), /第三方/);
assert.match(credentialSecretWarning(cbKind, 'abc'), /ck_/);
assert.equal(credentialSecretWarning(cred.kind('codexApiKey'), 'sk-abc'), '');
assert.equal(credentialSecretError(cbKind, 'sk-abc'), '');

// Delete swaps the action bar for the confirm bar without sending anything.
const pane = () => ({classList: {on: new Set(), toggle(name, on) { on ? this.on.add(name) : this.on.delete(name) }}});
const foot = pane(), confirm = pane();
const row = {dataset: {credentialRow: 'codexApiKey'}, querySelector: s => s.includes('foot') ? foot : s.includes('confirm') ? confirm : null};
credentialRowAction('delete', row);
assert.ok(foot.classList.on.has('is-off') && !confirm.classList.on.has('is-off') && foot.inert === true);
credentialRowAction('undelete', row);
assert.ok(!foot.classList.on.has('is-off') && confirm.classList.on.has('is-off'));
assert.equal(calls.length, 0);

(async () => {
  await deleteCredential('codexApiKey');
  assert.deepEqual(calls.at(-1).body, {codexApiKeyClear: true, codexBaseUrl: '', codexModels: []});

  // 添加凭证 starts at step 1 with configured kinds disabled.
  cred.wizard = {step: 1, kind: null, direct: false, secret: '', extra: '', guide: {}, error: ''};
  const one = credentialWizardStepOneHTML(user);
  assert.ok(one.includes('data-wizard-kind="gitToken" disabled') && one.includes('data-wizard-kind="codexApiKey" disabled'));
  assert.ok(!one.includes('data-wizard-kind="codeBuddyApiKey" disabled'));
  assert.ok(credentialWizardFootHTML().includes('data-wizard-next disabled'));
  // Going back to step 1 and picking another kind drops what was typed for the previous one.
  cred.wizard = {step: 1, kind: 'codeBuddyApiKey', direct: false, secret: 'cb_CARRY', extra: '', guide: {0: true}, error: 'x'};
  selectCredentialWizardKind('claudeCodeApiKey');
  assert.deepEqual([cred.wizard.kind, cred.wizard.secret, cred.wizard.guide, cred.wizard.error], ['claudeCodeApiKey', '', {}, '']);
  cred.wizard.secret = 'sk-KEEP';
  selectCredentialWizardKind('claudeCodeApiKey');
  assert.equal(cred.wizard.secret, 'sk-KEEP', 're-selecting the same kind keeps the input');
  assert.ok(!credentialFieldHTML({label: 'k', name: 'secret', value: 'sk-LEAK'}).includes('sk-LEAK'), 'secrets are never written into markup');
  assert.ok(credentialFieldHTML({label: 'u', name: 'extra', type: 'url', value: 'https://p.example'}).includes('value="https://p.example"'));
  // Choosing a provider opens its own form with a direct save action.
  cred.wizard = {step: 2, kind: 'codeBuddyApiKey', direct: true, secret: '', extra: 'ioa', guide: {}, error: ''};
  const cbStepTwo = credentialWizardStepTwoHTML(cred.kind('codeBuddyApiKey'));
  assert.ok(cbStepTwo.indexOf('<select name="extra">') < cbStepTwo.indexOf('name="secret"'), 'the network environment is picked before the key');
  assert.ok(cbStepTwo.includes('href="https://tencent.sso.copilot.tencent.com/profile/keys" target="_blank" rel="noopener noreferrer" data-wizard-link="1" data-environment-link>前往iOA获取 API Key'));
  assert.ok(!cbStepTwo.includes('https://www.codebuddy.ai/"'));
  cred.wizard.extra = '';
  assert.ok(credentialWizardStepTwoHTML(cred.kind('codeBuddyApiKey')).includes('新增 CodeBuddy'));
  assert.ok(credentialWizardFootHTML().includes('保存执行器'));
  assert.ok(!credentialWizardFootHTML().includes('data-wizard-back'));
  assert.ok(credentialWizardFootHTML().includes('data-wizard-next disabled'), 'an empty key blocks next');
  cred.wizard.direct = false;
  assert.ok(credentialWizardFootHTML().includes('data-wizard-back'));
  // Step 3 names the environment and repeats the third-party key warning without echoing the key.
  cred.wizard = {step: 3, kind: 'codeBuddyApiKey', direct: true, secret: 'sk-third', extra: 'internal', guide: {}, error: ''};
  const cbStepThree = credentialWizardStepThreeHTML(cred.kind('codeBuddyApiKey'));
  assert.ok(cbStepThree.includes('国内站 · copilot.tencent.com') && cbStepThree.includes('credential-vline is-warn') && !cbStepThree.includes('sk-third'));
  // Step 3 never echoes the secret.
  cred.wizard = {step: 3, kind: 'codeBuddyApiKey', direct: true, secret: 'cb_SECRET', extra: '', guide: {}, error: ''};
  assert.ok(!credentialWizardStepThreeHTML(cred.kind('codeBuddyApiKey')).includes('SECRET'));
  await advanceCredentialWizard();
  assert.deepEqual(calls.at(-1), {path: '/api/users/1/credentials', method: 'PUT', body: {codeBuddyApiKey: 'cb_SECRET', codeBuddyInternetEnvironment: ''}});
  assert.ok(cred.wizard.done && cred.wizard.secret === '');
  // A failed Git token verification in the wizard saves nothing.
  const before = calls.filter(c => c.method === 'PUT').length;
  cred.wizard = {step: 3, kind: 'gitToken', direct: true, secret: 'bad', extra: '', guide: {}, error: ''};
  await advanceCredentialWizard();
  assert.equal(calls.filter(c => c.method === 'PUT').length, before);
  assert.ok(cred.wizard.error.includes('令牌无效或已过期') && cred.wizard.error.includes('未保存'));
  assert.ok(!cred.wizard.done);
  assert.ok(!code.includes('通常包含 /v1'));
  assert.ok(!code.includes('不含 /v1/messages'));
})().catch(error => { console.error(error); process.exit(1); });
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("credential list and wizard failed: %v\n%s", err, out)
	}
}

func TestGotoCredentialBranches(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping credential navigation check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
let view = 'settings', now = 0, settingsRouteTransition = null;
const switchView = () => {}, activeViewName = () => view;
const performance = {now: () => (now += 100)};
const requestAnimationFrame = fn => fn();
let configured = {gitToken: true}, visible = true;
const panel = {
  getClientRects: () => visible ? [{}] : [],
  querySelector(selector) {
    const name = /data-credential-row="(\w+)"/.exec(selector)[1];
    return {classList: {contains: cls => cls === 'is-configured' && !!configured[name]}};
  }
};
const document = {querySelector: selector => selector === '#personalSettingsContent #credentialPanel' ? panel : null};
const opened = [], revealed = [];
const openCredentialWizard = key => opened.push(key), revealCredentialRow = key => revealed.push(key);
eval(code.slice(code.indexOf('let credentialFocusRequest=0;'), code.indexOf('const CREDENTIAL_RULES=')));
gotoCredential('gitToken');
assert.deepEqual(revealed, ['gitToken'], 'configured credentials expand in place');
gotoCredential('codeBuddyApiKey');
assert.deepEqual(opened, ['codeBuddyApiKey'], 'missing credentials open the wizard on step 2');
visible = false;
gotoCredential('codexApiKey');
view = 'issues'; visible = true;
gotoCredential('codexApiKey');
assert.deepEqual(opened, ['codeBuddyApiKey'], 'hidden panel or a different route does nothing');
assert.deepEqual(revealed, ['gitToken']);
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("gotoCredential branches failed: %v\n%s", err, out)
	}
}

func TestCredentialRowActivation(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping credential row activation check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const opened = [], toggled = [], acted = [];
const openCredentialWizard = key => opened.push(key), toggleCredentialRow = key => toggled.push(key);
const credentialRowAction = act => acted.push(act), credentialCatalogKeydown = () => {};
const credentialDialog = () => null;
eval(code.slice(code.indexOf('function activateCredentialRow('), code.indexOf('// gotoCredential lands here')));
const panel = {};
bindCredentialPanel(panel);
const el = matches => ({closest: s => matches[s] ?? null});
const rowOf = (key, configured) => ({dataset: {credentialRow: key}, classList: {contains: c => c === 'is-configured' && configured}});
const main = row => {
  const node = {closest: s => s === '[data-credential-row]' ? row : s === '.credential-row-main' || s === '.credential-row-main[role="button"]' ? node : null};
  return node;
};
const click = target => panel.onclick({target, stopPropagation() {}});
const key = (target, k) => { let prevented = false; panel.onkeydown({target, key: k, preventDefault() { prevented = true }}); return prevented; };

const empty = rowOf('codeBuddyApiKey', false), done = rowOf('gitToken', true);
click(main(empty));
assert.deepEqual(opened, ['codeBuddyApiKey'], 'clicking an unconfigured row opens the wizard');
click(main(done));
assert.deepEqual(toggled, ['gitToken'], 'clicking a configured row toggles its editor');
const emptyMain = main(empty);
assert.ok(key(emptyMain, 'Enter') && key(emptyMain, ' '));
assert.deepEqual(opened, ['codeBuddyApiKey', 'codeBuddyApiKey', 'codeBuddyApiKey'], 'Enter and Space open the wizard too');
key(main(done), 'Enter');
assert.deepEqual(toggled, ['gitToken', 'gitToken']);
// 去配置 runs its own action once and never falls through to the row.
const setup = {disabled: false, dataset: {credentialAct: 'setup'}};
click(el({'[data-credential-row]': empty, '[data-credential-act]': setup, '.credential-row-main': {}}));
assert.deepEqual(acted, ['setup']);
assert.equal(opened.length, 3);
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("credential row activation failed: %v\n%s", err, out)
	}
}

func TestRepositoryFormUsesSingleURL(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`name="repoUrl"`,
		`集成分支（可选）`,
		`检查多个 LOOP 执行后工作分支之间是否存在合并冲突`,
	} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("repository form missing %q", want)
		}
	}
	for _, not := range []string{
		`工蜂 Web URL`,
		`name="integrateBranch" value="agentflow/integration" required`,
	} {
		if strings.Contains(string(html), not) {
			t.Fatalf("repository form still has %q", not)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping repository URL helper check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
const state = { data: {} };
eval(require('node:fs').readFileSync('app/git-ssh.js', 'utf8'));
assert.equal(globalThis.LoopForgeGongfengRepository('Tencent/LoopForge', 'git.review.test'), null);
eval(code.slice(code.indexOf('function parseGongfengRepoURL(raw){'), code.indexOf('function fillRepositoryFromLink(')));
const web = parseGongfengRepoURL('https://git.example.com/example/agent-flow');
assert.equal(web.project, 'example/agent-flow');
assert.equal(web.cloneUrl, 'https://git.example.com/example/agent-flow.git');
const ssh = parseGongfengRepoURL('git@git.example.com:example/agent-flow.git');
assert.equal(ssh.provider, 'ssh');
assert.equal(ssh.cloneUrl, 'git@git.example.com:example/agent-flow.git');
const cloned = parseGongfengRepoURL('git clone git@git.example.com:example/agent-flow.git');
assert.equal(cloned.provider, 'ssh');
assert.equal(cloned.cloneUrl, 'git@git.example.com:example/agent-flow.git');
assert.equal(web.name, 'agent-flow');
const http = parseGongfengRepoURL('http://git.example.com/example/agent-flow.git');
assert.equal(http.provider, 'gongfeng');
assert.equal(http.project, 'example/agent-flow');
assert.equal(http.cloneUrl, 'https://git.example.com/example/agent-flow.git');
assert.equal(http.name, 'agent-flow');
const plain = parseGongfengRepoURL('git clone http://git.example.com/example/agent-flow.git');
assert.equal(plain.provider, 'http');
assert.equal(plain.cloneUrl, 'http://git.example.com/example/agent-flow.git');
assert.equal(parseGongfengRepoURL(globalThis.LoopForgeGitRepositoryReference(plain)).provider, 'http');
assert.equal(ssh.name, 'agent-flow');
const html = require('node:fs').readFileSync('index.html', 'utf8');
assert.ok(html.includes('集成分支用于检查多个 LOOP 执行后工作分支之间是否存在合并冲突'));
assert.ok(/<input[^>]*name="integrateBranch"[^>]*>/.test(html));
assert.ok(!/<input[^>]*name="integrateBranch"[^>]*value="agentflow\/integration"/.test(html));
assert.ok(!code.includes("integrateBranch:'agentflow/integration'"));
assert.ok(code.includes('项目名称'));
assert.ok(code.includes('data-repo-edit'));
assert.ok(code.includes('data-repo-delete'));
assert.ok(code.includes('target="_blank"'));
assert.ok(!code.includes('data-edit-col'));
assert.ok(!code.includes('data-save-col'));
assert.ok(!code.includes('（待保存）'));
assert.ok(!code.includes('function repoAdminRowHTML('));
assert.ok(!code.includes('function captureRepoRows('));
assert.ok(!code.includes('function persistRepoRow('));
assert.ok(code.includes("integrateBranch:f.elements.integrateBranch.value.trim()"));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("repository URL helper check failed: %v\n%s", err, out)
	}
}

func TestRequirementDirectCreateUI(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	if !strings.Contains(page, `nav-label">项目配置`) {
		t.Fatal("sidebar must label the projects page 项目配置")
	}
	if !strings.Contains(page, `<script src="/app/composer.js"></script>`) {
		t.Fatal("index.html must load the requirement composer script")
	}
	if !strings.Contains(page, `href="/app/requirement-wizard.css`) {
		t.Fatal("index.html must load the requirement wizard stylesheet")
	}
	reqStart := strings.Index(page, `id="requirementModal"`)
	if reqStart < 0 {
		t.Fatal("index.html is missing the requirement modal")
	}
	// The modal ends at its own closing tag; the TAPD import modal it used to
	// sit before went with the tracker.
	composer := page[reqStart:]
	if end := strings.Index(composer, `<div id="syncModal"`); end > 0 {
		composer = composer[:end]
	}
	for _, want := range []string{
		`id="reqStepDescribe"`, `id="reqFooter"`,
		`id="chatInput"`, `placeholder="要做什么？例如：`, `id="reqRepoChip"`, `id="reqRepoPopover"`, `id="reqRepository"`, `id="reqBaseBranch"`,
		`id="reqCreate"`, `创建需求`, `先写一句需求`,
	} {
		if !strings.Contains(composer, want) {
			t.Fatalf("requirement composer UI is missing %q", want)
		}
	}
	for _, gone := range []string{
		`id="chatSend"`, `id="chatCreateDirect"`, `id="chatCreateLocal"`, `id="chatCreateTAPD"`, `id="reqDestinations"`, `id="reqSetupRow"`, `id="reqContextLine"`, `id="chatCreateGongfeng"`, `需求套餐`, `id="reqSopRow"`, `id="reqPlaybooks"`, `问清再建单`, `再建模`, `已全部选择`,
		`id="reqSteps"`, `data-req-step=`, `id="reqStepClarify"`, `id="reqStepConfirm"`, `id="reqClarifyModelPicker"`, `id="reqThinking"`, `id="reqClarifyPanel"`, `id="reqAnswerHistory"`,
		`id="reqSkip"`, `跳过澄清`, `id="reqNext"`, `开始澄清`, `id="reqGoCreate"`, `id="reqBack"`, `id="chatStop"`, `id="reqReclarify"`,
	} {
		if strings.Contains(composer, gone) {
			t.Fatalf("requirement composer must not keep the clarification step %q", gone)
		}
	}
	createIdx := strings.Index(composer, `id="reqCreate"`)
	if createIdx < 0 {
		t.Fatal("the requirement composer needs a 创建需求 button")
	}
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	if strings.Contains(code, "直接创建会同步到 TAPD") {
		t.Fatal("直接创建 must not auto-sync TAPD; user chooses dest")
	}
	if strings.Contains(code, "patchCachedRun(runId,{pauseRequested:false,status:'running'})") {
		t.Fatal("the console must not force a run back to running")
	}
	if strings.Contains(code, "function runRetryNeedsContext(run)") {
		t.Fatal("the console must not offer a run retry")
	}
	for _, want := range []string{
		`function renderReqFooter(`, `function renderReqRepoChip(`, `persistRequirementComposer`, `agentflow.requirementComposer`, `未配置`, `if(modal.id==='requirementModal')return`,
		`assigneeUserId:Number(state.currentUser?.id)||0`,
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("requirement composer behavior is missing %q", want)
		}
	}
	composerJS, err := os.ReadFile("app/composer.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{`查看完整度详情`, `准入线`, `没有固定轮数`} {
		if strings.Contains(composer, gone) || strings.Contains(string(composerJS), gone) {
			t.Fatalf("requirement composer must not keep %q", gone)
		}
	}
	for _, gone := range []string{
		`composerAssessmentView`, `invalidateRequirementAnalysis`, `composerCreateReady`, `setComposerRetryMode`, `composerClarifyRetry`, `state.reqCreateMode`, `createIssueDirect`, `setChatCreateEnabled`, `syncReqComposerLayout`,
		`function goReqStep(`, `function startReqClarify(`, `function sendChatClarify(`, `function pickReqOption(`, `function submitReqAnswers(`, `function reclarifyRequirement(`, `'/api/chat-clarify'`,
	} {
		if strings.Contains(code, gone) {
			t.Fatalf("requirement composer must not keep %q", gone)
		}
	}
	createStart := strings.Index(code, "async function createIssueFromChat(")
	if createStart < 0 {
		t.Fatal("createIssueFromChat must exist")
	}
	createEnd := strings.Index(code[createStart+1:], "\nasync function ")
	if next := strings.Index(code[createStart+1:], "\nfunction "); next >= 0 && (createEnd < 0 || next < createEnd) {
		createEnd = next
	}
	if createEnd < 0 {
		t.Fatal("createIssueFromChat must be followed by another function")
	}
	if strings.Contains(code[createStart:createStart+1+createEnd], "afConfirm") {
		t.Fatal("creating a requirement must be one click, without afConfirm")
	}
}

func TestDeploymentDurationDashboardIsNotShipped(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{`data-view="deployments"`, `id="deploymentsView"`, `/deployment-dashboard.css`, `data-obs-tab="skillhub"`} {
		if strings.Contains(string(html), unwanted) {
			t.Fatalf("index.html still ships removed dashboard markup %q", unwanted)
		}
	}
	for _, unwanted := range []string{"renderDeploymentDashboard", "/api/deployment-metrics", "refreshSkillhubEfficiency"} {
		if strings.Contains(string(app), unwanted) {
			t.Fatalf("app bundle still ships removed dashboard behavior %q", unwanted)
		}
	}
	if _, err := os.Stat("deployment-dashboard.css"); err == nil {
		t.Fatal("deployment-dashboard.css should not be shipped")
	}
}

func TestPlaybooksPageRemoved(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, leftover := range []string{`data-view="playbooks"`, `id="playbooksView"`, `id="sopPlaybooks"`, `需求套餐`} {
		if strings.Contains(page, leftover) {
			t.Fatalf("需求套餐 page must be removed, still has %q", leftover)
		}
	}
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	for _, leftover := range []string{`function renderSOPPlaybooks`, `function addProjectPlaybook`, `需求套餐`} {
		if strings.Contains(code, leftover) {
			t.Fatalf("playbook editor must be removed from app.js, still has %q", leftover)
		}
	}
}

func TestUsageConsoleDropsRunQualitySurfaces(t *testing.T) {
	src, err := os.ReadFile("app/observability.js")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)
	// The usage console reports externally reported usage only. Every run-quality
	// surface (success rate, P95, tool-call rate, interventions, automation rate,
	// reworks, member and project tables, delivery funnel, activity cards and
	// the week-hour heatmap) must be gone from the interface and the code.
	for _, unwanted := range []string{
		"successRate", "p95", "toolSuccessRate", "humanInterventions", "automationRate",
		"rework", "runTraces", "obsScopedView", "weekHour", "funnel", "owners",
		"delivery", "vibe", "memberTable", "userTable", "导出报告",
	} {
		if strings.Contains(code, unwanted) {
			t.Fatalf("the usage console still contains the removed surface %q", unwanted)
		}
	}
	for _, want := range []string{
		"/api/usage", "obsUsageTokenPanel", "obsUsageCostPanel", "obsUsageCachePanel",
		"obsUsageTrendCard", "obsUsageBreakdownCard", "obsUsageExecutionCard", "obsUsageEmptyPanel",
		"/api/insights/v1/sources",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("the usage console is missing %q", want)
		}
	}
}

func TestUsageAnalysisUI(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	for _, want := range []string{`<h1>统计台</h1>`, `id="refreshObs"`, `id="obsManageSources"`, `id="obsBody"`, `id="obsExpand"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("usage console UI missing %q", want)
		}
	}
	// The overview/delivery tab bar, the delivery badge and the run-quality
	// HTML report export are gone.
	for _, not := range []string{`id="obsTabs"`, `data-obs-tab=`, `id="obsDeliveryCount"`, `id="exportObsReport"`, `id="obsLiveState"`} {
		if strings.Contains(page, not) {
			t.Fatalf("usage console still ships the removed control %q", not)
		}
	}
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	for _, want := range []string{
		"/api/usage", "grain=", "bucket=", "tz=",
		"obsUsageTokenPanel", "obsUsageCostPanel", "obsUsageCachePanel", "obsUsageTrendCard",
		"obsUsageBreakdownCard", "obsUsageExecutionCard", "obsUsageEmptyPanel",
		"obsUsageMoney", "obsUsageCostText", "obsUsageStepBucket",
		"openObsSources", "obsCanManageSources", "/api/insights/v1/sources",
		"isPreviewHost", "showDirectRunButton",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("app bundle is missing %q", want)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping usage console helper check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
global.state = {
  currentUser: {id: 7, loginName: 'alice'},
  usageGrain: 'day', usageBucket: '', usageBuckets: [],
  data: { users: [{id: 7, loginName: 'alice'}] },
};
function formatTokens(n){return String(n)}
function escapeHTML(s){return String(s)}
function lfT(zh){return zh}
eval(code.slice(code.indexOf('function obsUsageGrain(){'), code.indexOf('async function obsLoadUsage(){')));
eval(code.slice(code.indexOf('function obsUsageMoney(value,currency){'), code.indexOf('function obsUsageStatusText(status){')));
eval(code.slice(code.indexOf('function obsUsageStepBucket(buckets,current,step){'), code.indexOf('function shiftObsUsageBucket(step){')));
assert.equal(obsUsageGrain(), 'day');
state.usageGrain='nonsense';
assert.equal(obsUsageGrain(), 'day');
assert.equal(obsUsageMoney(8.61,'CNY'), '¥8.61');
assert.equal(obsUsageMoney(1.23,'USD'), '$1.23');
assert.match(obsUsageCostText(0,'unknown','CNY'), /未识别/);
assert.equal(obsUsageCostText(2.5,'reported','CNY'), '¥2.50');
assert.equal(obsUsageStepBucket(['2026-10-01','2026-10-02'],'2026-10-01',-1), null);
assert.equal(obsUsageStepBucket(['2026-10-01','2026-10-02'],'2026-10-01',1), '2026-10-02');
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("usage console helper check failed: %v\n%s", err, out)
	}
}

func TestListPaginationUI(t *testing.T) {
	js, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	code := string(js)
	for _, want := range []string{
		"function paginateList(",
		"function pagerHTML(",
		"function goListPage(",
		"function bindPager(",
		"paginateList('issues'",
		"paginateList('obs-executions'",
		"obs-executions",
		"data-page-key",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("app.js is missing pagination %q", want)
		}
	}
	for _, not := range []string{"paginateList('sops'", "paginateList('agents'", "paginateList('assets'", "paginateList('approvals'", "paginateList('sync'", "obs-sops", "obs-models"} {
		if strings.Contains(code, not) {
			t.Fatalf("app.js still paginates %q", not)
		}
	}
	css, err := os.ReadFile("platform.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".pager{") && !strings.Contains(string(css), ".pager {") {
		t.Fatal("platform.css is missing .pager styles")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping pagination helper check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
global.state = {listPages:{}};
function escapeHTML(v){return String(v??'')}
eval(code.slice(code.indexOf('const LIST_PAGE_SIZE='), code.indexOf('function goListPage')));
const items = Array.from({length:45}, (_,i)=>i+1);
const p1 = paginateList('issues', items);
assert.equal(p1.size, 20);
assert.equal(p1.page, 1);
assert.equal(p1.pages, 3);
assert.deepEqual(p1.items, items.slice(0,20));
assert.equal(pagerHTML('issues', p1).includes('data-page-key="issues"'), true);
assert.equal(pagerHTML('issues', p1).includes('第 1–20 条 / 共 45 条'), true);
state.listPages.issues = 3;
const p3 = paginateList('issues', items);
assert.equal(p3.page, 3);
assert.deepEqual(p3.items, items.slice(40,45));
assert.equal(pagerHTML('issues', {total:10,size:20}).trim(), '');
resetListPage('issues');
assert.equal(state.listPages.issues, 1);
const obs = paginateList('obs-executions', items);
assert.equal(obs.size, 20);
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("pagination helper check failed: %v\n%s", err, out)
	}
}

func TestIssueTitleFuzzySearch(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "模糊搜索标题") {
		t.Fatal("issue list must advertise title fuzzy search")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping issue title fuzzy-search behavior check")
	}
	script := `
const assert = require('node:assert/strict');
const code = require('node:fs').readFileSync('` + appBundlePath(t) + `', 'utf8');
eval(code.slice(code.indexOf('function normalizeIssueTitleSearch('), code.indexOf('function filteredIssues(')));
const issue = {number: 42, title: '全量 PHPUnit-回归', description: '覆盖 DemoDNS 主流程', externalRef: {externalId: 'TAPD-100'}};
const project = {key: 'DP'};
assert.equal(issueMatchesQuery(issue, project, 'php unit'), true);
assert.equal(issueMatchesQuery(issue, project, '回归 php'), true);
assert.equal(issueMatchesQuery(issue, project, 'PHPUNIT回归'), true);
assert.equal(issueMatchesQuery(issue, project, 'DP-42'), true);
assert.equal(issueMatchesQuery(issue, project, 'DemoDNS'), true);
assert.equal(issueMatchesQuery(issue, project, '不存在的标题'), false);
`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("issue title fuzzy-search behavior check failed: %v\n%s", err, out)
	}
}

func TestIssueFilterAndManualCloseUI(t *testing.T) {
	htmlBytes, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)
	for _, want := range []string{
		`id="issueStatusFilter"`,
		`<option value="backlog">待规划</option>`,
		`<option value="todo">待处理 / 待执行</option>`,
		`<option value="done">已完成</option>`,
		`<option value="closed">已关闭</option>`,
		`data-dtab="operations"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("index.html is missing %q", want)
		}
	}
	// One operator, one identity: the assignee filter and its label are gone.
	for _, gone := range []string{`id="issueAssigneeFilter"`, `未指定责任人`} {
		if strings.Contains(html, gone) {
			t.Fatalf("index.html still carries %q", gone)
		}
	}

	appBytes, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	app := string(appBytes)
	for _, want := range []string{
		"issueStatusFilter:''",
		"$('#issueStatusFilter')",
		"closeIssueById",
		"data-close-issue",
		"/api/issues/${encodeURIComponent(issueId)}/close",
		"data-issue-menu",
		"showIssueActionMenu",
		"ensureIssueMenuPopover",
		"issue-action-popover",
		"编辑需求",
		"issueMatchesStatusFilter",
		"['closed','Closed']",
		"operationsPane",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app.js is missing %q", want)
		}
	}
	if strings.Contains(app, "submit.textContent=isNew?'同步并保存':'保存 Issue'") {
		t.Fatalf("app.js still contains '保存 Issue' in submit button")
	}
	for _, gone := range []string{"issueAssigneeFilter", "renderIssueAssigneeFilter", "issueAssigneeName"} {
		if strings.Contains(app, gone) {
			t.Fatalf("app bundle still carries %q", gone)
		}
	}

	node, err := exec.LookPath("node")
	if err == nil {
		nodeScript := `
const fs = require('node:fs');
const code = fs.readFileSync('` + appBundlePath(t) + `', 'utf8');
const mockEl = () => ({
  classList: { toggle: () => {}, contains: () => false, add: () => {}, remove: () => {} },
  style: {},
  dataset: {},
  innerHTML: '',
  appendChild: () => {},
  addEventListener: () => {},
  querySelector: () => mockEl(),
  querySelectorAll: () => []
});
global.window = global;
global.addEventListener = () => {};
global.removeEventListener = () => {};
global.matchMedia = () => ({ matches: false, addEventListener() {}, removeEventListener() {} });
global.location = { origin: 'http://localhost', hash: '', href: 'http://localhost/', search: '' };
global.document = {
  hidden: false,
  addEventListener: () => {},
  documentElement: { dataset: {} },
  querySelector: () => mockEl(),
  querySelectorAll: () => [],
  getElementById: () => mockEl(),
  body: mockEl()
};
global.$ = () => mockEl();
global.$$ = () => [];
global.localStorage = { getItem: () => null, setItem: () => {}, removeItem: () => {} };
global.fetch = () => Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
eval(code + '; global.state = state; global.renderIssues = renderIssues; global.issueMatchesStatusFilter = issueMatchesStatusFilter; global.issueStatusBucket = issueStatusBucket;  global.issueOutcome = issueOutcome; global.issueCurrentStage = issueCurrentStage; global.reopenIssueButton = reopenIssueButton;');
state.data = { issues: [], projects: [], repositories: [], workflows: [], runs: [], tasks: [] };
state.listPages = {};
renderIssues();
state.data.issues = [{ id: 'iss-1', projectId: 'p1', number: 1, title: 'test', status: 'backlog' }];
renderIssues();
const noRunCases = [
  ['in_progress', 'in_progress'],
  ['in_review', 'in_review'],
  ['blocked', 'blocked']
];
for (const [status, expected] of noRunCases) {
  const issue = { id: 'case-' + status, status };
  if (!issueMatchesStatusFilter(issue, expected) || issueMatchesStatusFilter(issue, 'todo')) {
    throw new Error(status + ' without a run was swallowed by the todo filter');
  }
}
if (!issueMatchesStatusFilter({ id: 'backlog-case', status: 'backlog' }, 'backlog') || issueMatchesStatusFilter({ id: 'backlog-case', status: 'backlog' }, 'todo')) {
  throw new Error('backlog must follow its persisted status instead of being folded into todo');
}
// A requirement's own status drives its stage: no run, no flow and no tracker
// state can move it.
const stageCases = [
  ['in_progress', 'active'],
  ['in_review', 'active'],
  ['blocked', 'attention'],
  ['done', 'closed'],
  ['closed', 'closed'],
  ['todo', 'not_started'],
  ['backlog', 'not_started']
];
for (const [status, key] of stageCases) {
  const issue = { id: 'stage-' + status, status };
  if (issueCurrentStage(issue).key !== key) {
    throw new Error('status ' + status + ' must show the ' + key + ' stage');
  }
}
const closedRequirement = { id: 'closed-requirement', status: 'closed' };
if (issueOutcome(closedRequirement).title !== '需求已关闭') {
  throw new Error('a closed requirement must report that it is closed');
}
if (!reopenIssueButton(closedRequirement).includes('data-reopen-issue')) {
  throw new Error('a closed requirement must stay reopenable');
}
const statusMatrix = [
  ['backlog', 'backlog'],
  ['todo', 'todo'],
  ['in_progress', 'in_progress'],
  ['in_review', 'in_review'],
  ['done', 'done'],
  ['closed', 'closed'],
  ['blocked', 'blocked'],
  ['cancelled', 'cancelled']
];
for (const [stored, bucket] of statusMatrix) {
  const issue = { id: 'matrix-' + stored, status: stored };
  const matches = ['', 'backlog', 'todo', 'in_progress', 'in_review', 'done', 'closed', 'blocked', 'cancelled'].filter(filter => issueMatchesStatusFilter(issue, filter));
  if (issueStatusBucket(issue) !== bucket || matches.length !== 2 || !matches.includes('') || !matches.includes(bucket)) {
    throw new Error('status bucket is not exclusive for ' + stored + ': ' + matches.join(','));
  }
}
`
		if out, err := exec.Command(node, "-e", nodeScript).CombinedOutput(); err != nil {
			t.Fatalf("renderIssues node execution check failed: %v\n%s", err, out)
		}
	}
}

// The platform administrator side is gone: there is one account and no
// platform-wide management page.
func TestPlatformAdminViewIsGone(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	source, markup := string(app), string(html)

	for _, unwanted := range []string{
		`id="platformView"`,
		`id="platformNavGroup"`,
		`id="platformAdminPanel"`,
		`data-view="platform"`,
	} {
		if strings.Contains(markup, unwanted) {
			t.Fatalf("the platform admin page is still in the interface: %q", unwanted)
		}
	}
	for _, unwanted := range []string{
		"renderPlatformAdminPanel",
		"renderPlatformNav",
		"isPlatformAdmin",
	} {
		if strings.Contains(source, unwanted) {
			t.Fatalf("the platform admin page is still wired into the app: %q", unwanted)
		}
	}
}

func TestClarifyControlsAreGone(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)

	// The clarification round left the console: its option controls, its
	// buttons and its pane must not come back.
	for _, unwanted := range []string{
		"wasSelected=b.classList.contains('selected')",
		"issue-clarify-option",
		"data-start-clarify",
		"data-continue-clarify",
		"data-stop-clarify",
		"data-reclarify",
		"data-retry-clarify",
	} {
		if strings.Contains(source, unwanted) {
			t.Fatalf("the clarification round is still wired into the app: %q", unwanted)
		}
	}
}

// The platform administrator page left no styles behind.
func TestPlatformAdminStylesAreGone(t *testing.T) {
	app, err := readAppBundle()
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)
	for _, unwanted := range []string{
		"platform-metric-grid",
		"platformMetricTotal",
		"platformMetricLocked",
		"platformMetricGranted",
		"platformAdminSearch",
		"platformFilterTabs",
		"data-platform-nav",
	} {
		if strings.Contains(source, unwanted) {
			t.Fatalf("the platform admin page is still wired into the app: %q", unwanted)
		}
	}
	for _, path := range []string{"platform.css", "product-v4.css"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, unwanted := range []string{
			".platform-admin-panel",
			".platform-admin-layout",
			".platform-admin-avatar",
			".platform-admin-side",
			".platform-locked-tag",
			".platform-revoke-btn",
			".platform-source-badge",
		} {
			if strings.Contains(string(raw), unwanted) {
				t.Fatalf("%s still styles the removed platform admin page: %q", path, unwanted)
			}
		}
	}
}

func TestCSSBracesBalanced(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".css") {
			continue
		}
		content, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry.Name(), err)
		}
		open := 0
		for _, b := range content {
			if b == '{' {
				open++
			} else if b == '}' {
				open--
			}
		}
		if open != 0 {
			t.Fatalf("unbalanced braces in %s: open count is %d", entry.Name(), open)
		}
	}
}

func TestCodexStyleLeftMiniRailLayout(t *testing.T) {
	pageBytes, err := Files.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(pageBytes)

	// 1. 验证左侧小栏 platformRail 存在，且位于 app-shell 第一项
	railIdx := strings.Index(page, `id="platformRail"`)
	sidebarIdx := strings.Index(page, `class="platform-sidebar"`)
	if railIdx < 0 {
		t.Fatal("index.html must have #platformRail as left mini sidebar")
	}
	if sidebarIdx < 0 || railIdx > sidebarIdx {
		t.Fatal("#platformRail must sit before .platform-sidebar in DOM")
	}

	// 2. 左侧小栏只留 4 个日常入口和设置；流程模板、运行记录、定时任务收进设置导航。
	for _, view := range []string{"observability", "issues", "settings"} {
		needle := `data-view="` + view + `"`
		sub := page[railIdx:sidebarIdx]
		if !strings.Contains(sub, needle) {
			t.Fatalf("#platformRail must contain %s nav button", needle)
		}
	}
	// 流程模板、运行记录、定时任务随执行引擎一起离开，导航里也不再保留入口。
	for _, view := range []string{"sops", "runs", "aiSchedules"} {
		needle := `data-view="` + view + `"`
		if strings.Contains(page, needle) {
			t.Fatalf("the retired %s view must not be in the navigation", needle)
		}
	}
	// 图标栏第 2 位仍是统计；需求列表紧跟其后，两者都是日常工作入口。
	railNav := page[railIdx:sidebarIdx]
	if !strings.Contains(railNav, `data-view="issues" data-tooltip="需求列表" aria-label="需求列表"`) {
		t.Fatal("#platformRail must contain the issues entry labelled 需求列表")
	}
	if !strings.Contains(railNav, `data-view="observability" data-tooltip="统计台" aria-label="统计台"`) {
		t.Fatal("#platformRail observability button must be labelled 统计台")
	}
	if strings.Index(railNav, `data-view="observability"`) > strings.Index(railNav, `data-view="issues"`) {
		t.Fatal("统计台 must stay before 需求列表 in #platformRail")
	}
	if strings.Contains(page[sidebarIdx:], `data-view="observability"`) {
		t.Fatal("settings navigation must not duplicate the 统计台 entry")
	}
	if !strings.Contains(page[sidebarIdx:], `data-view="runners"`) {
		t.Fatal("settings navigation must contain the runners entry")
	}

	// 3. 底部包含完整个人信息与弹窗入口
	railContent := page[railIdx:sidebarIdx]
	if !strings.Contains(railContent, `id="userProfileWrap"`) || !strings.Contains(railContent, `id="userProfilePopover"`) {
		t.Fatal("#platformRail must host #userProfileWrap and #userProfilePopover at bottom")
	}

	// 4. 验证 CSS 定义了 --rail-w 且 app-shell 拥有三列布局
	cssBytes, err := Files.ReadFile("product-v4.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssBytes)
	if !strings.Contains(css, "--rail-w") {
		t.Fatal("product-v4.css must define --rail-w")
	}
	if !strings.Contains(css, "grid-template-columns: var(--rail-w) var(--sidebar-w) minmax(0, 1fr)") {
		t.Fatal("product-v4.css .app-shell must start with var(--rail-w)")
	}
	if !strings.Contains(css, ".platform-rail") {
		t.Fatal("product-v4.css must contain .platform-rail styles")
	}

	// 5. 验证已移除冗余的配置与管理 overview 页面与中间栏按钮，以及设置/执行机器的无侧栏模式
	if strings.Contains(page, `id="managementView"`) {
		t.Fatal("index.html must not contain #managementView overview page")
	}
	middleSidebarContent := page[sidebarIdx:]
	mainIdx := strings.Index(middleSidebarContent, `<main>`)
	if mainIdx > 0 && strings.Contains(middleSidebarContent[:mainIdx], `data-view="management"`) {
		t.Fatal(".platform-sidebar must not contain data-view=\"management\" entry")
	}
	if !strings.Contains(css, "body.no-sidebar .platform-sidebar") {
		t.Fatal("product-v4.css must support body.no-sidebar to hide platform-sidebar in settings/runners")
	}

	// 6. 验证个人信息弹窗已精简：仅保留退出登录按钮
	workbenchBytes, err := Files.ReadFile("app/workbench.js")
	if err != nil {
		t.Fatal(err)
	}
	wb := string(workbenchBytes)
	if strings.Contains(wb, "openPersonalSettings") {
		t.Fatal("user profile popover should not have redundant openPersonalSettings button")
	}
	if strings.Contains(wb, "uppReAuthBtn") {
		t.Fatal("user profile popover should not have redundant uppReAuthBtn button")
	}
	if !strings.Contains(wb, "uppLogoutBtn") {
		t.Fatal("user profile popover must retain uppLogoutBtn")
	}

	// 7. 验证中间侧边栏移除了与左上角重复的 Logo，且 app.js 在需求列表与运行记录等视图中自动隐藏对话侧栏
	if strings.Contains(middleSidebarContent[:mainIdx], `class="brand"`) {
		t.Fatal(".platform-sidebar must not contain .brand (duplicated with mini rail logo)")
	}
	appJsBytes, err := Files.ReadFile("app.js")
	if err != nil {
		t.Fatal(err)
	}
	appJs := string(appJsBytes)
	if !strings.Contains(appJs, "const noSidebarViews = ['issues', 'observability'];") {
		t.Fatal("app.js must hide the sidebar on the rail-only pages: 需求列表, 用量台")
	}
	managed := appJs[strings.Index(appJs, "const settingsManagementViews="):]
	managed = managed[:strings.Index(managed, "\n")]
	if strings.Contains(managed, "'observability'") {
		t.Fatal("用量台 must not enter settings management mode")
	}
	for _, view := range []string{"'projects'", "'runners'"} {
		if !strings.Contains(managed, view) {
			t.Fatalf("%s moved into the settings sidebar and must enter settings management mode", view)
		}
	}
	if strings.Contains(managed, "'projectCredentials'") {
		t.Fatal("凭据管理 was deleted and must not stay in the settings views")
	}
	if !strings.Contains(appJs, "observability:'统计台'") {
		t.Fatal("viewTitles.observability must be 统计台")
	}
	paletteBytes, err := Files.ReadFile("app/command-palette.js")
	if err != nil {
		t.Fatal(err)
	}
	palette := string(paletteBytes)
	for _, want := range []string{"['observability','统计台','平台数据 · Agent 用量']", "['issues','需求列表'"} {
		if !strings.Contains(palette, want) {
			t.Fatalf("command palette is missing %q", want)
		}
	}
}
