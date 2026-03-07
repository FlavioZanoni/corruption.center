import type {
  GraphResponse,
  SearchResponse,
  TimelineResponse,
} from "@/lib/types"

// ─── Nodes ────────────────────────────────────────────────────────────────────

export const MOCK_GRAPH: GraphResponse = {
  nodes: [
    // Politicians
    {
      id: "pol-lula",
      type: "politician",
      label: "Luiz Inácio Lula da Silva",
      properties: {
        name: "Luiz Inácio Lula da Silva",
        party_current: "PT",
        state: "SP",
        role_current: "Presidente da República",
        active: true,
        photo_url: "https://avatars.githubusercontent.com/u/67586641?s=400&u=715d9005673d3130d37ec0525aab57146cd788b6&v=4",
        tse_profile_url: "https://www.tse.jus.br/",
        wikipedia_url: "https://pt.wikipedia.org/wiki/Lula",
      },
    },
    {
      id: "pol-temer",
      type: "politician",
      label: "Michel Temer",
      properties: {
        name: "Michel Temer",
        party_current: "MDB",
        state: "SP",
        role_current: "Ex-Presidente da República",
        active: false,
        photo_url: "https://avatars.githubusercontent.com/u/67586641?s=400&u=715d9005673d3130d37ec0525aab57146cd788b6&v=4",
        wikipedia_url: "https://pt.wikipedia.org/wiki/Michel_Temer",
      },
    },
    {
      id: "pol-cunha",
      type: "politician",
      label: "Eduardo Cunha",
      properties: {
        name: "Eduardo Cunha",
        party_current: "MDB",
        state: "RJ",
        role_current: "Ex-Deputado Federal",
        active: false,
        photo_url: "https://avatars.githubusercontent.com/u/67586641?s=400&u=715d9005673d3130d37ec0525aab57146cd788b6&v=4",
        wikipedia_url: "https://pt.wikipedia.org/wiki/Eduardo_Cunha",
      },
    },
    {
      id: "pol-dirceu",
      type: "politician",
      label: "José Dirceu",
      properties: {
        name: "José Dirceu",
        party_current: "PT",
        state: "SP",
        role_current: "Ex-Ministro da Casa Civil",
        active: false,
        photo_url: "https://avatars.githubusercontent.com/u/67586641?s=400&u=715d9005673d3130d37ec0525aab57146cd788b6&v=4",
        wikipedia_url: "https://pt.wikipedia.org/wiki/Jos%C3%A9_Dirceu",
      },
    },
    {
      id: "pol-aecio",
      type: "politician",
      label: "Aécio Neves",
      properties: {
        name: "Aécio Neves",
        party_current: "PSDB",
        state: "MG",
        role_current: "Senador",
        active: true,
        photo_url: "https://avatars.githubusercontent.com/u/67586641?s=400&u=715d9005673d3130d37ec0525aab57146cd788b6&v=4",
        wikipedia_url: "https://pt.wikipedia.org/wiki/A%C3%A9cio_Neves",
      },
    },
    {
      id: "pol-delcidio",
      type: "politician",
      label: "Delcídio Amaral",
      properties: {
        name: "Delcídio Amaral",
        party_current: "PT",
        state: "MS",
        role_current: "Ex-Senador",
        active: false,
        wikipedia_url: "https://pt.wikipedia.org/wiki/Delc%C3%ADdio_Amaral",
      },
    },
    {
      id: "pol-cabral",
      type: "politician",
      label: "Sérgio Cabral",
      properties: {
        name: "Sérgio Cabral",
        party_current: "MDB",
        state: "RJ",
        role_current: "Ex-Governador do Rio de Janeiro",
        active: false,
        photo_url: "https://avatars.githubusercontent.com/u/67586641?s=400&u=715d9005673d3130d37ec0525aab57146cd788b6&v=4",
        wikipedia_url: "https://pt.wikipedia.org/wiki/S%C3%A9rgio_Cabral_Filho",
      },
    },
    {
      id: "pol-maluf",
      type: "politician",
      label: "Paulo Maluf",
      properties: {
        name: "Paulo Maluf",
        party_current: "PP",
        state: "SP",
        role_current: "Ex-Deputado Federal",
        active: false,
        photo_url: "https://avatars.githubusercontent.com/u/67586641?s=400&u=715d9005673d3130d37ec0525aab57146cd788b6&v=4",
        wikipedia_url: "https://pt.wikipedia.org/wiki/Paulo_Maluf",
      },
    },
    {
      id: "pol-collor",
      type: "politician",
      label: "Fernando Collor de Mello",
      properties: {
        name: "Fernando Collor de Mello",
        party_current: "Progressistas",
        state: "AL",
        role_current: "Ex-Presidente da República",
        active: false,
        photo_url: "https://avatars.githubusercontent.com/u/67586641?s=400&u=715d9005673d3130d37ec0525aab57146cd788b6&v=4",
        wikipedia_url: "https://pt.wikipedia.org/wiki/Fernando_Collor_de_Mello",
      },
    },
    {
      id: "pol-renan",
      type: "politician",
      label: "Renan Calheiros",
      properties: {
        name: "Renan Calheiros",
        party_current: "MDB",
        state: "AL",
        role_current: "Senador",
        active: true,
        photo_url: "https://avatars.githubusercontent.com/u/67586641?s=400&u=715d9005673d3130d37ec0525aab57146cd788b6&v=4",
        wikipedia_url: "https://pt.wikipedia.org/wiki/Renan_Calheiros",
      },
    },

    // Scandals
    {
      id: "scan-lava-jato",
      type: "scandal",
      label: "Operação Lava Jato",
      properties: {
        name: "Operação Lava Jato",
        description:
          "Maior operação anticorrupção da história do Brasil, investigando esquema de lavagem de dinheiro e corrupção na Petrobras.",
        date_start: "2014-03-17",
        date_end: "2021-02-01",
        status: "concluded",
        total_amount_brl: 6400000000,
        aliases: ["Lava Jato", "Car Wash"],
        wikipedia_url: "https://pt.wikipedia.org/wiki/Opera%C3%A7%C3%A3o_Lava_Jato",
      },
    },
    {
      id: "scan-mensalao",
      type: "scandal",
      label: "Escândalo do Mensalão",
      properties: {
        name: "Escândalo do Mensalão",
        description:
          "Esquema de compra de votos de parlamentares para apoio ao governo Lula no Congresso Nacional.",
        date_start: "2005-06-01",
        date_end: "2012-11-17",
        status: "concluded",
        total_amount_brl: 55000000,
        wikipedia_url: "https://pt.wikipedia.org/wiki/Mensal%C3%A3o",
      },
    },
    {
      id: "scan-pcfish",
      type: "scandal",
      label: "Escândalo PC Farias",
      properties: {
        name: "Escândalo PC Farias",
        description:
          "Esquema de corrupção envolvendo Paulo César Farias, tesoureiro da campanha de Collor, que resultou no impeachment do presidente.",
        date_start: "1992-05-01",
        date_end: "1992-12-29",
        status: "concluded",
        total_amount_brl: 6500000,
        wikipedia_url: "https://pt.wikipedia.org/wiki/Paulo_C%C3%A9sar_Farias",
      },
    },
    {
      id: "scan-petrobras",
      type: "scandal",
      label: "Esquema Petrobras",
      properties: {
        name: "Esquema Petrobras",
        description:
          "Diretores da Petrobras recebiam propina de empreiteiras em troca de contratos superfaturados.",
        date_start: "2004-01-01",
        date_end: "2014-03-17",
        status: "concluded",
        total_amount_brl: 2100000000,
        wikipedia_url: "https://pt.wikipedia.org/wiki/Opera%C3%A7%C3%A3o_Lava_Jato",
      },
    },
    {
      id: "scan-maluf-duto",
      type: "scandal",
      label: "Caso Duto Mogi",
      properties: {
        name: "Caso Duto Mogi",
        description:
          "Desvio de verbas públicas em obras de saneamento em São Paulo durante gestão de Paulo Maluf.",
        date_start: "1993-01-01",
        date_end: "1996-12-31",
        status: "concluded",
        total_amount_brl: 800000000,
      },
    },
    {
      id: "scan-cabral-rio",
      type: "scandal",
      label: "Esquema Cabral — Rio",
      properties: {
        name: "Esquema Cabral — Rio",
        description:
          "Esquema de corrupção sistêmica no governo do estado do Rio de Janeiro envolvendo propinas em contratos de saúde e obras.",
        date_start: "2007-01-01",
        date_end: "2016-11-17",
        status: "concluded",
        total_amount_brl: 224000000,
      },
    },

    // Organizations
    {
      id: "org-petrobras",
      type: "organization",
      label: "Petrobras",
      properties: {
        name: "Petrobras",
        cnpj: "33.000.167/0001-01",
        type: "public_agency",
        active: true,
      },
    },
    {
      id: "org-odebrecht",
      type: "organization",
      label: "Odebrecht",
      properties: {
        name: "Odebrecht S.A.",
        cnpj: "11.571.148/0001-50",
        type: "company",
        active: false,
      },
    },
    {
      id: "org-oas",
      type: "organization",
      label: "OAS",
      properties: {
        name: "OAS S.A.",
        cnpj: "05.929.025/0001-60",
        type: "company",
        active: false,
      },
    },
    {
      id: "org-andrade-gutierrez",
      type: "organization",
      label: "Andrade Gutierrez",
      properties: {
        name: "Andrade Gutierrez S.A.",
        cnpj: "17.260.580/0001-74",
        type: "company",
        active: true,
      },
    },
    {
      id: "org-pt",
      type: "organization",
      label: "Partido dos Trabalhadores",
      properties: {
        name: "Partido dos Trabalhadores",
        type: "party",
        active: true,
      },
    },
    {
      id: "org-mdb",
      type: "organization",
      label: "MDB",
      properties: {
        name: "Movimento Democrático Brasileiro",
        type: "party",
        active: true,
      },
    },

    // Legal Proceedings
    {
      id: "leg-ap470",
      type: "legal_proceeding",
      label: "AP 470 — Mensalão",
      properties: {
        case_number: "AP 470",
        court: "STF",
        type: "criminal",
        status: "concluded",
        date_filed: "2007-08-01",
        date_concluded: "2012-11-17",
        url: "https://portal.stf.jus.br/processos/detalhe.asp?incidente=11541",
      },
    },
    {
      id: "leg-lj-principal",
      type: "legal_proceeding",
      label: "Ação Penal Lava Jato",
      properties: {
        case_number: "5046512-94.2016.4.04.7000",
        court: "TRF4",
        type: "criminal",
        status: "concluded",
        date_filed: "2014-03-17",
        date_concluded: "2017-07-12",
        url: "https://www.trf4.jus.br/",
      },
    },
    {
      id: "leg-collor-imp",
      type: "legal_proceeding",
      label: "Impeachment Collor",
      properties: {
        case_number: "Imp. 1992",
        court: "Senado Federal",
        type: "administrative",
        status: "concluded",
        date_filed: "1992-09-29",
        date_concluded: "1992-12-29",
        url: "https://pt.wikipedia.org/wiki/Impeachment_de_Fernando_Collor_de_Mello",
      },
    },
    {
      id: "leg-cunha-cpc",
      type: "legal_proceeding",
      label: "Ação Penal — Eduardo Cunha",
      properties: {
        case_number: "AP 863",
        court: "STF",
        type: "criminal",
        status: "concluded",
        date_filed: "2015-08-01",
        date_concluded: "2017-10-24",
        url: "https://portal.stf.jus.br/processos/detalhe.asp?incidente=4777687",
      },
    },
  ],

  edges: [
    // Lula connections
    { id: "e1", from: "pol-lula", to: "scan-mensalao", type: "INVOLVED_IN", properties: { status: "cited", reliability: "low", date_from: "2005-06-01", role_at_time: "Presidente da República", party_at_time: "PT" } },
    { id: "e2", from: "pol-lula", to: "scan-lava-jato", type: "DEFENDANT_IN", properties: { status: "convicted", reliability: "high", date_from: "2016-09-14", role_at_time: "Ex-Presidente da República", party_at_time: "PT" } },
    { id: "e3", from: "pol-lula", to: "org-pt", type: "MEMBER_OF", properties: { status: "membership", reliability: "high", date_from: "1980-02-10", role_at_time: "Fundador" } },
    { id: "e4", from: "pol-lula", to: "scan-petrobras", type: "IMPLICATED_IN", properties: { status: "cited", reliability: "low", date_from: "2015-01-01", role_at_time: "Ex-Presidente da República", party_at_time: "PT" } },
    { id: "e5", from: "pol-lula", to: "leg-lj-principal", type: "DEFENDANT_IN", properties: { status: "convicted", reliability: "high", role_at_time: "Ex-Presidente da República" } },
    { id: "e6", from: "pol-lula", to: "org-odebrecht", type: "RELATED_TO", properties: { status: "cited", reliability: "low" } },

    // Dirceu connections
    { id: "e7", from: "pol-dirceu", to: "scan-mensalao", type: "INVOLVED_IN", properties: { status: "convicted", reliability: "high", date_from: "2005-06-01", role_at_time: "Ministro da Casa Civil", party_at_time: "PT" } },
    { id: "e8", from: "pol-dirceu", to: "org-pt", type: "MEMBER_OF", properties: { status: "membership", reliability: "high", role_at_time: "Presidente do Partido" } },
    { id: "e9", from: "pol-dirceu", to: "leg-ap470", type: "DEFENDANT_IN", properties: { status: "convicted", reliability: "high", role_at_time: "Ministro da Casa Civil" } },

    // Cunha connections
    { id: "e10", from: "pol-cunha", to: "scan-lava-jato", type: "INVOLVED_IN", properties: { status: "convicted", reliability: "high", date_from: "2015-01-01", role_at_time: "Presidente da Câmara dos Deputados", party_at_time: "MDB" } },
    { id: "e11", from: "pol-cunha", to: "org-mdb", type: "MEMBER_OF", properties: { status: "membership", reliability: "high", role_at_time: "Deputado Federal" } },
    { id: "e12", from: "pol-cunha", to: "leg-cunha-cpc", type: "DEFENDANT_IN", properties: { status: "convicted", reliability: "high", role_at_time: "Presidente da Câmara dos Deputados" } },
    { id: "e13", from: "pol-cunha", to: "org-odebrecht", type: "RELATED_TO", properties: { status: "indicted", reliability: "medium" } },

    // Temer connections
    { id: "e14", from: "pol-temer", to: "scan-lava-jato", type: "IMPLICATED_IN", properties: { status: "indicted", reliability: "medium", date_from: "2017-06-26", role_at_time: "Presidente da República", party_at_time: "MDB" } },
    { id: "e15", from: "pol-temer", to: "org-mdb", type: "MEMBER_OF", properties: { status: "membership", reliability: "high", role_at_time: "Vice-Presidente da República" } },
    { id: "e16", from: "pol-temer", to: "org-odebrecht", type: "RELATED_TO", properties: { status: "cited", reliability: "low" } },
    { id: "e17", from: "pol-temer", to: "org-oas", type: "RELATED_TO", properties: { status: "cited", reliability: "low" } },

    // Aécio connections
    { id: "e18", from: "pol-aecio", to: "scan-lava-jato", type: "IMPLICATED_IN", properties: { status: "indicted", reliability: "medium", date_from: "2017-05-18", role_at_time: "Senador", party_at_time: "PSDB" } },
    { id: "e19", from: "pol-aecio", to: "org-andrade-gutierrez", type: "RELATED_TO", properties: { status: "indicted", reliability: "medium" } },

    // Delcídio connections
    { id: "e20", from: "pol-delcidio", to: "scan-lava-jato", type: "INVOLVED_IN", properties: { status: "convicted", reliability: "high", date_from: "2015-11-25", role_at_time: "Senador", party_at_time: "PT" } },
    { id: "e21", from: "pol-delcidio", to: "org-pt", type: "MEMBER_OF", properties: { status: "membership", reliability: "high", role_at_time: "Senador" } },
    { id: "e22", from: "pol-delcidio", to: "org-petrobras", type: "RELATED_TO", properties: { status: "convicted", reliability: "high" } },

    // Cabral connections
    { id: "e23", from: "pol-cabral", to: "scan-cabral-rio", type: "INVOLVED_IN", properties: { status: "convicted", reliability: "high", date_from: "2007-01-01", role_at_time: "Governador do Rio de Janeiro", party_at_time: "MDB" } },
    { id: "e24", from: "pol-cabral", to: "org-mdb", type: "MEMBER_OF", properties: { status: "membership", reliability: "high", role_at_time: "Governador do Rio de Janeiro" } },
    { id: "e25", from: "pol-cabral", to: "org-odebrecht", type: "RELATED_TO", properties: { status: "convicted", reliability: "high" } },
    { id: "e26", from: "pol-cabral", to: "org-andrade-gutierrez", type: "RELATED_TO", properties: { status: "convicted", reliability: "high" } },

    // Maluf connections
    { id: "e27", from: "pol-maluf", to: "scan-maluf-duto", type: "INVOLVED_IN", properties: { status: "convicted", reliability: "high", date_from: "1993-01-01", role_at_time: "Prefeito de São Paulo", party_at_time: "PP" } },

    // Collor connections
    { id: "e28", from: "pol-collor", to: "scan-pcfish", type: "INVOLVED_IN", properties: { status: "cited", reliability: "medium", date_from: "1992-05-01", role_at_time: "Presidente da República", party_at_time: "PRN" } },
    { id: "e29", from: "pol-collor", to: "leg-collor-imp", type: "DEFENDANT_IN", properties: { status: "cited", reliability: "medium", role_at_time: "Presidente da República" } },

    // Renan connections
    { id: "e30", from: "pol-renan", to: "scan-lava-jato", type: "IMPLICATED_IN", properties: { status: "cited", reliability: "low", date_from: "2015-01-01", role_at_time: "Presidente do Senado Federal", party_at_time: "MDB" } },
    { id: "e31", from: "pol-renan", to: "org-mdb", type: "MEMBER_OF", properties: { status: "membership", reliability: "high", role_at_time: "Senador" } },

    // Orgs to scandals
    { id: "e32", from: "org-odebrecht", to: "scan-lava-jato", type: "INVOLVED_IN", properties: { status: "convicted", reliability: "high" } },
    { id: "e33", from: "org-oas", to: "scan-lava-jato", type: "INVOLVED_IN", properties: { status: "convicted", reliability: "high" } },
    { id: "e34", from: "org-andrade-gutierrez", to: "scan-lava-jato", type: "INVOLVED_IN", properties: { status: "convicted", reliability: "high" } },
    { id: "e35", from: "org-petrobras", to: "scan-petrobras", type: "RELATED_TO", properties: { status: "cited", reliability: "medium" } },
    { id: "e36", from: "org-petrobras", to: "scan-lava-jato", type: "RELATED_TO", properties: { status: "cited", reliability: "medium" } },

    // Legal proceedings to scandals
    { id: "e37", from: "leg-ap470", to: "scan-mensalao", type: "INVESTIGATES", properties: { status: "membership", reliability: "high" } },
    { id: "e38", from: "leg-lj-principal", to: "scan-lava-jato", type: "INVESTIGATES", properties: { status: "membership", reliability: "high" } },
    { id: "e39", from: "leg-collor-imp", to: "scan-pcfish", type: "INVESTIGATES", properties: { status: "membership", reliability: "high" } },
    { id: "e40", from: "leg-cunha-cpc", to: "scan-lava-jato", type: "INVESTIGATES", properties: { status: "membership", reliability: "high" } },
  ],
}

