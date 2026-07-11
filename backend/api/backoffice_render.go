package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
)

// urlPattern matches bare http(s) URLs inside review payloads so they can be
// rendered as clickable evidence links.
var urlPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

// linkifyEscaped HTML-escapes text and turns any http(s) URL substrings into
// clickable anchors. Non-URL text is fully escaped first so the result is safe.
func linkifyEscaped(s string) string {
	var b strings.Builder
	last := 0
	for _, loc := range urlPattern.FindAllStringIndex(s, -1) {
		b.WriteString(template.HTMLEscapeString(s[last:loc[0]]))
		raw := s[loc[0]:loc[1]]
		esc := template.HTMLEscapeString(raw)
		b.WriteString(`<a href="` + esc + `" target="_blank" rel="noopener noreferrer" class="text-cyan-700 underline">` + esc + `</a>`)
		last = loc[1]
	}
	b.WriteString(template.HTMLEscapeString(s[last:]))
	return b.String()
}

// prettyPayload pretty-prints a JSON payload and linkifies any URLs (evidence
// links) inside it. Falls back to the raw (escaped, linkified) string when the
// payload is not valid JSON.
func prettyPayload(payload string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(payload), "", "  "); err != nil {
		return linkifyEscaped(payload)
	}
	return linkifyEscaped(buf.String())
}

func renderPage(c *gin.Context, title string, body templ.Component) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	layout := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>`+template.HTMLEscapeString(title)+`</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <script src="https://unpkg.com/htmx.org@1.9.12"></script>
    <link rel="preconnect" href="https://fonts.googleapis.com"/>
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous"/>
    <link href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;700&display=swap" rel="stylesheet"/>
    <style>body { font-family: 'Space Grotesk', system-ui, sans-serif; }</style>
  </head>
  <body class="bg-slate-100 text-slate-900" hx-boost="true">
    <header class="bg-slate-900 text-white">
      <div class="mx-auto flex max-w-6xl items-center justify-between px-4 py-4">
        <div class="text-lg font-semibold">corruption.center backoffice</div>
        <nav class="space-x-4 text-sm">
          <a href="/backoffice" class="hover:text-cyan-300">Home</a>
          <a href="/backoffice/seed" class="hover:text-cyan-300">Seed DataJud</a>
          <a href="/backoffice/reviews" class="hover:text-cyan-300">Pending Reviews</a>
          <a href="/backoffice/removals" class="hover:text-cyan-300">Removals</a>
          <a href="/backoffice/logs" class="hover:text-cyan-300">Worker Logs &amp; Audit</a>
        </nav>
      </div>
    </header>
    <main class="mx-auto max-w-6xl px-4 py-8">`)
		if err != nil {
			return err
		}
		if err := body.Render(ctx, w); err != nil {
			return err
		}
		_, err = io.WriteString(w, `</main>
  </body>
</html>`)
		return err
	})
	c.Status(http.StatusOK)
	_ = layout.Render(c.Request.Context(), c.Writer)
}

func dashboardPage(counts []reviewTypeCountView, total int) templ.Component {
	b := strings.Builder{}
	b.WriteString(`<section class="grid gap-4 md:grid-cols-4">
  <a href="/backoffice/seed" class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm hover:shadow">
    <h2 class="font-semibold">Seed DataJud case</h2>
    <p class="mt-2 text-sm text-slate-600">Create watcher entries from a root case.</p>
  </a>
  <a href="/backoffice/reviews" class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm hover:shadow">
    <h2 class="font-semibold">Pending reviews</h2>
    <p class="mt-2 text-sm text-slate-600">Approve or reject worker-flagged records.</p>
  </a>
  <a href="/backoffice/removals" class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm hover:shadow">
    <h2 class="font-semibold">Removal requests</h2>
    <p class="mt-2 text-sm text-slate-600">LGPD data-removal queue and Person purge.</p>
  </a>
  <a href="/backoffice/logs" class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm hover:shadow">
    <h2 class="font-semibold">Worker logs &amp; audit</h2>
    <p class="mt-2 text-sm text-slate-600">Recent activity and the who/what/when audit trail.</p>
  </a>
