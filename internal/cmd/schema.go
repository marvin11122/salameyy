package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
	"github.com/spf13/cobra"
	"github.com/wiesnermarven-hue/salameyy/internal/config"
)

var schemaCmd = &cobra.Command{
	Use:    "schema",
	Short:  "Generate the SALAMEY configuration schema",
	Long:   "Generate the JSON schema for SALAMEY configuration. Legacy crush.json and .crush paths remain supported as compatibility fallbacks.",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		reflector := new(jsonschema.Reflector)
		schema := reflector.Reflect(&config.Config{})
		if model, ok := schema.Definitions["Model"]; ok {
			model.Required = []string{"id"}
		}
		schema.ID = jsonschema.ID("https://raw.githubusercontent.com/wiesnermarven-hue/salameyy/main/schema.json")
		bts, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal schema: %w", err)
		}
		fmt.Println(string(bts))
		return nil
	},
}
