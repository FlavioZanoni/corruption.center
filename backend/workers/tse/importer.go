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

// cpfIndex resolves a candidate's CPF, preferring the collision-proof (UF, SQ)
// key and falling back to SQ alone only when that SQ is unambiguous across the
// whole file set. The fallback exists for PRESIDENTE: the consulta row lives in
// the national file (SG_UF "BR") while the votação rows are per state, so the two
// sides disagree on UF and only the SQ can join them.
type cpfIndex struct {
	byKey     map[string]string
	bySQ      map[string]string
	ambiguous map[string]bool
}

func newCPFIndex() *cpfIndex {
	return &cpfIndex{
		byKey:     map[string]string{},
		bySQ:      map[string]string{},
		ambiguous: map[string]bool{},
	}
}

func (c *cpfIndex) add(uf, sq, cpf string) {
	sq = strings.TrimSpace(sq)
	if sq == "" {
		return
	}
	c.byKey[candidateKey(uf, sq)] = cpf
	if prev, seen := c.bySQ[sq]; seen && prev != cpf {
		// The same SQ carries different CPFs: joining on SQ alone would pick a
		// person at random, so refuse to use it as a fallback.
		c.ambiguous[sq] = true
		return
	}
	c.bySQ[sq] = cpf
}

func (c *cpfIndex) lookup(uf, sq string) string {
	if cpf, ok := c.byKey[candidateKey(uf, sq)]; ok {
		return cpf
	}
	if c.ambiguous[sq] {
		return ""
	}
	return c.bySQ[sq]
}

type ImportOptions struct {
	MinDiskBytes uint64
	MinMemBytes  uint64
}

func ImportYear(electionsCSV io.Reader, candidatesCSV io.Reader) (*ImportResult, error) {
	winners, stats, err := readWinnersFromReader(electionsCSV, true)
	if err != nil {
		return nil, err
	}
	cpfs, candidateRows, err := readCPFsFromReader(candidatesCSV, winners)
	if err != nil {
		return nil, err
	}
	stats.CandidateRowsRead = candidateRows
	return buildResult(winners, cpfs, stats), nil
}

