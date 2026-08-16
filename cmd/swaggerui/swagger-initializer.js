/*
 * Served in place of the one github.com/swaggo/files ships.
 *
 * Theirs exists and is reachable, but points at petstore.swagger.io — it is the upstream
 * demo initialiser, not a configurable one. This is the same file with the URL pointed at
 * the spec this binary actually serves, which the handler renders at ./doc.json from the
 * // @... annotations in cmd/main.go.
 *
 * Being a file rather than an inline block is the entire point: script-src 'self' allows it.
 */
window.onload = function () {
  window.ui = SwaggerUIBundle({
    url: './doc.json',
    dom_id: '#swagger-ui',
    deepLinking: true,
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
    plugins: [SwaggerUIBundle.plugins.DownloadUrl],
    layout: 'StandaloneLayout',
  });
};
