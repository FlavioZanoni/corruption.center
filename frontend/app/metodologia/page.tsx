import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Metodologia e Aviso Legal — corruption.center",
  description:
    "Como o corruption.center é construído: fontes oficiais, revisão humana, direito de remoção (LGPD) e limitações dos dados.",
};

const CONTACT_EMAIL = "contato@corruption.center";

type Source = {
  name: string;
  what: string;
  href: string;
};

const SOURCES: Source[] = [
  {
    name: "TSE — Tribunal Superior Eleitoral (Dados Abertos)",
    what: "Candidaturas, mandatos, filiações partidárias e dados eleitorais.",
    href: "https://dadosabertos.tse.jus.br/",
  },
  {
    name: "Câmara dos Deputados (Dados Abertos)",
    what: "Deputados federais, mandatos, partidos e atividade parlamentar.",
    href: "https://dadosabertos.camara.leg.br/",
  },
  {
    name: "Senado Federal (Dados Abertos)",
    what: "Senadores, mandatos e atividade parlamentar.",
    href: "https://legis.senado.leg.br/dadosabertos/",
  },
  {
    name: "CNJ — DataJud (API Pública)",
    what: "Processos judiciais e movimentações processuais dos tribunais brasileiros.",
    href: "https://www.cnj.jus.br/sistemas/datajud/api-publica/",
  },
  {
    name: "CNJ — DJEN (Diário de Justiça Eletrônico Nacional / API Comunica)",
    what: "Comunicações e intimações judiciais oficiais publicadas pelos tribunais.",
    href: "https://comunica.pje.jus.br/",
  },
  {
    name: "CGU — Portal da Transparência",
    what: "Sanções, acordos, recursos públicos e beneficiários de programas federais.",
    href: "https://portaldatransparencia.gov.br/",
  },
  {
    name: "TCU — Tribunal de Contas da União (Dados Abertos)",
    what: "Condenações, inabilitações e responsabilizações em contas públicas.",
    href: "https://portal.tcu.gov.br/dados-abertos/",
  },
  {
    name: "Receita Federal — CNPJ (via Minha Receita / CNPJ.ws)",
    what: "Dados cadastrais de empresas e quadro societário (QSA) a partir do CNPJ público.",
    href: "https://minhareceita.org/",
  },
  {
    name: "Wikimedia Commons",
    what: "Fotografias de agentes públicos, sempre com atribuição e licença informadas.",
    href: "https://commons.wikimedia.org/",
  },
];

function SectionTitle({ n, children }: { n: string; children: React.ReactNode }) {
  return (
    <h2 className="mt-14 mb-4 font-serif text-2xl font-semibold text-text">
      <span className="mr-3 font-mono text-base text-accent-yellow">{n}</span>
      {children}
    </h2>
  );
}