func ImportYearFromZipFiles(year int, votacaoZipPath, consultaZipPath, workDir string, opts ImportOptions) (*ImportResult, error) {
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

	votacaoDir := filepath.Join(tmpYearDir, "votacao")
	consultaDir := filepath.Join(tmpYearDir, "consulta")

	stats := ImportStats{}
	if err := unzipAndPrune(votacaoZipPath, votacaoDir); err != nil {
		return nil, err
	}
	if err := ensureSystemResources(tmpYearDir, opts); err != nil {
		return nil, err
	}
	if err := unzipAndPrune(consultaZipPath, consultaDir); err != nil {
		return nil, err
	}

	votacaoFiles, brFile, err := collectVotacaoFiles(year, votacaoDir)
	if err != nil {
		return nil, err
	}
	if brFile == "" {
		stats.MissingBRFile = true
	}

	winners := map[string]winnerRow{}
	if brFile != "" {
		// The office filter applies here too. The _BR file carries the national
		// offices (Presidente, Vice), which are in allowedCargos anyway, so
		// enforcing costs nothing today; skipping enforcement would mean that if
		// TSE ever ships a _BR file for a municipal year, every elected mayor and
		// councillor in it would enter the politician base unfiltered.
		if err := processVotacaoFile(brFile, winners, &stats, true); err != nil {
			return nil, err
		}
		stats.FilesProcessed++
		if err := deleteFile(brFile, &stats); err != nil {
			return nil, err
		}
	}

	for _, f := range votacaoFiles {
		if err := processVotacaoFile(f, winners, &stats, true); err != nil {
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

	consultaFiles, err := collectConsultaFiles(year, consultaDir)
	if err != nil {
		return nil, err
	}
	cpfs := newCPFIndex()
	for _, f := range consultaFiles {
		count, err := processConsultaFile(f, winners, cpfs)
		if err != nil {
			return nil, err
		}
		stats.CandidateRowsRead += count
		stats.FilesProcessed++
		if err := deleteFile(f, &stats); err != nil {
			return nil, err
		}
	}

	stats.WinningCandidates = len(winners)
	return buildResult(winners, cpfs, stats), nil
}

func buildResult(winners map[string]winnerRow, cpfs *cpfIndex, stats ImportStats) *ImportResult {
	result := &ImportResult{Stats: stats}
	result.Records = make([]PoliticianRecord, 0, len(winners))
	for _, winner := range winners {
		sq := winner.SQ
		cpf := normalizeNull(cpfs.lookup(winner.UF, sq))
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

func collectVotacaoFiles(year int, dir string) (ufFiles []string, brFile string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", fmt.Errorf("tse: read votacao dir: %w", err)
	}
	prefix := fmt.Sprintf("VOTACAO_CANDIDATO_MUNZONA_%d_", year)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToUpper(e.Name())
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".CSV") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if strings.HasSuffix(name, "_BR.CSV") {
			brFile = full
			continue
		}
		ufFiles = append(ufFiles, full)
	}
	return ufFiles, brFile, nil
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

func processVotacaoFile(path string, winners map[string]winnerRow, stats *ImportStats, enforceCargo bool) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("tse: open votacao file %s: %w", path, err)
	}
	defer f.Close()

	reader, headers, err := newCSVReaderLatin1(f)
	if err != nil {
		return err
	}
	required := []string{"SQ_CANDIDATO", "NR_TURNO", "DS_SIT_TOT_TURNO", "ANO_ELEICAO", "SG_UF", "SG_PARTIDO", "NM_CANDIDATO", "NM_URNA_CANDIDATO", "NM_SOCIAL_CANDIDATO"}
	if enforceCargo {
		required = append(required, "DS_CARGO")
	}
	if err := ensureHeaders(headers, required); err != nil {
		return err
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tse: read votacao row %s: %w", path, err)
		}
		stats.ElectionRowsRead++

		status := strings.ToUpper(strings.TrimSpace(cell(row, headers, "DS_SIT_TOT_TURNO")))
		if _, ok := allowedStatus[status]; !ok {
			stats.SkippedByStatus++
			continue
		}

		if enforceCargo {
			cargo := strings.ToUpper(strings.TrimSpace(cell(row, headers, "DS_CARGO")))
			if _, ok := allowedCargos[cargo]; !ok {
				stats.SkippedByCargo++
				continue
			}
		}

		sq := strings.TrimSpace(cell(row, headers, "SQ_CANDIDATO"))
		turnRaw := strings.TrimSpace(cell(row, headers, "NR_TURNO"))
		if sq == "" || turnRaw == "" {
			stats.SkippedByInvalidRow++
			continue
		}
		turn, err := strconv.Atoi(turnRaw)
		if err != nil {
			stats.SkippedByInvalidRow++
			continue
		}

		candidate := winnerRow{
			SQ:           sq,
			ElectionYear: strings.TrimSpace(cell(row, headers, "ANO_ELEICAO")),
			Turn:         turn,
			UF:           strings.TrimSpace(cell(row, headers, "SG_UF")),
			Party:        strings.TrimSpace(cell(row, headers, "SG_PARTIDO")),
			Name:         strings.TrimSpace(cell(row, headers, "NM_CANDIDATO")),
			UrnaName:     strings.TrimSpace(cell(row, headers, "NM_URNA_CANDIDATO")),
			SocialName:   strings.TrimSpace(cell(row, headers, "NM_SOCIAL_CANDIDATO")),
		}
		key := candidateKey(candidate.UF, sq)
		if existing, ok := winners[key]; !ok || candidate.Turn > existing.Turn {
			winners[key] = candidate
		}
	}
	return nil
}

