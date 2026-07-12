// ─── Document formatting ──────────────────────────────────────────────────────

/**
 * Format a CNPJ document (14 digits) as XX.XXX.XXX/XXXX-XX
 * Handles partial or invalid input gracefully (returns as-is if not 14 digits).
 */
export function formatCNPJ(cnpj: string): string {
  const clean = cnpj.replace(/\D/g, "")
  if (clean.length !== 14) return cnpj // Return original if not valid CNPJ length
  return `${clean.slice(0, 2)}.${clean.slice(2, 5)}.${clean.slice(5, 8)}/${clean.slice(8, 12)}-${clean.slice(12)}`
}

/**
 * Format a CPF document (11 digits) as XXX.XXX.XXX-XX
 * Handles partial or invalid input gracefully (returns as-is if not 11 digits).
 */
export function formatCPF(cpf: string): string {
  const clean = cpf.replace(/\D/g, "")
  if (clean.length !== 11) return cpf // Return original if not valid CPF length
  return `${clean.slice(0, 3)}.${clean.slice(3, 6)}.${clean.slice(6, 9)}-${clean.slice(9)}`
}

/**
 * Format a document (CNPJ or CPF) based on its length.
 * CNPJ: 14 digits, CPF: 11 digits.
 */
export function formatDocument(doc: string): string {
  const clean = doc.replace(/\D/g, "")
  if (clean.length === 14) return formatCNPJ(doc)
  if (clean.length === 11) return formatCPF(doc)
  return doc // Return original if neither valid CNPJ nor CPF
}

// A CNJ case number is stored as 20 bare digits and is unreadable that way. The
// canonical form is NNNNNNN-DD.AAAA.J.TR.OOOO — the same shape a lawyer, a court
// and a journalist all write. Anything that is not 20 digits is returned untouched
// rather than mangled into a shape it is not.
export function formatCNJ(caseNumber: string): string {
  const d = (caseNumber ?? "").replace(/\D/g, "")
  if (d.length !== 20) return caseNumber
  return `${d.slice(0, 7)}-${d.slice(7, 9)}.${d.slice(9, 13)}.${d.slice(13, 14)}.${d.slice(14, 16)}.${d.slice(16, 20)}`
}
