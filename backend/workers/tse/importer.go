package tse

import (
	"archive/zip"
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

const (
	defaultMinDiskBytes = 512 * 1024 * 1024
	defaultMinMemBytes  = 256 * 1024 * 1024
)

// Offices kept from a general election. State-level executives and legislators
// are included because they are heavily represented in corruption prosecutions
// (state governors especially) and a name that is not in this base can never be
// matched to a court party or a sanction: it stays an anonymous Person.
// Municipal offices (prefeito, vereador) are elected in different years and are
// not covered by these files.
var allowedCargos = map[string]struct{}{
	"DEPUTADO FEDERAL":   {},
	"SENADOR":            {},
	"PRESIDENTE":         {},
	"VICE-PRESIDENTE":    {},
	"GOVERNADOR":         {},
	"VICE-GOVERNADOR":    {},
	"DEPUTADO ESTADUAL":  {},
	"DEPUTADO DISTRITAL": {},
}

// Elected statuses. The label set is NOT stable across election years: 2002 and
// 2022 say "ELEITO POR MÉDIA", while 2006 says bare "MÉDIA". Missing a variant
// silently drops real winners (79 of the 513 federal deputies elected in 2006
// carry "MÉDIA"), so accept every spelling TSE has used.
var allowedStatus = map[string]struct{}{
	"ELEITO":           {},
	"ELEITO POR QP":    {},
	"ELEITO POR MÉDIA": {},
	"ELEITO POR MEDIA": {},
	"MÉDIA":            {},
	"MEDIA":            {},
	"QP":               {},
}

type ImportResult struct {
	Records []PoliticianRecord
	Stats   ImportStats
}

type ImportStats struct {
	ElectionRowsRead    int
	CandidateRowsRead   int
	WinningCandidates   int
	MissingCPF          int
	SkippedByCargo      int
	SkippedByStatus     int
	SkippedByInvalidRow int
	MissingBRFile       bool
	FilesProcessed      int
	FilesDeleted        int
}

type PoliticianRecord struct {
	CPF            string
	Name           string
	NameAliases    []string
	PartyCurrent   string
	State          string
	TSEProfileURLs []string
	ElectionYear   int
	CandidateSQ    string
	Active         bool
}

type winnerRow struct {
	SQ           string
	CPF          string
	ElectionYear string
	Turn         int
	UF           string
	Party        string
	Name         string
	UrnaName     string
	SocialName   string
}

// candidateKey identifies a candidate within one election year.
//
// SQ_CANDIDATO alone is NOT unique in the older TSE files: in 2006, SQ 10204 is
// Cláudio Cajado (BA), Marcos Ramos da Hora (PE) AND Givaldo Carimbão (AL). Keying
// winners by SQ alone silently overwrites real politicians, and joining the CPF on
// it can attach one person's document to another, which is the worst failure this
// project has: a wrong CPF links a sanction or a case to the wrong human. The
// state disambiguates it (SQ is unique per UF per year).
func candidateKey(uf, sq string) string {
	return strings.ToUpper(strings.TrimSpace(uf)) + "|" + strings.TrimSpace(sq)
}

type ImportOptions struct {
	MinDiskBytes uint64
	MinMemBytes  uint64
}

// ImportYear reads a single consulta_cand CSV and returns the elected officials
// in it.
//
// The votacao_candidato_munzona files used to be required as well, joined on
// SQ_CANDIDATO, but they were never needed: consulta_cand already states the
// office (DS_CARGO), the result (DS_SIT_TOT_TURNO), the state, the party, the
// names and the CPF. The votacao files only add vote tallies, which this project
// does not use, and they cost 552MB zipped per year (multiple GB unzipped)
// against 4MB for consulta_cand. Dropping them removed the disk pressure that
// made the 2022 import fail outright.
func ImportYear(consultaCSV io.Reader) (*ImportResult, error) {
	winners, stats, err := readWinnersFromReader(consultaCSV)
	if err != nil {
		return nil, err
	}
	stats.WinningCandidates = len(winners)
	return buildResult(winners, stats), nil
}

func ImportYearFromZipFiles(year int, consultaZipPath, workDir string, opts ImportOptions) (*ImportResult, error) {
	if strings.TrimSpace(workDir) == "" {
		workDir = os.TempDir()
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("tse: create workdir %s: %w", workDir, err)
	}

	if opts.MinDiskBytes == 0 {
		opts.MinDiskBytes = defaultMinDiskBytes
	}
	if opts.MinMemBytes == 0 {
		opts.MinMemBytes = defaultMinMemBytes
	}
	if err := ensureSystemResources(workDir, opts); err != nil {
		return nil, err
	}

	tmpYearDir, err := os.MkdirTemp(workDir, fmt.Sprintf("tse-%d-*", year))
	if err != nil {
		return nil, fmt.Errorf("tse: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpYearDir)

	consultaDir := filepath.Join(tmpYearDir, "consulta")
	stats := ImportStats{}
	if err := unzipAndPrune(consultaZipPath, consultaDir); err != nil {
		return nil, err
	}

	consultaFiles, err := collectConsultaFiles(year, consultaDir)
	if err != nil {
		return nil, err
	}

	winners := map[string]winnerRow{}
	for _, f := range consultaFiles {
		if err := processConsultaFile(f, winners, &stats); err != nil {
			return nil, err
		}
		stats.FilesProcessed++
		if err := deleteFile(f, &stats); err != nil {
			return nil, err
		}
		if stats.FilesProcessed%3 == 0 {
			runtime.GC()
			if err := ensureSystemResources(tmpYearDir, opts); err != nil {
				return nil, err
			}
		}
	}

	stats.WinningCandidates = len(winners)
	return buildResult(winners, stats), nil
}

func buildResult(winners map[string]winnerRow, stats ImportStats) *ImportResult {
	result := &ImportResult{Stats: stats}
	result.Records = make([]PoliticianRecord, 0, len(winners))
	for _, winner := range winners {
		sq := winner.SQ
		cpf := normalizeNull(winner.CPF)
		if cpf == "" {
			result.Stats.MissingCPF++
			continue
		}
		year, _ := strconv.Atoi(winner.ElectionYear)
		record := PoliticianRecord{
			CPF:            cpf,
			Name:           winner.Name,
			PartyCurrent:   winner.Party,
			State:          winner.UF,
			TSEProfileURLs: []string{buildProfileURL(winner.ElectionYear, sq)},
			ElectionYear:   year,
			CandidateSQ:    sq,
			Active:         false,
		}
		addAlias(&record.NameAliases, winner.UrnaName, winner.Name)
		addAlias(&record.NameAliases, winner.SocialName, winner.Name)
		result.Records = append(result.Records, record)
	}
	return result
}

func unzipAndPrune(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("tse: open zip %s: %w", zipPath, err)
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("tse: create unzip dir: %w", err)
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		target := filepath.Join(destDir, name)
		if strings.HasSuffix(strings.ToUpper(name), "_BRASIL.CSV") {
			continue
		}
		if err := extractZipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("tse: open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("tse: create extracted file %s: %w", target, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("tse: extract %s: %w", f.Name, err)
	}
	return nil
}

func collectConsultaFiles(year int, dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("tse: read consulta dir: %w", err)
	}
	prefix := fmt.Sprintf("CONSULTA_CAND_%d_", year)
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToUpper(e.Name())
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".CSV") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	return files, nil
}

// scanConsultaRow parses one consulta_cand row into a winner, or reports why it
// was dropped. Returns ok=false for anything that is not an elected official in
// an office we track.
func scanConsultaRow(row []string, headers map[string]int, stats *ImportStats) (winnerRow, string, bool) {
	status := strings.ToUpper(strings.TrimSpace(cell(row, headers, "DS_SIT_TOT_TURNO")))
	if _, ok := allowedStatus[status]; !ok {
		stats.SkippedByStatus++
		return winnerRow{}, "", false
	}
	cargo := strings.ToUpper(strings.TrimSpace(cell(row, headers, "DS_CARGO")))
	if _, ok := allowedCargos[cargo]; !ok {
		stats.SkippedByCargo++
		return winnerRow{}, "", false
	}

	sq := strings.TrimSpace(cell(row, headers, "SQ_CANDIDATO"))
	turn, err := strconv.Atoi(strings.TrimSpace(cell(row, headers, "NR_TURNO")))
	if sq == "" || err != nil {
		stats.SkippedByInvalidRow++
		return winnerRow{}, "", false
	}

	w := winnerRow{
		SQ:           sq,
		CPF:          strings.TrimSpace(cell(row, headers, "NR_CPF_CANDIDATO")),
		ElectionYear: strings.TrimSpace(cell(row, headers, "ANO_ELEICAO")),
		Turn:         turn,
		UF:           strings.TrimSpace(cell(row, headers, "SG_UF")),
		Party:        strings.TrimSpace(cell(row, headers, "SG_PARTIDO")),
		Name:         strings.TrimSpace(cell(row, headers, "NM_CANDIDATO")),
		UrnaName:     strings.TrimSpace(cell(row, headers, "NM_URNA_CANDIDATO")),
		SocialName:   strings.TrimSpace(cell(row, headers, "NM_SOCIAL_CANDIDATO")),
	}
	return w, candidateKey(w.UF, sq), true
}

// keepLatestTurn records a winner, preferring the row from the later round (an
// office decided in a runoff appears in both rounds).
func keepLatestTurn(winners map[string]winnerRow, key string, w winnerRow) {
	if existing, ok := winners[key]; !ok || w.Turn > existing.Turn {
		winners[key] = w
	}
}

var consultaHeaders = []string{
	"SQ_CANDIDATO", "NR_CPF_CANDIDATO", "NR_TURNO", "DS_CARGO", "DS_SIT_TOT_TURNO",
	"ANO_ELEICAO", "SG_UF", "SG_PARTIDO", "NM_CANDIDATO", "NM_URNA_CANDIDATO", "NM_SOCIAL_CANDIDATO",
}

func processConsultaFile(path string, winners map[string]winnerRow, stats *ImportStats) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("tse: open consulta file %s: %w", path, err)
	}
	defer f.Close()

	reader, headers, err := newCSVReaderLatin1(f)
	if err != nil {
		return err
	}
	if err := ensureHeaders(headers, consultaHeaders); err != nil {
		return err
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tse: read consulta row %s: %w", path, err)
		}
		stats.CandidateRowsRead++
		if w, key, ok := scanConsultaRow(row, headers, stats); ok {
			keepLatestTurn(winners, key, w)
		}
	}
	return nil
}

