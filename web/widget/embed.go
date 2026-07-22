// Package widget embeds the single-script chat widget so the API binary can
// serve it without a separate static file deployment.
package widget

import _ "embed"

// Script is the embeddable widget, served at GET /widget/v1/kontor.js.
//
//go:embed kontor.js
var Script []byte

// DemoPage is a minimal host page for trying the widget locally, served at
// GET /widget/v1/demo.
//
//go:embed demo.html
var DemoPage []byte
