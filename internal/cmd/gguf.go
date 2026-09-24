package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
	"github.com/wiesnermarven-hue/salameyy/internal/gguf"
	"github.com/wiesnermarven-hue/salameyy/internal/gguf/gpu"
	"github.com/wiesnermarven-hue/salameyy/internal/gguf/registry"
	"github.com/wiesnermarven-hue/salameyy/internal/llamacpp"
)

var ggufCmd = &cobra.Command{
	Use:   "gguf",
	Short: "Manage GGUF models for local inference",
	Args:  cobra.NoArgs,
}

var ggufListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List registered GGUF models",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, reg, err := loadGGUFRegistry(cmd)
		if err != nil {
			return err
		}
		filter, _ := cmd.Flags().GetString("filter")
		models := reg.ListModels(splitGGUFFilter(filter)...)
		if len(models) == 0 {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "No models registered. Use 'salamey gguf add <path>' or 'salamey gguf scan <dir>'.")
			return err
		}
		writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(writer, "ID\tNAME\tARCH\tPARAMS\tCTX\tQUANT\tSIZE\tSTATUS")
		for _, model := range models {
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%d\t%s\t%.2fGB\t%s\n", model.ID, model.GetDisplayName(), model.GetArchitecture(), model.GetParameterCount(), model.GetContextLength(), model.GetQuantType(), model.GetFileSizeGB(), model.Status)
		}
		_ = writer.Flush()
		return nil
	},
}

var ggufAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add a GGUF model to the registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, reg, err := loadGGUFRegistry(cmd)
		if err != nil {
			return err
		}
		entry, err := reg.AddModel(args[0])
		if err != nil {
			return err
		}
		if err := reg.Save(); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added model: %s (ID: %s)\n", entry.GetDisplayName(), entry.ID)
		return err
	},
}

var ggufScanCmd = &cobra.Command{
	Use:   "scan [directory]",
	Short: "Scan for GGUF models",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, reg, err := loadGGUFRegistry(cmd)
		if err != nil {
			return err
		}
		directory := "."
		if len(args) > 0 {
			directory = args[0]
		}
		recursive, _ := cmd.Flags().GetBool("recursive")
		paths, err := discoverGGUFPaths(directory, recursive)
		if err != nil {
			return err
		}
		discovered := make([]*registry.ModelEntry, 0, len(paths))
		for _, path := range paths {
			entry, addErr := reg.AddModel(path)
			if addErr != nil {
				continue
			}
			discovered = append(discovered, entry)
		}
		if err := reg.Save(); err != nil {
			return err
		}
		writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintf(writer, "Discovered %d model(s):\n", len(discovered))
		for _, entry := range discovered {
			_, _ = fmt.Fprintf(writer, "- %s (ID: %s)\n", entry.GetDisplayName(), entry.ID)
		}
		return writer.Flush()
	},
}

var ggufInfoCmd = &cobra.Command{
	Use:   "info <path>",
	Short: "Show GGUF model metadata",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		metadata, err := gguf.Parse(args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), metadata.String())
		return err
	},
}

var ggufDownloadCmd = &cobra.Command{
	Use:   "download <org/repo[/revision[/filename]]>",
	Short: "Download a GGUF model from Hugging Face",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadLlamaConfig(cmd)
		if err != nil {
			return err
		}
		parts := strings.Split(args[0], "/")
		if len(parts) < 2 || len(parts) > 4 {
			return fmt.Errorf("invalid model reference %q, expected org/repo[/revision[/filename]]", args[0])
		}
		for _, part := range parts {
			if strings.TrimSpace(part) == "" || part == "." || part == ".." || strings.ContainsAny(part, "\x00\r\n") {
				return fmt.Errorf("invalid model reference %q", args[0])
			}
		}
		modelID := parts[0] + "/" + parts[1]
		revision := "main"
		filename := ""
		if len(parts) >= 3 {
			revision = parts[2]
		}
		if len(parts) == 4 {
			filename = parts[3]
		}
		outputDir, _ := cmd.Flags().GetString("output-dir")
		if outputDir == "" {
			outputDir = filepath.Join(cfg.Options.DataDirectory, "models", "gguf")
		}
		resume, _ := cmd.Flags().GetBool("resume")
		verify, _ := cmd.Flags().GetBool("verify-checksum")
		token := os.Getenv("HF_TOKEN")
		downloader := gguf.NewDownloader(cfg.Options.DataDirectory)
		progress := func(downloaded, total int64, speed float64) {
			if total <= 0 {
				return
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\rDownloading %.1f%% %.1f MB/s", float64(downloaded)*100/float64(total), speed)
		}
		result, err := downloader.Download(cmd.Context(), gguf.DownloadOptions{
			ModelID:        modelID,
			Filename:       filename,
			Revision:       revision,
			OutputDir:      outputDir,
			VerifyChecksum: verify,
			Resume:         resume,
			Token:          token,
			Timeout:        30 * time.Minute,
		}, progress)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.ErrOrStderr())
		if add, _ := cmd.Flags().GetBool("add"); add {
			reg := registry.NewRegistry(cfg.Options.DataDirectory)
			if err := reg.Load(); err != nil {
				return err
			}
			entry, err := reg.AddModel(result.FilePath)
			if err != nil {
				return err
			}
			if err := reg.Save(); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added to registry: %s (ID: %s)\n", entry.GetDisplayName(), entry.ID)
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Downloaded to: %s\n", result.FilePath)
		return err
	},
}

