package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"menu/internal/auth"
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
	serveCmd.Flags().String("external-url", "", "public base URL shown in the startup banner (e.g. http://homelab:8080)")
	serveCmd.Flags().String("alexa-skill-id", "", "Alexa skill application ID (enables /alexa endpoint)")
	serveCmd.Flags().Bool("alexa-disable-verification", false, "disable ASK signature verification (local testing only)")
	serveCmd.Flags().String("alexa-school", nutrislice.DefaultSchools[0].Slug, "default school slug for Alexa queries")
	serveCmd.Flags().String("alexa-meal", "lunch", "default meal type for Alexa queries: lunch or breakfast")
	// OIDC / session settings (optional). When set, enables /login for settings access from WAN.
	serveCmd.Flags().String("oidc-issuer", "", "OIDC issuer URL (e.g. https://auth.vookie.net/application/o/menu/)")
	serveCmd.Flags().String("oidc-client-id", "", "OIDC client ID")
	serveCmd.Flags().String("oidc-client-secret", "", "OIDC client secret")
	serveCmd.Flags().String("oidc-redirect-url", "", "OIDC callback URL (e.g. https://menu.vookie.net/callback)")
	serveCmd.Flags().String("session-secret", "", "base64-encoded secret used to sign session cookies (32+ bytes)")
	for _, name := range []string{"port", "external-url", "alexa-skill-id", "alexa-disable-verification", "alexa-school", "alexa-meal", "oidc-issuer", "oidc-client-id", "oidc-client-secret", "oidc-redirect-url", "session-secret"} {
		if err := viper.BindPFlag(strings.ReplaceAll(name, "-", "_"), serveCmd.Flags().Lookup(name)); err != nil {
			panic(err)
		}
	}
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, _ []string) error {
	port := viper.GetInt("port")
	cacheDir := viper.GetString("cache_dir")
	externalURL := viper.GetString("external_url")
	dataDir, _ := cmd.Flags().GetString("data-dir")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".config", "menu")
	}

	var alexaCfg *server.AlexaConfig
	if skillID := viper.GetString("alexa_skill_id"); skillID != "" {
		alexaCfg = &server.AlexaConfig{
			ApplicationID:  skillID,
			VerifyRequests: !viper.GetBool("alexa_disable_verification"),
			DefaultSchool:  viper.GetString("alexa_school"),
			DefaultMeal:    viper.GetString("alexa_meal"),
		}
	}

	var authCfg *server.AuthConfig
	if sessionSecret := viper.GetString("session_secret"); sessionSecret != "" {
		sm, err := auth.NewSessionManager(sessionSecret)
		if err != nil {
			return fmt.Errorf("session manager: %w", err)
		}
		authCfg = &server.AuthConfig{
			OIDC: auth.OIDCConfig{
				Issuer:       viper.GetString("oidc_issuer"),
				ClientID:     viper.GetString("oidc_client_id"),
				ClientSecret: viper.GetString("oidc_client_secret"),
				RedirectURL:  viper.GetString("oidc_redirect_url"),
			},
			Sessions: sm,
		}
	}

	printServeBanner(port, externalURL, alexaCfg != nil, authCfg != nil)

	client := nutrislice.NewClient(cacheDir)
	mcpSrv := mcpserver.New(client)

	st, err := store.Open(filepath.Join(dataDir, "menu.db"))
	if err != nil {
		slog.Warn("could not open store; favorites/exclusions unavailable", "err", err)
	}

	srv := server.New(client, port, mcpSrv, st, Version, alexaCfg, authCfg)
	return srv.Start()
}

func printServeBanner(port int, externalURL string, alexaEnabled, authEnabled bool) {
	base := fmt.Sprintf("http://localhost:%d", port)
	if externalURL != "" {
		base = strings.TrimRight(externalURL, "/")
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
	urlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA")).Underline(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	verStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B"))

	rows := []string{
		titleStyle.Render("🍽️  Menu — School Lunch Calendar") + " " + verStyle.Render("v"+Version),
		"",
		dimStyle.Render("Calendar  ") + urlStyle.Render(base+"/"),
		dimStyle.Render("API Docs  ") + urlStyle.Render(base+"/api"),
		dimStyle.Render("Settings  ") + urlStyle.Render(base+"/settings"),
		dimStyle.Render("MCP HTTP  ") + urlStyle.Render(base+"/mcp"),
		dimStyle.Render("MCP stdio ") + urlStyle.Render("menu mcp"),
	}
	if alexaEnabled {
		rows = append(rows, dimStyle.Render("Alexa     ")+urlStyle.Render(base+"/alexa"))
	}
	if authEnabled {
		rows = append(rows, dimStyle.Render("Login     ")+urlStyle.Render(base+"/login"))
	}
	rows = append(rows, "", dimStyle.Render("Press Ctrl+C to stop"))

	banner := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Padding(0, 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	fmt.Println(banner)
}
