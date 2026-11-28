package httpapi

import (
	"crypto/subtle"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	stdsync "sync"
	"time"

	deployassets "agentflow/deploy"
	apidocs "agentflow/docs/api"
	"agentflow/internal/application"
	"agentflow/internal/auth"
	"agentflow/internal/domain"
	"agentflow/internal/mcpserver"
	"agentflow/internal/repo"
	"agentflow/internal/tracker"
	webassets "agentflow/web"

	"github.com/gin-gonic/gin"
)

// deps 聚合所有领域 handler 共享的基础依赖与跨领域行为。
type deps struct {
	repo       *repo.Store
	sync       *tracker.Service
	authClient *auth.Client
	api        http.Handler
	// passwordAttempts is this server's brute-force budget (auth_password.go).
	passwordAttempts *passwordLimiter
}

// getAuthClient 挂 deps（token 鉴权与 auth 流程共用）。
func (d deps) getAuthClient() *auth.Client {
	if d.authClient == nil {
		d.authClient = auth.NewClientFromEnv()
	}
	return d.authClient
}

// Server 组合各领域 handler，仅保留服务级端点与入口装配。
type Server struct {
	*deps
	auth         *authHandlers // auth.go
	github       *githubHandlers
	users        *usersHandlers // users.go
	projects     *projectsHandlers
	repositories *repositoriesHandlers
	credentials  *projectCredentialsHandlers
	insights     *insightsHandlers
	issues       *issuesHandlers
	runners      *runnersHandlers // runners.go
	tokens       *tokenHandlers   // auth_tokens.go
	syncAPI      *syncHandlers    // tracker.go
}

