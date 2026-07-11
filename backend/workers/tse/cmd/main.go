package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"corruption-center/db/memgraph"
	"corruption-center/db/psql"
	"corruption-center/workers/camara"
	"corruption-center/workers/senado"
	"corruption-center/workers/tse"
)

type output struct {
	Runs []yearRun `json:"runs"`
}

type yearRun struct {
	Year    int                    `json:"year"`
	Stats   tse.ImportStats        `json:"stats"`
	Records []tse.PoliticianRecord `json:"records"`
}

func main() {
	var (
		year        = flag.Int("year", 0, "Election year, e.g. 2022")
		fromYear    = flag.Int("from-year", 0, "Start year (inclusive) for range mode")
		toYear      = flag.Int("to-year", 0, "End year (inclusive) for range mode")
		allYears    = flag.Bool("all-years", false, "Run all even years from 2002 to current")
		votacaoZip  = flag.String("votacao-zip", "", "Path to votacao_candidato_munzona_{year}.zip (single-year mode only)")
		consultaZip = flag.String("consulta-zip", "", "Path to consulta_cand_{year}.zip (single-year mode only)")
		zipDir      = flag.String("zip-dir", "", "Directory containing yearly zip files for range/all modes")
		workDir     = flag.String("workdir", os.TempDir(), "Working directory for extraction and processing")
		minDiskMB   = flag.Uint64("min-disk-mb", 512, "Minimum free disk required in MB")
		minMemMB    = flag.Uint64("min-mem-mb", 256, "Minimum available memory required in MB")
		persistDB   = flag.Bool("persist-db", false, "Persist output to Postgres/Memgraph")
		skipDone    = flag.Bool("skip-processed", true, "Skip year when tse_import_log status is success")
		batchSize   = flag.Int("batch-size", 500, "Memgraph upsert batch size")
		triggerCam  = flag.Bool("trigger-camara", false, "Run Camara sync after TSE import completes")
		triggerSen  = flag.Bool("trigger-senado", false, "Run Senado sync after TSE import completes")
	)
	flag.Parse()
	ctx := context.Background()

	if (*triggerCam || *triggerSen) && !*persistDB {
		fmt.Fprintln(os.Stderr, "invalid flags: --trigger-camara/--trigger-senado require --persist-db")
		os.Exit(2)
	}

	years, err := resolveYears(*year, *fromYear, *toYear, *allYears)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid flags: %v\n", err)
		flag.Usage()
		os.Exit(2)
	}

	if len(years) > 1 && *zipDir == "" {
		fmt.Fprintln(os.Stderr, "info: --zip-dir not provided, files will be downloaded from TSE")
	}

	runs := make([]yearRun, 0, len(years))
	var (
		pg *psql.DB
		mg *memgraph.DB
	)
	if *persistDB {
		log := slog.New(slog.NewTextHandler(os.Stdout, nil))
		var err error
		pg, err = psql.New(ctx, mustEnv("DATABASE_URL"), log)
		if err != nil {
			fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
			os.Exit(1)
		}
		defer pg.Close()

		mg, err = memgraph.New(ctx, mustEnv("MEMGRAPH_URI"), os.Getenv("MEMGRAPH_USER"), os.Getenv("MEMGRAPH_PASS"), pg, log)
		if err != nil {
			fmt.Fprintf(os.Stderr, "connect memgraph: %v\n", err)
			os.Exit(1)
		}
		defer mg.Close(ctx)
	}

	for _, y := range years {
		fmt.Fprintf(os.Stderr, "[tse] year=%d starting\n", y)
		if *persistDB && *skipDone {
			done, err := pg.IsTSEYearSuccessful(ctx, y)
			if err != nil {
				fmt.Fprintf(os.Stderr, "check processed year %d: %v\n", y, err)
				os.Exit(1)
			}
			if done {
				runs = append(runs, yearRun{Year: y, Stats: tse.ImportStats{}})
				continue
			}
		}

		var jobID string
		if *persistDB {
			var err error
			jobID, err = pg.CreateJob(ctx, "tse_csv_import")
			if err != nil {
				fmt.Fprintf(os.Stderr, "create scraper job for year %d: %v\n", y, err)
				os.Exit(1)
			}
			if err := pg.UpsertTSEImportLog(ctx, y, psql.JobStatusRunning, 0, nil); err != nil {
				fmt.Fprintf(os.Stderr, "mark tse log running for year %d: %v\n", y, err)
				os.Exit(1)
			}
		}

		vPath, cPath, err := resolveZipPaths(y, *zipDir, *votacaoZip, *consultaZip)
		if err != nil {
			if *persistDB {
				errMsg := err.Error()
				_ = pg.UpdateJob(ctx, jobID, psql.JobStatusFailed, 0, &errMsg)
				_ = pg.UpsertTSEImportLog(ctx, y, psql.JobStatusFailed, 0, &errMsg)
			}
			fmt.Fprintf(os.Stderr, "resolve year %d zip paths: %v\n", y, err)
			os.Exit(1)
		}
		if vPath == "" || cPath == "" {
			downloadDir := filepath.Join(*workDir, "tse-downloads")
			if err := os.MkdirAll(downloadDir, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "create download dir: %v\n", err)
				os.Exit(1)
			}
			vPath, cPath, err = downloadYearZips(y, downloadDir)
			if err != nil {
				if *persistDB {
					errMsg := err.Error()
					_ = pg.UpdateJob(ctx, jobID, psql.JobStatusFailed, 0, &errMsg)
					_ = pg.UpsertTSEImportLog(ctx, y, psql.JobStatusFailed, 0, &errMsg)
				}
				fmt.Fprintf(os.Stderr, "download year %d zips: %v\n", y, err)
				os.Exit(1)
			}
		}

		result, err := tse.ImportYearFromZipFiles(
			y,
			vPath,
			cPath,
			*workDir,
			tse.ImportOptions{
				MinDiskBytes: *minDiskMB * 1024 * 1024,
				MinMemBytes:  *minMemMB * 1024 * 1024,
			},
		)
		if err != nil {
			if *persistDB {
				errMsg := err.Error()
				_ = pg.UpdateJob(ctx, jobID, psql.JobStatusFailed, 0, &errMsg)
				_ = pg.UpsertTSEImportLog(ctx, y, psql.JobStatusFailed, 0, &errMsg)
			}
			fmt.Fprintf(os.Stderr, "tse worker failed for year %d: %v\n", y, err)
			os.Exit(1)
		}
		if vPath != "" && strings.Contains(vPath, "tse-downloads") {
			_ = os.Remove(vPath)
		}
		if cPath != "" && strings.Contains(cPath, "tse-downloads") {
			_ = os.Remove(cPath)
		}

		upserted := len(result.Records)
		if *persistDB {
			upserted, err = mg.UpsertPoliticiansFromTSE(ctx, result.Records, *batchSize)
			if err != nil {
				errMsg := err.Error()
				_ = pg.UpdateJob(ctx, jobID, psql.JobStatusFailed, 0, &errMsg)
				_ = pg.UpsertTSEImportLog(ctx, y, psql.JobStatusFailed, 0, &errMsg)
				fmt.Fprintf(os.Stderr, "persist year %d to memgraph: %v\n", y, err)
				os.Exit(1)
			}
			if err := pg.UpdateJob(ctx, jobID, psql.JobStatusSuccess, upserted, nil); err != nil {
				fmt.Fprintf(os.Stderr, "finalize scraper job year %d: %v\n", y, err)
				os.Exit(1)
			}
			if err := pg.UpsertTSEImportLog(ctx, y, psql.JobStatusSuccess, upserted, nil); err != nil {
				fmt.Fprintf(os.Stderr, "finalize tse log year %d: %v\n", y, err)
				os.Exit(1)
			}
		}

		fmt.Fprintf(os.Stderr, "[tse] year=%d finished records=%d files_processed=%d files_deleted=%d\n", y, len(result.Records), result.Stats.FilesProcessed, result.Stats.FilesDeleted)

		runs = append(runs, yearRun{Year: y, Stats: result.Stats, Records: result.Records})
	}

	out := output{Runs: runs}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "encode output: %v\n", err)
		os.Exit(1)
	}

	if *persistDB && *triggerCam {
		if err := runCamaraSync(ctx, pg, mg, *batchSize); err != nil {
			fmt.Fprintf(os.Stderr, "trigger camara sync failed: %v\n", err)
			os.Exit(1)
		}
	}

	if *persistDB && *triggerSen {
		if err := runSenadoSync(ctx, pg, mg, *batchSize); err != nil {
			fmt.Fprintf(os.Stderr, "trigger senado sync failed: %v\n", err)
			os.Exit(1)
		}
	}
}

