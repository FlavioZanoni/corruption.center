package datajud

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func VerifyMovementCodes(ctx context.Context) (bool, error) {
	// Case-level state-machine movement codes (see docs/workerDetails/DATAJUD.md).
	required := map[string][]string{
		"51":  {"receb", "den"}, // Recebimento de denúncia
		"60":  {"conden"},       // Condenação
		"61":  {"absolv"},       // Absolvição
		"848": {"senten"},       // Sentença
		"901": {"prescri"},      // Prescrição
		"132": {"baixa"},        // Baixa definitiva
		"246": {"arquiv"},       // Arquivamento definitivo
	}

	for code, keywords := range required {
		ok, err := verifyCode(ctx, code, keywords)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func verifyCode(ctx context.Context, code string, keywords []string) (bool, error) {
	url := "https://www.cnj.jus.br/sgt/consulta_publica_movimentos.php?movimento=" + code
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("datajud: fetch TPU movement %s: %w", code, err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 512*1024))
	if err != nil {
		return false, err
	}
	lower := strings.ToLower(string(b))
	for _, kw := range keywords {
		if !strings.Contains(lower, kw) {
			return false, nil
		}
	}
	return true, nil
}
