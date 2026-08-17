package cmd

// Version is set at build time via -ldflags "-X menu/cmd.Version=x.y.z".
// Falls back to "dev" when built with go build or go run.
var Version = "dev"
