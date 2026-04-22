package api

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
)

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
          <a href="/backoffice/logs" class="hover:text-cyan-300">Worker Logs</a>
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

func dashboardPage() templ.Component {
	return rawHTML(`<section class="grid gap-4 md:grid-cols-3">
  <a href="/backoffice/seed" class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm hover:shadow">
    <h2 class="font-semibold">Seed DataJud case</h2>
    <p class="mt-2 text-sm text-slate-600">Create watcher entries from a root case.</p>
  </a>
  <a href="/backoffice/reviews" class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm hover:shadow">
    <h2 class="font-semibold">Pending reviews</h2>
    <p class="mt-2 text-sm text-slate-600">Approve or reject worker-flagged records.</p>
  </a>
  <a href="/backoffice/logs" class="rounded-xl border border-slate-200 bg-white p-5 shadow-sm hover:shadow">
    <h2 class="font-semibold">Worker logs</h2>
    <p class="mt-2 text-sm text-slate-600">Inspect recent sync/import/poll activity.</p>
  </a>
</section>`)
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
      <span class="text-sm font-medium">Scandal ID</span>
      <input class="mt-1 w-full rounded border border-slate-300 px-3 py-2" name="scandal_id" placeholder="scandal_lava_jato" value="%s" />
    </label>
    <button type="submit" class="rounded bg-slate-900 px-4 py-2 text-white hover:bg-slate-700">Seed case</button>
  </form>
</section>`, message, errMsg, template.HTMLEscapeString(v.CaseNumber), template.HTMLEscapeString(v.TribunalEndpoint), template.HTMLEscapeString(v.ScandalID)))
}

func reviewsPage(status string, items []pendingReviewView) templ.Component {
	b := strings.Builder{}
	b.WriteString(`<section class="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
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
			b.WriteString(`<tr class="align-top border-b border-slate-100">
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.Type) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.Status) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.Worker) + `</td>
    <td class="py-3 pr-3">` + template.HTMLEscapeString(item.CreatedAt) + `</td>
    <td class="py-3 pr-3"><pre class="max-w-xl whitespace-pre-wrap rounded bg-slate-50 p-2 text-xs">` + template.HTMLEscapeString(item.Payload) + `</pre></td>
    <td class="py-3 pr-3"><div class="flex gap-2">
      <form method="post" action="/backoffice/reviews/` + id + `/approve" hx-post="/backoffice/reviews/` + id + `/approve" hx-target="body" hx-swap="outerHTML"><button type="submit" class="rounded bg-emerald-600 px-2 py-1 text-xs text-white">Approve</button></form>
      <form method="post" action="/backoffice/reviews/` + id + `/reject" hx-post="/backoffice/reviews/` + id + `/reject" hx-target="body" hx-swap="outerHTML"><button type="submit" class="rounded bg-rose-600 px-2 py-1 text-xs text-white">Reject</button></form>
    </div></td>
  </tr>`)
		}
	}
	b.WriteString(`</tbody></table></div></section>`)
	return rawHTML(b.String())
}

func logsPage(items []logsView) templ.Component {
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
	return rawHTML(b.String())
}

func rawHTML(s string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, s)
		return err
	})
}