func New(s *repo.Store, syncSvc *tracker.Service, basicUser, basicPass string) *gin.Engine {
	d := &deps{repo: s, sync: syncSvc, authClient: auth.NewClientFromEnv(), passwordAttempts: newPasswordLimiter()}
	if trustGatewayIdentityHeaders() && strings.TrimSpace(os.Getenv("AGENTFLOW_IDENTITY_HEADER_SECRET")) == "" {
		log.Printf("warning: AGENTFLOW_TRUST_IDENTITY_HEADERS is on without AGENTFLOW_IDENTITY_HEADER_SECRET; any loopback caller without forwarding headers, including a proxy that passes client headers through, can sign in as any user")
	}
	if syncSvc != nil {
		syncSvc.ConfigureWithStore(s)
	}
	runnerControls := newRunnerControlHub()
	api := &Server{
		deps:         d,
		auth:         &authHandlers{deps: d},
		github:       newGitHubHandlers(d),
		users:        &usersHandlers{deps: d},
		insights:     &insightsHandlers{deps: d},
		projects:     &projectsHandlers{deps: d},
		repositories: &repositoriesHandlers{deps: d},
		credentials:  &projectCredentialsHandlers{deps: d},
		issues:       &issuesHandlers{deps: d},
		runners:      &runnersHandlers{deps: d, runnerControls: runnerControls, runnerBindCodes: map[string]runnerBindCode{}},
		tokens:       &tokenHandlers{deps: d},
		syncAPI:      &syncHandlers{deps: d},
	}
	api.runners.startRunnerSweeper()
	g := gin.New()
	g.HandleMethodNotAllowed = true
	// master(net/http) 对带尾斜杠的 /api 子树路径按各自 handler 语义返回
	// 405/404；gin 默认会 301 重定向到无斜杠路径，行为不同，关闭之。
	g.RedirectTrailingSlash = false
	// gin 只有在显式注册 NoMethod 后才会对"路径匹配但方法不符"返回 405，
	// 否则落入 404；这里同时把 405 响应统一成 JSON 形态。
	g.NoMethod(func(c *gin.Context) {
		if masterTrailingSlashCompat(c, api) {
			return
		}
		c.JSON(http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	})
	g.Use(logging(), securityHeaders(), cors(), gzipMaybe())
	g.Use(api.tokens.userAccessAuth())
	if basicUser != "" && basicPass != "" {
		g.Use(basicAuth(basicUser, basicPass))
	}

	// --- auth & users ---
	g.GET("/api/health", api.health)
	g.GET("/api/auth/current", api.auth.authCurrent)
	g.POST("/api/auth/password", api.auth.authPassword)
	g.GET("/api/auth/setup", api.auth.authSetup)
	g.POST("/api/auth/register", api.auth.authRegister)
	g.GET("/api/auth/local", api.auth.authLocal)
	g.POST("/api/auth/local", api.auth.authLocal)
	g.GET("/api/auth/github", api.github.status)
	g.PUT("/api/auth/github/config", api.github.configure)
	g.POST("/api/auth/github/device", api.github.start)
	g.POST("/api/auth/github/poll", api.github.poll)
	g.GET("/api/auth/github/repositories", api.github.repositories)
	g.GET("/api/auth/exchange", api.auth.authExchange)
	g.POST("/api/auth/exchange", api.auth.authExchange)
	g.POST("/api/auth/logout", api.auth.authLogout)
	g.GET("/api/auth/photo", api.auth.authPhoto)
	g.GET("/api/auth/photo/:spec/:username", api.auth.authPhoto)
	// master(net/http) 的 /api/users/ 子树只接受 PUT /api/users/<id>/credentials
	// 一种形态，其余任何方法/子路径都走 method(w) → 405 JSON。
	g.Any("/api/users/:id/credentials", func(c *gin.Context) {
		if c.Request.Method != http.MethodPut {
			c.JSON(http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		api.users.user(c)
	})
	g.GET("/api/users/:id/git-token/verify", api.users.verifyGitToken)
	g.POST("/api/users/:id/git-token/verify", api.users.verifyGitToken)
	g.GET("/api/users/:id/git-ssh-keys", api.users.gitSSHKeys)
	g.PUT("/api/users/:id/git-ssh-keys", api.users.gitSSHKeys)
	g.DELETE("/api/users/:id/git-ssh-keys/:keyID", api.users.gitSSHKeys)
	g.GET("/api/users/:id/git-http-credentials", api.users.gitHTTPCredentials)
	g.PUT("/api/users/:id/git-http-credentials", api.users.gitHTTPCredentials)
	g.DELETE("/api/users/:id/git-http-credentials/:credID", api.users.gitHTTPCredentials)
	g.POST("/api/users/:id/git-http-credentials/:credID/verify", api.users.verifyGitHTTPCredential)
	g.POST("/api/users/:id/cli-models/probe", api.users.probeCLIModels)
	g.Any("/api/users/:id", func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	})

	// --- service & personal surfaces ---
	g.GET("/api/capabilities", api.capabilities)
	g.GET("/api/model-catalog", api.modelCatalog)
	g.GET("/api/executor-models", api.executorModels)
	g.GET("/api/openapi.yaml", api.openAPI)
	g.GET("/api/bootstrap", api.bootstrap)
	g.GET("/api/projects", api.projects.projects)
	g.GET("/api/repositories", api.repositories.repositories)
	g.PUT("/api/repositories", api.repositories.repositories)
	g.POST("/api/repositories", api.repositories.repositories)
	g.DELETE("/api/repositories", api.repositories.repositories)
	g.GET("/api/repositories/:id", api.repositories.repository)
	g.PUT("/api/projects", api.projects.projects)
	g.POST("/api/projects", api.projects.projects)
	g.GET("/api/projects/:id", api.projects.project)
	g.GET("/api/projects/:id/credentials", api.credentials.list)
	g.POST("/api/projects/:id/credentials", api.credentials.save)
	g.PUT("/api/projects/:id/credentials/:credentialId", api.credentials.save)
	g.DELETE("/api/projects/:id/credentials/:credentialId", api.credentials.delete)
	g.DELETE("/api/projects/:id", api.projects.deleteProject)
	g.POST("/api/projects/:id/restore", api.projects.restoreProject)
	g.GET("/api/issues", api.issues.issues)
	g.PUT("/api/issues", api.issues.issues)
	g.POST("/api/issues", api.issues.issues)
	g.Any("/api/issues/:id", api.issues.issue)
	g.GET("/api/issues/:id/images/:imageId", api.issues.issue)
	g.Any("/api/issues/:id/comments", api.issues.issue)
	g.Any("/api/issues/:id/reopen", api.issues.issue)
	g.Any("/api/issues/:id/close", api.issues.issue)
	g.Any("/api/issues/:id/requirement", api.issues.issue)
	g.POST(insightIngestPath, api.insights.ingest)
	g.GET("/api/insights/v1/sources", api.insights.sources)
	g.POST("/api/insights/v1/sources", api.insights.sources)
	g.DELETE("/api/insights/v1/sources/:id", api.insights.revokeSource)
	g.GET("/api/usage", api.insights.usage)
	g.GET("/api/observability/spec", api.observabilitySpec)

	// --- sync & tracker import ---
	g.GET("/api/tracker/import", api.syncAPI.trackerImport)
	g.POST("/api/tracker/import", api.syncAPI.trackerImport)

	// --- machine registry (inventory only) ---
	g.GET("/api/runners", api.runners.runners)
	g.POST("/api/runners/register", api.runners.runnerRegister)
	g.POST("/api/runners/deregister", api.runners.runnerDeregister)
	g.POST("/api/runners/disconnect", api.runners.runnerDisconnect)
	g.POST("/api/runners/bind-codes", api.runners.issueRunnerBindCode)
	g.DELETE("/api/runners/:id", api.runners.runnerRemove)
	g.GET("/api/runners/:id/maintenance", api.runners.runnerMaintenance)
	g.POST("/api/runners/:id/maintenance", api.runners.runnerMaintenance)
	g.POST("/api/runners/:id/models/probe", api.runners.probeRunnerModels)
	g.PUT("/api/runners/:id/models", api.runners.saveRunnerLoginModels)
	g.POST("/api/runners/heartbeat", api.runners.runnerHeartbeat)
	g.PUT("/api/runners/runtimes", api.runners.runnerRuntimeStatus)
	g.POST("/api/runners/runtimes", api.runners.runnerRuntimeStatus)

	// Machine plane: this prefix is safe to exempt from gateway interactive
	// login. Public install assets are read-only; daemon APIs authenticate with
	// one-time bind codes or runner session tokens instead of a user identity.
	g.GET("/runners/install.sh", serveBindCLIScript)
	g.GET("/runners/agentflow-runner", serveRunnerBinary)
	g.GET("/runners/health", api.health)
	g.GET("/runners/connect", api.runners.runnerControlConnect)
	g.POST("/runners/register", api.runners.runnerRegister)
	g.POST("/runners/deregister", api.runners.runnerDeregister)
	g.POST("/runners/disconnect", api.runners.runnerDisconnect)
	g.POST("/runners/heartbeat", api.runners.runnerHeartbeat)
	g.GET("/deploy/bind-cli.sh", serveBindCLIScript)
	g.GET("/deploy/agentflow-runner", serveRunnerBinary)
	mcpHandler := mcpserver.New(mcpserver.LocalInvoker{Handler: g}).HTTPHandler()
	g.Any("/mcp", func(c *gin.Context) {
		mcpHandler.ServeHTTP(c.Writer, c.Request)
	})
	d.api = g
	static, _ := fs.Sub(webassets.Files, ".")
	g.NoRoute(func(c *gin.Context) {
		if masterTrailingSlashCompat(c, api) {
			return
		}
		// gin 预置 NoRoute 状态码为 404；studioFileServer 的 index 分支只写
		// body 不显式 WriteHeader，会继承该 404。先显式置 200，静态文件
		// 缺失时 FileServer 仍会以显式 WriteHeader(404) 覆盖。
		c.Status(http.StatusOK)
		studioFileServer(static).ServeHTTP(c.Writer, c.Request)
	})
	return g
}

// serveBindCLIScript serves the machine-facing bind-cli installer so onboarding
// is a single copy-pasted curl command (个人 → 执行器 页面).
func serveBindCLIScript(c *gin.Context) {
	script := deployassets.BindCLIScript()
	if len(script) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	_, _ = c.Writer.Write(script)
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, map[string]any{"status": "ok", "role": "api", "pid": os.Getpid()})
}

func (s *Server) capabilities(c *gin.Context) {
	c.JSON(http.StatusOK, map[string]any{
		"operations": application.PublicOperations(),
		"mcp": map[string]any{
			"streamableHttp": "/mcp",
			"stdioCommand":   "agentflow-mcp",
		},
	})
}

func (s *Server) openAPI(c *gin.Context) {
	c.Header("Content-Type", "application/yaml; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=300")
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(apidocs.OpenAPI)
}

func (s *Server) bootstrap(c *gin.Context) {
	var projects []domain.Project
	var repositories []domain.Repository
	var users []domain.User
	var load stdsync.WaitGroup
	load.Add(3)
	go func() { defer load.Done(); projects = s.repo.Projects() }()
	go func() { defer load.Done(); repositories = s.repo.Repositories() }()
	go func() { defer load.Done(); users = s.repo.Users() }()
	load.Wait()
	c.JSON(200, map[string]any{
		"projects":     domain.PublicProjects(projects),
		"repositories": domain.PublicRepositories(repositories),
		"users":        domain.PublicUsers(users),
		"storage":      s.repo.StorageInfo(),
	})
}

func (s *Server) observabilitySpec(c *gin.Context) {
	c.JSON(200, domain.ObservabilitySpec{
		Version:         domain.ObservabilitySpecVersion,
		Categories:      []string{"delivery", "usage", "verification"},
		Phases:          []string{"start", "finish"},
		RequiredContext: []string{"nodeId"},
		MetricConventions: map[string]string{
			"cost":  "usd",
			"count": "integer",
		},
		Adapters: []domain.AdapterDescriptor{{
			ID:              "reported-execution",
			SpecVersion:     domain.ObservabilitySpecVersion,
			ExecutorKinds:   []string{"external"},
			NativeProtocols: []string{"http"},
			Capabilities:    []string{"usage"},
			RawEventPolicy:  "aggregate",
		}},
	})
}

func isLoopbackAddr(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// logging 记录每个 HTTP 请求（method/path/status/bytes/耗时），用于排查上传等问题。
func logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("[HTTP] %s %s -> %d (%d bytes, %s)",
			c.Request.Method, redactAgentToolPath(c.Request.URL.Path),
			c.Writer.Status(), c.Writer.Size(), time.Since(start).Truncate(time.Millisecond))
	}
}

// basicAuth 挂 runner 外网访问的 Basic 鉴权。
func basicAuth(user, pass string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// The runner machine plane has its own bind/session authentication and
		// must remain usable behind a gateway login.
		if c.Request.URL.Path == insightIngestPath || strings.HasPrefix(c.Request.URL.Path, "/runners/") || strings.HasPrefix(c.Request.URL.Path, "/internal/agent-tools/") {
			c.Next()
			return
		}
		gotUser, gotPass, ok := c.Request.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(gotUser), []byte(user)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(gotPass), []byte(pass)) == 1
		if !ok || !userOK || !passOK {
			if strings.HasPrefix(c.Request.URL.Path, "/api/otel/") && isLoopbackAddr(c.Request.RemoteAddr) {
				c.Next()
				return
			}
			c.Header("WWW-Authenticate", `Basic realm="AgentFlow"`)
			c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			c.Abort()
			return
		}
		c.Next()
	}
}
