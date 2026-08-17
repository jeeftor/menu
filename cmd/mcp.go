package cmd

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"menu/internal/mcpserver"
	"menu/internal/nutrislice"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run the MCP server over stdio",
	Long: `Start the food MCP server using stdio transport.

Intended for use with Claude Desktop, Claude Code, or any MCP client
that launches a subprocess and communicates over stdin/stdout.

Add to Claude Desktop config (~/.config/claude/claude_desktop_config.json):
  {
    "mcpServers": {
      "menu": {
        "command": "/path/to/menu",
        "args": ["mcp"]
      }
    }
  }

Tools exposed:
  get_lunch        – menu for a specific date and school
  get_lunch_week   – full week menu
  list_schools     – available schools`,
	RunE: runMCP,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(_ *cobra.Command, _ []string) error {
	cacheDir := viper.GetString("cache_dir")
	client := nutrislice.NewClient(cacheDir)
	srv := mcpserver.New(client)

	return srv.Run(context.Background(), &mcp.StdioTransport{})
}
