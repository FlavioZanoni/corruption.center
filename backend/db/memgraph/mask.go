package memgraph

// maskCPF reduces a full 11-digit CPF to the form the source itself publishes —
// ***.XXX.XXX-** — hiding the first three and last two digits, showing only the
// middle six. Anything that is not 11 digits is returned unchanged (already
// masked, a CNPJ, or empty).
//
// Why this exists: some sanction registries (CEAF servidores, parts of TCU)
// carry a FULL CPF, and 16,938 Person nodes ended up holding one. Serving it to
// an anonymous caller is a data-minimization breach — CGU deliberately masks
// CPFs on its public registries, and this project's own methodology page
// promises to show no more than CGU does. So the full number is used internally
// for matching (it is how a sanction fuses to the right person) and never
// leaves the building intact.
func maskCPF(cpf string) string {
	if len(cpf) != 11 {
		return cpf
	}
	for _, r := range cpf {
		if r < '0' || r > '9' {
			return cpf
		}
	}
	return "***." + cpf[3:6] + "." + cpf[6:9] + "-**"
}
