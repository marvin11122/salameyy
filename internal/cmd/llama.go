package cmd

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
	"github.com/wiesnermarven-hue/salameyy/internal/llamacpp"
)

var llamaCmd = &cobra.Command{
	Use:   "llama",
	Short: "Manage a local llama.cpp runtime",
	Args:  cobra.NoArgs,
}

var llamaStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start llama.cpp and optionally select its models",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadLlamaConfig(cmd)
		if err != nil {
			return err
		}
		opts, err := mergedLlamaOptions(cmd, cfg.Options.LlamaCPP)
		if err != nil {
			return err
		}
		if opts.Model == "" {
			return fmt.Errorf("a GGUF model path is required")
		}
		if !llamaFlagChanged(cmd.Flags(), "auto-start") {
			opts.AutoStart = true
		}
		manager := llamacpp.NewManager(cfg.Options.DataDirectory)
		status, err := manager.Start(cmd.Context(), toLlamaStartOptions(opts))
		if err != nil {
			return err
		}
		if err := persistLlamaOptions(cfg, opts); err != nil {
			return err
		}
		if selectModels, _ := cmd.Flags().GetBool("select"); selectModels {
			if err := selectLocalLlamaModels(cfg, status.Models); err != nil {
				return err
			}
		}
		return printLlamaFields(cmd.OutOrStdout(), []llamaField{
			{"status", "started"},
			{"base_url", status.BaseURL},
			{"model_path", opts.Model},
			{"pid", fmt.Sprint(processID(status))},
			{"models", joinModelIDs(status.Models)},
		})
	},
}

var llamaStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report the local llama.cpp runtime status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadLlamaConfig(cmd)
		if err != nil {
			return err
		}
		opts, err := mergedLlamaOptions(cmd, cfg.Options.LlamaCPP)
		if err != nil {
			return err
		}
		status, err := llamacpp.NewManager(cfg.Options.DataDirectory).Status(cmd.Context(), opts.BaseURL)
		if err != nil {
			return err
		}
		fields := []llamaField{
			{"managed", fmt.Sprint(status.Managed)},
			{"reachable", fmt.Sprint(status.Reachable)},
			{"base_url", status.BaseURL},
			{"models", joinModelIDs(status.Models)},
		}
		if status.State != nil {
			fields = append(fields,
				llamaField{"pid", fmt.Sprint(status.State.PID)},
				llamaField{"pid_running", fmt.Sprint(status.PIDRunning)},
				llamaField{"model_path", status.State.ModelPath},
				llamaField{"log", status.State.LogPath},
			)
		}
		if status.LastError != nil {
			fields = append(fields, llamaField{"error", status.LastError.Error()})
		}
		return printLlamaFields(cmd.OutOrStdout(), fields)
	},
}

var llamaDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Inspect local llama.cpp readiness",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadLlamaConfig(cmd)
		if err != nil {
			return err
		}
		opts, err := mergedLlamaOptions(cmd, cfg.Options.LlamaCPP)
		if err != nil {
			return err
		}
		manager := llamacpp.NewManager(cfg.Options.DataDirectory)
		status, err := manager.Status(cmd.Context(), opts.BaseURL)
		if err != nil {
			return err
		}
		recommendation := llamacpp.RecommendOptions(toLlamaStartOptions(opts))
		fields := []llamaField{
			{"reachable", fmt.Sprint(status.Reachable)},
			{"managed", fmt.Sprint(status.Managed)},
			{"base_url", status.BaseURL},
			{"models", joinModelIDs(status.Models)},
			{"threads.current", fmt.Sprint(opts.Threads)},
			{"threads.recommended", fmt.Sprint(recommendation.Threads)},
			{"parallel.current", fmt.Sprint(opts.Parallel)},
			{"parallel.recommended", fmt.Sprint(recommendation.Parallel)},
			{"ctx.current", fmt.Sprint(opts.ContextSize)},
			{"ctx.recommended", fmt.Sprint(recommendation.ContextSize)},
		}
		if status.State != nil {
			fields = append(fields, llamaField{"model_path", status.State.ModelPath}, llamaField{"log", status.State.LogPath})
		}
		for i, note := range recommendation.Notes {
			fields = append(fields, llamaField{fmt.Sprintf("note.%d", i+1), note})
		}
		if status.LastError != nil {
			fields = append(fields, llamaField{"error", status.LastError.Error()})
		}
		return printLlamaFields(cmd.OutOrStdout(), fields)
	},
}

var llamaStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the managed llama.cpp runtime",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadLlamaConfig(cmd)
		if err != nil {
			return err
		}
		force, _ := cmd.Flags().GetBool("force")
		status, err := llamacpp.NewManager(cfg.Options.DataDirectory).Stop(cmd.Context(), force)
		if err != nil {
			return err
		}
		return printLlamaFields(cmd.OutOrStdout(), []llamaField{{"status", "stopped"}, {"base_url", status.BaseURL}})
	},
}

var llamaOptimizeCmd = &cobra.Command{
	Use:   "optimize",
	Short: "Persist recommended local llama.cpp runtime settings",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := loadLlamaConfig(cmd)
		if err != nil {
			return err
		}
		opts, err := mergedLlamaOptions(cmd, cfg.Options.LlamaCPP)
		if err != nil {
			return err
		}
		recommendation := llamacpp.RecommendOptions(toLlamaStartOptions(opts))
		if opts.Threads <= 0 {
			opts.Threads = recommendation.Threads
		}
		if opts.Parallel <= 0 {
			opts.Parallel = recommendation.Parallel
		}
		if opts.ContextSize <= 0 {
			opts.ContextSize = recommendation.ContextSize
		}
		if err := persistLlamaOptions(cfg, opts); err != nil {
			return err
		}
		fields := []llamaField{
			{"status", "optimized"},
			{"threads", fmt.Sprint(opts.Threads)},
			{"parallel", fmt.Sprint(opts.Parallel)},
			{"ctx_size", fmt.Sprint(opts.ContextSize)},
			{"auto_start", fmt.Sprint(opts.AutoStart)},
		}
		if selectModels, _ := cmd.Flags().GetBool("select"); selectModels {
			manager := llamacpp.NewManager(cfg.Options.DataDirectory)
			if status, statusErr := manager.Status(cmd.Context(), opts.BaseURL); statusErr == nil && status.Reachable && len(status.Models) > 0 {
				if err := selectLocalLlamaModels(cfg, status.Models); err != nil {
					return err
				}
				fields = append(fields, llamaField{"models_selected", "true"})
			}
		}
		return printLlamaFields(cmd.OutOrStdout(), fields)
	},
}

type llamaField struct {
	key   string
	value string
}

func loadLlamaConfig(cmd *cobra.Command) (*config.Config, error) {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return nil, err
	}
	dataDir, _ := cmd.Flags().GetString("data-dir")
	debug, _ := cmd.Flags().GetBool("debug")
	return config.LoadOptions(cwd, dataDir, debug)
}

func mergedLlamaOptions(cmd *cobra.Command, base *config.LlamaCPPOptions) (*config.LlamaCPPOptions, error) {
	opts := cloneLlamaOptions(base)
	flags := cmd.Flags()
	if llamaFlagChanged(flags, "model") {
		value, _ := flags.GetString("model")
		opts.Model = value
	}
	if llamaFlagChanged(flags, "command") {
		value, _ := flags.GetString("command")
		opts.Command = value
	}
	if llamaFlagChanged(flags, "arg") {
		value, _ := flags.GetStringArray("arg")
		opts.Args = value
	}
	if llamaFlagChanged(flags, "env") {
		value, _ := flags.GetStringArray("env")
		envMap, err := parseEnvPairs(value)
		if err != nil {
			return nil, err
		}
		opts.Env = envMap
	}
	if llamaFlagChanged(flags, "base-url") {
		value, _ := flags.GetString("base-url")
		opts.BaseURL = value
	}
	if llamaFlagChanged(flags, "host") {
		value, _ := flags.GetString("host")
		opts.Host = value
	}
	if llamaFlagChanged(flags, "port") {
		value, _ := flags.GetInt("port")
		opts.Port = value
	}
	if llamaFlagChanged(flags, "working-dir") {
		value, _ := flags.GetString("working-dir")
		opts.WorkingDir = value
	}
	if llamaFlagChanged(flags, "healthcheck-timeout") {
		value, _ := flags.GetInt("healthcheck-timeout")
		opts.HealthcheckTimeout = value
	}
	if llamaFlagChanged(flags, "ctx-size") {
		value, _ := flags.GetInt("ctx-size")
		opts.ContextSize = value
	}
	if llamaFlagChanged(flags, "gpu-layers") {
		value, _ := flags.GetInt("gpu-layers")
		opts.GPULayers = value
	}
	if llamaFlagChanged(flags, "threads") {
		value, _ := flags.GetInt("threads")
		opts.Threads = value
	}
	if llamaFlagChanged(flags, "parallel") {
		value, _ := flags.GetInt("parallel")
		opts.Parallel = value
	}
	if llamaFlagChanged(flags, "flash-attention") {
		value, _ := flags.GetBool("flash-attention")
		opts.FlashAttention = &value
	}
	if llamaFlagChanged(flags, "no-mmap") {
		value, _ := flags.GetBool("no-mmap")
		opts.NoMMap = &value
	}
	if llamaFlagChanged(flags, "auto-start") {
		value, _ := flags.GetBool("auto-start")
		opts.AutoStart = value
	}
	if !llamaFlagChanged(flags, "base-url") && (llamaFlagChanged(flags, "host") || llamaFlagChanged(flags, "port")) {
		opts.BaseURL = ""
	}

	opts.Command = strings.TrimSpace(opts.Command)
	if opts.Command == "" {
		opts.Command = "llama-server"
	}
	opts.Host = strings.TrimSpace(opts.Host)
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.Port == 0 {
		opts.Port = 8080
	}
	if opts.HealthcheckTimeout <= 0 {
		opts.HealthcheckTimeout = 20
	}
	if opts.Model != "" {
		modelPath, err := filepath.Abs(os.ExpandEnv(opts.Model))
		if err != nil {
			return nil, fmt.Errorf("resolve model path: %w", err)
		}
		opts.Model = filepath.Clean(modelPath)
	}
	if opts.WorkingDir != "" {
		workingDir, err := filepath.Abs(os.ExpandEnv(opts.WorkingDir))
		if err != nil {
			return nil, fmt.Errorf("resolve working directory: %w", err)
		}
		opts.WorkingDir = filepath.Clean(workingDir)
	}
	opts.BaseURL = llamacpp.NormalizeBaseURL(opts.BaseURL, opts.Host, opts.Port)
	return opts, nil
}

