// Package operator embeds the operator console so the API binary can
// serve it without a separate static file deployment.
package operator

import _ "embed"

// IndexPage is the operator console SPA, served at GET /operator.
//
//go:embed index.html
var IndexPage []byte

// DSStyles is the design-system stylesheet served at GET /operator/ds/styles.css.
//
//go:embed ds-styles.css
var DSStyles []byte

// DSBundle is the design-system JS bundle served at GET /operator/ds/bundle.js.
//
//go:embed ds-bundle.js
var DSBundle []byte

// React and ReactDOM are pinned, locally vendored runtime dependencies. The
// operator page handles an admin token, so no third-party script is allowed to
// execute in that origin at runtime.

//go:embed vendor/react-18.3.1.production.min.js
var React []byte

//go:embed vendor/react-dom-18.3.1.production.min.js
var ReactDOM []byte
