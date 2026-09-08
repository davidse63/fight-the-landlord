// SPDX-License-Identifier: GPL-3.0-or-later
package web

import "embed"

//go:embed index.html app.js style.css assets/*
var Files embed.FS
