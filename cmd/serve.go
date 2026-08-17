package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"menu/internal/mcpserver"
	"menu/internal/nutrislice"
	"menu/internal/server"
	"menu/internal/store"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web calendar server",
	Long:  "Serve a monthly HTML lunch calendar at http://localhost:<port>/",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().Int("port", 8080, "port to listen on")
	serveCmd.Flags().String("data-dir", "", "directory for SQLite database (default: ~/.config/menu)")
	if err := viper.BindPFlag("port", serveCmd.Flags().Lookup("port")); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, _ []string) error {
	port := viper.GetInt("port")
	cacheDir := viper.GetString("cache_dir")
	dataDir, _ := cmd.Flags().GetString("data-dir")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".config", "menu")
	}

	printServeBanner(port)

	client := nutrislice.NewClient(cacheDir)
	mcpSrv := mcpserver.New(client)

	st, err := store.Open(filepath.Join(dataDir, "menu.db"))
	if err != nil {
		slog.Warn("could not open store; favorites/exclusions unavailable", "err", err)
	}

	srv := server.New(client, port, mcpSrv, st)
	return srv.Start()
}

func printServeBanner(port int) {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
	urlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA")).Underline(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))

	banner := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Padding(0, 2).
		Render(
			lipgloss.JoinVertical(lipgloss.Left,
				titleStyle.Render("🍽️  Food — School Lunch Calendar"),
				"",
				dimStyle.Render("Calendar  ")+urlStyle.Render(fmt.Sprintf("http://localhost:%d/", port)),
				dimStyle.Render("REST API  ")+urlStyle.Render(fmt.Sprintf("http://localhost:%d/api/v1/lunch?date=today", port)),
				dimStyle.Render("MCP HTTP  ")+urlStyle.Render(fmt.Sprintf("http://localhost:%d/mcp", port)),
				dimStyle.Render("MCP stdio ")+urlStyle.Render("menu mcp"),
				"",
				dimStyle.Render("Press Ctrl+C to stop"),
			),
		)
	fmt.Println(banner)
}