func readWinnersFromReader(r io.Reader) (map[string]winnerRow, ImportStats, error) {
	stats := ImportStats{}
	reader, headers, err := newCSVReaderLatin1(r)
	if err != nil {
		return nil, stats, err
	}
	if err := ensureHeaders(headers, consultaHeaders); err != nil {
		return nil, stats, err
	}

	winners := map[string]winnerRow{}
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, stats, err
		}
		stats.CandidateRowsRead++
		if w, key, ok := scanConsultaRow(row, headers, &stats); ok {
			keepLatestTurn(winners, key, w)
		}
	}
	return winners, stats, nil
}

func newCSVReaderLatin1(r io.Reader) (*csv.Reader, map[string]int, error) {
	decoded := transform.NewReader(r, charmap.ISO8859_1.NewDecoder())
	br := bufio.NewReader(decoded)
	cr := csv.NewReader(br)
	cr.Comma = ';'
	cr.LazyQuotes = true
	cr.TrimLeadingSpace = true
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil, fmt.Errorf("tse: empty csv")
		}
		return nil, nil, err
	}
	return cr, rowToIndex(header), nil
}

func rowToIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, col := range header {
		idx[strings.TrimSpace(col)] = i
	}
	return idx
}

func ensureHeaders(idx map[string]int, required []string) error {
	for _, key := range required {
		if _, ok := idx[key]; !ok {
			return fmt.Errorf("tse: missing required header %q", key)
		}
	}
	return nil
}