func toLlamaStartOptions(opts *config.LlamaCPPOptions) llamacpp.StartOptions {
	return llamacpp.StartOptions{
		Command:            opts.Command,
		Args:               append([]string(nil), opts.Args...),
		Env:                cloneStringMap(opts.Env),
		BaseURL:            opts.BaseURL,
		ModelPath:          opts.Model,
		Host:               opts.Host,
		Port:               opts.Port,
		WorkingDir:         opts.WorkingDir,
		HealthcheckTimeout: time.Duration(opts.HealthcheckTimeout) * time.Second,
		ContextSize:        opts.ContextSize,
		GPULayers:          opts.GPULayers,
		Threads:            opts.Threads,
		Parallel:           opts.Parallel,
		FlashAttention:     opts.FlashAttention,
		NoMMap:             opts.NoMMap,
	}
}

func persistLlamaOptions(cfg *config.Config, opts *config.LlamaCPPOptions) error {
	fields := []struct {
		key   string
		value any
	}{
		{"options.llama_cpp.command", opts.Command},
		{"options.llama_cpp.args", opts.Args},
		{"options.llama_cpp.env", opts.Env},
		{"options.llama_cpp.base_url", opts.BaseURL},
		{"options.llama_cpp.model", opts.Model},
		{"options.llama_cpp.host", opts.Host},
		{"options.llama_cpp.port", opts.Port},
		{"options.llama_cpp.working_dir", opts.WorkingDir},
		{"options.llama_cpp.auto_start", opts.AutoStart},
		{"options.llama_cpp.healthcheck_timeout", opts.HealthcheckTimeout},
		{"options.llama_cpp.context_size", opts.ContextSize},
		{"options.llama_cpp.gpu_layers", opts.GPULayers},
		{"options.llama_cpp.threads", opts.Threads},
		{"options.llama_cpp.parallel", opts.Parallel},
	}
	for _, field := range fields {
		if err := cfg.SetConfigField(field.key, field.value); err != nil {
			return err
		}
	}
	if opts.FlashAttention != nil {
		if err := cfg.SetConfigField("options.llama_cpp.flash_attention", *opts.FlashAttention); err != nil {
			return err
		}
	}
	if opts.NoMMap != nil {
		if err := cfg.SetConfigField("options.llama_cpp.no_mmap", *opts.NoMMap); err != nil {
			return err
		}
	}
	return nil
}

func selectLocalLlamaModels(cfg *config.Config, models []llamacpp.ModelInfo) error {
	large, small := llamacpp.SelectDefaultModels(models)
	if large.ID == "" {
		return fmt.Errorf("llama.cpp returned no selectable models")
	}
	largeContext := large.ContextWindow
	if largeContext <= 0 {
		largeContext = 32768
	}
	smallContext := small.ContextWindow
	if smallContext <= 0 {
		smallContext = largeContext
	}
	if err := cfg.UpdatePreferredModel(config.SelectedModelTypeLarge, config.SelectedModel{
		Provider:  "llamacpp",
		Model:     large.ID,
		MaxTokens: min(int64(8192), largeContext),
	}); err != nil {
		return err
	}
	return cfg.UpdatePreferredModel(config.SelectedModelTypeSmall, config.SelectedModel{
		Provider:  "llamacpp",
		Model:     cmp.Or(small.ID, large.ID),
		MaxTokens: min(int64(4096), smallContext),
	})
}