func runCamaraSync(ctx context.Context, pg *psql.DB, mg *memgraph.DB, batchSize int) error {
	fmt.Fprintln(os.Stderr, "[tse] triggering Camara sync")
	res, err := camara.SyncCurrentDeputies(ctx, camara.SyncOptions{Items: 100})
	if err != nil {
		return err
	}
	run, err := pg.CreateCamaraSyncRun(ctx)
	if err != nil {
		return err
	}
	upserted, err := mg.UpsertPoliticiansFromCamara(ctx, res.Records, batchSize)
	if err != nil {
		errMsg := err.Error()
		_ = pg.FinalizeCamaraSyncRun(ctx, run.ID, psql.JobStatusFailed, res.Stats, 0, &errMsg)
		return err
	}
	if err := pg.FinalizeCamaraSyncRun(ctx, run.ID, psql.JobStatusSuccess, res.Stats, upserted, nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[tse] camara sync finished upserted=%d listed=%d\n", upserted, res.Stats.ListedDeputies)
	return nil
}

func runSenadoSync(ctx context.Context, pg *psql.DB, mg *memgraph.DB, batchSize int) error {
	fmt.Fprintln(os.Stderr, "[tse] triggering Senado sync")
	res, err := senado.SyncCurrentSenators(ctx, senado.SyncOptions{})
	if err != nil {
		return err
	}
	run, err := pg.CreateSenadoSyncRun(ctx)
	if err != nil {
		return err
	}
	upserted, err := mg.UpsertPoliticiansFromSenado(ctx, res.Records, batchSize)
	if err != nil {
		errMsg := err.Error()
		_ = pg.FinalizeSenadoSyncRun(ctx, run.ID, psql.JobStatusFailed, res.Stats, 0, &errMsg)
		return err
	}
	if err := pg.FinalizeSenadoSyncRun(ctx, run.ID, psql.JobStatusSuccess, res.Stats, upserted, nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[tse] senado sync finished upserted=%d listed=%d\n", upserted, res.Stats.ListedSenators)
	return nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		fmt.Fprintf(os.Stderr, "missing required environment variable: %s\n", key)
		os.Exit(2)
	}
	return v
}

func resolveYears(year, fromYear, toYear int, allYears bool) ([]int, error) {
	currentYear := time.Now().Year()
	// election years are always even
	latestElectionYear := currentYear
	if latestElectionYear%2 != 0 {
		latestElectionYear--
	}

	modeCount := 0
	if year > 0 {
		modeCount++
	}
	if fromYear > 0 || toYear > 0 {
		modeCount++
	}
	if allYears {
		modeCount++
	}
	if modeCount != 1 {
		return nil, fmt.Errorf("choose exactly one mode: --year OR --from-year/--to-year OR --all-years")
	}

	if year > 0 {
		if year < 2002 || year > latestElectionYear || year%2 != 0 {
			return nil, fmt.Errorf("--year must be an even number between 2002 and %d", latestElectionYear)
		}
		return []int{year}, nil
	}

	if allYears {
		years := make([]int, 0, (latestElectionYear-2002)/2+1)
		for y := 2002; y <= latestElectionYear; y += 2 {
			years = append(years, y)
		}
		return years, nil
	}

	if fromYear <= 0 || toYear <= 0 || fromYear > toYear {
		return nil, fmt.Errorf("invalid range: use --from-year <= --to-year and both > 0")
	}
	if fromYear%2 != 0 || toYear%2 != 0 {
		return nil, fmt.Errorf("range mode expects even election years")
	}
	if fromYear < 2002 || toYear > latestElectionYear {
		return nil, fmt.Errorf("range must be between 2002 and %d", latestElectionYear)
	}

	years := make([]int, 0, ((toYear-fromYear)/2)+1)
	for y := fromYear; y <= toYear; y += 2 {
		years = append(years, y)
	}
	return years, nil
}

func resolveZipPaths(year int, zipDir, singleVotacao, singleConsulta string) (string, string, error) {
	if zipDir == "" {
		if singleVotacao == "" && singleConsulta == "" {
			return "", "", nil
		}
		if singleVotacao == "" || singleConsulta == "" {
			return "", "", fmt.Errorf("if one zip path is set, both --votacao-zip and --consulta-zip are required")
		}
		return singleVotacao, singleConsulta, nil
	}
	v := filepath.Join(zipDir, fmt.Sprintf("votacao_candidato_munzona_%d.zip", year))
	c := filepath.Join(zipDir, fmt.Sprintf("consulta_cand_%d.zip", year))
	if _, err := os.Stat(v); err != nil {
		return "", "", nil
	}
	if _, err := os.Stat(c); err != nil {
		return "", "", nil
	}
	return v, c, nil
}

func downloadYearZips(year int, dir string) (string, string, error) {
	vURL := fmt.Sprintf("https://cdn.tse.jus.br/estatistica/sead/odsele/votacao_candidato_munzona/votacao_candidato_munzona_%d.zip", year)
	cURL := fmt.Sprintf("https://cdn.tse.jus.br/estatistica/sead/odsele/consulta_cand/consulta_cand_%d.zip", year)
	vPath := filepath.Join(dir, fmt.Sprintf("votacao_candidato_munzona_%d.zip", year))
	cPath := filepath.Join(dir, fmt.Sprintf("consulta_cand_%d.zip", year))

	if err := downloadFileIfMissing(vURL, vPath); err != nil {
		return "", "", err
	}
	if err := downloadFileIfMissing(cURL, cPath); err != nil {
		return "", "", err
	}
	return vPath, cPath, nil
}

// downloadRetries bounds the resume attempts for one file. The TSE CDN routinely
// resets the connection partway through these multi-hundred-MB zips, so a single
// GET is not enough: each attempt resumes the partial .tmp with a Range request
// instead of starting over.
const downloadRetries = 6

func downloadFileIfMissing(url, dest string) error {
	if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
		fmt.Fprintf(os.Stderr, "[tse] using cached %s\n", dest)
		return nil
	}
	fmt.Fprintf(os.Stderr, "[tse] downloading %s\n", url)

	tmp := dest + ".tmp"
	var lastErr error
	for attempt := 1; attempt <= downloadRetries; attempt++ {
		done, err := resumeDownload(url, tmp)
		if done {
			if err := os.Rename(tmp, dest); err != nil {
				_ = os.Remove(tmp)
				return fmt.Errorf("rename %s -> %s: %w", tmp, dest, err)
			}
			return nil
		}
		lastErr = err
		var have int64
		if st, serr := os.Stat(tmp); serr == nil {
			have = st.Size()
		}
		fmt.Fprintf(os.Stderr, "[tse] download interrupted at %d bytes (attempt %d/%d): %v\n",
			have, attempt, downloadRetries, err)
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	_ = os.Remove(tmp)
	return fmt.Errorf("download %s: giving up after %d attempts: %w", url, downloadRetries, lastErr)
}

// resumeDownload appends to tmp from wherever it left off. It reports done=true
// when the body was fully copied; on a mid-transfer failure it leaves tmp in
// place so the next attempt can resume from it.
func resumeDownload(url, tmp string) (bool, error) {
	var have int64
	if st, err := os.Stat(tmp); err == nil {
		have = st.Size()
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusPartialContent:
		// Server honoured the Range: append.
	case http.StatusOK:
		// No range support (or a fresh start): rewrite from byte zero.
		have = 0
	default:
		return false, fmt.Errorf("status %d", resp.StatusCode)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if have > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	out, err := os.OpenFile(tmp, flags, 0o644)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", tmp, err)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return false, fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := out.Close(); err != nil {
		return false, fmt.Errorf("close %s: %w", tmp, err)
	}
	return true, nil
}