</section>`)

	b.WriteString(`<section class="mt-6 rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
  <div class="flex items-baseline justify-between">
    <h2 class="text-lg font-semibold">Pending review backlog</h2>
    <span class="text-sm text-slate-600">` + strconv.Itoa(total) + ` pending</span>
  </div>`)
	if len(counts) == 0 {
		b.WriteString(`<p class="mt-3 text-sm text-slate-500">No pending reviews.</p>`)
	} else {
		b.WriteString(`<div class="mt-4 flex flex-wrap gap-3">`)
		for _, ct := range counts {
			t := template.HTMLEscapeString(ct.Type)
			b.WriteString(`<a href="/backoffice/reviews?status=pending&amp;type=` + template.HTMLEscapeString(url.QueryEscape(ct.Type)) + `" class="flex items-center gap-2 rounded-full border border-slate-300 bg-slate-50 px-3 py-1 text-sm hover:bg-slate-100">
      <span>` + t + `</span>
      <span class="rounded-full bg-slate-900 px-2 text-xs font-semibold text-white">` + strconv.Itoa(ct.Count) + `</span>
    </a>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</section>`)
	return rawHTML(b.String())
}

// scandalSelector renders a <select> populated from real Scandal nodes plus a
// free-text input for registering a case against a brand-new scandal id. The
// backend prefers new_scandal_id over scandal_id (see resolveScandalID).
func scandalSelector(scandals []memgraph.ScandalOption, selectedID, compact string) string {
	selCls := "mt-1 w-full rounded border border-slate-300 px-3 py-2"
	newCls := "mt-1 w-full rounded border border-slate-300 px-3 py-2"
	if compact == "compact" {
		selCls = "w-44 rounded border border-slate-300 px-2 py-1 text-xs"
		newCls = "w-40 rounded border border-slate-300 px-2 py-1 text-xs"
	}
	b := strings.Builder{}
	b.WriteString(`<select name="scandal_id" class="` + selCls + `">`)
	b.WriteString(`<option value="">— select a scandal —</option>`)
	for _, s := range scandals {
		label := s.Name
		if strings.TrimSpace(label) == "" {
			label = s.ID
		}
		sel := ""
		if s.ID == selectedID {
			sel = " selected"
		}
		b.WriteString(`<option value="` + template.HTMLEscapeString(s.ID) + `"` + sel + `>` + template.HTMLEscapeString(label) + `</option>`)
	}
	b.WriteString(`</select>`)
	b.WriteString(`<input name="new_scandal_id" placeholder="…or new scandal id (e.g. scandal_x)" class="` + newCls + `" />`)
	b.WriteString(`<input name="scandal_name" placeholder="Scandal display name (new scandals only)" class="` + newCls + `" />`)
	return b.String()
}

func seedCasePage(v seedCaseView) templ.Component {
	message := ""
	if v.Message != "" {
		message = `<div class="mt-4 rounded border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">` + template.HTMLEscapeString(v.Message) + `</div>`
	}
	errMsg := ""
	if v.Error != "" {
		errMsg = `<div class="mt-4 rounded border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-800">` + template.HTMLEscapeString(v.Error) + `</div>`
	}
	return rawHTML(fmt.Sprintf(`<section class="max-w-2xl rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
  <h1 class="text-xl font-semibold">Seed DataJud watcher case</h1>
  <p class="mt-2 text-sm text-slate-600">This creates/updates a LegalProceeding node and registers it in watcher_tracking.</p>
  %s
  %s
  <form method="post" action="/backoffice/seed" class="mt-6 space-y-4" hx-post="/backoffice/seed" hx-target="body" hx-swap="outerHTML">
    <label class="block">
      <span class="text-sm font-medium">Case number</span>
      <input class="mt-1 w-full rounded border border-slate-300 px-3 py-2" name="case_number" placeholder="5046512-94.2016.4.04.7000" value="%s" />
    </label>
    <label class="block">
      <span class="text-sm font-medium">Tribunal endpoint</span>
      <input class="mt-1 w-full rounded border border-slate-300 px-3 py-2" name="tribunal_endpoint" placeholder="api_publica_trf4" value="%s" />
    </label>
    <label class="block">
      <span class="text-sm font-medium">Scandal</span>
      %s
    </label>
    <button type="submit" class="rounded bg-slate-900 px-4 py-2 text-white hover:bg-slate-700">Seed case</button>
  </form>