func cloneLlamaOptions(base *config.LlamaCPPOptions) *config.LlamaCPPOptions {
	if base == nil {
		return &config.LlamaCPPOptions{}
	}
	out := *base
	out.Args = append([]string(nil), base.Args...)
	out.Env = cloneStringMap(base.Env)
	if base.FlashAttention != nil {
		value := *base.FlashAttention
		out.FlashAttention = &value
	}
	if base.NoMMap != nil {
		value := *base.NoMMap
		out.NoMMap = &value
	}
	return &out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func parseEnvPairs(items []string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\x00\r\n=") {
			return nil, fmt.Errorf("invalid env entry %q, expected KEY=VALUE", item)
		}
		out[key] = value
	}
	return out, nil
}

func joinModelIDs(models []llamacpp.ModelInfo) string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return strings.Join(ids, ", ")
}

func processID(status *llamacpp.Status) int {
	if status == nil || status.State == nil {
		return 0
	}
	return status.State.PID
}

func printLlamaFields(output io.Writer, fields []llamaField) error {
	writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	for _, field := range fields {
		if _, err := fmt.Fprintf(writer, "%s\t%s\n", field.key, field.value); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func llamaFlagChanged(flags *pflag.FlagSet, name string) bool {
	return flags != nil && flags.Lookup(name) != nil && flags.Changed(name)
}

func init() {
	llamaStartCmd.Flags().String("model", "", "Path to the GGUF model")
	llamaStartCmd.Flags().String("command", "", "llama.cpp server executable")
	llamaStartCmd.Flags().StringArray("arg", nil, "Additional llama.cpp argument")
	llamaStartCmd.Flags().StringArray("env", nil, "Environment variable in KEY=VALUE form")
	llamaStartCmd.Flags().String("base-url", "", "OpenAI-compatible llama.cpp endpoint")
	llamaStartCmd.Flags().String("host", "", "llama.cpp bind host")
	llamaStartCmd.Flags().Int("port", 0, "llama.cpp server port")
	llamaStartCmd.Flags().String("working-dir", "", "llama.cpp working directory")
	llamaStartCmd.Flags().Int("healthcheck-timeout", 0, "Seconds to wait for readiness")
	llamaStartCmd.Flags().Int("ctx-size", 0, "Context size")
	llamaStartCmd.Flags().Int("gpu-layers", 0, "GPU layers, -1 for all")
	llamaStartCmd.Flags().Int("threads", 0, "CPU threads")
	llamaStartCmd.Flags().Int("parallel", 0, "Parallel slots")
	llamaStartCmd.Flags().Bool("flash-attention", false, "Enable flash attention")
	llamaStartCmd.Flags().Bool("no-mmap", false, "Disable memory mapping")
	llamaStartCmd.Flags().Bool("auto-start", true, "Persist automatic startup")
	llamaStartCmd.Flags().Bool("select", false, "Select local models as large and small models")

	llamaStatusCmd.Flags().String("base-url", "", "Endpoint to probe")
	llamaStatusCmd.Flags().String("host", "", "Endpoint host")
	llamaStatusCmd.Flags().Int("port", 0, "Endpoint port")
	llamaDoctorCmd.Flags().String("base-url", "", "Endpoint to probe")
	llamaDoctorCmd.Flags().String("host", "", "Endpoint host")
	llamaDoctorCmd.Flags().Int("port", 0, "Endpoint port")
	llamaStopCmd.Flags().Bool("force", false, "Force termination")
	llamaOptimizeCmd.Flags().String("model", "", "Path to the GGUF model")
	llamaOptimizeCmd.Flags().String("base-url", "", "OpenAI-compatible llama.cpp endpoint")
	llamaOptimizeCmd.Flags().String("host", "", "llama.cpp bind host")
	llamaOptimizeCmd.Flags().Int("port", 0, "llama.cpp server port")
	llamaOptimizeCmd.Flags().Int("ctx-size", 0, "Context size")
	llamaOptimizeCmd.Flags().Int("threads", 0, "CPU threads")
	llamaOptimizeCmd.Flags().Int("parallel", 0, "Parallel slots")
	llamaOptimizeCmd.Flags().Bool("select", false, "Select reachable local models")

	llamaCmd.AddCommand(llamaStartCmd, llamaStatusCmd, llamaDoctorCmd, llamaStopCmd, llamaOptimizeCmd)
	rootCmd.AddCommand(llamaCmd)
}
