package routes

import (
	"fmt"

	"xivi/backend/pkg/middleware"
	xividocs "xivi/docs"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	swaggerFiles "github.com/swaggo/files"
	"github.com/swaggo/swag"
)

var documentationAssets = map[string]struct{}{
	"favicon-16x16.png":               {},
	"favicon-32x32.png":               {},
	"swagger-ui-bundle.js":            {},
	"swagger-ui-standalone-preset.js": {},
	"swagger-ui.css":                  {},
}

const documentationHTML = `<!doctype html>
<html lang="en">
	<head>
		<meta charset="utf-8" />
		<meta name="viewport" content="width=device-width, initial-scale=1" />
		<title>%s</title>
		<link rel="icon" type="image/png" href="/docs/assets/favicon-32x32.png" />
		<link rel="stylesheet" href="/docs/assets/swagger-ui.css" />
		<link rel="stylesheet" href="/docs/assets/xivi-docs.css" />
	</head>
	<body data-spec-url="%s">
		<header class="xivi-docs-header %s">
			<div><strong>Xivi</strong><span>%s</span></div>
			<nav><a href="%s">%s</a><a href="/studio">Back to Studio</a></nav>
		</header>
		<div id="swagger-ui"></div>
		<script src="/docs/assets/swagger-ui-bundle.js" defer></script>
		<script src="/docs/assets/swagger-ui-standalone-preset.js" defer></script>
		<script src="/docs/assets/xivi-docs.js" defer></script>
	</body>
</html>`

const documentationCSS = `
:root { color-scheme: light; }
body { margin: 0; background: #f7f7f2; }
.xivi-docs-header {
	display: flex; align-items: center; justify-content: space-between; gap: 1rem;
	position: sticky; z-index: 20; top: 0; min-height: 3.75rem; padding: 0.65rem 1.25rem;
	border-bottom: 1px solid #d9dcd5; background: #10131a; color: #f7f7f2;
	font: 600 0.85rem/1.3 system-ui, sans-serif;
}
.xivi-docs-header > div { display: flex; align-items: baseline; gap: 0.65rem; }
.xivi-docs-header strong { color: #2bd9c0; font-size: 1.15rem; }
.xivi-docs-header span { color: #a8b2c7; }
.xivi-docs-header nav { display: flex; flex-wrap: wrap; gap: 0.45rem; }
.xivi-docs-header a {
	border: 1px solid #3b4355; border-radius: 0.55rem; padding: 0.42rem 0.65rem;
	color: #f7f7f2; text-decoration: none;
}
.xivi-docs-header a:hover, .xivi-docs-header a:focus-visible { border-color: #2bd9c0; outline: none; }
.xivi-docs-header.legacy { border-bottom-color: #ff6b5e; }
.xivi-docs-header.legacy strong { color: #ff6b5e; }
.swagger-ui .info { margin: 32px 0 20px; }
.swagger-ui .scheme-container { margin: 0 0 24px; box-shadow: none; }
@media (max-width: 640px) {
	.xivi-docs-header { align-items: flex-start; flex-direction: column; padding: 0.7rem; }
}
`

const documentationInitializer = `
(async function () {
	const specURL = document.body.dataset.specUrl;
	const safeMethods = new Set(['GET', 'HEAD', 'OPTIONS']);
	async function csrfToken() {
		const response = await fetch('/api/v2/auth/session', {
			credentials: 'same-origin',
			headers: { Accept: 'application/json' },
			cache: 'no-store'
		});
		if (!response.ok) throw new Error('Your Xivi administrator session is unavailable.');
		const session = await response.json();
		if (!session.csrf_token) throw new Error('The Xivi CSRF token is unavailable.');
		return session.csrf_token;
	}
	window.ui = SwaggerUIBundle({
		url: specURL,
		dom_id: '#swagger-ui',
		deepLinking: true,
		displayOperationId: true,
		displayRequestDuration: true,
		filter: true,
		withCredentials: true,
		validatorUrl: 'none',
		presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
		plugins: [SwaggerUIBundle.plugins.DownloadUrl],
		layout: 'BaseLayout',
		requestInterceptor: async function (request) {
			request.credentials = 'same-origin';
			const method = String(request.method || 'GET').toUpperCase();
			const path = new URL(request.url, window.location.origin).pathname;
			if (!safeMethods.has(method) && path !== '/api/v2/auth/login') {
				request.headers = request.headers || {};
				request.headers['X-CSRF-Token'] = await csrfToken();
			}
			return request;
		}
	});
})();
`

