package api

import (
	"html/template"

	"net/http"
	"sort"
	"strconv"
	"strings"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
)

// The outcome queue: who a court actually convicted, and who was only ever a
// defendant.
//
// This is the only place in the system that may make that claim. LegalProceeding.
// has_conviction is case-level and official, but a case with ten defendants that
// ends in a conviction says nothing about which of the ten, and no source we use
// closes the gap: DataJud's public API exposes no parties, DJEN publishes names
// with no outcome. So the per-person fact is entered by a human who has read the
// decision, and a conviction cannot be saved without a link to it.
//
// Until this existed, the design said "per-defendant outcomes are set only via
// backoffice review" and there was no such page — so the claim the database exists
// to make was the one claim it could not record.

func (h *backofficeHandler) outcomesList(c *gin.Context) {
	h.renderOutcomes(c, "", "")
}

func (h *backofficeHandler) renderOutcomes(c *gin.Context, msg, errMsg string) {
	items, err := h.server.memgraph.ListProceedingsForOutcome(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load proceedings: %v", err)
		return
	}
	renderPage(c, "Case outcomes", outcomesPage(items, msg, errMsg))
}

func (h *backofficeHandler) outcomeCase(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.String(http.StatusBadRequest, "missing proceeding id")
		return
	}
	h.renderOutcomeCase(c, id, "", "")
}

func (h *backofficeHandler) renderOutcomeCase(c *gin.Context, id, msg, errMsg string) {
	defendants, err := h.server.memgraph.ListDefendants(c.Request.Context(), id)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load defendants: %v", err)
		return
	}
	renderPage(c, "Case outcomes", outcomeCasePage(id, defendants, msg, errMsg))
}

// outcomeSubmit records one defendant's outcome. Every write is stamped
// outcome_source='human' by the graph layer, and a conviction without an evidence
// URL is refused there rather than here — the rule belongs next to the write, not
// next to the form.
func (h *backofficeHandler) outcomeSubmit(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	partyID := strings.TrimSpace(c.PostForm("party_id"))
	outcome := strings.TrimSpace(c.PostForm("outcome"))
	evidence := strings.TrimSpace(c.PostForm("evidence_url"))

	if id == "" || partyID == "" {
		h.renderOutcomeCase(c, id, "", "missing proceeding or party id")
		return
	}

	err := h.server.memgraph.SetDefendantOutcome(
		c.Request.Context(), id, partyID, outcome, evidence, currentUser(c))
	if err != nil {
		h.renderOutcomeCase(c, id, "", err.Error())
		return
	}

	// Audit it: a human asserting that a named person was convicted is the most
	// consequential write in this database, and it must be attributable.
	_ = h.server.psql.LogAudit(c.Request.Context(), currentUser(c), psql.AuditActionUpdate, "defendant_outcome", partyID, map[string]any{
		"proceeding_id": id,
		"party_id":      partyID,
		"outcome":       outcome,
		"evidence_url":  evidence,
	})

	h.renderOutcomeCase(c, id, "Outcome recorded for "+partyID+".", "")
}

// ─── Pages ───────────────────────────────────────────────────────────────────

func outcomesPage(items []memgraph.ProceedingSummary, msg, errMsg string) templ.Component {
	b := strings.Builder{}
	b.WriteString(`<section class="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">`)
	writeBanners(&b, msg, errMsg)

	b.WriteString(`<h1 class="mb-1 text-xl font-semibold">Case outcomes</h1>
<p class="mb-4 max-w-3xl text-sm text-slate-600">
Who the court actually convicted. A case flagged <strong>com condenação</strong> ended in a
conviction, but that is a fact about the <em>case</em> — it never says which defendant.
No official source publishes the per-person outcome, so it is recorded here, by a
human, from the decision. A conviction requires a link to that decision.
</p>`)

	if len(items) == 0 {
		b.WriteString(`<p class="text-sm text-slate-500">No case has a defendant roster yet. DJEN case mode builds one when it polls a case.</p></section>`)
		return rawHTML(b.String())
	}

	b.WriteString(`<table class="w-full text-left text-sm">
<thead class="border-b border-slate-200 text-xs uppercase tracking-wide text-slate-500">
<tr><th class="py-2">Case</th><th>Case-level</th><th>Defendants</th><th>Recorded</th><th></th></tr>
</thead><tbody>`)

	for _, it := range items {
		label := it.CaseNumber
		if it.ClassName != "" {
			label = it.ClassName + " · " + it.CaseNumber
		}

		conviction := `<span class="text-slate-400">—</span>`
		if it.HasConviction {
			conviction = `<span class="rounded bg-rose-100 px-2 py-0.5 text-xs font-medium text-rose-800">com condenação</span>`
		}

		done := `<span class="text-slate-400">0/` + strconv.Itoa(it.Defendants) + `</span>`
		if it.Recorded > 0 {
			cls := "text-amber-700"
			if it.Recorded >= it.Defendants {
				cls = "text-emerald-700"
			}
			done = `<span class="` + cls + ` font-medium">` + strconv.Itoa(it.Recorded) + `/` + strconv.Itoa(it.Defendants) + `</span>`
		}

		b.WriteString(`<tr class="border-b border-slate-100">
<td class="py-2 pr-4"><div class="font-medium">` + template.HTMLEscapeString(label) + `</div>
<div class="text-xs text-slate-500">` + template.HTMLEscapeString(it.Court) + `</div></td>
<td class="pr-4">` + conviction + `</td>
<td class="pr-4">` + strconv.Itoa(it.Defendants) + `</td>
<td class="pr-4">` + done + `</td>
<td class="py-2"><a class="rounded bg-slate-900 px-3 py-1 text-xs font-medium text-white hover:bg-slate-700"
   href="/backoffice/outcomes/` + template.HTMLEscapeString(it.ProceedingID) + `">Record</a></td>
</tr>`)
	}
	b.WriteString(`</tbody></table></section>`)
	return rawHTML(b.String())
}