</section>`, message, errMsg, template.HTMLEscapeString(v.CaseNumber), template.HTMLEscapeString(v.TribunalEndpoint), scandalSelector(v.Scandals, v.ScandalID, "")))
}

func reviewsPage(status, typ string, items []pendingReviewView, scandals []memgraph.ScandalOption, errMsg string) templ.Component {
	b := strings.Builder{}
	b.WriteString(`<section class="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">`)
	if errMsg != "" {
		b.WriteString(`<div class="mb-4 rounded border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-800">` + template.HTMLEscapeString(errMsg) + `</div>`)
	}
	b.WriteString(`
  <div class="flex items-end justify-between gap-3">
    <h1 class="text-xl font-semibold">Pending reviews</h1>
    <form method="get" action="/backoffice/reviews" class="flex items-center gap-2 text-sm" hx-get="/backoffice/reviews" hx-target="body" hx-swap="outerHTML">
      <label>Status</label>
      <select name="status" class="rounded border border-slate-300 px-2 py-1">`)
	for _, candidate := range []string{"", "pending", "approved", "rejected", "deferred"} {
		label := candidate
		if candidate == "" {
			label = "all"
		}
		selected := ""
		if status == candidate {
			selected = " selected"
		}
		b.WriteString(`<option value="` + candidate + `"` + selected + `>` + template.HTMLEscapeString(label) + `</option>`)
	}
	b.WriteString(`</select>
      <label>Type</label>
      <select name="type" class="rounded border border-slate-300 px-2 py-1">`)
	reviewTypes := []string{"", "unknown_cpf", "unknown_cnpj", "cpf_partial_match", "possible_politician_in_qsa", "scandal_cluster", "unlinked_spinoff", reviewTypeDJENCandidate}
	// Ensure the active type is selectable even if it is not in the known set.
	known := false
	for _, t := range reviewTypes {
		if t == typ {
			known = true
			break
		}
	}
	if !known && typ != "" {
		reviewTypes = append(reviewTypes, typ)
	}
	for _, candidate := range reviewTypes {
		label := candidate
		if candidate == "" {
			label = "all"
		}
		selected := ""
		if typ == candidate {
			selected = " selected"
		}
		b.WriteString(`<option value="` + template.HTMLEscapeString(candidate) + `"` + selected + `>` + template.HTMLEscapeString(label) + `</option>`)
	}
	b.WriteString(`</select>
      <button type="submit" class="rounded bg-slate-900 px-3 py-1 text-white">Filter</button>
    </form>
  </div>
  <div class="mt-4 overflow-x-auto">
    <table class="w-full min-w-[900px] text-sm">
      <thead>
        <tr class="border-b border-slate-200 text-left text-slate-600">
          <th class="py-2 pr-3">Type</th>
          <th class="py-2 pr-3">Status</th>
          <th class="py-2 pr-3">Worker</th>
          <th class="py-2 pr-3">Created</th>
          <th class="py-2 pr-3">Payload</th>
          <th class="py-2 pr-3">Actions</th>
        </tr>
      </thead>
      <tbody>`)
	if len(items) == 0 {
		b.WriteString(`<tr><td colspan="6" class="py-6 text-slate-500">No items found.</td></tr>`)
	} else {
		for _, item := range items {
			id := template.HTMLEscapeString(item.ID)
			approveForm := `<form method="post" action="/backoffice/reviews/` + id + `/approve" hx-post="/backoffice/reviews/` + id + `/approve" hx-target="body" hx-swap="outerHTML"><button type="submit" class="rounded bg-emerald-600 px-2 py-1 text-xs text-white">Approve</button></form>`
			if item.IsDJENCandidate {
				// DJEN case candidates need a scandal to link the proceeding to.
				// Use a dropdown of real Scandal nodes plus a new-id fallback.
				approveForm = `<form method="post" action="/backoffice/reviews/` + id + `/approve" hx-post="/backoffice/reviews/` + id + `/approve" hx-target="body" hx-swap="outerHTML" class="flex flex-col gap-1">
        ` + scandalSelector(scandals, "", "compact") + `
        <button type="submit" class="rounded bg-emerald-600 px-2 py-1 text-xs text-white">Approve</button>
      </form>`
			}
			// Rejections capture a reason so the watcher does not re-flag the same
			// false match (LGPD "pending review discipline").
			rejectForm := `<form method="post" action="/backoffice/reviews/` + id + `/reject" hx-post="/backoffice/reviews/` + id + `/reject" hx-target="body" hx-swap="outerHTML" class="flex flex-col gap-1">
        <input name="reason" placeholder="reject reason" class="w-40 rounded border border-slate-300 px-2 py-1 text-xs" />
        <button type="submit" class="rounded bg-rose-600 px-2 py-1 text-xs text-white">Reject</button>
      </form>`
			b.WriteString(`<tr class="align-top border-b border-slate-100">
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.Type) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.Status) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.Worker) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.CreatedAt) + `</td>
    <td class="py-3 pr-3"><pre class="max-w-xl whitespace-pre-wrap rounded bg-slate-50 p-2 text-xs">` + prettyPayload(item.Payload) + `</pre></td>
    <td class="py-3 pr-3"><div class="flex flex-wrap items-start gap-2">
      ` + approveForm + rejectForm + `
    </div></td>
  </tr>`)
		}
	}
	b.WriteString(`</tbody></table></div></section>`)
	return rawHTML(b.String())
}