var ggufLoadCmd = &cobra.Command{
	Use:   "load <model>",
	Short: "Start llama.cpp with a registered GGUF model",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, reg, err := loadGGUFRegistry(cmd)
		if err != nil {
			return err
		}
		entry, ok := reg.GetModel(args[0])
		if !ok {
			models := reg.ListModels(args[0])
			if len(models) == 0 {
				return fmt.Errorf("model not found: %s", args[0])
			}
			entry = models[0]
		}
		if entry.Status == registry.StatusMissing {
			return fmt.Errorf("model file is missing: %s", entry.Path)
		}
		opts := cloneLlamaOptions(cfg.Options.LlamaCPP)
		opts.Model = entry.Path
		if cmd.Flags().Changed("port") {
			opts.Port, _ = cmd.Flags().GetInt("port")
			opts.BaseURL = ""
		}
		if cmd.Flags().Changed("ctx-size") {
			opts.ContextSize, _ = cmd.Flags().GetInt("ctx-size")
		}
		if cmd.Flags().Changed("gpu-layers") {
			opts.GPULayers, _ = cmd.Flags().GetInt("gpu-layers")
		}
		if cmd.Flags().Changed("threads") {
			opts.Threads, _ = cmd.Flags().GetInt("threads")
		}
		opts.BaseURL = llamacpp.NormalizeBaseURL(opts.BaseURL, opts.Host, opts.Port)
		manager := llamacpp.NewManager(cfg.Options.DataDirectory)
		status, err := manager.Start(cmd.Context(), toLlamaStartOptions(opts))
		if err != nil {
			return err
		}
		if err := reg.MarkUsed(entry.ID); err != nil {
			return err
		}
		if err := reg.Save(); err != nil {
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
		fields := []llamaField{{"status", "started"}, {"base_url", status.BaseURL}, {"model", entry.GetDisplayName()}, {"path", entry.Path}, {"pid", fmt.Sprint(processID(status))}}
		return printLlamaFields(cmd.OutOrStdout(), fields)
	},
}

var ggufGPUCmd = &cobra.Command{
	Use:   "gpu",
	Short: "Show detected local GPU information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		info, err := gpu.DetectSystemContext(cmd.Context())
		if err != nil {
			return err
		}
		writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintf(writer, "platform\t%s/%s\n", runtime.GOOS, runtime.GOARCH)
		_, _ = fmt.Fprintf(writer, "cpu\t%s\n", info.CPUModel)
		_, _ = fmt.Fprintf(writer, "ram\t%s total / %s free\n", gpu.FormatMemory(info.TotalRAMMB), gpu.FormatMemory(info.FreeRAMMB))
		if len(info.GPUs) == 0 {
			_, _ = fmt.Fprintln(writer, "gpu\tnone detected")
		} else {
			for _, device := range info.GPUs {
				_, _ = fmt.Fprintf(writer, "gpu.%d\t%s (%s) %s\n", device.ID, device.Name, device.Type, gpu.FormatMemory(device.MemoryMB))
			}
		}
		return writer.Flush()
	},
}

func loadGGUFRegistry(cmd *cobra.Command) (*config.Config, *registry.Registry, error) {
	cfg, err := loadLlamaConfig(cmd)
	if err != nil {
		return nil, nil, err
	}
	reg := registry.NewRegistry(cfg.Options.DataDirectory)
	if err := reg.Load(); err != nil {
		return nil, nil, err
	}
	return cfg, reg, nil
}

func splitGGUFFilter(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func discoverGGUFPaths(directory string, recursive bool) ([]string, error) {
	if recursive {
		return gguf.DiscoverGGUF(directory)
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if strings.HasSuffix(strings.ToLower(info.Name()), ".gguf") {
			return []string{directory}, nil
		}
		return nil, nil
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".gguf") {
			continue
		}
		paths = append(paths, filepath.Join(directory, entry.Name()))
	}
	return paths, nil
}

func init() {
	ggufListCmd.Flags().StringP("filter", "f", "", "Filter models by name or tag")
	ggufScanCmd.Flags().BoolP("recursive", "r", true, "Scan directories recursively")
	ggufDownloadCmd.Flags().StringP("output-dir", "o", "", "Output directory")
	ggufDownloadCmd.Flags().Bool("resume", false, "Resume an interrupted download")
	ggufDownloadCmd.Flags().Bool("verify-checksum", false, "Verify the published SHA-256 checksum")
	ggufDownloadCmd.Flags().BoolP("add", "a", true, "Add the downloaded model to the registry")
	ggufLoadCmd.Flags().Int("port", 0, "llama.cpp server port")
	ggufLoadCmd.Flags().Int("ctx-size", 0, "Context size")
	ggufLoadCmd.Flags().Int("gpu-layers", 0, "GPU layers, -1 for all")
	ggufLoadCmd.Flags().Int("threads", 0, "CPU threads")
	ggufLoadCmd.Flags().Bool("select", false, "Select local models as large and small models")

	ggufCmd.AddCommand(ggufListCmd, ggufAddCmd, ggufScanCmd, ggufInfoCmd, ggufDownloadCmd, ggufLoadCmd, ggufGPUCmd)
	rootCmd.AddCommand(ggufCmd)
}
