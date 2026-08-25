package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	baseDir = "/home/lowelodev/ai-lab/bench/robustness-battery"
	runsDir = "/home/lowelodev/ai-lab/bench/runs"
	gocache = "/tmp/gocache-battery"
)

var resultsFile = baseDir + "/results.json"
var logFile = baseDir + "/run-all.log"

// Config ist die Struktur einer battery.json je Task.
type Config struct {
	Task       string         `json:"task"`
	Lang       string         `json:"lang"`
	Cwd        string         `json:"cwd"`
	Install    []InstallEntry `json:"install"`
	Cmd        string         `json:"cmd"`
	RealPrefix string         `json:"real_prefix"`
	PathPrefix string         `json:"path_prefix"`
	Timeout    int            `json:"timeout"`
}

type InstallEntry struct {
	Src string `json:"src"`
	Dst string `json:"dst"`
}

// BatteryMeta beschreibt eine Batterie aus den Testnamen der Quell-Datei(en).
type BatteryMeta struct {
	Lang      string   `json:"lang"`
	RealTotal int      `json:"real_total"`
	PathTotal int      `json:"path_total"`
	Tests     []string `json:"tests"`
}

// TaskResult ist das Ergebnis je (task,label)-Paar. Bei buildable:false sind die
// Pass-Zaehler null und "error" gesetzt (Schema 2 der Konvention).
type TaskResult struct {
	Buildable     bool     `json:"buildable"`
	RealPass      *int     `json:"real_pass"`
	RealTotal     int      `json:"real_total"`
	PathPass      *int     `json:"path_pass"`
	PathTotal     int      `json:"path_total"`
	Failed        []string `json:"failed"`
	WsFingerprint string   `json:"ws_fingerprint"`
	Computed      string   `json:"computed"`
	Seconds       float64  `json:"seconds"`
	Error         string   `json:"error,omitempty"`
}

type Results struct {
	Schema    int                              `json:"schema"`
	Generated string                           `json:"generated"`
	Batteries map[string]BatteryMeta           `json:"batteries"`
	Results   map[string]map[string]TaskResult `json:"results"`
}

type pair struct {
	task  string
	label string
	ws    string
	cfg   *Config
	meta  BatteryMeta
}

var goTestRe = regexp.MustCompile(`(?m)^func (TestZZBat[A-Za-z0-9_]+)\(`)
var nodeTestRe = regexp.MustCompile(`test(?:\.\w+)?\(\s*['"](ZZBat[^'"]*)['"]`)

// testMatches prueft den Konfig-Prefix (z. B. "ZZBatReal") gegen einen Testnamen.
// Go-Event-Namen tragen das fachliche "Test"-Praefix (TestZZBatReal*), Node-TAP-Namen
// beginnen direkt mit "ZZBatReal" — beides muss demselben real_prefix/path_prefix
// zugeordnet werden.
func testMatches(name, prefix string) bool {
	return strings.HasPrefix(name, prefix) || strings.HasPrefix(name, "Test"+prefix)
}

