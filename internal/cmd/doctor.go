package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/qjebbs/go-jsons"
	"github.com/spf13/cobra"
	"github.com/wiesnermarven-hue/salameyy/internal/charmdocs"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
	"github.com/wiesnermarven-hue/salameyy/internal/home"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Report SALAMEY config, command, docs, and data status",
	Args:  cobra.NoArgs,
	Example: `
# Inspect the current project
salamey doctor

# Inspect another working directory
salamey doctor --cwd /path/to/project
  `,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}

		dataDirFlag, _ := cmd.Flags().GetString("data-dir")
		report := inspectDoctor(cwd, dataDirFlag)

		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "SECTION\tSTATUS\tDETAIL")

		for _, row := range report.rows() {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", row.Section, row.Status, row.Detail)
		}

		return tw.Flush()
	},
}

type doctorReport struct {
	cwd         string
	configInfo  doctorConfigInfo
	docsInfo    doctorDocsInfo
	commandInfo []doctorStatusRow
	dataInfo    []doctorStatusRow
}

type doctorConfigInfo struct {
	globalPath        string
	globalDataPath    string
	projectConfigPath []string
	loadError         error
	resolvedDataDir   string
	initializeAs      string
}

type doctorDocsInfo struct {
	status string
	detail string
}

type doctorStatusRow struct {
	Section string
	Status  string
	Detail  string
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func inspectDoctor(cwd, dataDirFlag string) doctorReport {
	configInfo := inspectDoctorConfig(cwd, dataDirFlag)

	report := doctorReport{
		cwd:        cwd,
		configInfo: configInfo,
		docsInfo:   inspectDoctorDocs(cwd),
	}

	report.commandInfo = inspectDoctorCommands(cwd, configInfo.resolvedDataDir)
	report.dataInfo = inspectDoctorData(configInfo.resolvedDataDir)

	return report
}

func (r doctorReport) rows() []doctorStatusRow {
	rows := []doctorStatusRow{
		{
			Section: "cwd",
			Status:  "ok",
			Detail:  r.cwd,
		},
		{
			Section: "config.global",
			Status:  fileStatus(r.configInfo.globalPath),
			Detail:  r.configInfo.globalPath,
		},
		{
			Section: "config.global_data",
			Status:  fileStatus(r.configInfo.globalDataPath),
			Detail:  r.configInfo.globalDataPath,
		},
	}

	projectDetail := "no project config files found"
	projectStatus := "missing"
	if len(r.configInfo.projectConfigPath) > 0 {
		projectStatus = "ok"
		projectDetail = strings.Join(r.configInfo.projectConfigPath, ", ")
	}
	rows = append(rows, doctorStatusRow{
		Section: "config.project",
		Status:  projectStatus,
		Detail:  projectDetail,
	})

	loadStatus := "ok"
	loadDetail := fmt.Sprintf(
		"data_dir=%s initialize_as=%s",
		r.configInfo.resolvedDataDir,
		r.configInfo.initializeAs,
	)
	if r.configInfo.loadError != nil {
		loadStatus = "error"
		loadDetail = r.configInfo.loadError.Error()
	}
	rows = append(rows, doctorStatusRow{
		Section: "config.load",
		Status:  loadStatus,
		Detail:  loadDetail,
	})

	rows = append(rows, doctorStatusRow{
		Section: "docs.catalog",
		Status:  r.docsInfo.status,
		Detail:  r.docsInfo.detail,
	})

	rows = append(rows, r.commandInfo...)
	rows = append(rows, r.dataInfo...)

	return rows
}

func inspectDoctorConfig(cwd, dataDirFlag string) doctorConfigInfo {
	globalPath := config.GlobalConfig()
	globalDataPath := config.GlobalConfigData()
	projectConfigPaths := findProjectConfigs(cwd)

	paths := make([]string, 0, 2+len(projectConfigPaths))
	paths = append(paths, globalPath, globalDataPath)
	paths = append(paths, projectConfigPaths...)

	cfg, err := loadDoctorConfig(paths)
	resolvedDataDir := resolveDoctorDataDir(cwd, dataDirFlag, cfg)
	initializeAs := "AGENTS.md"
	if cfg != nil && cfg.Options != nil && cfg.Options.InitializeAs != "" {
		initializeAs = cfg.Options.InitializeAs
	}

	return doctorConfigInfo{
		globalPath:        globalPath,
		globalDataPath:    globalDataPath,
		projectConfigPath: projectConfigPaths,
		loadError:         err,
		resolvedDataDir:   resolvedDataDir,
		initializeAs:      initializeAs,
	}
}

func inspectDoctorDocs(cwd string) doctorDocsInfo {
	catalog, err := charmdocs.Load(cwd)
	if err != nil {
		return doctorDocsInfo{
			status: "error",
			detail: err.Error(),
		}
	}

	return doctorDocsInfo{
		status: "ok",
		detail: fmt.Sprintf("%d docs in %s", len(catalog.Slugs()), catalog.DocsDir()),
	}
}

func inspectDoctorCommands(cwd, dataDir string) []doctorStatusRow {
	sources := []struct {
		section string
		path    string
	}{
		{
			section: "commands.xdg_salamey",
			path:    xdgCommandsDir("salamey"),
		},
		{
			section: "commands.xdg_crush",
			path:    xdgCommandsDir("crush"),
		},
		{
			section: "commands.home_salamey",
			path:    userCommandsDir(".salamey"),
		},
		{
			section: "commands.home_crush",
			path:    userCommandsDir(".crush"),
		},
		{
			section: "commands.project_salamey",
			path:    filepath.Join(cwd, ".salamey", "commands"),
		},
		{
			section: "commands.project_data",
			path:    filepath.Join(dataDir, "commands"),
		},
	}

	rows := make([]doctorStatusRow, 0, len(sources))
	for _, source := range sources {
		rows = append(rows, inspectMarkdownDir(source.section, source.path))
	}

	return rows
}

func inspectDoctorData(dataDir string) []doctorStatusRow {
	rows := []doctorStatusRow{
		{
			Section: "data.dir",
			Status:  dirStatus(dataDir),
			Detail:  dataDir,
		},
		inspectPath("data.db", filepath.Join(dataDir, config.RuntimeDatabaseFileName())),
		inspectPath("data.logs", filepath.Join(dataDir, "logs", config.RuntimeLogFileName())),
		inspectPath("data.init", filepath.Join(dataDir, config.InitFlagFilename)),
	}

	return rows
}

func inspectMarkdownDir(section, path string) doctorStatusRow {
	count, err := countMarkdownFiles(path)
	if err != nil {
		return doctorStatusRow{
			Section: section,
			Status:  "error",
			Detail:  fmt.Sprintf("%s (%v)", displayPath(path), err),
		}
	}
	if count == 0 {
		return doctorStatusRow{
			Section: section,
			Status:  dirStatus(path),
			Detail:  displayPath(path),
		}
	}
	return doctorStatusRow{
		Section: section,
		Status:  "ok",
		Detail:  fmt.Sprintf("%d markdown commands in %s", count, path),
	}
}

func inspectPath(section, path string) doctorStatusRow {
	return doctorStatusRow{
		Section: section,
		Status:  fileStatus(path),
		Detail:  displayPath(path),
	}
}

func loadDoctorConfig(paths []string) (*config.Config, error) {
	var configs [][]byte
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
		if len(data) == 0 {
			continue
		}
		configs = append(configs, data)
	}

	if len(configs) == 0 {
		return &config.Config{}, nil
	}

	merged, err := jsons.Merge(configs)
	if err != nil {
		return nil, fmt.Errorf("merge config JSON: %w", err)
	}

	var cfg config.Config
	if err := json.Unmarshal(merged, &cfg); err != nil {
		return nil, fmt.Errorf("decode merged config: %w", err)
	}

	return &cfg, nil
}

