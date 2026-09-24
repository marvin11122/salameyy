package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/wiesnermarven-hue/salameyy/internal/charmdocs"
)

var docsCmd = &cobra.Command{
	Use:   "docs [slug]",
	Short: "Browse local Charm docs from docs/charm",
	Long: `Browse the local Charm docs catalog cached in docs/charm.

Run without arguments to list the available docs, or pass a slug to render one
with Glamour.`,
	Args: cobra.MaximumNArgs(1),
	Example: `
# List local docs
salamey docs

# Render one doc
salamey docs bubbletea

# Render via explicit subcommand
salamey docs render glamour
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return runDocsList(cmd)
		}
		return runDocsRender(cmd, args[0])
	},
}

var docsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List local Charm docs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDocsList(cmd)
	},
}

var docsRenderCmd = &cobra.Command{
	Use:   "render <slug>",
	Short: "Render one local Charm doc with Glamour",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDocsRender(cmd, args[0])
	},
}

func init() {
	docsCmd.Flags().IntP("width", "w", 0, "Markdown wrap width (0 = terminal width or 80)")
	docsRenderCmd.Flags().IntP("width", "w", 0, "Markdown wrap width (0 = terminal width or 80)")

	docsCmd.AddCommand(docsListCmd, docsRenderCmd)
	rootCmd.AddCommand(docsCmd)
}

func runDocsList(cmd *cobra.Command) error {
	catalog, err := loadDocsCatalog(cmd)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SLUG\tTITLE\tPATH")
	for _, slug := range catalog.Slugs() {
		doc, ok := catalog.Document(slug)
		if !ok {
			continue
		}
		_, _ = fmt.Fprintf(
			tw,
			"%s\t%s\t%s\n",
			slug,
			doc.Title,
			relativeToRoot(catalog.Root(), doc.Path),
		)
	}
	return tw.Flush()
}

func runDocsRender(cmd *cobra.Command, slug string) error {
	catalog, err := loadDocsCatalog(cmd)
	if err != nil {
		return err
	}

	rendered, err := catalog.Render(slug, docsWidth(cmd))
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), rendered)
	return err
}

func loadDocsCatalog(cmd *cobra.Command) (charmdocs.Catalog, error) {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return charmdocs.Catalog{}, err
	}
	return charmdocs.Load(cwd)
}

func docsWidth(cmd *cobra.Command) int {
	width, _ := cmd.Flags().GetInt("width")
	if width > 0 {
		return width
	}

	if columns := os.Getenv("COLUMNS"); columns != "" {
		if parsed, err := strconv.Atoi(columns); err == nil && parsed > 0 {
			return parsed
		}
	}

	return 80
}

func relativeToRoot(root, path string) string {
	if root == "" {
		return path
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}

	return rel
}