func outcomeCasePage(proceedingID string, defendants []memgraph.Defendant, msg, errMsg string) templ.Component {
	b := strings.Builder{}
	b.WriteString(`<section class="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">`)
	writeBanners(&b, msg, errMsg)

	b.WriteString(`<a href="/backoffice/outcomes" class="text-xs text-cyan-700 underline">&larr; back to cases</a>
<h1 class="mt-2 mb-1 text-xl font-semibold">` + template.HTMLEscapeString(proceedingID) + `</h1>
<p class="mb-4 max-w-3xl text-sm text-slate-600">
Record what the court decided for <em>each</em> defendant. Leave a defendant alone if
you do not know — an unrecorded outcome reads as "we have not checked", which is
true. A guess does not.
</p>`)

	if len(defendants) == 0 {
		b.WriteString(`<p class="text-sm text-slate-500">This case has no defendant roster yet.</p></section>`)
		return rawHTML(b.String())
	}

	// Stable order so the page does not reshuffle under the operator between saves.
	sort.Slice(defendants, func(i, j int) bool { return defendants[i].Name < defendants[j].Name })

	for _, d := range defendants {
		b.WriteString(`<form method="post" action="/backoffice/outcomes/` + template.HTMLEscapeString(proceedingID) + `"
   class="mb-3 rounded-lg border border-slate-200 p-4">
<input type="hidden" name="party_id" value="` + template.HTMLEscapeString(d.PartyID) + `" />
<div class="mb-2 flex items-center gap-2">
  <span class="rounded bg-slate-100 px-2 py-0.5 text-xs text-slate-600">` + template.HTMLEscapeString(d.Label) + `</span>
  <span class="font-medium">` + template.HTMLEscapeString(d.Name) + `</span>`)

		// Gate on the human stamp, not on Outcome: DJEN stamps outcome="cited" on
		// every edge it creates, so an Outcome check would claim a human had signed
		// off on every defendant the worker ever saw.
		if d.RecordedBy != "" {
			b.WriteString(`<span class="ml-auto text-xs text-slate-500">recorded by ` +
				template.HTMLEscapeString(d.RecordedBy) + ` on ` + template.HTMLEscapeString(d.RecordedAt) + `</span>`)
		} else {
			b.WriteString(`<span class="ml-auto text-xs text-slate-400">not reviewed — DJEN only saw the name</span>`)
		}
		b.WriteString(`</div>
<div class="flex flex-wrap items-center gap-2">
  <select name="outcome" class="rounded border border-slate-300 px-2 py-1 text-sm">`)

		// Deterministic order: a map would reshuffle the dropdown on every render.
		for _, key := range []string{"convicted", "acquitted", "dismissed", "indicted", "cited"} {
			sel := ""
			if d.Outcome == key {
				sel = " selected"
			}
			b.WriteString(`<option value="` + key + `"` + sel + `>` +
				template.HTMLEscapeString(memgraph.DefendantOutcome[key]) + `</option>`)
		}

		b.WriteString(`</select>
  <input type="url" name="evidence_url" placeholder="URL da decisão (obrigatório p/ condenação)"
     value="` + template.HTMLEscapeString(d.EvidenceURL) + `"
     class="min-w-80 flex-1 rounded border border-slate-300 px-2 py-1 text-sm" />
  <button type="submit" class="rounded bg-slate-900 px-3 py-1 text-sm font-medium text-white hover:bg-slate-700">Save</button>
</div></form>`)
	}

	b.WriteString(`</section>`)
	return rawHTML(b.String())
}

func writeBanners(b *strings.Builder, msg, errMsg string) {
	if errMsg != "" {
		b.WriteString(`<div class="mb-4 rounded border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-800">` +
			template.HTMLEscapeString(errMsg) + `</div>`)
	}
	if msg != "" {
		b.WriteString(`<div class="mb-4 rounded border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">` +
			template.HTMLEscapeString(msg) + `</div>`)
	}
}