export default function MetodologiaPage() {
  return (
    <main className="h-screen overflow-y-auto bg-bg text-text">
      <div className="mx-auto max-w-3xl px-6 py-16">
        <nav className="mb-10 flex items-center justify-between text-sm">
          <Link href="/" className="font-mono text-text-muted hover:text-text">
            ← corruption.center
          </Link>
          <span className="font-mono text-xs uppercase tracking-widest text-text-dim">
            Metodologia
          </span>
        </nav>

        <header>
          <h1 className="font-serif text-4xl font-bold leading-tight text-text">
            Metodologia e Aviso Legal
          </h1>
          <p className="mt-4 text-lg text-text-muted">
            Como esta ferramenta é construída, de onde vêm os dados, o que
            significam os rótulos exibidos e como exercer o seu direito de
            remoção.
          </p>
        </header>

        {/* 1 — O que é o projeto */}
        <SectionTitle n="01">O que é o corruption.center</SectionTitle>
        <div className="space-y-4 text-text-muted">
          <p>
            O corruption.center é uma ferramenta de transparência construída
            <strong className="text-text"> exclusivamente a partir de registros
            públicos oficiais brasileiros</strong>, com finalidade de interesse
            público e prestação de contas (accountability).
          </p>
          <p>
            O tratamento de dados aqui realizado se apoia no{" "}
            <strong className="text-text">art. 23 da Lei Geral de Proteção de
            Dados (LGPD, Lei nº 13.709/2018)</strong>, que autoriza o tratamento
            de dados pessoais pelo poder público — e, por extensão, o uso de
            dados já tornados públicos por órgãos oficiais — para o atendimento
            de finalidade pública, no exercício de suas competências ou em
            cumprimento de suas atribuições legais.
          </p>
          <p>
            O projeto <strong className="text-text">não coleta dados de
            front-ends de tribunais nem de agregadores de terceiros</strong>:
            todo dado é obtido diretamente de serviços oficiais de dados abertos
            do governo brasileiro, listados abaixo. O projeto é
            <strong className="text-text"> não comercial</strong>.
          </p>
        </div>

        {/* 2 — Fontes */}
        <SectionTitle n="02">De onde vem cada tipo de dado</SectionTitle>
        <p className="mb-6 text-text-muted">
          Cada informação exibida remonta a uma das fontes oficiais abaixo. São
          todos serviços públicos de dados abertos.
        </p>
        <ul className="space-y-4">
          {SOURCES.map((s) => (
            <li
              key={s.href}
              className="rounded-lg border border-border bg-surface p-4"
            >
              <a
                href={s.href}
                target="_blank"
                rel="noopener noreferrer"
                className="font-medium text-text underline decoration-text-dim underline-offset-4 hover:decoration-text"
              >
                {s.name}
              </a>
              <p className="mt-1 text-sm text-text-muted">{s.what}</p>
              <p className="mt-1 break-all font-mono text-xs text-text-dim">
                {s.href}
              </p>
            </li>
          ))}
        </ul>
        <p className="mt-4 text-sm text-text-muted">
          <strong className="text-text">Fotografias:</strong> imagens de agentes
          públicos são obtidas do Wikimedia Commons e exibidas com a devida
          atribuição de autoria e licença. Caso a licença de uma imagem não
          possa ser confirmada, a imagem não é utilizada.
        </p>

        {/* 3 — Pendente de confirmação */}
        <SectionTitle n="03">
          Como identificamos uma pessoa (e o índice de identificação)
        </SectionTitle>
        <div className="space-y-4 text-text-muted">
          <p>
            As fontes oficiais são confiáveis quanto{" "}
            <strong className="text-text">ao que aconteceu</strong>, mas com
            frequência silenciam quanto a{" "}
            <strong className="text-text">com quem aconteceu</strong>. O DJEN
            publica nomes de partes sem nenhum documento; a CGU divulga o CPF
            mascarado (<span className="font-mono">***.435.151-**</span>). Ligar
            um registro a um político específico é, portanto, uma inferência
            nossa — e não um fato afirmado pela fonte. Homônimos são abundantes:
            uma única busca por um nome comum no DJEN retorna milhares de
            publicações de pessoas distintas.
          </p>
          <p>
            Por isso cada vínculo carrega um{" "}
            <strong className="text-text">índice de identificação</strong>, que
            pondera as evidências e determina como o vínculo é criado:
          </p>
          <ul className="ml-5 list-disc space-y-2">
            <li>
              <strong className="text-text">CPF ou CNPJ completo na fonte</strong>{" "}
              (100%) — identificação determinística. O vínculo é criado
              automaticamente.
            </li>
            <li>
              <strong className="text-text">
                CPF parcial da fonte + nome idêntico
              </strong>{" "}
              (95%) — o vínculo é criado automaticamente, exibindo as evidências.
            </li>
            <li>
              <strong className="text-text">
                Apenas o nome, sem nenhum documento
              </strong>{" "}
              (no máximo 35%) — <strong className="text-text">nunca</strong> gera
              vínculo automático. Vai para uma{" "}
              <strong className="text-text">fila de revisão humana</strong> e só
              é exibido após aprovação manual. Um nome, por mais exato que seja,
              é uma pista — jamais uma identificação.
            </li>
          </ul>
          <p>
            O limite para criação automática é de 90%, calibrado de modo que{" "}
            <strong className="text-text">
              nenhuma combinação de evidências baseadas apenas em nome consiga
              alcançá-lo
            </strong>
            . Evidência que sirva a mais de uma pessoa é rebaixada e enviada para
            revisão humana.
          </p>
          <p>
            Cada vínculo exibe como foi estabelecido — “identificação: 95%”, com
            as evidências, ou “confirmado por revisão humana” — além da{" "}
            <strong className="text-text">fonte oficial de origem</strong>: um
            número de processo do DataJud, um registro da API da Câmara, uma
            sanção do Portal da Transparência, e assim por diante.
          </p>
          <p>
            Registrar um <em>processo</em> para acompanhamento não afirma nada
            sobre nenhuma pessoa e é feito automaticamente. A afirmação “este
            político é réu neste processo” é que depende das regras acima.
          </p>
        </div>

        {/* 4 — Remoção */}
        <SectionTitle n="04">Como solicitar a remoção de dados</SectionTitle>
        <div className="space-y-4 text-text-muted">
          <p>
            Você pode solicitar a remoção ou a correção de dados pessoais
            enviando um e-mail para:
          </p>
          <p>
            <a
              href={`mailto:${CONTACT_EMAIL}`}
              className="inline-block rounded-lg border border-border bg-surface px-4 py-2 font-mono text-text underline decoration-text-dim underline-offset-4 hover:decoration-text"
            >
              {CONTACT_EMAIL}
            </a>
          </p>
          <p>
            Descreva, na medida do possível, qual registro ou vínculo deseja que
            seja revisto. As solicitações são{" "}
            <strong className="text-text">processadas em até 15 dias</strong>,
            prazo adotado em conformidade com a LGPD (art. 18) e com a prática
            recomendada pela ANPD.
          </p>
          <p className="rounded-lg border border-border bg-surface p-4 text-sm">
            <strong className="text-text">Agentes públicos.</strong> Dados de
            políticos e demais agentes públicos, no exercício de suas funções,
            são tratados com fundamento no interesse público e na prestação de
            contas (LGPD, art. 23) e, por essa razão, não são removidos do
            grafo. Ainda assim, correções de erros factuais são sempre
            bem-vindas pelo mesmo canal.
          </p>
        </div>

        {/* 5 — Limitações */}
        <SectionTitle n="05">Limitações e isenção de responsabilidade</SectionTitle>
        <div className="space-y-4 text-text-muted">
          <p>
            Os dados aqui apresentados{" "}
            <strong className="text-text">refletem registros oficiais</strong> e
            podem conter erros originados na própria fonte. O projeto{" "}
            <strong className="text-text">não adiciona nem infere informações
            além do que consta nos registros oficiais</strong>.
          </p>
          <p>
            A existência de um processo, de uma citação ou de um vínculo
            societário <strong className="text-text">não implica culpa,
            condenação ou irregularidade</strong>. Situações processuais mudam
            ao longo do tempo, e este site pode não refletir o estado mais
            atual de um registro. Em caso de divergência,{" "}
            <strong className="text-text">prevalece sempre a fonte oficial</strong>.
          </p>
          <p>
            Para qualquer dúvida, correção ou solicitação, utilize o contato
            indicado na seção 04.
          </p>
        </div>

        <footer className="mt-16 border-t border-border pt-6">
          <p className="font-mono text-xs text-text-dim">
            corruption.center — ferramenta de transparência não comercial,
            construída sobre dados públicos oficiais.
          </p>
        </footer>
      </div>
    </main>
  );
}