func cell(row []string, idx map[string]int, key string) string {
	i := idx[key]
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

// normalizeNull maps TSE's "no value" markers to the empty string.
//
// TSE writes these both bare and hash-wrapped (#NULO and #NULO#), and the two
// forms are mixed across columns and years. Listing them one by one missed the
// wrapped form, so NM_SOCIAL_CANDIDATO handed "#NULO#" to 2,397 politicians as
// a name alias, a name half the base then shared. No real name starts with '#',
// so treat the prefix itself as the marker and stop enumerating spellings.
func normalizeNull(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "#") || v == "-1" || v == "-3" {
		return ""
	}
	return v
}

func addAlias(aliases *[]string, raw string, legalName string) {
	alias := normalizeNull(raw)
	if alias == "" {
		return
	}
	if strings.EqualFold(alias, strings.TrimSpace(legalName)) {
		return
	}
	for _, existing := range *aliases {
		if strings.EqualFold(existing, alias) {
			return
		}
	}
	*aliases = append(*aliases, alias)
}

func buildProfileURL(year string, sq string) string {
	return fmt.Sprintf("https://divulgacandcontas.tse.jus.br/divulga/#/candidato/%s/%s", year, sq)
}

func deleteFile(path string, stats *ImportStats) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("tse: delete processed file %s: %w", path, err)
	}
	stats.FilesDeleted++
	return nil
}

func ensureSystemResources(path string, opts ImportOptions) error {
	if err := checkDisk(path, opts.MinDiskBytes); err != nil {
		return err
	}
	if err := checkMemory(opts.MinMemBytes); err != nil {
		return err
	}
	return nil
}

func checkDisk(path string, minBytes uint64) error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return fmt.Errorf("tse: disk check failed: %w", err)
	}
	available := st.Bavail * uint64(st.Bsize)
	if available < minBytes {
		return fmt.Errorf("tse: insufficient disk: available=%d required=%d", available, minBytes)
	}
	return nil
}

func checkMemory(minBytes uint64) error {
	available, ok := readMemAvailableLinux()
	if !ok {
		// /proc/meminfo unavailable (non-Linux): we cannot know available
		// memory, so don't block the import.
		return nil
	}
	if available < minBytes {
		return fmt.Errorf("tse: insufficient memory: available=%d required=%d", available, minBytes)
	}
	return nil
}

func readMemAvailableLinux() (uint64, bool) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}