func main() {
	force := flag.Bool("force", false, "alle (task,label)-Paare neu berechnen")
	taskF := flag.String("task", "", "nur diesen Task bearbeiten")
	labelF := flag.String("label", "", "nur dieses Label bearbeiten")
	jobs := flag.Int("jobs", 2, "Parallelitaet")
	flag.Parse()
	if *jobs < 1 {
		*jobs = 1
	}

	cfgs, err := loadConfigs()
	if err != nil {
		fmt.Fprintln(os.Stderr, "battery:", err)
		os.Exit(1)
	}

	var pairs []pair
	for _, cfg := range cfgs {
		if *taskF != "" && cfg.Task != *taskF {
			continue
		}
		meta := deriveMeta(&cfg)
		globs, _ := filepath.Glob(filepath.Join(runsDir, "*", cfg.Task, "ws"))
		for _, ws := range globs {
			st, err := os.Stat(ws)
			if err != nil || !st.IsDir() {
				continue
			}
			label := filepath.Base(filepath.Dir(filepath.Dir(ws)))
			if *labelF != "" && label != *labelF {
				continue
			}
			pairs = append(pairs, pair{task: cfg.Task, label: label, ws: ws, cfg: &cfg, meta: meta})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].task != pairs[j].task {
			return pairs[i].task < pairs[j].task
		}
		return pairs[i].label < pairs[j].label
	})

	res := loadResults()
	if res.Results == nil {
		res.Results = map[string]map[string]TaskResult{}
	}
	for _, cfg := range cfgs {
		if res.Results[cfg.Task] == nil {
			res.Results[cfg.Task] = map[string]TaskResult{}
		}
	}

	logf, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "battery: log:", err)
		os.Exit(1)
	}
	defer logf.Close()

	var mu sync.Mutex
	changed := false
	recomputed := 0

	queue := make(chan pair)
	var wg sync.WaitGroup
	for i := 0; i < *jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range queue {
				fp, ferr := fingerprint(p.ws, p.cfg)
				if ferr != nil {
					// Fingerprint nicht moeglich (z. B. cwd fehlt) — als nicht
					// baubar verbuchen, damit der Fall sichtbar bleibt.
					tr := buildFailResult(&p, "", ferr.Error(), 0)
					mu.Lock()
					res.Results[p.task][p.label] = tr
					changed = true
					recomputed++
					logf.WriteString(logLine(&p, &tr, "fingerprint-error"))
					mu.Unlock()
					continue
				}
				mu.Lock()
				cur, exists := res.Results[p.task][p.label]
				mu.Unlock()
				if !*force && exists && cur.WsFingerprint == fp {
					continue
				}
				tr := runPair(&p, fp)
				mu.Lock()
				res.Results[p.task][p.label] = tr
				changed = true
				recomputed++
				logf.WriteString(logLine(&p, &tr, ""))
				mu.Unlock()
			}
		}()
	}
	for _, p := range pairs {
		queue <- p
	}
	close(queue)
	wg.Wait()
	logf.Sync()

	newMeta := map[string]BatteryMeta{}
	for _, cfg := range cfgs {
		newMeta[cfg.Task] = deriveMeta(&cfg)
	}
	metaChanged := !jsonEqual(res.Batteries, newMeta)

	if changed || metaChanged {
		res.Schema = 2
		res.Generated = time.Now().UTC().Format(time.RFC3339)
		res.Batteries = newMeta
		if err := writeResults(res); err != nil {
			fmt.Fprintln(os.Stderr, "battery: write:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("battery: %d Paare geprueft, %d neu berechnet\n", len(pairs), recomputed)
}

func logLine(p *pair, tr *TaskResult, note string) string {
	rp, pp := "-", "-"
	if tr.RealPass != nil {
		rp = fmt.Sprintf("%d/%d", *tr.RealPass, tr.RealTotal)
	}
	if tr.PathPass != nil {
		pp = fmt.Sprintf("%d/%d", *tr.PathPass, tr.PathTotal)
	}
	s := fmt.Sprintf("%s task=%s label=%s buildable=%v real=%s path=%s fp=%s sec=%.2f",
		time.Now().Format(time.RFC3339), p.task, p.label, tr.Buildable, rp, pp, tr.WsFingerprint, tr.Seconds)
	if note != "" {
		s += " note=" + note
	}
	if tr.Error != "" {
		s += " error=" + tr.Error
	}
	return s + "\n"
}

// loadConfigs liest alle robustness-battery/*/battery.json; Verzeichnisse ohne
// battery.json (a5/, a6/) werden uebersprungen.
func loadConfigs() ([]Config, error) {
	dirs, err := filepath.Glob(filepath.Join(baseDir, "*", "battery.json"))
	if err != nil {
		return nil, err
	}
	var cfgs []Config
	for _, f := range dirs {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var c Config
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if c.Timeout <= 0 {
			c.Timeout = 150
		}
		cfgs = append(cfgs, c)
	}
	sort.Slice(cfgs, func(i, j int) bool { return cfgs[i].Task < cfgs[j].Task })
	return cfgs, nil
}

// deriveMeta liest die Testnamen aus den Install-Quellen der Batterie.
func deriveMeta(cfg *Config) BatteryMeta {
	m := BatteryMeta{Lang: cfg.Lang}
	seen := map[string]bool{}
	for _, inst := range cfg.Install {
		b, err := os.ReadFile(filepath.Join(baseDir, cfg.Task, inst.Src))
		if err != nil {
			continue
		}
		var names []string
		if cfg.Lang == "go" {
			for _, sm := range goTestRe.FindAllStringSubmatch(string(b), -1) {
				names = append(names, sm[1])
			}
		} else {
			for _, sm := range nodeTestRe.FindAllStringSubmatch(string(b), -1) {
				names = append(names, sm[1])
			}
		}
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				m.Tests = append(m.Tests, n)
			}
		}
	}
	for _, t := range m.Tests {
		if testMatches(t, cfg.RealPrefix) {
			m.RealTotal++
		}
		if testMatches(t, cfg.PathPrefix) {
			m.PathTotal++
		}
	}
	return m
}