func processConsultaFile(path string, winners map[string]winnerRow, cpfs *cpfIndex) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("tse: open consulta file %s: %w", path, err)
	}
	defer f.Close()

	reader, headers, err := newCSVReaderLatin1(f)
	if err != nil {
		return 0, err
	}
	if err := ensureHeaders(headers, []string{"SQ_CANDIDATO", "NR_CPF_CANDIDATO"}); err != nil {
		return 0, err
	}

	count := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, fmt.Errorf("tse: read consulta row %s: %w", path, err)
		}
		count++
		sq := strings.TrimSpace(cell(row, headers, "SQ_CANDIDATO"))
		if sq == "" {
			continue
		}
		// SG_UF is absent from some layouts; the index degrades to an SQ-only
		// join for those rows, guarded against ambiguity.
		uf := strings.TrimSpace(cell(row, headers, "SG_UF"))
		cpfs.add(uf, sq, strings.TrimSpace(cell(row, headers, "NR_CPF_CANDIDATO")))
	}
	return count, nil
}

func readWinnersFromReader(r io.Reader, enforceCargo bool) (map[string]winnerRow, ImportStats, error) {
	stats := ImportStats{}
	reader, headers, err := newCSVReaderLatin1(r)
	if err != nil {
		return nil, stats, err
	}
	required := []string{"SQ_CANDIDATO", "NR_TURNO", "DS_SIT_TOT_TURNO", "ANO_ELEICAO", "SG_UF", "SG_PARTIDO", "NM_CANDIDATO", "NM_URNA_CANDIDATO", "NM_SOCIAL_CANDIDATO"}
	if enforceCargo {
		required = append(required, "DS_CARGO")
	}
	if err := ensureHeaders(headers, required); err != nil {
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
		stats.ElectionRowsRead++
		status := strings.ToUpper(strings.TrimSpace(cell(row, headers, "DS_SIT_TOT_TURNO")))
		if _, ok := allowedStatus[status]; !ok {
			stats.SkippedByStatus++
			continue
		}
		if enforceCargo {
			cargo := strings.ToUpper(strings.TrimSpace(cell(row, headers, "DS_CARGO")))
			if _, ok := allowedCargos[cargo]; !ok {
				stats.SkippedByCargo++
				continue
			}
		}
		sq := strings.TrimSpace(cell(row, headers, "SQ_CANDIDATO"))
		turn, err := strconv.Atoi(strings.TrimSpace(cell(row, headers, "NR_TURNO")))
		if sq == "" || err != nil {
			stats.SkippedByInvalidRow++
			continue
		}
		candidate := winnerRow{
			SQ:           sq,
			ElectionYear: strings.TrimSpace(cell(row, headers, "ANO_ELEICAO")),
			Turn:         turn,
			UF:           strings.TrimSpace(cell(row, headers, "SG_UF")),
			Party:        strings.TrimSpace(cell(row, headers, "SG_PARTIDO")),
			Name:         strings.TrimSpace(cell(row, headers, "NM_CANDIDATO")),
			UrnaName:     strings.TrimSpace(cell(row, headers, "NM_URNA_CANDIDATO")),
			SocialName:   strings.TrimSpace(cell(row, headers, "NM_SOCIAL_CANDIDATO")),
		}
		key := candidateKey(candidate.UF, sq)
		if existing, ok := winners[key]; !ok || candidate.Turn > existing.Turn {
			winners[key] = candidate
		}
	}
	stats.WinningCandidates = len(winners)
	return winners, stats, nil
}

func readCPFsFromReader(r io.Reader, winners map[string]winnerRow) (*cpfIndex, int, error) {
	reader, headers, err := newCSVReaderLatin1(r)
	if err != nil {
		return nil, 0, err
	}
	if err := ensureHeaders(headers, []string{"SQ_CANDIDATO", "NR_CPF_CANDIDATO"}); err != nil {
		return nil, 0, err
	}
	cpfs := newCPFIndex()
	count := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, count, err
		}
		count++
		sq := strings.TrimSpace(cell(row, headers, "SQ_CANDIDATO"))
		if sq == "" {
			continue
		}
		uf := strings.TrimSpace(cell(row, headers, "SG_UF"))
		cpfs.add(uf, sq, strings.TrimSpace(cell(row, headers, "NR_CPF_CANDIDATO")))
	}
	return cpfs, count, nil
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

func normalizeNull(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "#NULO" || v == "#NE" || v == "-1" || v == "-3" {
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
