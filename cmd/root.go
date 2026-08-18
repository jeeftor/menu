// Package cmd provides the CLI entry points via Cobra.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "menu",
	Short: "🍽️  School lunch menu viewer",
	Long: `menu — browse ASD20 school lunch menus in your terminal or browser.

Commands:
  show   Print today's (or any day's) menu to the terminal
  serve  Start the web calendar server
  fetch  Pre-fetch and cache a month of menus`,
}

// Execute runs the cobra root command and exits on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./menu.yaml)")
	rootCmd.PersistentFlags().String("cache-dir", ".cache", "directory for cached API responses")

	if err := viper.BindPFlag("cache_dir", rootCmd.PersistentFlags().Lookup("cache-dir")); err != nil {
		panic(err)
	}
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("menu")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.config/menu")
	}

	viper.SetEnvPrefix("MENU")
	viper.AutomaticEnv()

	viper.SetDefault("port", 8080)
	viper.SetDefault("cache_dir", ".cache")

	_ = viper.ReadInConfig() // ignore "file not found" errors
}