// fingerprint ist der sha256 ueber sortierte relative Pfade + Inhalte aller
// Nicht-Test-Quelldateien unter ws/<cwd>.
func fingerprint(ws string, cfg *Config) (string, error) {
	root := filepath.Join(ws, cfg.Cwd)
	entries, err := collectSourceFiles(root, cfg.Lang)
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	h := sha256.New()
	for _, rel := range entries {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", err
		}
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func collectSourceFiles(root, lang string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" {
				return fs.SkipDir
			}
			if lang == "node" && (name == "node_modules" || name == "test" || name == "tests") {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		switch lang {
		case "go":
			if strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") {
				files = append(files, rel)
			}
		case "node":
			if strings.HasSuffix(rel, ".js") || strings.HasSuffix(rel, ".mjs") || strings.HasSuffix(rel, ".ts") {
				files = append(files, rel)
			}
		}
		return nil
	})
	return files, err
}

// runPair kopiert den Workspace in ein Wegwerf-Verzeichnis, kopiert die
// Batterie-Dateien ein und fuehrt cmd mit Timeout aus.
func runPair(p *pair, fp string) TaskResult {
	start := time.Now()
	tmp, err := os.MkdirTemp("", "battery-ws-*")
	if err != nil {
		return buildFailResult(p, fp, "tmp: "+err.Error(), time.Since(start).Seconds())
	}
	defer os.RemoveAll(tmp)

	if err := copyTree(p.ws, tmp, p.cfg.Lang); err != nil {
		return buildFailResult(p, fp, "copy: "+err.Error(), time.Since(start).Seconds())
	}
	for _, inst := range p.cfg.Install {
		src := filepath.Join(baseDir, p.task, inst.Src)
		dst := filepath.Join(tmp, p.cfg.Cwd, inst.Dst)
		if err := copyFile(src, dst); err != nil {
			return buildFailResult(p, fp, "install "+inst.Src+": "+err.Error(), time.Since(start).Seconds())
		}
	}
	workdir := filepath.Join(tmp, p.cfg.Cwd)
	if st, err := os.Stat(workdir); err != nil || !st.IsDir() {
		return buildFailResult(p, fp, "cwd nicht gefunden: "+p.cfg.Cwd, time.Since(start).Seconds())
	}

	timeout := time.Duration(p.cfg.Timeout) * time.Second
	stdout, stderr, rc, killed := runCmd(workdir, p.cfg.Cmd, timeout)

	var tr TaskResult
	if killed {
		tr = buildFailResult(p, fp, fmt.Sprintf("cmd nach %ds abgebrochen (Prozessgruppe)", p.cfg.Timeout), time.Since(start).Seconds())
	} else if p.cfg.Lang == "go" {
		tr = parseGoOutput(p, stdout, stderr, rc)
	} else {
		tr = parseTAP(p, stdout, stderr, rc)
	}
	tr.WsFingerprint = fp
	tr.Computed = time.Now().UTC().Format(time.RFC3339)
	tr.Seconds = time.Since(start).Seconds()
	return tr
}

