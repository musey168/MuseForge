package web

import "embed"

//go:embed boot.js i18n.js i18n-en.js preferences.css login.html login.js boot-theme.js gongfeng-oauth-done.html gongfeng-oauth-done.js index.html app.js app assistant-models.css mcp-setup.txt styles.css platform.css observability.css sync.css access.css product-v3.css product-v4.css images/credentials/*.svg images/credentials/kimi-code.png images/loopforge-logo.png favicon.ico favicon.png favicon.svg apple-touch-icon.png mcp-icons vendor
var Files embed.FS
