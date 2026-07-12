import Link from "next/link";

// Every notFound() — a politician id that does not exist, a scandal that was
// removed under LGPD — used to land on Next's stock English page ("This page
// could not be found") on an otherwise pt-BR site. A removed record is exactly
// the moment a reader deserves an explanation rather than a dead end, so this
// says which of the two happened as far as we can honestly tell: we cannot
// distinguish a typo from a purge, and we say so instead of guessing.
export default function NotFound() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 px-6 text-center">
      <p className="font-mono text-sm text-text-dim">404</p>
      <h1 className="font-serif text-3xl">Registro não encontrado</h1>
      <p className="max-w-md text-sm leading-relaxed text-text-dim">
        Este endereço não corresponde a nenhum registro. Ou o identificador está
        incorreto, ou o registro foi removido — inclusive por solicitação de
        remoção de dados pessoais, que atendemos.
      </p>
      <div className="flex flex-wrap items-center justify-center gap-3 text-sm">
        <Link
          href="/"
          className="rounded border border-border px-4 py-2 hover:bg-surface"
        >
          Voltar ao grafo
        </Link>
        <Link
          href="/politicos"
          className="rounded border border-border px-4 py-2 hover:bg-surface"
        >
          Explorar políticos
        </Link>
        <Link
          href="/metodologia"
          className="rounded border border-border px-4 py-2 hover:bg-surface"
        >
          Metodologia e fontes
        </Link>
      </div>
    </main>
  );
}