func buildFailResult(p *pair, fp, errMsg string, sec float64) TaskResult {
	tr := TaskResult{
		Buildable:     false,
		RealPass:      nil,
		PathPass:      nil,
		RealTotal:     p.meta.RealTotal,
		PathTotal:     p.meta.PathTotal,
		Failed:        []string{},
		WsFingerprint: fp,
		Computed:      time.Now().UTC().Format(time.RFC3339),
		Seconds:       sec,
		Error:         errMsg,
	}
	return tr
}

func runCmd(dir, cmdStr string, timeout time.Duration) (stdout, stderr string, rc int, killed bool) {
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOFLAGS=-mod=mod", "GOCACHE="+gocache)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		return out.String(), errb.String(), -1, false
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		rc = 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				rc = ee.ExitCode()
			} else {
				rc = -1
			}
		}
		return out.String(), errb.String(), rc, false
	case <-time.After(timeout):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return out.String(), errb.String(), -1, true
	}
}

type goEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
	Output string `json:"Output"`
}

// parseGoOutput wertet go test -json aus. Build-Fehler (Action build-fail)
// ergeben buildable:false; die erste Fehlerzeile landet in Error.
func parseGoOutput(p *pair, stdout, stderr string, rc int) TaskResult {
	buildFailed := false
	var buildOut []string
	verdict := map[string]string{}
	var order []string
	sc := bufio.NewScanner(strings.NewReader(stdout))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev goEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		switch ev.Action {
		case "build-output":
			if o := strings.TrimSpace(ev.Output); o != "" {
				buildOut = append(buildOut, o)
			}
		case "build-fail":
			buildFailed = true
		}
		if ev.Test == "" {
			continue
		}
		if ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip" {
			if _, seen := verdict[ev.Test]; !seen {
				order = append(order, ev.Test)
			}
			verdict[ev.Test] = ev.Action
		}
	}
	if buildFailed || (rc != 0 && len(order) == 0) {
		return buildFailResult(p, "", firstErrLine(buildOut, stderr), 0)
	}
	return tally(p, verdict, order)
}

func tally(p *pair, verdict map[string]string, order []string) TaskResult {
	tr := TaskResult{Buildable: true, Failed: []string{}}
	type counts struct {
		pass, fail, skip int
		failed           []string
	}
	real, path := counts{}, counts{}
	for _, name := range order {
		var c *counts
		switch {
		case testMatches(name, p.cfg.RealPrefix):
			c = &real
		case testMatches(name, p.cfg.PathPrefix):
			c = &path
		default:
			continue
		}
		switch verdict[name] {
		case "pass":
			c.pass++
		case "fail":
			c.fail++
			c.failed = append(c.failed, name)
		case "skip":
			c.skip++
		}
	}
	tr.RealTotal = real.pass + real.fail + real.skip
	tr.PathTotal = path.pass + path.fail + path.skip
	rp := real.pass
	pp := path.pass
	tr.RealPass = &rp
	tr.PathPass = &pp
	tr.Failed = []string{}
	tr.Failed = append(tr.Failed, real.failed...)
	tr.Failed = append(tr.Failed, path.failed...)
	sort.Strings(tr.Failed)
	return tr
}

