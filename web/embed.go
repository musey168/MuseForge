package web

import "embed"

// Files is the static console the platform serves. The chat workbench, the
// LOOP designer, the runner IDE and the knowledge pages are gone, so their
// assets are not embedded. The Monaco editor and the VS Code workbench went
// with the runner IDE; the vendor directory still carries the usage charts.
//
//go:embed boot.js i18n.js i18n-en.js preferences.css boot-theme.js gongfeng-oauth-done.html gongfeng-oauth-done.js index.html app.js app assistant-models.css mcp-setup.txt styles.css platform.css observability.css sync.css product-v3.css product-v4.css images/credentials/*.svg images/credentials/kimi-code.png images/loopforge-logo.png favicon.ico favicon.png favicon.svg apple-touch-icon.png mcp-icons vendor
var Files embed.FS
