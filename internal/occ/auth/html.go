// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"html"
	"net/http"
)

const successHTML = `<!doctype html>
<meta charset="utf-8">
<title>Success: OpenChoreo CLI</title>
<style type="text/css">
body {
  color: #1B1F23;
  background: #F6F8FA;
  font-size: 14px;
  font-family: -apple-system, "Segoe UI", Helvetica, Arial, sans-serif;
  line-height: 1.5;
  max-width: 620px;
  margin: 28px auto;
  text-align: center;
}
h1 { font-size: 24px; margin-bottom: 0; }
p { margin-top: 0; }
.box {
  border: 1px solid #E1E4E8;
  background: white;
  padding: 24px;
  margin: 28px;
  border-radius: 6px;
}
.success { color: #22863a; }
</style>
<body>
  <div class="box">
    <h1 class="success">Successfully authenticated OpenChoreo CLI</h1>
    <p>You may now close this tab and return to the terminal.</p>
  </div>
</body>`

const errorHTML = `<!doctype html>
<meta charset="utf-8">
<title>Error: OpenChoreo CLI</title>
<style type="text/css">
body {
  color: #1B1F23;
  background: #F6F8FA;
  font-size: 14px;
  font-family: -apple-system, "Segoe UI", Helvetica, Arial, sans-serif;
  line-height: 1.5;
  max-width: 620px;
  margin: 28px auto;
  text-align: center;
}
h1 { font-size: 24px; margin-bottom: 0; }
p { margin-top: 0; }
.box {
  border: 1px solid #E1E4E8;
  background: white;
  padding: 24px;
  margin: 28px;
  border-radius: 6px;
}
.error { color: #cb2431; }
</style>
<body>
  <div class="box">
    <h1 class="error">Authentication Failed</h1>
    <p>%s</p>
    <p>Please close this tab and try again.</p>
  </div>
</body>`

func writeSuccessHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, successHTML)
}

func writeErrorHTML(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, errorHTML, html.EscapeString(errMsg))
}