func firstErrLine(buildOut []string, stderr string) string {
	for _, l := range buildOut {
		if strings.Contains(l, ".go:") {
			return l
		}
	}
	if len(buildOut) > 0 {
		return buildOut[0]
	}
	for _, l := range strings.Split(stderr, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return "build failed"
}

// parseTAP wertet node --test TAP-Ausgabe aus. Gezaehlt werden nur
// Top-Level-Zeilen "ok N - Name" / "not ok N - Name" (Subtests sind eingerueckt).
func parseTAP(p *pair, stdout, stderr string, rc int) TaskResult {
	type counts struct {
		pass, fail, skip int
		failed           []string
	}
	real, path := counts{}, counts{}
	matched := 0
	topRe := regexp.MustCompile(`^(ok|not ok) \d+ - (.+)$`)
	sc := bufio.NewScanner(strings.NewReader(stdout))
	for sc.Scan() {
		m := topRe.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		ok := m[1] == "ok"
		name := m[2]
		skip := false
		if idx := strings.Index(name, "# SKIP"); idx >= 0 {
			name = strings.TrimSpace(name[:idx])
			skip = true
		}
		var c *counts
		switch {
		case testMatches(name, p.cfg.RealPrefix):
			c = &real
		case testMatches(name, p.cfg.PathPrefix):
			c = &path
		default:
			continue
		}
		matched++
		if ok && !skip {
			c.pass++
		} else if ok && skip {
			c.skip++
		} else {
			c.fail++
			c.failed = append(c.failed, name)
		}
	}
	if matched == 0 && rc != 0 {
		// Start-/Lauf-Fehler ohne TAP-Tests => nicht baubar.
		tr := TaskResult{
			Buildable: false,
			RealPass:  nil,
			PathPass:  nil,
			RealTotal: p.meta.RealTotal,
			PathTotal: p.meta.PathTotal,
			Failed:    []string{},
			Error:     firstNonEmpty(stderr, stdout),
		}
		return tr
	}
	tr := TaskResult{Buildable: true, Failed: []string{}}
	tr.RealTotal = real.pass + real.fail + real.skip
	tr.PathTotal = path.pass + path.fail + path.skip
	rp := real.pass
	pp := path.pass
	tr.RealPass = &rp
	tr.PathPass = &pp
	// Statt zwei Nils zu verketten (was bei lauter gruenen Tests failed:null ergaebe und das
	// Schema-2-Feld "failed": [...] verletzt) wird bewusst von einer leeren Slice aus begonnen,
	// damit die Liste auch im Erfolgsfall als [] serialisiert wird (Schema-konform).
	tr.Failed = append([]string{}, real.failed...)
	tr.Failed = append(tr.Failed, path.failed...)
	sort.Strings(tr.Failed)
	return tr
}

func firstNonEmpty(a, b string) string {
	for _, s := range []string{a, b} {
		for _, l := range strings.Split(s, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				return l
			}
		}
	}
	return "noch keine Ausgabe"
}

// copyTree kopiert den Workspace; .git wird uebersprungen, node_modules wird
// auf das Original verlinkt statt kopiert.
func copyTree(src, dst, lang string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		tgt := filepath.Join(dst, rel)
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			if lang == "node" && d.Name() == "node_modules" {
				if err := os.Symlink(path, tgt); err != nil {
					return err
				}
				return fs.SkipDir
			}
			return os.MkdirAll(tgt, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, tgt)
		}
		return copyFile(path, tgt)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func loadResults() *Results {
	r := &Results{
		Schema:    2,
		Batteries: map[string]BatteryMeta{},
		Results:   map[string]map[string]TaskResult{},
	}
	b, err := os.ReadFile(resultsFile)
	if err != nil {
		return r
	}
	_ = json.Unmarshal(b, r)
	if r.Batteries == nil {
		r.Batteries = map[string]BatteryMeta{}
	}
	if r.Results == nil {
		r.Results = map[string]map[string]TaskResult{}
	}
	return r
}

func writeResults(res *Results) error {
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := resultsFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, resultsFile)
}

func jsonEqual(a, b map[string]BatteryMeta) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return bytes.Equal(aj, bj)
}