// ─── Search index ─────────────────────────────────────────────────────────────

export function mockSearch(query: string): SearchResponse {
  const q = query.toLowerCase().trim()
  const results = MOCK_GRAPH.nodes
    .filter((n) => {
      const props = n.properties as Record<string, string | undefined>
      return (
        n.label.toLowerCase().includes(q) ||
        (props.description ?? "").toLowerCase().includes(q) ||
        (props.party_current ?? "").toLowerCase().includes(q)
      )
    })
    .slice(0, 20)
    .map((n) => {
      const props = n.properties as Record<string, string | undefined>
      return {
        id: n.id,
        type: n.type,
        label: n.label,
        snippet: props.description ?? props.role_current ?? props.sector,
        properties: n.properties,
      }
    })

  return { results, total: results.length }
}

// ─── Timeline filter ──────────────────────────────────────────────────────────

export function mockTimeline(from: number, to: number): TimelineResponse {
  const scandalIds = new Set(
    MOCK_GRAPH.nodes
      .filter((n) => {
        if (n.type !== "scandal") return true
        const props = n.properties as Record<string, string | undefined>
        const start = props.date_start ? parseInt(props.date_start) : 0
        const end = props.date_end ? parseInt(props.date_end) : 9999
        return start <= to && end >= from
      })
      .map((n) => n.id)
  )

  const visibleNodes = MOCK_GRAPH.nodes.filter(
    (n) =>
      n.type !== "scandal" ||
      scandalIds.has(n.id)
  )
  const visibleIds = new Set(visibleNodes.map((n) => n.id))

  const visibleEdges = MOCK_GRAPH.edges.filter(
    (e) => visibleIds.has(e.from) && visibleIds.has(e.to)
  )

  return {
    nodes: visibleNodes,
    edges: visibleEdges,
    from: `${from}-01-01`,
    to: `${to}-12-31`,
  }
}

// ─── Graph expand ─────────────────────────────────────────────────────────────

export function mockExpand(id: string, hops: number): GraphResponse {
  const visited = new Set<string>([id])
  let frontier = new Set<string>([id])

  for (let h = 0; h < hops; h++) {
    const next = new Set<string>()
    for (const nodeId of frontier) {
      for (const edge of MOCK_GRAPH.edges) {
        if (edge.from === nodeId && !visited.has(edge.to)) {
          visited.add(edge.to)
          next.add(edge.to)
        }
        if (edge.to === nodeId && !visited.has(edge.from)) {
          visited.add(edge.from)
          next.add(edge.from)
        }
      }
    }
    frontier = next
  }

  const nodes = MOCK_GRAPH.nodes.filter((n) => visited.has(n.id))
  const edges = MOCK_GRAPH.edges.filter(
    (e) => visited.has(e.from) && visited.has(e.to)
  )
  return { nodes, edges }
}