func logsPage(items []logsView, audit []auditView, filter psql.AuditFilter) templ.Component {
	b := strings.Builder{}
	b.WriteString(`<section class="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
  <h1 class="text-xl font-semibold">Worker logs</h1>
  <p class="mt-2 text-sm text-slate-600">Combined view from scraper jobs, TSE/Camara/Senado logs, and DataJud poll timestamps.</p>
  <div class="mt-4 overflow-x-auto"><table class="w-full min-w-[900px] text-sm">
    <thead>
      <tr class="border-b border-slate-200 text-left text-slate-600">
        <th class="py-2 pr-3">Source</th>
        <th class="py-2 pr-3">Status</th>
        <th class="py-2 pr-3">Started</th>
        <th class="py-2 pr-3">Finished</th>
        <th class="py-2 pr-3">Records</th>
        <th class="py-2 pr-3">Details</th>
        <th class="py-2 pr-3">Error</th>
      </tr>
    </thead>
    <tbody>`)
	if len(items) == 0 {
		b.WriteString(`<tr><td colspan="7" class="py-6 text-slate-500">No logs yet.</td></tr>`)
	} else {
		for _, item := range items {
			b.WriteString(`<tr class="align-top border-b border-slate-100">
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.Source) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.Status) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.StartedAt) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.FinishedAt) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(strconv.Itoa(item.RecordsUpserted)) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.Details) + `</td>
    <td class="py-3 pr-3 text-rose-700">` + template.HTMLEscapeString(item.ErrorMessage) + `</td>
  </tr>`)
		}
	}
	b.WriteString(`</tbody></table></div></section>`)

	// ─── Audit log (LGPD who/what/when) ───────────────────────────────────────
	b.WriteString(`<section class="mt-6 rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
  <h1 class="text-xl font-semibold">Audit log</h1>
  <p class="mt-2 text-sm text-slate-600">Who created, modified or deleted each node/edge/review, and when. This is the LGPD accountability trail.</p>
  <form method="get" action="/backoffice/logs" class="mt-4 flex flex-wrap items-end gap-2 text-sm" hx-get="/backoffice/logs" hx-target="body" hx-swap="outerHTML">
    <label class="flex flex-col">Actor
      <input name="actor" value="` + template.HTMLEscapeString(filter.ActorID) + `" placeholder="operator" class="mt-1 rounded border border-slate-300 px-2 py-1" /></label>
    <label class="flex flex-col">Action
      <select name="action" class="mt-1 rounded border border-slate-300 px-2 py-1">`)
	for _, a := range []string{"", "create", "update", "delete"} {
		label := a
		if a == "" {
			label = "all"
		}
		sel := ""
		if filter.Action == a {
			sel = " selected"
		}
		b.WriteString(`<option value="` + a + `"` + sel + `>` + label + `</option>`)
	}
	b.WriteString(`</select></label>
    <label class="flex flex-col">Target type
      <input name="target_type" value="` + template.HTMLEscapeString(filter.TargetType) + `" placeholder="graph_node, pending_review…" class="mt-1 rounded border border-slate-300 px-2 py-1" /></label>
    <button type="submit" class="rounded bg-slate-900 px-3 py-1 text-white">Filter</button>
  </form>
  <div class="mt-4 overflow-x-auto"><table class="w-full min-w-[900px] text-sm">
    <thead>
      <tr class="border-b border-slate-200 text-left text-slate-600">
        <th class="py-2 pr-3">When</th>
        <th class="py-2 pr-3">Actor</th>
        <th class="py-2 pr-3">Action</th>
        <th class="py-2 pr-3">Target type</th>
        <th class="py-2 pr-3">Target id</th>
        <th class="py-2 pr-3">Metadata</th>
      </tr>
    </thead>
    <tbody>`)
	if len(audit) == 0 {
		b.WriteString(`<tr><td colspan="6" class="py-6 text-slate-500">No audit entries.</td></tr>`)
	} else {
		for _, a := range audit {
			b.WriteString(`<tr class="align-top border-b border-slate-100">
    <td class="py-3 pr-3 whitespace-nowrap">` + template.HTMLEscapeString(a.CreatedAt) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(a.ActorID) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(a.Action) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(a.TargetType) + `</td>
    <td class="py-3 pr-3 font-mono text-xs">` + template.HTMLEscapeString(a.TargetID) + `</td>
    <td class="py-3 pr-3"><pre class="max-w-md whitespace-pre-wrap rounded bg-slate-50 p-2 text-xs">` + template.HTMLEscapeString(a.Metadata) + `</pre></td>
  </tr>`)
		}
	}
	b.WriteString(`</tbody></table></div></section>`)
	return rawHTML(b.String())
}

func removalsPage(status string, items []removalRequestView, msg, errMsg string) templ.Component {
	b := strings.Builder{}
	b.WriteString(`<section class="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">`)
	if msg != "" {
		b.WriteString(`<div class="mb-4 rounded border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">` + template.HTMLEscapeString(msg) + `</div>`)
	}
	if errMsg != "" {
		b.WriteString(`<div class="mb-4 rounded border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-800">` + template.HTMLEscapeString(errMsg) + `</div>`)
	}
	b.WriteString(`<h1 class="text-xl font-semibold">Data removal requests</h1>
  <p class="mt-2 text-sm text-slate-600">LGPD art. 18 queue. Politicians are public officials (LGPD art. 23) and are never purgeable — a purge against a Politician node is refused in code and must be closed as <em>rejected</em> with a documented justification.</p>`)

	// New-request form.
	b.WriteString(`<form method="post" action="/backoffice/removals" class="mt-6 grid gap-3 md:grid-cols-2" hx-post="/backoffice/removals" hx-target="body" hx-swap="outerHTML">
    <label class="block"><span class="text-sm font-medium">Requester (identity / email)</span>
      <input name="requester" required class="mt-1 w-full rounded border border-slate-300 px-3 py-2" /></label>
    <label class="block"><span class="text-sm font-medium">Target type</span>
      <input name="target_type" required placeholder="Person / Organization / edge" class="mt-1 w-full rounded border border-slate-300 px-3 py-2" /></label>
    <label class="block"><span class="text-sm font-medium">Target node/edge id</span>
      <input name="target_id" required placeholder="person_djen_fulano_de_tal" class="mt-1 w-full rounded border border-slate-300 px-3 py-2" /></label>
    <label class="block"><span class="text-sm font-medium">Reason (optional)</span>
      <input name="reason" class="mt-1 w-full rounded border border-slate-300 px-3 py-2" /></label>
    <div><button type="submit" class="rounded bg-slate-900 px-4 py-2 text-white hover:bg-slate-700">Record request</button></div>
  </form>`)

	// Status filter.
	b.WriteString(`<form method="get" action="/backoffice/removals" class="mt-6 flex items-center gap-2 text-sm" hx-get="/backoffice/removals" hx-target="body" hx-swap="outerHTML">
    <label>Status</label>
    <select name="status" class="rounded border border-slate-300 px-2 py-1">`)
	for _, candidate := range []string{"", "pending", "resolved", "rejected"} {
		label := candidate
		if candidate == "" {
			label = "all"
		}
		sel := ""
		if status == candidate {
			sel = " selected"
		}
		b.WriteString(`<option value="` + candidate + `"` + sel + `>` + label + `</option>`)
	}
	b.WriteString(`</select>
    <button type="submit" class="rounded bg-slate-900 px-3 py-1 text-white">Filter</button>
  </form>`)

	b.WriteString(`<div class="mt-4 space-y-4">`)
	if len(items) == 0 {
		b.WriteString(`<p class="py-6 text-slate-500">No removal requests.</p>`)
	}
	for _, it := range items {
		id := template.HTMLEscapeString(it.ID)
		b.WriteString(`<div class="rounded-lg border border-slate-200 p-4">
      <div class="flex flex-wrap items-baseline justify-between gap-2">
        <div class="text-sm"><span class="font-semibold">` + template.HTMLEscapeString(it.TargetType) + `</span>
          <span class="font-mono text-xs text-slate-600">` + template.HTMLEscapeString(it.TargetID) + `</span></div>
        <span class="rounded-full bg-slate-100 px-2 py-0.5 text-xs">` + template.HTMLEscapeString(it.Status) + `</span>
      </div>
      <div class="mt-1 text-xs text-slate-600">from ` + template.HTMLEscapeString(it.Requester) + ` · received ` + template.HTMLEscapeString(it.ReceivedAt) + `</div>`)
		if it.Reason != "" {
			b.WriteString(`<div class="mt-2 text-sm">Reason: ` + template.HTMLEscapeString(it.Reason) + `</div>`)
		}
		// Provenance (creation reason) of the targeted node.
		if it.ProvErr != "" {
			b.WriteString(`<div class="mt-2 rounded bg-amber-50 px-2 py-1 text-xs text-amber-800">provenance lookup failed: ` + template.HTMLEscapeString(it.ProvErr) + `</div>`)
		} else if it.Provenance == nil {
			b.WriteString(`<div class="mt-2 rounded bg-slate-50 px-2 py-1 text-xs text-slate-600">targeted node not found in the graph (may already be purged)</div>`)
		} else {
			p := it.Provenance
			polBadge := ""
			if p.IsPolitician {
				polBadge = ` <span class="rounded bg-rose-100 px-2 py-0.5 text-xs font-semibold text-rose-800">POLITICIAN — not purgeable (LGPD art. 23)</span>`
			}
			b.WriteString(`<div class="mt-2 rounded bg-slate-50 px-3 py-2 text-xs">
        <div><span class="font-semibold">` + template.HTMLEscapeString(p.Label) + `</span> ` + template.HTMLEscapeString(p.Name) + polBadge + `</div>
        <div class="mt-1 text-slate-600">Creation reason: ` + template.HTMLEscapeString(p.CreationReason) + `</div>`)
			if p.Link != "" {
				b.WriteString(`<div class="mt-1">Source: ` + linkifyEscaped(p.Link) + `</div>`)
			}
			b.WriteString(`<div class="mt-1 text-slate-500">` + strconv.Itoa(p.EdgeCount) + ` edge(s)</div></div>`)
		}

		if it.IsPending {
			// Purge is offered only for non-Politician nodes; politicians must be
			// closed as rejected.
			purgeBtn := ""
			if !it.IsPolitician {
				purgeBtn = `<form method="post" action="/backoffice/removals/` + id + `/resolve" hx-post="/backoffice/removals/` + id + `/resolve" hx-target="body" hx-swap="outerHTML" onsubmit="return confirm('Purge this node and ALL its edges? This cannot be undone.');" class="flex items-center gap-1">
          <input type="hidden" name="action" value="purge" />
          <input name="resolution" placeholder="resolution note" class="w-52 rounded border border-slate-300 px-2 py-1 text-xs" />
          <button type="submit" class="rounded bg-rose-600 px-3 py-1 text-xs text-white">Purge node + edges</button>
        </form>`
			}
			b.WriteString(`<div class="mt-3 flex flex-wrap items-end gap-3">` + purgeBtn + `
        <form method="post" action="/backoffice/removals/` + id + `/resolve" hx-post="/backoffice/removals/` + id + `/resolve" hx-target="body" hx-swap="outerHTML" class="flex items-center gap-1">
          <input type="hidden" name="action" value="reject" />
          <input name="resolution" placeholder="why refused" class="w-52 rounded border border-slate-300 px-2 py-1 text-xs" />
          <button type="submit" class="rounded bg-slate-700 px-3 py-1 text-xs text-white">Reject request</button>
        </form>
        <form method="post" action="/backoffice/removals/` + id + `/resolve" hx-post="/backoffice/removals/` + id + `/resolve" hx-target="body" hx-swap="outerHTML" class="flex items-center gap-1">
          <input type="hidden" name="action" value="manual" />
          <input name="resolution" placeholder="resolution note" class="w-52 rounded border border-slate-300 px-2 py-1 text-xs" />
          <button type="submit" class="rounded bg-slate-500 px-3 py-1 text-xs text-white">Mark resolved</button>
        </form>
      </div>`)
		} else {
			b.WriteString(`<div class="mt-2 text-xs text-slate-600">Resolution: ` + template.HTMLEscapeString(it.Resolution) + ` · by ` + template.HTMLEscapeString(it.ResolvedBy) + ` · ` + template.HTMLEscapeString(it.ResolvedAt) + `</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></section>`)
	return rawHTML(b.String())
}

func rawHTML(s string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}