func findProjectConfigs(cwd string) []string {
	baseName := filepath.Base(config.GlobalConfig())
	names := []string{baseName, "." + baseName}

	var found []string
	for dir := cwd; ; dir = filepath.Dir(dir) {
		for _, name := range names {
			path := filepath.Join(dir, name)
			if isRegularFile(path) {
				found = append(found, path)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	slices.Reverse(found)
	return found
}

func resolveDoctorDataDir(cwd, dataDirFlag string, cfg *config.Config) string {
	switch {
	case dataDirFlag != "":
		return absPath(cwd, dataDirFlag)
	case cfg != nil && cfg.Options != nil && cfg.Options.DataDirectory != "":
		return absPath(cwd, cfg.Options.DataDirectory)
	}

	if closest := findClosestDataDir(cwd); closest != "" {
		return closest
	}

	return filepath.Join(cwd, config.RuntimeDataDirectoryName())
}

func findClosestDataDir(cwd string) string {
	bestPath := ""
	bestDistance := int(^uint(0) >> 1)
	for _, name := range []string{config.RuntimeDataDirectoryName(), config.RuntimeFallbackDataDirectoryName()} {
		if path, distance, ok := findClosestNamedDataDir(cwd, name); ok && distance < bestDistance {
			bestPath = path
			bestDistance = distance
		}
	}
	return bestPath
}

func findClosestNamedDataDir(cwd, dirName string) (string, int, bool) {
	distance := 0
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, dirName)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, distance, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", 0, false
		}
		distance++
	}
}

func xdgCommandsDir(appName string) string {
	xdgHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgHome == "" && home.Dir() != "" {
		xdgHome = filepath.Join(home.Dir(), ".config")
	}
	if xdgHome == "" {
		return ""
	}
	return filepath.Join(xdgHome, appName, "commands")
}

func countMarkdownFiles(path string) (int, error) {
	if path == "" {
		return 0, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, nil
	}

	count := 0
	err = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			count++
		}
		return nil
	})
	return count, err
}

func dirStatus(path string) string {
	if path == "" {
		return "missing"
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "error"
	}
	if info.IsDir() {
		return "ok"
	}
	return "error"
}

func fileStatus(path string) string {
	if path == "" {
		return "missing"
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "error"
	}
	if info.Mode().IsRegular() {
		return "ok"
	}
	return "error"
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func absPath(cwd, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}

func userCommandsDir(name string) string {
	if home.Dir() == "" {
		return ""
	}
	return filepath.Join(home.Dir(), name, "commands")
}

func displayPath(path string) string {
	if path == "" {
		return "unavailable"
	}
	return path
}