func documentationSecurityHeaders(c *fiber.Ctx) error {
	// Swagger UI uses style attributes for a small amount of runtime layout.
	// Scripts, network requests, frames, and every other resource remain
	// same-origin; no inline script or external CDN is permitted.
	c.Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self'; style-src 'self'; style-src-elem 'self'; style-src-attr 'unsafe-inline'; font-src 'self'; img-src 'self' data: blob:; connect-src 'self'; worker-src 'self' blob:")
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Next()
}

func documentationPage(legacy bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if legacy {
			return c.Type("html").SendString(fmt.Sprintf(documentationHTML,
				"Xivi legacy API documentation", "/docs/legacy/openapi.json", "legacy",
				"Legacy administrator API · deprecated", "/docs/", "Current API"))
		}
		return c.Type("html").SendString(fmt.Sprintf(documentationHTML,
			"Xivi API documentation", "/docs/openapi.yaml", "current",
			"Current Watch and Studio API · OpenAPI 3.1", "/docs/legacy/", "Legacy API"))
	}
}

func serveCurrentOpenAPI(c *fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "application/yaml; charset=utf-8")
	return c.SendString(xividocs.OpenAPIV2)
}

func serveLegacyOpenAPI(c *fiber.Ctx) error {
	document, err := swag.ReadDoc()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Legacy API documentation is unavailable.")
	}
	return c.Type("json").SendString(document)
}

func serveDocumentationAsset(c *fiber.Ctx) error {
	asset := c.Params("asset")
	if asset == "xivi-docs.css" {
		return c.Type("css").SendString(documentationCSS)
	}
	if asset == "xivi-docs.js" {
		return c.Type("js").SendString(documentationInitializer)
	}
	if _, allowed := documentationAssets[asset]; !allowed {
		return fiber.ErrNotFound
	}
	c.Set(fiber.HeaderCacheControl, "private, max-age=86400")
	return filesystem.SendFile(c, swaggerFiles.HTTP, "/"+asset)
}

// APIDocumentationRoutes serves the current OpenAPI contract and a clearly
// separated legacy contract. Documentation is never available anonymously.
func APIDocumentationRoutes(a *fiber.App) {
	admin := []fiber.Handler{
		middleware.DeclareRoutePolicy("admin"),
		middleware.RequireAdmin(),
		middleware.RequirePasswordChanged(),
		documentationSecurityHeaders,
	}
	docs := a.Group("/docs", admin...)
	docs.Get("", func(c *fiber.Ctx) error { return c.Redirect("/docs/", fiber.StatusTemporaryRedirect) })
	docs.Get("/", documentationPage(false))
	docs.Get("/index.html", documentationPage(false))
	docs.Get("/openapi.yaml", serveCurrentOpenAPI)
	docs.Get("/assets/:asset", serveDocumentationAsset)
	docs.Get("/legacy", func(c *fiber.Ctx) error { return c.Redirect("/docs/legacy/", fiber.StatusTemporaryRedirect) })
	docs.Get("/legacy/", documentationPage(true))
	docs.Get("/legacy/index.html", documentationPage(true))
	docs.Get("/legacy/openapi.json", serveLegacyOpenAPI)

	legacy := a.Group("/swagger", admin...)
	redirectLegacy := func(c *fiber.Ctx) error {
		return c.Redirect("/docs/legacy/", fiber.StatusPermanentRedirect)
	}
	legacy.Get("", redirectLegacy)
	legacy.Get("/*", redirectLegacy)
}
