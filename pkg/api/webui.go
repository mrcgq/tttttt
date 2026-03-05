
package api

import "embed"

// WebUIFS embeds all static files under the webui directory tree.
// The go:embed directive recursively includes every file.
//
//go:embed webui
var WebUIFS embed.FS
